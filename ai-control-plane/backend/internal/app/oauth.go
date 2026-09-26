package app

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

type oidcService struct {
	cfg      Config
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    oauth2.Config
}

type oauthState struct {
	Binding  string `json:"binding"`
	Verifier string `json:"verifier"`
	Nonce    string `json:"nonce"`
}

func newOIDC(ctx context.Context, cfg Config) (*oidcService, error) {
	if !cfg.AuthEnabled {
		return nil, nil
	}
	provider, err := oidc.NewProvider(ctx, cfg.GoogleIssuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover Google OIDC provider: %w", err)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.GoogleClientID, SupportedSigningAlgs: []string{"RS256"}})
	return &oidcService{
		cfg:      cfg,
		provider: provider,
		verifier: verifier,
		oauth: oauth2.Config{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  strings.TrimRight(cfg.PublicBaseURL, "/") + "/api/auth/google/callback",
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
	}, nil
}

func (o *oidcService) start(ctx context.Context, d *database, w http.ResponseWriter, r *http.Request) error {
	if o == nil || d == nil || d.redis == nil {
		return errors.New("authentication is not ready")
	}
	state, err := randomURLValue(32)
	if err != nil {
		return err
	}
	binding, err := randomURLValue(32)
	if err != nil {
		return err
	}
	verifier, err := randomURLValue(32)
	if err != nil {
		return err
	}
	nonce, err := randomURLValue(32)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(oauthState{Binding: binding, Verifier: verifier, Nonce: nonce})
	if err != nil {
		return err
	}
	key := oauthStateKey(o.cfg.SessionSecret, state)
	if err := d.redis.Set(ctx, key, payload, o.cfg.OAuthStateTTL).Err(); err != nil {
		return fmt.Errorf("store OAuth state: %w", err)
	}
	setOAuthBindingCookie(w, o.cfg, binding)
	redirect := o.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce))
	http.Redirect(w, r, redirect, http.StatusFound)
	return nil
}

func (o *oidcService) callback(ctx context.Context, d *database, w http.ResponseWriter, r *http.Request) (result identity, session string, err error) {
	if o == nil || d == nil || d.redis == nil {
		return identity{}, "", errors.New("authentication is not ready")
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		return identity{}, "", errors.New("missing OAuth state")
	}
	key := oauthStateKey(o.cfg.SessionSecret, state)
	raw, err := d.redis.GetDel(ctx, key).Bytes()
	if err != nil {
		return identity{}, "", errors.New("expired or invalid OAuth state")
	}
	var stored oauthState
	if json.Unmarshal(raw, &stored) != nil || stored.Binding == "" || stored.Verifier == "" || stored.Nonce == "" {
		return identity{}, "", errors.New("invalid OAuth state")
	}
	cookie, err := r.Cookie(oauthBindingCookieName(o.cfg))
	if err != nil || !constantEqual(cookie.Value, stored.Binding) {
		return identity{}, "", errors.New("OAuth browser binding mismatch")
	}
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		return identity{}, "", errors.New("Google authorization was not completed")
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		return identity{}, "", errors.New("missing OAuth authorization code")
	}
	exchangeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	token, err := o.oauth.Exchange(exchangeCtx, code, oauth2.VerifierOption(stored.Verifier))
	if err != nil {
		return identity{}, "", errors.New("Google authorization code exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return identity{}, "", errors.New("Google did not return an ID token")
	}
	idToken, err := o.verifier.Verify(exchangeCtx, rawIDToken)
	if err != nil {
		return identity{}, "", errors.New("Google ID token verification failed")
	}
	var claims struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Nonce         string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Subject == "" || claims.Email == "" || !claims.EmailVerified {
		return identity{}, "", errors.New("Google account is missing a verified email identity")
	}
	if !constantEqual(claims.Nonce, stored.Nonce) {
		return identity{}, "", errors.New("Google ID token nonce mismatch")
	}
	result, err = d.upsertIdentity(ctx, claims.Subject, claims.Email, claims.EmailVerified)
	if err != nil {
		return identity{}, "", errors.New("account bootstrap failed")
	}
	session, err = d.createSession(ctx, o.cfg, result, r.UserAgent(), hashAddress(o.cfg.KeyPepper, r))
	if err != nil {
		return identity{}, "", errors.New("session creation failed")
	}
	clearOAuthBindingCookie(w, o.cfg)
	return result, session, nil
}

func randomURLValue(size int) (string, error) {
	value, err := randomBytes(size)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func oauthStateKey(secret, state string) string {
	return "oauth:state:" + base64.RawURLEncoding.EncodeToString(keyedDigest(secret, []byte(state)))
}

func constantEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func oauthBindingCookieName(cfg Config) string {
	if strings.EqualFold(mustScheme(cfg.PublicBaseURL), "https") {
		return "__Host-cp_oauth_binding"
	}
	return "cp_oauth_binding"
}

func setOAuthBindingCookie(w http.ResponseWriter, cfg Config, value string) {
	http.SetCookie(w, &http.Cookie{
		Name: oauthBindingCookieName(cfg), Value: value, Path: "/", HttpOnly: true,
		Secure: strings.EqualFold(mustScheme(cfg.PublicBaseURL), "https"), SameSite: http.SameSiteLaxMode,
		MaxAge: int(cfg.OAuthStateTTL.Seconds()),
	})
}

func clearOAuthBindingCookie(w http.ResponseWriter, cfg Config) {
	http.SetCookie(w, &http.Cookie{Name: oauthBindingCookieName(cfg), Value: "", Path: "/", HttpOnly: true, Secure: strings.EqualFold(mustScheme(cfg.PublicBaseURL), "https"), SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}

func mustScheme(raw string) string {
	u, _ := url.Parse(raw)
	if u == nil {
		return ""
	}
	return u.Scheme
}

func redisStateExists(ctx context.Context, client *redis.Client, key string) bool {
	return client.Exists(ctx, key).Val() == 1
}

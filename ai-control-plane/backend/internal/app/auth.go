package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type principal struct {
	identity
	SessionRaw string
	CSRFToken  string
}

func (s *server) sessionPrincipal(r *http.Request) (principal, error) {
	if !s.cfg.AuthEnabled || s.db == nil {
		return principal{}, errors.New("authentication is not enabled")
	}
	cookie, err := r.Cookie(s.cfg.SessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return principal{}, errors.New("authentication required")
	}
	record, err := s.db.loadSession(r.Context(), s.cfg, cookie.Value)
	if err != nil {
		return principal{}, errors.New("authentication required")
	}
	return principal{identity: record.identity, SessionRaw: cookie.Value, CSRFToken: csrfToken(s.cfg.SessionSecret, cookie.Value)}, nil
}

func (s *server) requireSession(w http.ResponseWriter, r *http.Request) (principal, bool) {
	p, err := s.sessionPrincipal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Authentication required.", "authentication_error")
		return principal{}, false
	}
	return p, true
}

func requireSameOrigin(w http.ResponseWriter, r *http.Request, publicBaseURL string) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		if referer := r.Header.Get("Referer"); referer != "" {
			origin = referer
		}
	}
	if origin == "" {
		return true
	}
	expected, err := url.Parse(publicBaseURL)
	if err != nil {
		writeError(w, http.StatusForbidden, "Request origin is not allowed.", "csrf_error")
		return false
	}
	actual, err := url.Parse(origin)
	if err != nil || !strings.EqualFold(actual.Scheme, expected.Scheme) || !strings.EqualFold(actual.Host, expected.Host) {
		writeError(w, http.StatusForbidden, "Request origin is not allowed.", "csrf_error")
		return false
	}
	return true
}

func (s *server) requireCSRF(w http.ResponseWriter, r *http.Request, p principal) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	if !requireSameOrigin(w, r, s.cfg.PublicBaseURL) {
		return false
	}
	provided := r.Header.Get("X-CSRF-Token")
	if !constantEqual(provided, p.CSRFToken) {
		writeError(w, http.StatusForbidden, "CSRF validation failed.", "csrf_error")
		return false
	}
	return true
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireSession(w, r)
	if !ok || !s.requireCSRF(w, r, p) {
		return
	}
	if err := s.db.revokeSession(r.Context(), s.cfg, p.SessionRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not end the session.", "api_error")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cfg.SessionCookieName, Value: "", Path: "/", HttpOnly: true, Secure: secureCookies(s.cfg), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
}

func secureCookies(cfg Config) bool {
	return strings.EqualFold(mustScheme(cfg.PublicBaseURL), "https")
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":         map[string]any{"id": p.UserID, "email": p.Email, "email_verified": p.EmailVerified},
		"organization": map[string]any{"id": p.OrganizationID, "role": p.Role},
		"csrf_token":   p.CSRFToken,
	})
}

func (s *server) managementNotImplemented(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireSession(w, r); !ok {
		return
	}
	writeError(w, http.StatusNotImplemented, "This management API is not enabled yet.", "api_error")
}

func requestContext(ctx context.Context, p principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

type principalContextKey string

const principalKey principalContextKey = "principal"

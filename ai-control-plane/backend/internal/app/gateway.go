package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type selectedGrant struct {
	apiKeyGrant
}

type workerRoute struct {
	Provider    string
	InternalURL string
	WorkerToken string
}

func (d *database) verifyAPIKey(ctx context.Context, cfg Config, raw string) (verifiedAPIKey, error) {
	prefix, secret, ok := parseAPIKey(raw)
	if !ok || d == nil {
		return verifiedAPIKey{}, errors.New("invalid API key")
	}
	var id, organizationID, name, keyType, state string
	var stored []byte
	var expiresAt, lastUsedAt, createdAt, revokedAt interface{}
	err := d.pool.QueryRow(ctx, `
		SELECT id::text, organization_id::text, name, key_type, state, verifier, expires_at, last_used_at, created_at, revoked_at
		FROM api_keys
		WHERE prefix = $1 AND state = 'active'
		  AND (expires_at IS NULL OR expires_at > now())`, prefix).Scan(&id, &organizationID, &name, &keyType, &state, &stored, &expiresAt, &lastUsedAt, &createdAt, &revokedAt)
	if err != nil || !apiKeySecretMatches(stored, keyedDigest(cfg.KeyPepper, []byte(secret))) {
		return verifiedAPIKey{}, errors.New("invalid API key")
	}
	grants, err := d.listKeyGrants(ctx, id)
	if err != nil || len(grants) == 0 {
		return verifiedAPIKey{}, errors.New("API key has no active grants")
	}
	_, _ = d.pool.Exec(ctx, `UPDATE api_keys SET last_used_at = now() WHERE id = $1`, id)
	return verifiedAPIKey{Metadata: apiKeyMetadata{ID: id, OrganizationID: organizationID, Name: name, Prefix: prefix, Type: keyType, State: state, Grants: grants}}, nil
}

func (k verifiedAPIKey) selectGrant(route, profileID, model string) (selectedGrant, bool) {
	var matches []apiKeyGrant
	for _, grant := range k.Metadata.Grants {
		if grant.Route != route || (profileID != "" && grant.ProfileID != profileID) {
			continue
		}
		if grant.ModelPattern != "" && grant.ModelPattern != "*" && grant.ModelPattern != model {
			continue
		}
		matches = append(matches, grant)
	}
	if len(matches) != 1 {
		return selectedGrant{}, false
	}
	return selectedGrant{apiKeyGrant: matches[0]}, true
}

func (d *database) rateLimit(ctx context.Context, keyID, profileID, route string, limit int) (bool, error) {
	if d == nil || d.redis == nil {
		return false, errors.New("rate limiter unavailable")
	}
	bucket := "gateway:rate:" + keyID + ":" + profileID + ":" + route + ":" + strconv.FormatInt(time.Now().Unix()/60, 10)
	count, err := d.redis.Incr(ctx, bucket).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		_ = d.redis.Expire(ctx, bucket, 70*time.Second).Err()
	}
	return count <= int64(limit), nil
}

func (d *database) getWorkerRoute(ctx context.Context, cfg Config, organizationID, profileID string) (workerRoute, error) {
	var route workerRoute
	var ciphertext []byte
	err := d.pool.QueryRow(ctx, `
		SELECT p.provider, w.internal_url, w.credential_ciphertext
		FROM provider_profiles p
		JOIN worker_runtimes w ON w.provider_profile_id = p.id
		WHERE p.id = $1 AND p.organization_id = $2 AND p.status <> 'disabled'
		  AND w.status = 'online'
		  AND w.last_heartbeat_at > now() - interval '2 minutes'`, profileID, organizationID).Scan(&route.Provider, &route.InternalURL, &ciphertext)
	if err != nil {
		return workerRoute{}, errors.New("worker is not online")
	}
	if route.InternalURL == "" || len(ciphertext) == 0 {
		return workerRoute{}, errors.New("worker route is not configured")
	}
	route.WorkerToken, err = decryptSecret(cfg.KeyPepper, ciphertext)
	if err != nil || route.WorkerToken == "" {
		return workerRoute{}, errors.New("worker transport credential unavailable")
	}
	return route, nil
}

func (d *database) recordUsage(ctx context.Context, organizationID, keyID, profileID, model, route string, status, latency int64) {
	_, _ = d.pool.Exec(ctx, `
		INSERT INTO usage_events (organization_id, api_key_id, provider_profile_id, model, route, status_code, latency_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, organizationID, nullIfEmpty(keyID), nullIfEmpty(profileID), nullIfEmpty(model), route, status, latency)
}

func (s *server) publicGateway(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AuthEnabled || s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "Public API gateway is not configured.", "api_error")
		return
	}
	route := ""
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
		route = "models"
	case r.Method == http.MethodPost && r.URL.Path == "/v1/responses":
		route = "responses"
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		route = "chat.completions"
	default:
		writeError(w, http.StatusNotFound, "Unknown API endpoint.", "invalid_request_error")
		return
	}
	scheme, token, headerOK := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if !headerOK || !strings.EqualFold(scheme, "Bearer") {
		writeError(w, http.StatusUnauthorized, "Incorrect API key provided.", "authentication_error")
		return
	}
	key, err := s.db.verifyAPIKey(r.Context(), s.cfg, token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Incorrect API key provided.", "authentication_error")
		return
	}
	profileID := strings.TrimSpace(r.Header.Get("X-Provider-Profile"))
	model := ""
	if r.Method == http.MethodPost {
		data, readErr := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxBodyBytes))
		r.Body = io.NopCloser(bytes.NewReader(data))
		if readErr != nil {
			writeError(w, http.StatusBadRequest, "Request body is too large or unreadable.", "invalid_request_error")
			return
		}
		var body map[string]any
		if json.Unmarshal(data, &body) != nil {
			writeError(w, http.StatusBadRequest, "Invalid JSON body.", "invalid_request_error")
			return
		}
		model, _ = body["model"].(string)
	}
	grant, ok := key.selectGrant(route, profileID, model)
	if !ok {
		writeError(w, http.StatusForbidden, "API key is not authorized for this route, model, or profile.", "authorization_error")
		return
	}
	if profileID == "" {
		profileID = grant.ProfileID
	}
	allowed, err := s.db.rateLimit(r.Context(), key.Metadata.ID, profileID, route, grant.RequestsPerMinute)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Rate limiter unavailable.", "api_error")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "Rate limit exceeded.", "rate_limit_error")
		return
	}
	select {
	case s.gatewaySlots <- struct{}{}:
		defer func() { <-s.gatewaySlots }()
	case <-r.Context().Done():
		return
	default:
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, "Gateway concurrency limit reached.", "api_error")
		return
	}
	routeInfo, err := s.db.getWorkerRoute(r.Context(), s.cfg, key.Metadata.OrganizationID, profileID)
	if err != nil || routeInfo.Provider != grant.Provider {
		writeError(w, http.StatusServiceUnavailable, "Selected provider worker is unavailable.", "api_error")
		return
	}
	started := time.Now()
	status, err := s.forwardWorker(w, r, routeInfo, route)
	latency := time.Since(started).Milliseconds()
	usageStatus := status
	if usageStatus < 100 || usageStatus > 599 {
		usageStatus = http.StatusBadGateway
	}
	usageCtx, usageCancel := context.WithTimeout(context.Background(), time.Second)
	s.db.recordUsage(usageCtx, key.Metadata.OrganizationID, key.Metadata.ID, profileID, model, route, usageStatus, latency)
	usageCancel()
	if err != nil {
		writeError(w, http.StatusBadGateway, "Provider worker request failed.", "api_error")
	}
}

func (s *server) forwardWorker(w http.ResponseWriter, r *http.Request, route workerRoute, apiRoute string) (int64, error) {
	base, err := url.Parse(route.InternalURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return 0, errors.New("invalid worker URL")
	}
	endpointPath := "/v1/" + apiRoute
	if apiRoute == "chat.completions" {
		endpointPath = "/v1/chat/completions"
	}
	base.Path = strings.TrimRight(base.Path, "/") + endpointPath
	base.RawQuery = r.URL.RawQuery
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.GatewayTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, r.Method, base.String(), r.Body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+route.WorkerToken)
	req.Header.Set("X-Request-ID", requestIDFromContext(r.Context()))
	for _, name := range []string{"Accept", "Content-Type", "OpenAI-Beta"} {
		if value := r.Header.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	transport := privateTransport(s.cfg, base.Hostname())
	if transport == nil {
		return 0, errors.New("mTLS worker transport unavailable")
	}
	client := &http.Client{Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, errors.New("worker returned an error")
	}
	for _, name := range []string{"Content-Type", "Cache-Control", "X-Accel-Buffering"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(w, io.LimitReader(resp.Body, s.cfg.MaxBodyBytes*16))
	return int64(resp.StatusCode), copyErr
}

func privateTransport(cfg Config, serverName string) http.RoundTripper {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			host, _, err := net.SplitHostPort(conn.RemoteAddr().String())
			if err != nil || !isPrivateWorkerIP(net.ParseIP(host)) {
				_ = conn.Close()
				return nil, errors.New("worker connection was not private")
			}
			return conn, nil
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	if cfg.MTLSEnabled {
		tlsConfig, err := clientMTLSConfig(cfg, serverName)
		if err != nil {
			return nil
		}
		transport.TLSClientConfig = tlsConfig
	}
	return transport
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type workerEnrollRequest struct {
	Token       string `json:"token"`
	AgentID     string `json:"agent_id"`
	InternalURL string `json:"internal_url"`
}

type workerHeartbeatRequest struct {
	Status string `json:"status"`
}

func (d *database) enrollWorker(ctx context.Context, cfg Config, request workerEnrollRequest) (string, string, string, error) {
	if !strings.HasPrefix(request.Token, "enroll_") || len(request.AgentID) < 8 || len(request.AgentID) > 200 {
		return "", "", "", errors.New("invalid enrollment request")
	}
	internalURL, err := validateWorkerURL(ctx, cfg, request.InternalURL)
	if err != nil {
		return "", "", "", err
	}
	rawToken := strings.TrimPrefix(request.Token, "enroll_")
	if rawToken == "" {
		return "", "", "", errors.New("invalid enrollment token")
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return "", "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var attemptID, profileID, provider string
	if err := tx.QueryRow(ctx, `
		SELECT c.id::text, c.provider_profile_id::text, p.provider
		FROM connection_attempts c
		JOIN provider_profiles p ON p.id = c.provider_profile_id
		WHERE c.token_digest = $1 AND c.consumed_at IS NULL AND c.expires_at > now()
		FOR UPDATE`, keyedDigest(cfg.KeyPepper, []byte(rawToken))).Scan(&attemptID, &profileID, &provider); err != nil {
		return "", "", "", errors.New("invalid or expired enrollment token")
	}
	workerRaw, err := randomURLValue(32)
	if err != nil {
		return "", "", "", err
	}
	ciphertext, err := encryptSecret(cfg.KeyPepper, workerRaw)
	if err != nil {
		return "", "", "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE connection_attempts SET consumed_at = now() WHERE id = $1`, attemptID); err != nil {
		return "", "", "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO worker_runtimes (provider_profile_id, agent_id, status, credential_digest, internal_url, credential_ciphertext, last_heartbeat_at)
		VALUES ($1, $2, 'online', $3, $4, $5, now())
		ON CONFLICT (provider_profile_id) DO UPDATE SET
			agent_id = EXCLUDED.agent_id,
			status = 'online',
			credential_digest = EXCLUDED.credential_digest,
			internal_url = EXCLUDED.internal_url,
			credential_ciphertext = EXCLUDED.credential_ciphertext,
			last_heartbeat_at = now(),
			updated_at = now()`, profileID, request.AgentID, keyedDigest(cfg.KeyPepper, []byte(workerRaw)), internalURL, ciphertext); err != nil {
		return "", "", "", err
	}
	if _, err := tx.Exec(ctx, `UPDATE provider_profiles SET status = 'connecting', updated_at = now() WHERE id = $1`, profileID); err != nil {
		return "", "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", "", err
	}
	return workerRaw, profileID, provider, nil
}

func (d *database) heartbeatWorker(ctx context.Context, cfg Config, rawToken, status string) error {
	if status != "online" && status != "draining" && status != "offline" {
		status = "online"
	}
	result, err := d.pool.Exec(ctx, `
		UPDATE worker_runtimes
		SET status = $1, last_heartbeat_at = now(), updated_at = now()
		WHERE credential_digest = $2 AND status <> 'revoked'`, status, keyedDigest(cfg.KeyPepper, []byte(rawToken)))
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("worker credential not recognized")
	}
	return nil
}

func workerToken(r *http.Request) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func (s *server) internalWorkerRoute(w http.ResponseWriter, r *http.Request) {
	applySecurityHeaders(w, s.cfg.Environment)
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "Worker control plane is not configured.", "api_error")
		return
	}
	switch {
	case r.URL.Path == "/internal/worker/enroll" && r.Method == http.MethodPost:
		var request workerEnrollRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid worker enrollment request.", "invalid_request_error")
			return
		}
		raw, profileID, provider, err := s.db.enrollWorker(r.Context(), s.cfg, request)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Worker enrollment was rejected.", "authentication_error")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"worker_token": raw, "profile_id": profileID, "provider": provider, "warning": "Store this worker token only in the isolated worker runtime."})
	case r.URL.Path == "/internal/worker/heartbeat" && r.Method == http.MethodPost:
		token := workerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "Worker authentication required.", "authentication_error")
			return
		}
		var request workerHeartbeatRequest
		if r.Body != nil && r.ContentLength != 0 {
			_ = json.NewDecoder(r.Body).Decode(&request)
		}
		if err := s.db.heartbeatWorker(r.Context(), s.cfg, token, request.Status); err != nil {
			writeError(w, http.StatusUnauthorized, "Worker authentication failed.", "authentication_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
	default:
		writeError(w, http.StatusNotFound, "Not found.", "invalid_request_error")
	}
}

func validateWorkerURL(ctx context.Context, cfg Config, raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("worker internal_url must be an http(s) URL without credentials or query")
	}
	if cfg.MTLSEnabled && u.Scheme != "https" {
		return "", errors.New("mTLS-enabled workers must use https internal_url")
	}
	host := strings.ToLower(u.Hostname())
	allowed := false
	for _, candidate := range strings.Split(cfg.WorkerAllowedHosts, ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), host) {
			allowed = true
			break
		}
	}
	if !allowed {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return "", errors.New("worker internal_url host is not allowed")
		}
		for _, ip := range ips {
			if !isPrivateWorkerIP(ip) {
				return "", errors.New("worker internal_url must resolve only to private addresses")
			}
		}
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func isPrivateWorkerIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	return false
}

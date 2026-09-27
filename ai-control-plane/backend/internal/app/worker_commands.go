package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var errNoWorkerCommand = errors.New("no worker command available")
var errWorkerCredential = errors.New("worker credential not recognized")

type workerCommand struct {
	ID        string    `json:"command_id"`
	Type      string    `json:"type"`
	ProfileID string    `json:"profile_id"`
	AttemptID string    `json:"attempt_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type workerEventRequest struct {
	CommandID        string `json:"command_id"`
	AttemptID        string `json:"attempt_id"`
	State            string `json:"state"`
	AuthorizationURL string `json:"authorization_url"`
	UserCode         string `json:"user_code"`
	FailureCode      string `json:"failure_code"`
	FailureMessage   string `json:"failure_message"`
}

type loginChallenge struct {
	AuthorizationURL string    `json:"authorization_url"`
	UserCode         string    `json:"user_code"`
	ExpiresAt        time.Time `json:"expires_at"`
}

func (d *database) pollWorkerCommand(ctx context.Context, cfg Config, rawToken string) (workerCommand, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return workerCommand{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM worker_runtimes
			WHERE credential_digest = $1 AND status <> 'revoked'
		)`, keyedDigest(cfg.KeyPepper, []byte(rawToken))).Scan(&exists); err != nil {
		return workerCommand{}, err
	}
	if !exists {
		return workerCommand{}, errWorkerCredential
	}
	var command workerCommand
	var attemptID *string
	err = tx.QueryRow(ctx, `
		SELECT c.id::text, c.command_type, c.provider_profile_id::text,
		       c.provider_login_attempt_id::text, c.expires_at
		FROM worker_commands c
		JOIN worker_runtimes w ON w.provider_profile_id = c.provider_profile_id
		WHERE w.credential_digest = $1
		  AND w.status <> 'revoked'
		  AND c.status = 'queued'
		  AND c.expires_at > now()
		ORDER BY c.created_at
		FOR UPDATE OF c SKIP LOCKED
		LIMIT 1`, keyedDigest(cfg.KeyPepper, []byte(rawToken))).Scan(
		&command.ID, &command.Type, &command.ProfileID, &attemptID, &command.ExpiresAt)
	if err != nil {
		return workerCommand{}, errNoWorkerCommand
	}
	if attemptID != nil {
		command.AttemptID = *attemptID
	}
	if _, err := tx.Exec(ctx, `UPDATE worker_commands SET status = 'claimed', claimed_at = now() WHERE id = $1`, command.ID); err != nil {
		return workerCommand{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return workerCommand{}, err
	}
	return command, nil
}

func (d *database) applyWorkerEvent(ctx context.Context, cfg Config, rawToken string, request workerEventRequest) error {
	if request.CommandID == "" || request.AttemptID == "" {
		return errors.New("worker event command and attempt are required")
	}
	state := strings.TrimSpace(request.State)
	if state != "awaiting_authorization" && state != "validating" && state != "succeeded" && state != "failed" && state != "cancelled" && state != "expired" {
		return errors.New("worker event state is invalid")
	}
	urlValue, err := sanitizeLoginURL(request.AuthorizationURL)
	if err != nil {
		return err
	}
	userCode := sanitizeUserCode(request.UserCode)
	failureCode := truncate(strings.TrimSpace(request.FailureCode), 80)
	failureMessage := truncate(strings.TrimSpace(request.FailureMessage), 240)

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var profileID string
	if err := tx.QueryRow(ctx, `
		SELECT c.provider_profile_id::text
		FROM worker_commands c
		JOIN worker_runtimes w ON w.provider_profile_id = c.provider_profile_id
		WHERE c.id = $1 AND c.provider_login_attempt_id = $2
		  AND w.credential_digest = $3
		  AND w.status <> 'revoked'`, request.CommandID, request.AttemptID, keyedDigest(cfg.KeyPepper, []byte(rawToken))).Scan(&profileID); err != nil {
		return errWorkerCredential
	}
	attemptStatus, commandStatus, profileStatus := loginStateMapping(state)
	if _, err := tx.Exec(ctx, `
		UPDATE provider_login_attempts
		SET status = $1, failure_code = NULLIF($2, ''), failure_message = NULLIF($3, ''),
		    completed_at = CASE WHEN $1 IN ('succeeded', 'failed', 'cancelled', 'expired') THEN now() ELSE completed_at END
		WHERE id = $4 AND provider_profile_id = $5`, attemptStatus, failureCode, failureMessage, request.AttemptID, profileID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE worker_commands
		SET status = $1,
		    completed_at = CASE WHEN $1 IN ('completed', 'failed', 'cancelled', 'expired') THEN now() ELSE completed_at END,
		    result_code = NULLIF($2, '')
		WHERE id = $3 AND provider_profile_id = $4`, commandStatus, failureCode, request.CommandID, profileID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE provider_profiles SET status = $1, updated_at = now() WHERE id = $2 AND status <> 'disabled'`, profileStatus, profileID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if d.redis != nil {
		key := "provider-login:challenge:" + request.AttemptID
		if state == "awaiting_authorization" && urlValue != "" && userCode != "" {
			payload, _ := json.Marshal(loginChallenge{AuthorizationURL: urlValue, UserCode: userCode, ExpiresAt: time.Now().Add(cfg.ProviderLoginTTL)})
			_ = d.redis.Set(ctx, key, payload, cfg.ProviderLoginTTL).Err()
		} else if state != "awaiting_authorization" {
			_ = d.redis.Del(ctx, key).Err()
		}
	}
	return nil
}

func loginStateMapping(state string) (attemptStatus, commandStatus, profileStatus string) {
	switch state {
	case "awaiting_authorization":
		return "awaiting_authorization", "claimed", "login_pending"
	case "validating":
		return "validating", "claimed", "validating"
	case "succeeded":
		return "succeeded", "completed", "ready"
	case "failed":
		return "failed", "failed", "needs_relogin"
	case "cancelled":
		return "cancelled", "cancelled", "pending"
	default:
		return "expired", "expired", "needs_relogin"
	}
}

func sanitizeLoginURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	u, err := url.Parse(value)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("worker authorization URL is invalid")
	}
	if !strings.EqualFold(u.Hostname(), "auth.openai.com") {
		return "", errors.New("worker authorization URL host is not allowed")
	}
	return u.String(), nil
}

func sanitizeUserCode(value string) string {
	value = truncate(strings.TrimSpace(value), 64)
	for _, r := range value {
		if !(r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return ""
		}
	}
	return value
}

func (s *server) internalWorkerCommands(w http.ResponseWriter, r *http.Request) {
	token := workerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "Worker authentication required.", "authentication_error")
		return
	}
	command, err := s.db.pollWorkerCommand(r.Context(), s.cfg, token)
	if err != nil {
		if errors.Is(err, errNoWorkerCommand) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeError(w, http.StatusUnauthorized, "Worker authentication failed.", "authentication_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"command": command})
}

func (s *server) internalWorkerEvents(w http.ResponseWriter, r *http.Request) {
	token := workerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "Worker authentication required.", "authentication_error")
		return
	}
	var request workerEventRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid worker event.", "invalid_request_error")
		return
	}
	if err := s.db.applyWorkerEvent(r.Context(), s.cfg, token, request); err != nil {
		if errors.Is(err, errWorkerCredential) {
			writeError(w, http.StatusUnauthorized, "Worker authentication failed.", "authentication_error")
			return
		}
		writeError(w, http.StatusBadRequest, "Worker event was rejected.", "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

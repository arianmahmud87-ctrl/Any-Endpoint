package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type providerLoginAttempt struct {
	ID                 string     `json:"id"`
	ProfileID          string     `json:"profile_id"`
	Provider           string     `json:"provider"`
	Status             string     `json:"status"`
	CommandID          string     `json:"command_id,omitempty"`
	ExpiresAt          time.Time  `json:"expires_at"`
	CreatedAt          time.Time  `json:"created_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	FailureCode        string     `json:"failure_code,omitempty"`
	FailureMsg         string     `json:"failure_message,omitempty"`
	AuthorizationURL   string     `json:"authorization_url,omitempty"`
	UserCode           string     `json:"user_code,omitempty"`
	ChallengeExpiresAt *time.Time `json:"challenge_expires_at,omitempty"`
}

func (d *database) startProviderLogin(ctx context.Context, cfg Config, organizationID, actorID, profileID string) (providerLoginAttempt, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return providerLoginAttempt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var provider, profileStatus string
	if err := tx.QueryRow(ctx, `
		SELECT provider, status
		FROM provider_profiles
		WHERE id = $1 AND organization_id = $2
		FOR UPDATE`, profileID, organizationID).Scan(&provider, &profileStatus); err != nil {
		return providerLoginAttempt{}, errors.New("provider profile not found")
	}
	if provider != "codex" {
		return providerLoginAttempt{}, errors.New("direct provider login currently supports codex only")
	}
	if profileStatus == "disabled" {
		return providerLoginAttempt{}, errors.New("provider profile is disabled")
	}
	var active int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM provider_login_attempts
		WHERE provider_profile_id = $1
		  AND status IN ('pending', 'awaiting_authorization', 'validating')
		  AND expires_at > now()`, profileID).Scan(&active); err != nil {
		return providerLoginAttempt{}, err
	}
	if active > 0 {
		return providerLoginAttempt{}, errors.New("provider login is already in progress")
	}

	rawNonce, err := randomURLValue(32)
	if err != nil {
		return providerLoginAttempt{}, err
	}
	expiresAt := time.Now().Add(cfg.ProviderLoginTTL)
	var attempt providerLoginAttempt
	err = tx.QueryRow(ctx, `
		INSERT INTO provider_login_attempts
			(organization_id, provider_profile_id, initiated_by, provider, status, nonce_digest, expires_at)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6)
		RETURNING id::text, provider_profile_id::text, provider, status, expires_at, created_at`,
		organizationID, profileID, actorID, provider, keyedDigest(cfg.KeyPepper, []byte(rawNonce)), expiresAt).Scan(
		&attempt.ID, &attempt.ProfileID, &attempt.Provider, &attempt.Status, &attempt.ExpiresAt, &attempt.CreatedAt)
	if err != nil {
		return providerLoginAttempt{}, err
	}
	var commandID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO worker_commands (provider_profile_id, provider_login_attempt_id, command_type, expires_at)
		VALUES ($1, $2, 'provider_login_start', $3)
		RETURNING id::text`, profileID, attempt.ID, expiresAt).Scan(&commandID); err != nil {
		return providerLoginAttempt{}, err
	}
	attempt.CommandID = commandID
	if _, err := tx.Exec(ctx, `
		UPDATE provider_profiles
		SET status = 'login_pending', updated_at = now()
		WHERE id = $1 AND organization_id = $2`, profileID, organizationID); err != nil {
		return providerLoginAttempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return providerLoginAttempt{}, err
	}
	return attempt, nil
}

func (d *database) loadProviderLogin(ctx context.Context, organizationID, profileID, attemptID string) (providerLoginAttempt, error) {
	var attempt providerLoginAttempt
	err := d.pool.QueryRow(ctx, `
		SELECT a.id::text, a.provider_profile_id::text, a.provider, a.status,
		       COALESCE(c.id::text, ''), a.expires_at, a.created_at, a.completed_at,
		       COALESCE(a.failure_code, ''), COALESCE(a.failure_message, '')
		FROM provider_login_attempts a
		LEFT JOIN worker_commands c
		  ON c.provider_login_attempt_id = a.id AND c.command_type = 'provider_login_start'
		WHERE a.id = $1 AND a.provider_profile_id = $2 AND a.organization_id = $3`,
		attemptID, profileID, organizationID).Scan(
		&attempt.ID, &attempt.ProfileID, &attempt.Provider, &attempt.Status,
		&attempt.CommandID, &attempt.ExpiresAt, &attempt.CreatedAt, &attempt.CompletedAt,
		&attempt.FailureCode, &attempt.FailureMsg)
	if err != nil {
		return providerLoginAttempt{}, errors.New("provider login attempt not found")
	}
	if d.redis != nil {
		if payload, redisErr := d.redis.Get(ctx, "provider-login:challenge:"+attempt.ID).Bytes(); redisErr == nil {
			var challenge loginChallenge
			if json.Unmarshal(payload, &challenge) == nil && time.Now().Before(challenge.ExpiresAt) {
				attempt.AuthorizationURL = challenge.AuthorizationURL
				attempt.UserCode = challenge.UserCode
				attempt.ChallengeExpiresAt = &challenge.ExpiresAt
			}
		}
	}
	if time.Now().After(attempt.ExpiresAt) && isActiveLoginStatus(attempt.Status) {
		_, _ = d.pool.Exec(ctx, `
			UPDATE provider_login_attempts
			SET status = 'expired', completed_at = COALESCE(completed_at, now())
			WHERE id = $1 AND status IN ('pending', 'awaiting_authorization', 'validating')`, attempt.ID)
		attempt.Status = "expired"
		attempt.CompletedAt = timePtr(time.Now())
	}
	return attempt, nil
}

func (d *database) cancelProviderLogin(ctx context.Context, organizationID, profileID, attemptID string) (providerLoginAttempt, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return providerLoginAttempt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var attempt providerLoginAttempt
	err = tx.QueryRow(ctx, `
		UPDATE provider_login_attempts
		SET status = 'cancelled', completed_at = now()
		WHERE id = $1 AND provider_profile_id = $2 AND organization_id = $3
		  AND status IN ('pending', 'awaiting_authorization', 'validating')
		RETURNING id::text, provider_profile_id::text, provider, status, expires_at,
		          created_at, completed_at, COALESCE(failure_code, ''), COALESCE(failure_message, '')`,
		attemptID, profileID, organizationID).Scan(
		&attempt.ID, &attempt.ProfileID, &attempt.Provider, &attempt.Status, &attempt.ExpiresAt,
		&attempt.CreatedAt, &attempt.CompletedAt, &attempt.FailureCode, &attempt.FailureMsg)
	if err != nil {
		return providerLoginAttempt{}, errors.New("active provider login attempt not found")
	}
	if _, err := tx.Exec(ctx, `
		UPDATE provider_profiles
		SET status = 'pending', updated_at = now()
		WHERE id = $1 AND organization_id = $2 AND status <> 'disabled'`, profileID, organizationID); err != nil {
		return providerLoginAttempt{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE worker_commands
		SET status = 'cancelled', completed_at = now()
		WHERE provider_login_attempt_id = $1 AND status IN ('queued', 'claimed')`, attemptID); err != nil {
		return providerLoginAttempt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return providerLoginAttempt{}, err
	}
	return attempt, nil
}

func isActiveLoginStatus(status string) bool {
	return status == "pending" || status == "awaiting_authorization" || status == "validating"
}

func timePtr(value time.Time) *time.Time { return &value }

func (s *server) startProviderLogin(w http.ResponseWriter, r *http.Request, profileID string) {
	p, ok := s.requireSession(w, r)
	if !ok || !s.requireCSRF(w, r, p) {
		return
	}
	if p.Role != "owner" && p.Role != "admin" {
		writeError(w, http.StatusForbidden, "Organization administrator access required.", "authorization_error")
		return
	}
	attempt, err := s.db.startProviderLogin(r.Context(), s.cfg, p.OrganizationID, p.UserID, profileID)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "already in progress") {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error(), "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"login":       attempt,
		"next_action": "worker_login_dispatch_pending",
		"warning":     "Provider credentials remain in the isolated worker and are never returned to the browser.",
	})
}

func (s *server) providerLoginStatus(w http.ResponseWriter, r *http.Request, profileID, attemptID string) {
	p, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	attempt, err := s.db.loadProviderLogin(r.Context(), p.OrganizationID, profileID, attemptID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Provider login attempt not found.", "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"login": attempt})
}

func (s *server) cancelProviderLogin(w http.ResponseWriter, r *http.Request, profileID, attemptID string) {
	p, ok := s.requireSession(w, r)
	if !ok || !s.requireCSRF(w, r, p) {
		return
	}
	if p.Role != "owner" && p.Role != "admin" {
		writeError(w, http.StatusForbidden, "Organization administrator access required.", "authorization_error")
		return
	}
	attempt, err := s.db.cancelProviderLogin(r.Context(), p.OrganizationID, profileID, attemptID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Active provider login attempt not found.", "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"login": attempt})
}

func (s *server) retryProviderLogin(w http.ResponseWriter, r *http.Request, profileID, attemptID string) {
	p, ok := s.requireSession(w, r)
	if !ok || !s.requireCSRF(w, r, p) {
		return
	}
	if p.Role != "owner" && p.Role != "admin" {
		writeError(w, http.StatusForbidden, "Organization administrator access required.", "authorization_error")
		return
	}
	if _, err := s.db.cancelProviderLogin(r.Context(), p.OrganizationID, profileID, attemptID); err != nil {
		writeError(w, http.StatusNotFound, "Active provider login attempt not found.", "invalid_request_error")
		return
	}
	attempt, err := s.db.startProviderLogin(r.Context(), s.cfg, p.OrganizationID, p.UserID, profileID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"login": attempt, "next_action": "worker_login_dispatch_pending"})
}

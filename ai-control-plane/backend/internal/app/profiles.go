package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type providerProfile struct {
	ID            string     `json:"id"`
	Provider      string     `json:"provider"`
	Label         string     `json:"label"`
	Status        string     `json:"status"`
	AllowedModels []string   `json:"allowed_models"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	WorkerStatus  string     `json:"worker_status"`
	LastHeartbeat *time.Time `json:"last_heartbeat"`
}

type createProfileRequest struct {
	Provider      string   `json:"provider"`
	Label         string   `json:"label"`
	AllowedModels []string `json:"allowed_models"`
}

func (d *database) listProfiles(ctx context.Context, organizationID string) ([]providerProfile, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT p.id::text, p.provider, p.label, p.status, p.allowed_models,
		       p.created_at, p.updated_at, COALESCE(w.status, 'not_enrolled'), w.last_heartbeat_at
		FROM provider_profiles p
		LEFT JOIN worker_runtimes w ON w.provider_profile_id = p.id
		WHERE p.organization_id = $1
		ORDER BY p.created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles []providerProfile
	for rows.Next() {
		var profile providerProfile
		var models []byte
		if err := rows.Scan(&profile.ID, &profile.Provider, &profile.Label, &profile.Status, &models, &profile.CreatedAt, &profile.UpdatedAt, &profile.WorkerStatus, &profile.LastHeartbeat); err != nil {
			return nil, err
		}
		if len(models) > 0 && string(models) != "null" {
			if err := json.Unmarshal(models, &profile.AllowedModels); err != nil {
				return nil, fmt.Errorf("decode profile models: %w", err)
			}
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (d *database) createProfile(ctx context.Context, organizationID, actorID string, request createProfileRequest, secretBackend string) (providerProfile, error) {
	provider := strings.TrimSpace(request.Provider)
	label := strings.TrimSpace(request.Label)
	if provider != "codex" && provider != "claude_code" {
		return providerProfile{}, errors.New("provider must be codex or claude_code")
	}
	if label == "" || len([]rune(label)) > 120 {
		return providerProfile{}, errors.New("label must contain 1 to 120 characters")
	}
	models, err := json.Marshal(normalizeModels(request.AllowedModels))
	if err != nil {
		return providerProfile{}, err
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return providerProfile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var profile providerProfile
	var modelsJSON []byte
	if err := tx.QueryRow(ctx, `
		INSERT INTO provider_profiles (organization_id, provider, label, status, allowed_models)
		VALUES ($1, $2, $3, 'pending', $4)
		RETURNING id::text, provider, label, status, allowed_models, created_at, updated_at`, organizationID, provider, label, models).Scan(
		&profile.ID, &profile.Provider, &profile.Label, &profile.Status, &modelsJSON, &profile.CreatedAt, &profile.UpdatedAt); err != nil {
		return providerProfile{}, fmt.Errorf("create provider profile: %w", err)
	}
	profile.AllowedModels = normalizeModels(nil)
	if err := json.Unmarshal(modelsJSON, &profile.AllowedModels); err != nil {
		return providerProfile{}, fmt.Errorf("decode created profile models: %w", err)
	}
	externalRef := "profile/" + profile.ID
	var secretID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO secret_references (provider_profile_id, backend, external_ref)
		VALUES ($1, $2, $3)
		RETURNING id::text`, profile.ID, normalizeSecretBackend(secretBackend), externalRef).Scan(&secretID); err != nil {
		return providerProfile{}, fmt.Errorf("create secret reference: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE provider_profiles SET secret_reference_id = $1 WHERE id = $2`, secretID, profile.ID); err != nil {
		return providerProfile{}, fmt.Errorf("link secret reference: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return providerProfile{}, err
	}
	profile.WorkerStatus = "not_enrolled"
	return profile, nil
}

func (d *database) createConnectionAttempt(ctx context.Context, cfg Config, organizationID, actorID, profileID string) (string, time.Time, error) {
	var provider string
	if err := d.pool.QueryRow(ctx, `SELECT provider FROM provider_profiles WHERE id = $1 AND organization_id = $2 AND status <> 'disabled'`, profileID, organizationID).Scan(&provider); err != nil {
		return "", time.Time{}, errors.New("provider profile not found")
	}
	raw, err := randomURLValue(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(cfg.EnrollmentTTL)
	_, err = d.pool.Exec(ctx, `
		INSERT INTO connection_attempts (provider_profile_id, token_digest, created_by, expires_at)
		VALUES ($1, $2, $3, $4)`, profileID, keyedDigest(cfg.KeyPepper, []byte(raw)), actorID, expires)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("create enrollment attempt: %w", err)
	}
	if _, err := d.pool.Exec(ctx, `UPDATE provider_profiles SET status = 'connecting', updated_at = now() WHERE id = $1 AND organization_id = $2`, profileID, organizationID); err != nil {
		return "", time.Time{}, err
	}
	return "enroll_" + raw, expires, nil
}

func normalizeModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	result := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 120 {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
		if len(result) == 100 {
			break
		}
	}
	return result
}

func normalizeSecretBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "aws", "aws_secrets_manager", "aws-secrets-manager":
		return "aws_secrets_manager"
	case "gcore", "gcore_secret_store", "gcore-secret-store":
		return "gcore_secret_store"
	case "vault":
		return "vault"
	default:
		return "local_dev"
	}
}

func (s *server) profiles(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		profiles, err := s.db.listProfiles(r.Context(), p.OrganizationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not list provider profiles.", "api_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": profiles, "object": "list"})
		return
	}
	if r.Method != http.MethodPost || !s.requireCSRF(w, r, p) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
		}
		return
	}
	if p.Role != "owner" && p.Role != "admin" {
		writeError(w, http.StatusForbidden, "Organization administrator access required.", "authorization_error")
		return
	}
	var request createProfileRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid profile request.", "invalid_request_error")
		return
	}
	profile, err := s.db.createProfile(r.Context(), p.OrganizationID, p.UserID, request, s.cfg.SecretBackend)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"profile": profile, "secret": map[string]any{"status": "reference_created"}})
}

func (s *server) connectProfile(w http.ResponseWriter, r *http.Request, profileID string) {
	p, ok := s.requireSession(w, r)
	if !ok || !s.requireCSRF(w, r, p) {
		return
	}
	if p.Role != "owner" && p.Role != "admin" {
		writeError(w, http.StatusForbidden, "Organization administrator access required.", "authorization_error")
		return
	}
	token, expires, err := s.db.createConnectionAttempt(r.Context(), s.cfg, p.OrganizationID, p.UserID, profileID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Provider profile not found.", "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"profile_id":       profileID,
		"enrollment_token": token,
		"expires_at":       expires,
		"warning":          "Copy this token now. It will not be shown again and must only be used by the private worker agent.",
	})
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after JSON object")
	}
	return nil
}

func (s *server) profileSubroute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/profiles/"), "/")
	if len(parts) == 3 && parts[0] != "" && parts[1] == "login" {
		switch {
		case parts[2] == "start" && r.Method == http.MethodPost:
			s.startProviderLogin(w, r, parts[0])
		case parts[2] == "status" && r.Method == http.MethodGet:
			s.providerLoginStatus(w, r, parts[0], r.URL.Query().Get("attempt_id"))
		case parts[2] == "cancel" && r.Method == http.MethodPost:
			s.cancelProviderLogin(w, r, parts[0], r.URL.Query().Get("attempt_id"))
		case parts[2] == "retry" && r.Method == http.MethodPost:
			s.retryProviderLogin(w, r, parts[0], r.URL.Query().Get("attempt_id"))
		default:
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
		}
		return
	}
	if len(parts) == 2 && parts[0] != "" && (parts[1] == "connect" || parts[1] == "disable" || parts[1] == "reconnect") && r.Method == http.MethodPost {
		if parts[1] == "connect" {
			s.connectProfile(w, r, parts[0])
		} else {
			s.lifecycleProfile(w, r, parts[0], parts[1])
		}
		return
	}
	if len(parts) == 1 && parts[0] != "" && r.Method == http.MethodGet {
		p, ok := s.requireSession(w, r)
		if !ok {
			return
		}
		profiles, err := s.db.listProfiles(r.Context(), p.OrganizationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not load provider profile.", "api_error")
			return
		}
		for _, profile := range profiles {
			if profile.ID == parts[0] {
				writeJSON(w, http.StatusOK, map[string]any{"profile": profile})
				return
			}
		}
	}
	writeError(w, http.StatusNotFound, "Provider profile not found.", "invalid_request_error")
}

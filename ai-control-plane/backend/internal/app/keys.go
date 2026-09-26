package app

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type keyGrantRequest struct {
	ProfileID         string   `json:"profile_id"`
	ModelPattern      string   `json:"model_pattern"`
	Routes            []string `json:"routes"`
	RequestsPerMinute int      `json:"requests_per_minute"`
}

type createKeyRequest struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	ExpiresAt *time.Time        `json:"expires_at"`
	Grants    []keyGrantRequest `json:"grants"`
}

type apiKeyGrant struct {
	ProfileID         string `json:"profile_id"`
	Provider          string `json:"provider"`
	ModelPattern      string `json:"model_pattern,omitempty"`
	Route             string `json:"route"`
	RequestsPerMinute int    `json:"requests_per_minute"`
}

type apiKeyMetadata struct {
	ID             string        `json:"id"`
	OrganizationID string        `json:"-"`
	Name           string        `json:"name"`
	Prefix         string        `json:"prefix"`
	Type           string        `json:"type"`
	State          string        `json:"state"`
	ExpiresAt      *time.Time    `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time    `json:"last_used_at,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	RevokedAt      *time.Time    `json:"revoked_at,omitempty"`
	Grants         []apiKeyGrant `json:"grants"`
}

type verifiedAPIKey struct {
	Metadata apiKeyMetadata
}

func generateAPIKey(cfg Config) (prefix, raw string, verifier []byte, err error) {
	id, err := randomURLValue(9)
	if err != nil {
		return "", "", nil, err
	}
	secret, err := randomURLValue(32)
	if err != nil {
		return "", "", nil, err
	}
	prefix = "skv1_" + id
	raw = prefix + "_" + secret
	return prefix, raw, keyedDigest(cfg.KeyPepper, []byte(secret)), nil
}

func parseAPIKey(raw string) (prefix, secret string, ok bool) {
	parts := strings.Split(strings.TrimSpace(raw), "_")
	if len(parts) != 3 || parts[0] != "skv1" || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	return "skv1_" + parts[1], parts[2], true
}

func (d *database) createAPIKey(ctx context.Context, cfg Config, organizationID, actorID string, request createKeyRequest) (apiKeyMetadata, string, error) {
	name := strings.TrimSpace(request.Name)
	keyType := strings.ToLower(strings.TrimSpace(request.Type))
	if name == "" || len([]rune(name)) > 120 {
		return apiKeyMetadata{}, "", errors.New("key name must contain 1 to 120 characters")
	}
	if keyType != "codex" && keyType != "claude" && keyType != "universal" {
		return apiKeyMetadata{}, "", errors.New("key type must be codex, claude, or universal")
	}
	if len(request.Grants) == 0 || len(request.Grants) > 100 {
		return apiKeyMetadata{}, "", errors.New("at least one and at most 100 grants are required")
	}
	if request.ExpiresAt != nil {
		if !request.ExpiresAt.After(time.Now()) || request.ExpiresAt.After(time.Now().Add(10*365*24*time.Hour)) {
			return apiKeyMetadata{}, "", errors.New("expires_at must be in the future and within 10 years")
		}
	}
	prefix, raw, verifier, err := generateAPIKey(cfg)
	if err != nil {
		return apiKeyMetadata{}, "", err
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return apiKeyMetadata{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var metadata apiKeyMetadata
	if err := tx.QueryRow(ctx, `
		INSERT INTO api_keys (organization_id, name, prefix, verifier, key_type, state, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, 'active', $6, $7)
		RETURNING id::text, name, prefix, key_type, state, expires_at, created_at`, organizationID, name, prefix, verifier, keyType, request.ExpiresAt, actorID).Scan(
		&metadata.ID, &metadata.Name, &metadata.Prefix, &metadata.Type, &metadata.State, &metadata.ExpiresAt, &metadata.CreatedAt); err != nil {
		return apiKeyMetadata{}, "", fmt.Errorf("create API key: %w", err)
	}
	for _, grantRequest := range request.Grants {
		provider, err := validateGrantProfile(ctx, tx, organizationID, keyType, grantRequest.ProfileID)
		if err != nil {
			return apiKeyMetadata{}, "", err
		}
		routes := normalizeRoutes(grantRequest.Routes)
		if len(routes) == 0 {
			return apiKeyMetadata{}, "", errors.New("each grant needs at least one valid route")
		}
		rpm := grantRequest.RequestsPerMinute
		if rpm == 0 {
			rpm = 60
		}
		if rpm < 1 || rpm > 100000 {
			return apiKeyMetadata{}, "", errors.New("requests_per_minute is out of range")
		}
		pattern := strings.TrimSpace(grantRequest.ModelPattern)
		if len(pattern) > 120 {
			return apiKeyMetadata{}, "", errors.New("model_pattern is too long")
		}
		for _, route := range routes {
			if provider == "claude_code" && route == "responses" {
				return apiKeyMetadata{}, "", errors.New("Claude Code grants do not support the Responses route")
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO api_key_grants (api_key_id, provider_profile_id, provider, model_pattern, route, requests_per_minute)
				VALUES ($1, $2, $3, $4, $5, $6)`, metadata.ID, grantRequest.ProfileID, provider, nullIfEmpty(pattern), route, rpm); err != nil {
				return apiKeyMetadata{}, "", fmt.Errorf("create API key grant: %w", err)
			}
			metadata.Grants = append(metadata.Grants, apiKeyGrant{ProfileID: grantRequest.ProfileID, Provider: provider, ModelPattern: pattern, Route: route, RequestsPerMinute: rpm})
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return apiKeyMetadata{}, "", err
	}
	return metadata, raw, nil
}

func validateGrantProfile(ctx context.Context, tx pgx.Tx, organizationID, keyType, profileID string) (string, error) {
	var provider string
	if err := tx.QueryRow(ctx, `SELECT provider FROM provider_profiles WHERE id = $1 AND organization_id = $2 AND status <> 'disabled'`, profileID, organizationID).Scan(&provider); err != nil {
		return "", errors.New("grant profile is not available in this organization")
	}
	if (keyType == "codex" && provider != "codex") || (keyType == "claude" && provider != "claude_code") {
		return "", errors.New("grant provider does not match key type")
	}
	return provider, nil
}

func (d *database) listAPIKeys(ctx context.Context, organizationID string) ([]apiKeyMetadata, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id::text, name, prefix, key_type, state, expires_at, last_used_at, created_at, revoked_at
		FROM api_keys WHERE organization_id = $1 ORDER BY created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []apiKeyMetadata
	for rows.Next() {
		var item apiKeyMetadata
		if err := rows.Scan(&item.ID, &item.Name, &item.Prefix, &item.Type, &item.State, &item.ExpiresAt, &item.LastUsedAt, &item.CreatedAt, &item.RevokedAt); err != nil {
			return nil, err
		}
		item.Grants, err = d.listKeyGrants(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (d *database) listKeyGrants(ctx context.Context, keyID string) ([]apiKeyGrant, error) {
	rows, err := d.pool.Query(ctx, `SELECT provider_profile_id::text, provider, COALESCE(model_pattern, ''), route, requests_per_minute FROM api_key_grants WHERE api_key_id = $1 ORDER BY provider_profile_id, route`, keyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []apiKeyGrant
	for rows.Next() {
		var item apiKeyGrant
		if err := rows.Scan(&item.ProfileID, &item.Provider, &item.ModelPattern, &item.Route, &item.RequestsPerMinute); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (d *database) revokeAPIKey(ctx context.Context, organizationID, actorID, keyID string) error {
	result, err := d.pool.Exec(ctx, `UPDATE api_keys SET state = 'revoked', revoked_at = now() WHERE id = $1 AND organization_id = $2 AND state <> 'revoked'`, keyID, organizationID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("API key not found or already revoked")
	}
	return nil
}

func (d *database) rotateAPIKey(ctx context.Context, cfg Config, organizationID, actorID, keyID string) (apiKeyMetadata, string, error) {
	var request createKeyRequest
	err := d.pool.QueryRow(ctx, `SELECT name, key_type, expires_at FROM api_keys WHERE id = $1 AND organization_id = $2 AND state <> 'revoked'`, keyID, organizationID).Scan(&request.Name, &request.Type, &request.ExpiresAt)
	if err != nil {
		return apiKeyMetadata{}, "", errors.New("API key not found or already revoked")
	}
	rows, err := d.pool.Query(ctx, `SELECT provider_profile_id::text, COALESCE(model_pattern, ''), route, requests_per_minute FROM api_key_grants WHERE api_key_id = $1`, keyID)
	if err != nil {
		return apiKeyMetadata{}, "", err
	}
	for rows.Next() {
		var grant keyGrantRequest
		var route string
		if err := rows.Scan(&grant.ProfileID, &grant.ModelPattern, &route, &grant.RequestsPerMinute); err != nil {
			rows.Close()
			return apiKeyMetadata{}, "", err
		}
		grant.Routes = []string{route}
		request.Grants = append(request.Grants, grant)
	}
	rows.Close()
	metadata, raw, err := d.createAPIKey(ctx, cfg, organizationID, actorID, request)
	if err != nil {
		return apiKeyMetadata{}, "", err
	}
	if _, err := d.pool.Exec(ctx, `UPDATE api_keys SET rotated_from = $1 WHERE id = $2 AND organization_id = $3`, keyID, metadata.ID, organizationID); err != nil {
		_, _ = d.pool.Exec(ctx, `UPDATE api_keys SET state = 'revoked', revoked_at = now() WHERE id = $1`, metadata.ID)
		return apiKeyMetadata{}, "", err
	}
	if err := d.revokeAPIKey(ctx, organizationID, actorID, keyID); err != nil {
		_, _ = d.pool.Exec(ctx, `UPDATE api_keys SET state = 'revoked', revoked_at = now() WHERE id = $1`, metadata.ID)
		return apiKeyMetadata{}, "", err
	}
	return metadata, raw, nil
}

func (s *server) apiKeys(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		keys, err := s.db.listAPIKeys(r.Context(), p.OrganizationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not list API keys.", "api_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": keys})
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
	var request createKeyRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid API key request.", "invalid_request_error")
		return
	}
	metadata, raw, err := s.db.createAPIKey(r.Context(), s.cfg, p.OrganizationID, p.UserID, request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "invalid_request_error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"key": raw, "metadata": metadata, "warning": "Save this key now. It will never be shown again."})
}

func (s *server) apiKeySubroute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/keys/"), "/")
	if len(parts) == 2 && parts[0] != "" && (parts[1] == "rotate" || parts[1] == "revoke") && r.Method == http.MethodPost {
		p, ok := s.requireSession(w, r)
		if !ok || !s.requireCSRF(w, r, p) {
			return
		}
		if p.Role != "owner" && p.Role != "admin" {
			writeError(w, http.StatusForbidden, "Organization administrator access required.", "authorization_error")
			return
		}
		if parts[1] == "revoke" {
			if err := s.db.revokeAPIKey(r.Context(), p.OrganizationID, p.UserID, parts[0]); err != nil {
				writeError(w, http.StatusNotFound, "API key not found or already revoked.", "invalid_request_error")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
			return
		}
		metadata, raw, err := s.db.rotateAPIKey(r.Context(), s.cfg, p.OrganizationID, p.UserID, parts[0])
		if err != nil {
			writeError(w, http.StatusNotFound, "API key could not be rotated.", "invalid_request_error")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"key": raw, "metadata": metadata, "warning": "Save this key now. It will never be shown again."})
		return
	}
	writeError(w, http.StatusNotFound, "API key not found.", "invalid_request_error")
}

func normalizeRoutes(routes []string) []string {
	seen := make(map[string]struct{}, len(routes))
	var result []string
	for _, route := range routes {
		route = strings.TrimSpace(route)
		if route != "models" && route != "responses" && route != "chat.completions" {
			continue
		}
		if _, exists := seen[route]; exists {
			continue
		}
		seen[route] = struct{}{}
		result = append(result, route)
	}
	return result
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func apiKeySecretMatches(stored, candidate []byte) bool {
	return len(stored) == len(candidate) && subtle.ConstantTimeCompare(stored, candidate) == 1
}

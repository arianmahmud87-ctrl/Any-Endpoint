package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type usageRow struct {
	Route           string    `json:"route"`
	Model           string    `json:"model,omitempty"`
	StatusCode      int       `json:"status_code"`
	LatencyMS       int       `json:"latency_ms,omitempty"`
	InputTokens     int       `json:"input_tokens,omitempty"`
	OutputTokens    int       `json:"output_tokens,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	KeyID           string    `json:"key_id,omitempty"`
	ProviderProfile string    `json:"provider_profile_id,omitempty"`
}

func (d *database) listUsage(ctx context.Context, organizationID string, limit int) ([]usageRow, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT route, COALESCE(model, ''), status_code, COALESCE(latency_ms, 0),
		       COALESCE(input_tokens, 0), COALESCE(output_tokens, 0), created_at,
		       COALESCE(api_key_id::text, ''), COALESCE(provider_profile_id::text, '')
		FROM usage_events
		WHERE organization_id = $1
		ORDER BY created_at DESC LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []usageRow
	for rows.Next() {
		var row usageRow
		if err := rows.Scan(&row.Route, &row.Model, &row.StatusCode, &row.LatencyMS, &row.InputTokens, &row.OutputTokens, &row.CreatedAt, &row.KeyID, &row.ProviderProfile); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (d *database) operationsSummary(ctx context.Context, organizationID string) (map[string]any, error) {
	var profiles, activeKeys, onlineWorkers, staleWorkers, requests, failures int64
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM provider_profiles WHERE organization_id = $1`, organizationID).Scan(&profiles); err != nil {
		return nil, err
	}
	if err := d.pool.QueryRow(ctx, `SELECT count(*) FROM api_keys WHERE organization_id = $1 AND state = 'active' AND (expires_at IS NULL OR expires_at > now())`, organizationID).Scan(&activeKeys); err != nil {
		return nil, err
	}
	if err := d.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE w.status = 'online' AND w.last_heartbeat_at > now() - interval '2 minutes'),
		       count(*) FILTER (WHERE w.status = 'online' AND (w.last_heartbeat_at IS NULL OR w.last_heartbeat_at <= now() - interval '2 minutes'))
		FROM worker_runtimes w JOIN provider_profiles p ON p.id = w.provider_profile_id
		WHERE p.organization_id = $1`, organizationID).Scan(&onlineWorkers, &staleWorkers); err != nil {
		return nil, err
	}
	if err := d.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status_code >= 400) FROM usage_events WHERE organization_id = $1 AND created_at > now() - interval '24 hours'`, organizationID).Scan(&requests, &failures); err != nil {
		return nil, err
	}
	return map[string]any{
		"profiles":      map[string]any{"total": profiles},
		"keys":          map[string]any{"active": activeKeys},
		"workers":       map[string]any{"online": onlineWorkers, "stale": staleWorkers},
		"last_24_hours": map[string]any{"requests": requests, "failures": failures},
	}, nil
}

func (d *database) disableProfile(ctx context.Context, organizationID, actorID, profileID string) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `UPDATE provider_profiles SET status = 'disabled', updated_at = now() WHERE id = $1 AND organization_id = $2`, profileID, organizationID)
	if err != nil || result.RowsAffected() != 1 {
		return errors.New("provider profile not found")
	}
	if _, err := tx.Exec(ctx, `UPDATE worker_runtimes SET status = 'revoked', credential_digest = NULL, credential_ciphertext = NULL, updated_at = now() WHERE provider_profile_id = $1`, profileID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *database) prepareProfileReconnect(ctx context.Context, organizationID, actorID, profileID string) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `UPDATE provider_profiles SET status = 'pending', updated_at = now() WHERE id = $1 AND organization_id = $2`, profileID, organizationID)
	if err != nil || result.RowsAffected() != 1 {
		return errors.New("provider profile not found")
	}
	if _, err := tx.Exec(ctx, `UPDATE worker_runtimes SET status = 'draining', credential_digest = NULL, credential_ciphertext = NULL, updated_at = now() WHERE provider_profile_id = $1`, profileID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func limitFromRequest(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if value < 1 {
		return 50
	}
	if value > 200 {
		return 200
	}
	return value
}

func (s *server) usage(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	rows, err := s.db.listUsage(r.Context(), p.OrganizationID, limitFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load usage.", "api_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": rows})
}

func (s *server) operations(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	summary, err := s.db.operationsSummary(r.Context(), p.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load operations summary.", "api_error")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *server) lifecycleProfile(w http.ResponseWriter, r *http.Request, profileID, action string) {
	p, ok := s.requireSession(w, r)
	if !ok || !s.requireCSRF(w, r, p) {
		return
	}
	if p.Role != "owner" && p.Role != "admin" {
		writeError(w, http.StatusForbidden, "Organization administrator access required.", "authorization_error")
		return
	}
	if action == "disable" {
		if err := s.db.disableProfile(r.Context(), p.OrganizationID, p.UserID, profileID); err != nil {
			writeError(w, http.StatusNotFound, "Provider profile not found.", "invalid_request_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
		return
	}
	if action == "reconnect" {
		if err := s.db.prepareProfileReconnect(r.Context(), p.OrganizationID, p.UserID, profileID); err != nil {
			writeError(w, http.StatusNotFound, "Provider profile not found.", "invalid_request_error")
			return
		}
		token, expires, err := s.db.createConnectionAttempt(r.Context(), s.cfg, p.OrganizationID, p.UserID, profileID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create reconnect attempt.", "api_error")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"status": "connecting", "profile_id": profileID, "enrollment_token": token, "expires_at": expires})
		return
	}
	writeError(w, http.StatusNotFound, "Unknown lifecycle action.", "invalid_request_error")
}

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type playgroundRequest struct {
	ProfileID string `json:"profile_id"`
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	Stream    bool   `json:"stream"`
}

type playgroundProfile struct {
	Provider string
	Models   []string
}

func (d *database) getPlaygroundProfile(ctx context.Context, organizationID, profileID string) (playgroundProfile, error) {
	var profile playgroundProfile
	var rawModels []byte
	err := d.pool.QueryRow(ctx, `
		SELECT provider, allowed_models
		FROM provider_profiles
		WHERE id = $1 AND organization_id = $2 AND status <> 'disabled'`, profileID, organizationID).Scan(&profile.Provider, &rawModels)
	if err != nil {
		return playgroundProfile{}, errors.New("profile is not available")
	}
	if len(rawModels) == 0 || json.Unmarshal(rawModels, &profile.Models) != nil || len(profile.Models) == 0 {
		return playgroundProfile{}, errors.New("profile has no configured model scope")
	}
	return profile, nil
}

func modelAllowed(models []string, model string) bool {
	for _, allowed := range models {
		if allowed == model || allowed == "*" {
			return true
		}
	}
	return false
}

func (s *server) playground(w http.ResponseWriter, r *http.Request) {
	p, ok := s.requireSession(w, r)
	if !ok || !s.requireCSRF(w, r, p) {
		return
	}
	var request playgroundRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid playground request.", "invalid_request_error")
		return
	}
	request.ProfileID = strings.TrimSpace(request.ProfileID)
	request.Model = strings.TrimSpace(request.Model)
	if request.ProfileID == "" || request.Model == "" || len([]rune(request.Prompt)) == 0 || len([]rune(request.Prompt)) > 20000 {
		writeError(w, http.StatusBadRequest, "profile_id, model, and a prompt of 1–20000 characters are required.", "invalid_request_error")
		return
	}
	profile, err := s.db.getPlaygroundProfile(r.Context(), p.OrganizationID, request.ProfileID)
	if err != nil || !modelAllowed(profile.Models, request.Model) {
		writeError(w, http.StatusForbidden, "The selected profile or model is not authorized.", "authorization_error")
		return
	}
	if profile.Provider == "claude_code" && request.Stream {
		// The current Claude bridge emits buffered compatibility SSE, so keep the
		// UI contract explicit rather than implying token-level streaming.
		request.Stream = false
	}
	allowed, err := s.db.rateLimit(r.Context(), "playground:"+p.UserID, request.ProfileID, "playground", s.cfg.PlaygroundRPM)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Playground rate limiter unavailable.", "api_error")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "Playground rate limit exceeded.", "rate_limit_error")
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
	route, err := s.db.getWorkerRoute(r.Context(), s.cfg, p.OrganizationID, request.ProfileID)
	if err != nil || route.Provider != profile.Provider {
		writeError(w, http.StatusServiceUnavailable, "Selected provider worker is unavailable.", "api_error")
		return
	}
	payload := map[string]any{
		"model":    request.Model,
		"messages": []any{map[string]any{"role": "user", "content": request.Prompt}},
		"stream":   request.Stream,
	}
	data, _ := json.Marshal(payload)
	workerRequest := r.Clone(r.Context())
	workerRequest.Method = http.MethodPost
	workerRequest.URL.Path = "/v1/chat/completions"
	workerRequest.Body = io.NopCloser(bytes.NewReader(data))
	workerRequest.ContentLength = int64(len(data))
	workerRequest.Header = make(http.Header)
	workerRequest.Header.Set("Content-Type", "application/json")
	started := time.Now()
	status, err := s.forwardWorker(w, workerRequest, route, "chat.completions")
	latency := time.Since(started).Milliseconds()
	if status < 100 || status > 599 {
		status = http.StatusBadGateway
	}
	usageCtx, usageCancel := context.WithTimeout(context.Background(), time.Second)
	s.db.recordUsage(usageCtx, p.OrganizationID, "", request.ProfileID, request.Model, "playground", status, latency)
	usageCancel()
	if err != nil {
		writeError(w, http.StatusBadGateway, "Provider worker request failed.", "api_error")
	}
}

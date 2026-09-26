package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type server struct {
	cfg          Config
	logger       *slog.Logger
	db           *database
	oidc         *oidcService
	gatewaySlots chan struct{}
}

type contextKey string

const requestIDKey contextKey = "request_id"

func Run() error {
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("configuration rejected before listener bind: %w", err)
	}
	logger := newLogger(cfg.Environment)
	db, err := openDatabase(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("authentication dependencies rejected before listener bind: %w", err)
	}
	defer db.close()
	oidcCtx, cancelOIDC := context.WithTimeout(context.Background(), 15*time.Second)
	oidc, err := newOIDC(oidcCtx, cfg)
	cancelOIDC()
	if err != nil {
		return fmt.Errorf("OIDC configuration rejected before listener bind: %w", err)
	}
	s := &server{cfg: cfg, logger: logger, db: db, oidc: oidc, gatewaySlots: make(chan struct{}, cfg.GatewayMaxConcurrent)}
	publicServer := newHTTPServer(cfg, cfg.HTTPAddr, s)
	internalServer := newHTTPServer(cfg, cfg.InternalHTTPAddr, http.HandlerFunc(s.internalServeHTTP))
	if cfg.MTLSEnabled {
		tlsConfig, err := serverMTLSConfig(cfg)
		if err != nil {
			return fmt.Errorf("mTLS configuration rejected before listener bind: %w", err)
		}
		internalServer.TLSConfig = tlsConfig
	}

	logger.Info("server_starting", "addr", cfg.HTTPAddr, "internal_addr", cfg.InternalHTTPAddr, "environment", cfg.Environment, "max_body_bytes", cfg.MaxBodyBytes)
	serveErr := make(chan error, 2)
	go func() { serveErr <- publicServer.ListenAndServe() }()
	go func() {
		if cfg.MTLSEnabled {
			serveErr <- internalServer.ListenAndServeTLS("", "")
			return
		}
		serveErr <- internalServer.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErr:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = publicServer.Shutdown(shutdownCtx)
		_ = internalServer.Shutdown(shutdownCtx)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		logger.Info("server_shutdown", "reason", "signal")
		if err := publicServer.Shutdown(shutdownCtx); err != nil {
			_ = internalServer.Close()
			return fmt.Errorf("public graceful shutdown failed: %w", err)
		}
		if err := internalServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("internal graceful shutdown failed: %w", err)
		}
		return nil
	}
}

func newHTTPServer(cfg Config, address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := newRequestID()
	w.Header().Set("X-Request-ID", requestID)
	applySecurityHeaders(w, s.cfg.Environment)
	captured := &responseCapture{ResponseWriter: w}
	started := time.Now()
	if r.Body != nil {
		r.Body = http.MaxBytesReader(captured, r.Body, s.cfg.MaxBodyBytes)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error("request_panic", "request_id", requestID, "panic_type", fmt.Sprintf("%T", recovered))
			if !captured.wroteHeader {
				writeError(captured, http.StatusInternalServerError, "Internal server error.", "api_error")
			}
		}
		s.logger.Info("request_complete", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", captured.statusCode(), "bytes", captured.bytes, "duration_ms", time.Since(started).Milliseconds())
	}()
	s.route(captured, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
}

func (s *server) route(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/healthz":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "ai-control-plane"})
	case r.URL.Path == "/readyz":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		s.readiness(w, r)
	case r.URL.Path == "/api/auth/google/start":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		if s.oidc == nil {
			writeError(w, http.StatusServiceUnavailable, "Google authentication is not configured.", "api_error")
			return
		}
		if err := s.oidc.start(r.Context(), s.db, w, r); err != nil {
			s.logger.Error("oauth_start_failed", "request_id", requestIDFromContext(r.Context()), "reason", "state_store_unavailable")
			writeError(w, http.StatusServiceUnavailable, "Google authentication is not ready.", "api_error")
		}
	case r.URL.Path == "/api/auth/google/callback":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		if s.oidc == nil {
			writeError(w, http.StatusServiceUnavailable, "Google authentication is not configured.", "api_error")
			return
		}
		identity, session, err := s.oidc.callback(r.Context(), s.db, w, r)
		if err != nil {
			s.logger.Warn("oauth_callback_rejected", "request_id", requestIDFromContext(r.Context()), "reason", "verification_or_exchange_failed")
			writeError(w, http.StatusBadRequest, "Google sign-in could not be completed.", "authentication_error")
			return
		}
		s.setSessionCookie(w, session)
		s.logger.Info("oauth_login_succeeded", "request_id", requestIDFromContext(r.Context()), "user_id", identity.UserID)
		http.Redirect(w, r, "/", http.StatusFound)
	case r.URL.Path == "/api/auth/logout":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		s.logout(w, r)
	case r.URL.Path == "/api/playground/runs":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		s.playground(w, r)
	case r.URL.Path == "/api/usage":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		s.usage(w, r)
	case r.URL.Path == "/api/operations/summary":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		s.operations(w, r)
	case r.URL.Path == "/api/keys":
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		s.apiKeys(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/keys/"):
		s.apiKeySubroute(w, r)
	case r.URL.Path == "/api/profiles":
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		s.profiles(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/profiles/"):
		s.profileSubroute(w, r)
	case r.URL.Path == "/api/me":
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "Method not allowed.", "invalid_request_error")
			return
		}
		s.me(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/"):
		s.managementNotImplemented(w, r)
	case strings.HasPrefix(r.URL.Path, "/v1/"):
		s.publicGateway(w, r)
	case r.URL.Path == "/":
		writeError(w, http.StatusNotImplemented, "This API surface is not enabled in Phase 4.", "api_error")
	default:
		writeError(w, http.StatusNotFound, "Not found.", "invalid_request_error")
	}
}

func (s *server) readiness(w http.ResponseWriter, r *http.Request) {
	var results map[string]dependencyResult
	if s.db != nil {
		results = map[string]dependencyResult{"database": {Status: "reachable"}, "redis": {Status: "reachable"}}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		err := s.db.ping(ctx)
		cancel()
		if err != nil {
			results["database"] = dependencyResult{Status: "unreachable"}
			results["redis"] = dependencyResult{Status: "unreachable"}
		}
	} else {
		results = map[string]dependencyResult{}
		var wg sync.WaitGroup
		var mu sync.Mutex
		for name, rawURL := range map[string]string{"database": s.cfg.DatabaseURL, "redis": s.cfg.RedisURL} {
			name, rawURL := name, rawURL
			wg.Add(1)
			go func() {
				defer wg.Done()
				fallbackPort := "5432"
				if name == "redis" {
					fallbackPort = "6379"
				}
				result := probeDependency(r.Context(), rawURL, fallbackPort)
				mu.Lock()
				results[name] = result
				mu.Unlock()
			}()
		}
		wg.Wait()
	}
	ready := dependencyReady(results["database"], s.cfg.Environment == "production") && dependencyReady(results["redis"], s.cfg.Environment == "production")
	if s.cfg.AuthEnabled && s.oidc == nil {
		ready = false
	}
	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}
	writeJSON(w, status, map[string]any{
		"status":         state,
		"service":        "ai-control-plane",
		"dependencies":   results,
		"authentication": map[string]any{"enabled": s.cfg.AuthEnabled, "configured": s.oidc != nil},
	})
}

func applySecurityHeaders(w http.ResponseWriter, environment string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	if environment == "production" {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

type responseCapture struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *responseCapture) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseCapture) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(data)
	w.bytes += int64(written)
	return written, err
}

func (w *responseCapture) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message, typ string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": message, "type": typ}})
}

func newRequestID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err == nil {
		return hex.EncodeToString(value)
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func (s *server) setSessionCookie(w http.ResponseWriter, raw string) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   secureCookies(s.cfg),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
}

func (s *server) internalServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := newRequestID()
	w.Header().Set("X-Request-ID", requestID)
	applySecurityHeaders(w, s.cfg.Environment)
	captured := &responseCapture{ResponseWriter: w}
	started := time.Now()
	if r.Body != nil {
		r.Body = http.MaxBytesReader(captured, r.Body, s.cfg.MaxBodyBytes)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error("internal_request_panic", "request_id", requestID, "panic_type", fmt.Sprintf("%T", recovered))
			if !captured.wroteHeader {
				writeError(captured, http.StatusInternalServerError, "Internal server error.", "api_error")
			}
		}
		s.logger.Info("internal_request_complete", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", captured.statusCode(), "bytes", captured.bytes, "duration_ms", time.Since(started).Milliseconds())
	}()
	s.internalWorkerRoute(captured, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
}

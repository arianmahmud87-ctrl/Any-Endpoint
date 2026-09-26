package runtime

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/local/ai-control-plane/worker-agent/internal/agent"
)

type Runtime struct {
	cfg         Config
	logger      *slog.Logger
	child       *exec.Cmd
	childCancel context.CancelFunc
	childDone   chan error
	token       string
}

func Run(cfg Config) error {
	logger := slog.Default()
	r := &Runtime{cfg: cfg, logger: logger}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := agent.New(agent.Config{ControlPlaneURL: cfg.ControlPlaneURL, EnrollmentToken: cfg.EnrollmentToken, AgentID: cfg.AgentID, InternalURL: cfg.InternalURL, TokenFile: cfg.TokenFile, CAFile: cfg.CAFile, CertFile: cfg.ClientCertFile, KeyFile: cfg.ClientKeyFile})
	if err != nil {
		return err
	}
	if err := client.LoadToken(); err != nil {
		return err
	}

	childURL := "http://127.0.0.1:" + strconv.Itoa(cfg.ChildPort)
	proxy, err := r.proxyHandler(childURL)
	if err != nil {
		return err
	}
	server, err := r.listenServer(proxy)
	if err != nil {
		return err
	}
	serverErr := make(chan error, 1)
	go func() {
		if strings.HasPrefix(strings.ToLower(cfg.InternalURL), "https://") {
			serverErr <- server.ListenAndServeTLS("", "")
			return
		}
		serverErr <- server.ListenAndServe()
	}()

	if err := waitTCP(ctx, cfg.ListenAddr); err != nil {
		_ = server.Close()
		return err
	}
	if client.Token() == "" {
		if err := client.Enroll(ctx); err != nil {
			_ = server.Close()
			return err
		}
	}
	r.token = client.Token()
	if err := r.startChild(ctx); err != nil {
		_ = client.Heartbeat(context.Background(), "offline")
		_ = server.Close()
		return err
	}
	if err := waitHealth(ctx, childURL); err != nil {
		_ = client.Heartbeat(context.Background(), "offline")
		_ = r.stopChild()
		_ = server.Close()
		return err
	}
	if err := client.Heartbeat(ctx, "online"); err != nil {
		_ = r.stopChild()
		_ = server.Close()
		return err
	}
	logger.Info("worker_online", "provider", cfg.Provider, "agent_id", cfg.AgentID)

	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
			_ = client.Heartbeat(shutdownCtx, "draining")
			_ = r.stopChild()
			_ = server.Shutdown(shutdownCtx)
			cancel()
			return nil
		case err := <-serverErr:
			if err != nil && err != http.ErrServerClosed {
				_ = r.stopChild()
				return err
			}
		case <-ticker.C:
			if err := client.Heartbeat(ctx, "online"); err != nil {
				logger.Error("heartbeat_failed", "error", err)
				return err
			}
		case childErr := <-r.childDone:
			_ = client.Heartbeat(context.Background(), "offline")
			_ = server.Close()
			return fmt.Errorf("provider child exited: %w", childErr)
		}
	}
}

func (r *Runtime) listenServer(handler http.Handler) (*http.Server, error) {
	server := &http.Server{Addr: r.cfg.ListenAddr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 300 * time.Second, IdleTimeout: 120 * time.Second}
	if strings.HasPrefix(strings.ToLower(r.cfg.InternalURL), "https://") {
		tlsConfig, err := serverTLS(r.cfg)
		if err != nil {
			return nil, err
		}
		server.TLSConfig = tlsConfig
	}
	return server, nil
}

func (r *Runtime) proxyHandler(childURL string) (http.Handler, error) {
	target, err := url.Parse(childURL)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "worker child unavailable", http.StatusBadGateway)
		r.logger.Error("child_proxy_error", "error", err)
	}
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.Header.Set("Authorization", "Bearer "+r.token)
		for name := range req.Header {
			if name != "Accept" && name != "Content-Type" && name != "Openai-Beta" && name != "Authorization" {
				req.Header.Del(name)
			}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/healthz" {
			if req.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := waitHealth(req.Context(), childURL); err != nil {
				http.Error(w, "worker child unavailable", http.StatusServiceUnavailable)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		if !strings.HasPrefix(req.URL.Path, "/v1/") {
			http.NotFound(w, req)
			return
		}
		if !validToken(req.Header.Get("Authorization"), r.token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		proxy.ServeHTTP(w, req)
	}), nil
}

func (r *Runtime) startChild(ctx context.Context) error {
	args := []string{"serve", "--host", "127.0.0.1", "--port", strconv.Itoa(r.cfg.ChildPort), "--api-key", r.token}
	if r.cfg.DefaultModel != "" {
		args = append(args, "--default-model", r.cfg.DefaultModel)
	}
	if r.cfg.Provider == "codex" {
		if r.cfg.CodexAuthJSON != "" {
			args = append(args, "--auth-json", r.cfg.CodexAuthJSON)
		}
	} else {
		args = append(args, "--claude-command", r.cfg.ClaudeCommand)
	}
	childCtx, cancel := context.WithCancel(ctx)
	r.childCancel = cancel
	r.child = exec.CommandContext(childCtx, r.cfg.ChildCommand, args...)
	r.child.Env = os.Environ()
	if r.cfg.CodexHome != "" {
		r.child.Env = append(r.child.Env, "CODEX_HOME="+r.cfg.CodexHome)
	}
	if r.cfg.ClaudeConfigDir != "" {
		r.child.Env = append(r.child.Env, "CLAUDE_CONFIG_DIR="+r.cfg.ClaudeConfigDir)
	}
	r.child.Stdout = os.Stdout
	r.child.Stderr = os.Stderr
	if err := r.child.Start(); err != nil {
		return fmt.Errorf("start provider child: %w", err)
	}
	r.childDone = make(chan error, 1)
	go func() { r.childDone <- r.child.Wait() }()
	return nil
}

func (r *Runtime) stopChild() error {
	if r.childCancel != nil {
		r.childCancel()
	}
	if r.child == nil || r.child.Process == nil {
		return nil
	}
	if err := r.child.Process.Kill(); err != nil {
		return err
	}
	if r.childDone != nil {
		select {
		case <-r.childDone:
		case <-time.After(5 * time.Second):
		}
	}
	return nil
}

func waitHealth(ctx context.Context, endpoint string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		request, _ := http.NewRequestWithContext(deadline, http.MethodGet, endpoint+"/healthz", nil)
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("provider child did not become healthy")
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func validToken(header, expected string) bool {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	return ok && strings.EqualFold(scheme, "bearer") && len(token) == len(expected) && subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func serverTLS(cfg Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.ServerCertFile, cfg.ServerKeyFile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("worker CA has no valid certificates")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}, nil
}

func waitTCP(ctx context.Context, address string) error {
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		conn, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("worker listener did not become ready")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	ControlPlaneURL string
	EnrollmentToken string
	AgentID         string
	InternalURL     string
	TokenFile       string
	CAFile          string
	CertFile        string
	KeyFile         string
}

type Client struct {
	cfg   Config
	http  *http.Client
	token string
}

func New(cfg Config) (*Client, error) {
	transport := &http.Transport{Proxy: nil}
	if strings.HasPrefix(strings.ToLower(cfg.ControlPlaneURL), "https://") {
		if cfg.CAFile == "" || cfg.CertFile == "" || cfg.KeyFile == "" {
			return nil, fmt.Errorf("HTTPS control-plane requires CA, client certificate, and key")
		}
	}
	if cfg.CAFile != "" || cfg.CertFile != "" || cfg.KeyFile != "" {
		tlsConfig, err := loadTLS(cfg)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConfig
	}
	return &Client{cfg: cfg, http: &http.Client{Transport: transport, Timeout: 20 * time.Second}}, nil
}

func (c *Client) LoadToken() error {
	if c.cfg.TokenFile == "" {
		return nil
	}
	data, err := os.ReadFile(c.cfg.TokenFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	value := strings.TrimSpace(string(data))
	if value == "" || len(value) > 512 {
		return fmt.Errorf("worker token file is invalid")
	}
	c.token = value
	return nil
}

func (c *Client) Token() string { return c.token }

func (c *Client) Enroll(ctx context.Context) error {
	payload, _ := json.Marshal(map[string]string{"token": c.cfg.EnrollmentToken, "agent_id": c.cfg.AgentID, "internal_url": c.cfg.InternalURL})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.ControlPlaneURL, "/")+"/internal/worker/enroll", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("worker enrollment failed with HTTP %d", response.StatusCode)
	}
	var body struct {
		WorkerToken string `json:"worker_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil || body.WorkerToken == "" {
		return fmt.Errorf("worker enrollment returned no token")
	}
	c.token = body.WorkerToken
	if c.cfg.TokenFile == "" {
		return fmt.Errorf("worker token file is required")
	}
	return atomicWrite(c.cfg.TokenFile, []byte(c.token+"\n"), 0600)
}

func (c *Client) Heartbeat(ctx context.Context, status string) error {
	if c.token == "" {
		return fmt.Errorf("worker is not enrolled")
	}
	payload, _ := json.Marshal(map[string]string{"status": status})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.ControlPlaneURL, "/")+"/internal/worker/heartbeat", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("worker heartbeat failed with HTTP %d", response.StatusCode)
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepathDir(path), 0700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func filepathDir(path string) string {
	index := strings.LastIndexAny(path, `/\\`)
	if index < 0 {
		return "."
	}
	return path[:index]
}

func loadTLS(cfg Config) (*tls.Config, error) {
	caData, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("worker CA has no valid certificates")
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{cert}}, nil
}

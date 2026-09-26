package runtime

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Provider          string
	ControlPlaneURL   string
	EnrollmentToken   string
	AgentID           string
	InternalURL       string
	ListenAddr        string
	ChildPort         int
	ChildCommand      string
	CodexAuthJSON     string
	CodexHome         string
	ClaudeCommand     string
	ClaudeConfigDir   string
	DefaultModel      string
	TokenFile         string
	CAFile            string
	ClientCertFile    string
	ClientKeyFile     string
	ServerCertFile    string
	ServerKeyFile     string
	ShutdownTimeout   time.Duration
	HeartbeatInterval time.Duration
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Provider:          envString("WORKER_PROVIDER", ""),
		ControlPlaneURL:   strings.TrimRight(envString("CONTROL_PLANE_URL", ""), "/"),
		EnrollmentToken:   os.Getenv("WORKER_ENROLLMENT_TOKEN"),
		AgentID:           envString("WORKER_AGENT_ID", ""),
		InternalURL:       strings.TrimRight(envString("WORKER_INTERNAL_URL", ""), "/"),
		ListenAddr:        envString("WORKER_LISTEN_ADDR", "0.0.0.0:19080"),
		ChildPort:         envInt("WORKER_CHILD_PORT", 19081),
		ChildCommand:      envString("WORKER_CHILD_COMMAND", ""),
		CodexAuthJSON:     strings.TrimSpace(os.Getenv("CODEX_AUTH_JSON")),
		CodexHome:         strings.TrimSpace(os.Getenv("CODEX_HOME")),
		ClaudeCommand:     envString("CLAUDE_COMMAND", "claude"),
		ClaudeConfigDir:   strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")),
		DefaultModel:      envString("WORKER_DEFAULT_MODEL", ""),
		TokenFile:         envString("WORKER_TOKEN_FILE", "./state/worker-token"),
		CAFile:            strings.TrimSpace(os.Getenv("MTLS_CA_FILE")),
		ClientCertFile:    strings.TrimSpace(os.Getenv("MTLS_CLIENT_CERT_FILE")),
		ClientKeyFile:     strings.TrimSpace(os.Getenv("MTLS_CLIENT_KEY_FILE")),
		ServerCertFile:    strings.TrimSpace(os.Getenv("WORKER_SERVER_CERT_FILE")),
		ServerKeyFile:     strings.TrimSpace(os.Getenv("WORKER_SERVER_KEY_FILE")),
		ShutdownTimeout:   envDuration("WORKER_SHUTDOWN_TIMEOUT", 15*time.Second),
		HeartbeatInterval: envDuration("WORKER_HEARTBEAT_INTERVAL", 20*time.Second),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Provider != "codex" && c.Provider != "claude_code" {
		return fmt.Errorf("WORKER_PROVIDER must be codex or claude_code")
	}
	for name, value := range map[string]string{"CONTROL_PLANE_URL": c.ControlPlaneURL, "WORKER_AGENT_ID": c.AgentID, "WORKER_INTERNAL_URL": c.InternalURL} {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if c.EnrollmentToken == "" {
		if _, err := os.Stat(c.TokenFile); err != nil {
			return fmt.Errorf("WORKER_ENROLLMENT_TOKEN is required when WORKER_TOKEN_FILE does not exist")
		}
	}
	if c.ChildPort < 1 || c.ChildPort > 65535 || c.ListenAddr == "" || c.ChildCommand == "" {
		return fmt.Errorf("worker listen/child configuration is invalid")
	}
	if c.Provider == "codex" && c.CodexAuthJSON == "" && c.CodexHome == "" {
		return fmt.Errorf("Codex worker requires CODEX_AUTH_JSON or CODEX_HOME")
	}
	if c.Provider == "claude_code" && c.ClaudeCommand == "" {
		return fmt.Errorf("CLAUDE_COMMAND is required")
	}
	if c.ShutdownTimeout <= 0 || c.HeartbeatInterval <= 0 {
		return fmt.Errorf("worker timeouts must be positive")
	}
	if strings.HasPrefix(strings.ToLower(c.InternalURL), "https://") {
		for name, value := range map[string]string{"MTLS_CA_FILE": c.CAFile, "WORKER_SERVER_CERT_FILE": c.ServerCertFile, "WORKER_SERVER_KEY_FILE": c.ServerKeyFile} {
			if value == "" {
				return fmt.Errorf("HTTPS worker listener requires %s", name)
			}
		}
	}
	if strings.HasPrefix(strings.ToLower(c.ControlPlaneURL), "https://") {
		for name, value := range map[string]string{"MTLS_CA_FILE": c.CAFile, "MTLS_CLIENT_CERT_FILE": c.ClientCertFile, "MTLS_CLIENT_KEY_FILE": c.ClientKeyFile, "WORKER_SERVER_CERT_FILE": c.ServerCertFile, "WORKER_SERVER_KEY_FILE": c.ServerKeyFile} {
			if value == "" {
				return fmt.Errorf("HTTPS worker requires %s", name)
			}
		}
	}
	return nil
}

func envString(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err == nil && value > 0 {
		return value
	}
	return fallback
}
func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err == nil && parsed > 0 {
		return parsed
	}
	return 0
}

package app

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Environment               string
	HTTPAddr                  string
	InternalHTTPAddr          string
	PublicBaseURL             string
	TrustProxy                string
	DatabaseURL               string
	RedisURL                  string
	SessionSecret             string
	KeyPepper                 string
	SecretBackend             string
	AuthEnabled               bool
	GoogleIssuerURL           string
	GoogleClientID            string
	GoogleClientSecret        string
	SessionCookieName         string
	SessionTTL                time.Duration
	OAuthStateTTL             time.Duration
	EnrollmentTTL             time.Duration
	ProviderLoginTTL          time.Duration
	WorkerAllowedHosts        string
	WorkerProvisioningEnabled bool
	WorkerProvisioningToken   string
	GatewayMaxConcurrent      int
	GatewayTimeout            time.Duration
	PlaygroundRPM             int
	MTLSEnabled               bool
	MTLSCAFile                string
	MTLSServerCertFile        string
	MTLSServerKeyFile         string
	MTLSClientCertFile        string
	MTLSClientKeyFile         string
	ReadHeaderTimeout         time.Duration
	ReadTimeout               time.Duration
	WriteTimeout              time.Duration
	IdleTimeout               time.Duration
	ShutdownTimeout           time.Duration
	MaxBodyBytes              int64
}

func LoadConfig() (Config, error) {
	cookieName := "cp_session"
	if envString("APP_ENV", "development") == "production" {
		cookieName = "__Host-cp_session"
	}
	cfg := Config{
		Environment:               envString("APP_ENV", "development"),
		HTTPAddr:                  envString("HTTP_ADDR", "127.0.0.1:8080"),
		InternalHTTPAddr:          envString("INTERNAL_HTTP_ADDR", "127.0.0.1:8081"),
		PublicBaseURL:             envString("PUBLIC_BASE_URL", "http://127.0.0.1:8080"),
		TrustProxy:                strings.TrimSpace(os.Getenv("TRUST_PROXY")),
		DatabaseURL:               strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:                  strings.TrimSpace(os.Getenv("REDIS_URL")),
		SessionSecret:             os.Getenv("SESSION_SECRET"),
		KeyPepper:                 os.Getenv("KEY_PEPPER"),
		SecretBackend:             envString("SECRET_BACKEND", "local"),
		AuthEnabled:               envBool("AUTH_ENABLED", false),
		GoogleIssuerURL:           envString("GOOGLE_ISSUER_URL", "https://accounts.google.com"),
		GoogleClientID:            strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret:        os.Getenv("GOOGLE_CLIENT_SECRET"),
		SessionCookieName:         envString("SESSION_COOKIE_NAME", cookieName),
		SessionTTL:                envDuration("SESSION_TTL", 12*time.Hour),
		OAuthStateTTL:             envDuration("OAUTH_STATE_TTL", 10*time.Minute),
		EnrollmentTTL:             envDuration("WORKER_ENROLLMENT_TTL", 10*time.Minute),
		ProviderLoginTTL:          envDuration("PROVIDER_LOGIN_TTL", 10*time.Minute),
		WorkerAllowedHosts:        envString("WORKER_ALLOWED_HOSTS", "127.0.0.1,localhost,worker"),
		WorkerProvisioningEnabled: envBool("WORKER_PROVISIONING_ENABLED", false),
		WorkerProvisioningToken:   strings.TrimSpace(os.Getenv("WORKER_PROVISIONING_TOKEN")),
		GatewayMaxConcurrent:      envInt("GATEWAY_MAX_CONCURRENT", 10),
		GatewayTimeout:            envDuration("GATEWAY_TIMEOUT", 300*time.Second),
		PlaygroundRPM:             envInt("PLAYGROUND_REQUESTS_PER_MINUTE", 10),
		MTLSEnabled:               envBool("MTLS_ENABLED", false),
		MTLSCAFile:                strings.TrimSpace(os.Getenv("MTLS_CA_FILE")),
		MTLSServerCertFile:        strings.TrimSpace(os.Getenv("MTLS_SERVER_CERT_FILE")),
		MTLSServerKeyFile:         strings.TrimSpace(os.Getenv("MTLS_SERVER_KEY_FILE")),
		MTLSClientCertFile:        strings.TrimSpace(os.Getenv("MTLS_CLIENT_CERT_FILE")),
		MTLSClientKeyFile:         strings.TrimSpace(os.Getenv("MTLS_CLIENT_KEY_FILE")),
		ReadHeaderTimeout:         envDuration("READ_HEADER_TIMEOUT", 10*time.Second),
		ReadTimeout:               envDuration("READ_TIMEOUT", 30*time.Second),
		WriteTimeout:              envDuration("WRITE_TIMEOUT", 60*time.Second),
		IdleTimeout:               envDuration("IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout:           envDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		MaxBodyBytes:              envInt64("MAX_BODY_BYTES", 4<<20),
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return fmt.Errorf("APP_ENV must be development, test, or production")
	}
	if strings.TrimSpace(c.HTTPAddr) == "" || strings.TrimSpace(c.InternalHTTPAddr) == "" {
		return fmt.Errorf("HTTP_ADDR and INTERNAL_HTTP_ADDR must not be empty")
	}
	if c.HTTPAddr == c.InternalHTTPAddr {
		return fmt.Errorf("HTTP_ADDR and INTERNAL_HTTP_ADDR must be different")
	}
	if err := validatePublicBaseURL(c.PublicBaseURL, c.Environment == "production"); err != nil {
		return err
	}
	if c.ReadHeaderTimeout <= 0 || c.ReadTimeout <= 0 || c.WriteTimeout <= 0 || c.IdleTimeout <= 0 || c.ShutdownTimeout <= 0 {
		return fmt.Errorf("HTTP timeouts must be positive")
	}
	if c.MaxBodyBytes < 1024 || c.MaxBodyBytes > 64<<20 {
		return fmt.Errorf("MAX_BODY_BYTES must be between 1024 and 67108864")
	}
	if c.SessionTTL <= 0 || c.SessionTTL > 30*24*time.Hour {
		return fmt.Errorf("SESSION_TTL must be between 1 and 720 hours")
	}
	if c.OAuthStateTTL <= 0 || c.OAuthStateTTL > 30*time.Minute {
		return fmt.Errorf("OAUTH_STATE_TTL must be between 1 and 30 minutes")
	}
	if c.EnrollmentTTL <= 0 || c.EnrollmentTTL > 30*time.Minute {
		return fmt.Errorf("WORKER_ENROLLMENT_TTL must be between 1 and 30 minutes")
	}
	if c.ProviderLoginTTL <= 0 || c.ProviderLoginTTL > 30*time.Minute {
		return fmt.Errorf("PROVIDER_LOGIN_TTL must be between 1 and 30 minutes")
	}
	if c.GatewayMaxConcurrent < 1 || c.GatewayMaxConcurrent > 10000 {
		return fmt.Errorf("GATEWAY_MAX_CONCURRENT must be between 1 and 10000")
	}
	if c.GatewayTimeout <= 0 || c.GatewayTimeout > 30*time.Minute {
		return fmt.Errorf("GATEWAY_TIMEOUT must be between 1 and 30 minutes")
	}
	if c.PlaygroundRPM < 1 || c.PlaygroundRPM > 1000 {
		return fmt.Errorf("PLAYGROUND_REQUESTS_PER_MINUTE must be between 1 and 1000")
	}
	if !c.AuthEnabled && c.SessionCookieName == "" {
		return fmt.Errorf("SESSION_COOKIE_NAME must not be empty")
	}
	if c.AuthEnabled {
		if err := validateURL(c.GoogleIssuerURL, "https"); err != nil {
			return fmt.Errorf("GOOGLE_ISSUER_URL: %w", err)
		}
		if strings.TrimSpace(c.GoogleClientID) == "" || strings.TrimSpace(c.GoogleClientSecret) == "" {
			return fmt.Errorf("AUTH_ENABLED requires GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET")
		}
		if c.DatabaseURL == "" || c.RedisURL == "" {
			return fmt.Errorf("AUTH_ENABLED requires DATABASE_URL and REDIS_URL")
		}
		if len([]byte(c.SessionSecret)) < 32 || len([]byte(c.KeyPepper)) < 32 {
			return fmt.Errorf("AUTH_ENABLED requires SESSION_SECRET and KEY_PEPPER of at least 32 bytes")
		}
		if !strings.HasPrefix(c.SessionCookieName, "__Host-") {
			return fmt.Errorf("SESSION_COOKIE_NAME must use the __Host- prefix")
		}
	}
	if c.DatabaseURL != "" {
		if err := validateURL(c.DatabaseURL, "postgres", "postgresql"); err != nil {
			return fmt.Errorf("DATABASE_URL: %w", err)
		}
	}
	if c.RedisURL != "" {
		if err := validateURL(c.RedisURL, "redis", "rediss"); err != nil {
			return fmt.Errorf("REDIS_URL: %w", err)
		}
	}
	if c.WorkerProvisioningEnabled {
		if len([]byte(c.WorkerProvisioningToken)) < 32 {
			return fmt.Errorf("WORKER_PROVISIONING_ENABLED requires WORKER_PROVISIONING_TOKEN of at least 32 bytes")
		}
		if !c.MTLSEnabled {
			return fmt.Errorf("WORKER_PROVISIONING_ENABLED requires MTLS_ENABLED=true")
		}
	}
	if c.MTLSEnabled {
		for name, value := range map[string]string{
			"MTLS_CA_FILE": c.MTLSCAFile, "MTLS_SERVER_CERT_FILE": c.MTLSServerCertFile,
			"MTLS_SERVER_KEY_FILE": c.MTLSServerKeyFile, "MTLS_CLIENT_CERT_FILE": c.MTLSClientCertFile,
			"MTLS_CLIENT_KEY_FILE": c.MTLSClientKeyFile,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("MTLS_ENABLED requires %s", name)
			}
		}
	}
	if c.Environment == "production" {
		if !strings.HasPrefix(strings.ToLower(c.PublicBaseURL), "https://") {
			return fmt.Errorf("production requires PUBLIC_BASE_URL to use https://")
		}
		if strings.TrimSpace(c.TrustProxy) == "" {
			return fmt.Errorf("production requires explicit TRUST_PROXY configuration")
		}
		if len([]byte(c.SessionSecret)) < 32 {
			return fmt.Errorf("production requires SESSION_SECRET of at least 32 bytes")
		}
		if len([]byte(c.KeyPepper)) < 32 {
			return fmt.Errorf("production requires KEY_PEPPER of at least 32 bytes")
		}
		if c.SecretBackend == "local" || strings.TrimSpace(c.SecretBackend) == "" {
			return fmt.Errorf("production requires a non-local SECRET_BACKEND")
		}
		if c.DatabaseURL == "" || c.RedisURL == "" {
			return fmt.Errorf("production requires DATABASE_URL and REDIS_URL")
		}
		if !c.AuthEnabled {
			return fmt.Errorf("production requires AUTH_ENABLED=true")
		}
		if !c.MTLSEnabled {
			return fmt.Errorf("production requires MTLS_ENABLED=true")
		}
	}
	return nil
}

func validateURL(raw string, schemes ...string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("must be an absolute URL")
	}
	for _, scheme := range schemes {
		if strings.EqualFold(u.Scheme, scheme) {
			return nil
		}
	}
	return fmt.Errorf("unsupported URL scheme")
}

func envString(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && strings.TrimSpace(value) != "" {
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
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func envBool(name string, fallback bool) bool {
	value, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed
}

func validatePublicBaseURL(raw string, production bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("PUBLIC_BASE_URL must be an absolute URL without credentials, query, or fragment")
	}
	if !production && !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("PUBLIC_BASE_URL must use http or https")
	}
	if production && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("production requires PUBLIC_BASE_URL to use https://")
	}
	return nil
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

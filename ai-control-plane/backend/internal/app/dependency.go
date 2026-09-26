package app

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type dependencyResult struct {
	Status string `json:"status"`
}

func probeDependency(ctx context.Context, rawURL string, defaultPort string) dependencyResult {
	if strings.TrimSpace(rawURL) == "" {
		return dependencyResult{Status: "not_configured"}
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return dependencyResult{Status: "invalid_config"}
	}
	port := u.Port()
	if port == "" {
		port = defaultPort
	}
	address := net.JoinHostPort(u.Hostname(), port)
	probeCtx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(probeCtx, "tcp", address)
	if err != nil {
		return dependencyResult{Status: "unreachable"}
	}
	_ = conn.Close()
	return dependencyResult{Status: "reachable"}
}

func dependencyReady(result dependencyResult, production bool) bool {
	if result.Status == "reachable" {
		return true
	}
	return !production && result.Status == "not_configured"
}

func validateDependencyScheme(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid dependency URL")
	}
	return nil
}

func portOrDefault(u *url.URL, fallback int) string {
	if value := u.Port(); value != "" {
		return value
	}
	return strconv.Itoa(fallback)
}

package app

import (
	"fmt"
	"net"
	"strings"
)

// validateNetworkExposure keeps the default local workflow frictionless while
// refusing to expose a user's Codex-backed endpoint to a network without auth.
func validateNetworkExposure(host, apiKey string) error {
	if isLoopbackHost(host) || strings.TrimSpace(apiKey) != "" {
		return nil
	}
	return fmt.Errorf("an incoming API key is required when --host %q is not loopback; set --api-key or OPENAI_VIA_CODEX_API_KEY", host)
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

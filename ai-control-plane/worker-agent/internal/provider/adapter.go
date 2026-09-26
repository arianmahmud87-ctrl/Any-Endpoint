package provider

import "context"

// Adapter is the profile-bound provider process boundary. Implementations must
// keep provider credentials inside the worker runtime and expose only a private
// OpenAI-compatible endpoint to the control plane.
type Adapter interface {
	Provider() string
	Start(context.Context) error
	Stop(context.Context) error
	Health(context.Context) error
	Endpoint() string
	Models() []string
}

type Config struct {
	Provider       string
	Endpoint       string
	CredentialHome string
	DefaultModel   string
}

func New(cfg Config) Adapter {
	switch cfg.Provider {
	case "codex":
		return &CodexAdapter{config: cfg}
	case "claude_code":
		return &ClaudeAdapter{config: cfg}
	default:
		return &unsupportedAdapter{provider: cfg.Provider}
	}
}

type unsupportedAdapter struct{ provider string }

func (a *unsupportedAdapter) Provider() string             { return a.provider }
func (a *unsupportedAdapter) Start(context.Context) error  { return errNotImplemented(a.provider) }
func (a *unsupportedAdapter) Stop(context.Context) error   { return nil }
func (a *unsupportedAdapter) Health(context.Context) error { return errNotImplemented(a.provider) }
func (a *unsupportedAdapter) Endpoint() string             { return "" }
func (a *unsupportedAdapter) Models() []string             { return nil }

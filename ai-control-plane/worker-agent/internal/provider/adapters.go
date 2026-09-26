package provider

import (
	"context"
	"errors"
)

var errProviderAdapterNotImplemented = errors.New("provider adapter is not implemented")

func errNotImplemented(provider string) error {
	return errors.New(provider + ": " + errProviderAdapterNotImplemented.Error())
}

type CodexAdapter struct{ config Config }

func (a *CodexAdapter) Provider() string             { return "codex" }
func (a *CodexAdapter) Start(context.Context) error  { return errNotImplemented("codex") }
func (a *CodexAdapter) Stop(context.Context) error   { return nil }
func (a *CodexAdapter) Health(context.Context) error { return errNotImplemented("codex") }
func (a *CodexAdapter) Endpoint() string             { return a.config.Endpoint }
func (a *CodexAdapter) Models() []string             { return []string{a.config.DefaultModel} }

type ClaudeAdapter struct{ config Config }

func (a *ClaudeAdapter) Provider() string             { return "claude_code" }
func (a *ClaudeAdapter) Start(context.Context) error  { return errNotImplemented("claude_code") }
func (a *ClaudeAdapter) Stop(context.Context) error   { return nil }
func (a *ClaudeAdapter) Health(context.Context) error { return errNotImplemented("claude_code") }
func (a *ClaudeAdapter) Endpoint() string             { return a.config.Endpoint }
func (a *ClaudeAdapter) Models() []string             { return []string{a.config.DefaultModel} }

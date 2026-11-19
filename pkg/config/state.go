package config

import "sync/atomic"

// ConfigState holds the current Config and allows atomic, lock-free reads.
// This enables hot-reloading of configuration via SIGHUP while ensuring all consumers
// (e.g., HTTP middleware) always see the latest config snapshot.
//
// Non-nil invariant: once constructed via NewConfigState with a non-nil
// *Config, Load always returns a non-nil pointer. Store silently ignores
// nil to preserve this invariant for downstream consumers that dereference
// without a nil check.
type ConfigState struct {
	ref atomic.Pointer[Config]
}

// NewConfigState creates a new ConfigState initialized with the given config.
// cfg must be non-nil; passing nil violates the non-nil invariant of Load.
func NewConfigState(cfg *Config) *ConfigState {
	s := &ConfigState{}
	s.ref.Store(cfg)
	return s
}

// Load returns the current Config. Safe for concurrent use.
// Guaranteed non-nil when the state was constructed via NewConfigState
// with a non-nil config; Store(nil) is a no-op.
func (s *ConfigState) Load() *Config {
	return s.ref.Load()
}

// Store atomically replaces the current Config.
// nil is silently ignored to preserve the non-nil invariant of Load.
func (s *ConfigState) Store(cfg *Config) {
	if cfg == nil {
		return
	}
	s.ref.Store(cfg)
}

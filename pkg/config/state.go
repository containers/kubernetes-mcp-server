package config

import "sync/atomic"

// StaticConfigState holds the current StaticConfig and allows atomic, lock-free reads.
// This enables hot-reloading of configuration via SIGHUP while ensuring all consumers
// (e.g., HTTP middleware) always see the latest config snapshot.
//
// Non-nil invariant: once constructed via NewStaticConfigState with a non-nil
// *StaticConfig, Load always returns a non-nil pointer. Store silently ignores
// nil to preserve this invariant for downstream consumers that dereference
// without a nil check.
type StaticConfigState struct {
	ref atomic.Pointer[StaticConfig]
}

// NewStaticConfigState creates a new StaticConfigState initialized with the given config.
// cfg must be non-nil; passing nil violates the non-nil invariant of Load.
func NewStaticConfigState(cfg *StaticConfig) *StaticConfigState {
	s := &StaticConfigState{}
	s.ref.Store(cfg)
	return s
}

// Load returns the current StaticConfig. Safe for concurrent use.
// Guaranteed non-nil when the state was constructed via NewStaticConfigState
// with a non-nil config; Store(nil) is a no-op.
func (s *StaticConfigState) Load() *StaticConfig {
	return s.ref.Load()
}

// Store atomically replaces the current StaticConfig.
// nil is silently ignored to preserve the non-nil invariant of Load.
func (s *StaticConfigState) Store(cfg *StaticConfig) {
	if cfg == nil {
		return
	}
	s.ref.Store(cfg)
}

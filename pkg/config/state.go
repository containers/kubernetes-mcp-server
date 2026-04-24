package config

import "sync/atomic"

// StaticConfigState holds the current StaticConfig and allows atomic, lock-free reads.
// This enables hot-reloading of configuration via SIGHUP while ensuring all consumers
// (e.g., HTTP middleware) always see the latest config snapshot.
type StaticConfigState struct {
	ref atomic.Pointer[StaticConfig]
}

// NewStaticConfigState creates a new StaticConfigState initialized with the given config.
func NewStaticConfigState(cfg *StaticConfig) *StaticConfigState {
	s := &StaticConfigState{}
	s.ref.Store(cfg)
	return s
}

// Load returns the current StaticConfig. Safe for concurrent use.
func (s *StaticConfigState) Load() *StaticConfig {
	return s.ref.Load()
}

// Store atomically replaces the current StaticConfig.
func (s *StaticConfigState) Store(cfg *StaticConfig) {
	s.ref.Store(cfg)
}

package kiali

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type Manager struct {
	BearerToken   string
	KialiURL      string
	KialiInsecure bool
}

func NewManager(config *config.StaticConfig) *Manager {
	m := &Manager{
		BearerToken:   "",
		KialiURL:      "",
		KialiInsecure: false,
	}
	if cfg, ok := config.GetToolsetConfig("kiali"); ok {
		if kc, ok := cfg.(*Config); ok && kc != nil {
			m.KialiURL = kc.Url
			m.KialiInsecure = kc.Insecure
		}
	}
	return m
}

func (m *Manager) Derived(_ context.Context) (*Kiali, error) {
	return &Kiali{manager: m}, nil
}

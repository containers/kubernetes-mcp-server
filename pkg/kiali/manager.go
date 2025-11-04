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
	return &Manager{
		BearerToken:   "",
		KialiURL:      config.KialiOptions.Url,
		KialiInsecure: config.KialiOptions.Insecure,
	}
}

func (m *Manager) Derived(_ context.Context) (*Kiali, error) {
	return &Kiali{manager: m}, nil
}

// Package configtest helps tests set Config Option values.
package configtest

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

// NewTelemetry returns a TelemetryConfig with Option metadata, for tests that
// pass *TelemetryConfig into telemetry/metrics constructors.
func NewTelemetry() *config.TelemetryConfig {
	c := config.New()
	return &c.Telemetry
}

// Telemetry returns a TelemetryConfig with the given endpoint and protocol.
func Telemetry(endpoint, protocol string) *config.TelemetryConfig {
	t := NewTelemetry()
	if endpoint != "" {
		t.Endpoint.SetForTest(endpoint)
	}
	if protocol != "" {
		t.Protocol.SetForTest(protocol)
	}
	return t
}

// MustReadTOML parses TOML into a Config and fails the test on error.
func MustReadTOML(t testing.TB, data string, opts ...config.ReadConfigOpt) *config.Config {
	t.Helper()
	cfg, err := config.ReadToml([]byte(data), opts...)
	if err != nil {
		t.Fatalf("ReadToml: %v", err)
	}
	return cfg
}

// OverlayTOML replaces cfg with TOML-parsed config. Options still at their
// default source keep the previous value (so suite SetForTest kubeconfig,
// list_output, toolsets, etc. survive). TOML and env applied by ReadToml win.
func OverlayTOML(t testing.TB, cfg **config.Config, data string) {
	t.Helper()
	prev := *cfg
	loaded := MustReadTOML(t, data)
	config.KeepPreviousIfDefault(prev, loaded)
	*cfg = loaded
}

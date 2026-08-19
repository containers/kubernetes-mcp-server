package telemetry

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/logtest"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

type LogsSuite struct {
	suite.Suite
}

func TestLogs(t *testing.T) {
	suite.Run(t, new(LogsSuite))
}

func (s *LogsSuite) TestNewLogProviderDisabledCases() {
	s.Run("returns nil when cfg is nil", func() {
		provider, err := NewLogProvider(s.T().Context(), nil, "svc", "1.0")
		s.NoError(err)
		s.Nil(provider)
	})

	s.Run("returns nil when telemetry is explicitly disabled", func() {
		disabled := false
		cfg := &config.TelemetryConfig{
			Enabled:  &disabled,
			Endpoint: "http://localhost:4317",
		}
		provider, err := NewLogProvider(s.T().Context(), cfg, "svc", "1.0")
		s.NoError(err)
		s.Nil(provider)
	})

	s.Run("returns nil when no endpoint is configured", func() {
		cfg := &config.TelemetryConfig{}
		provider, err := NewLogProvider(s.T().Context(), cfg, "svc", "1.0")
		s.NoError(err)
		s.Nil(provider)
	})

	s.Run("returns nil when OTEL_LOGS_EXPORTER is none", func() {
		s.T().Setenv("OTEL_LOGS_EXPORTER", "none")
		cfg := &config.TelemetryConfig{Endpoint: "http://localhost:4317"}
		provider, err := NewLogProvider(s.T().Context(), cfg, "svc", "1.0")
		s.NoError(err)
		s.Nil(provider, "OTEL_LOGS_EXPORTER=none must disable log export even when endpoint is set")
	})
}

func (s *LogsSuite) TestNewLogProviderWithValidConfig() {
	s.Run("returns provider for grpc protocol", func() {
		cfg := &config.TelemetryConfig{Endpoint: "http://localhost:4317", Protocol: "grpc"}
		provider, err := NewLogProvider(s.T().Context(), cfg, "test-svc", "1.0.0")
		s.Require().NoError(err)
		s.NotNil(provider)
		defer func() { _ = provider.Shutdown(s.T().Context()) }()
	})

	s.Run("returns provider for http/protobuf protocol", func() {
		cfg := &config.TelemetryConfig{Endpoint: "http://localhost:4318", Protocol: "http/protobuf"}
		provider, err := NewLogProvider(s.T().Context(), cfg, "test-svc", "1.0.0")
		s.Require().NoError(err)
		s.NotNil(provider)
		defer func() { _ = provider.Shutdown(s.T().Context()) }()
	})

	s.Run("defaults to grpc when protocol is empty", func() {
		cfg := &config.TelemetryConfig{Endpoint: "http://localhost:4317"}
		provider, err := NewLogProvider(s.T().Context(), cfg, "test-svc", "1.0.0")
		s.Require().NoError(err)
		s.NotNil(provider)
		defer func() { _ = provider.Shutdown(s.T().Context()) }()
	})
}

func (s *LogsSuite) TestCreateLogExporter() {
	s.Run("creates gRPC exporter by default when protocol is empty", func() {
		cfg := &config.TelemetryConfig{Endpoint: "http://localhost:4317"}
		exporter, err := createLogExporter(s.T().Context(), cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(s.T().Context()) }()
	})

	s.Run("creates HTTP exporter for http/protobuf protocol", func() {
		cfg := &config.TelemetryConfig{Endpoint: "http://localhost:4318", Protocol: "http/protobuf"}
		exporter, err := createLogExporter(s.T().Context(), cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(s.T().Context()) }()
	})

	s.Run("handles case-insensitive protocol values", func() {
		cfg := &config.TelemetryConfig{Endpoint: "http://localhost:4318", Protocol: "HTTP/PROTOBUF"}
		exporter, err := createLogExporter(s.T().Context(), cfg)
		s.Require().NoError(err)
		s.NotNil(exporter)
		defer func() { _ = exporter.Shutdown(s.T().Context()) }()
	})
}

// TestNewLogSinkCapturesLogs calls the real NewLogSink (the same function
// production uses) with a logtest.Recorder, then verifies that log records
// arrive in the OTel pipeline with correct severity, body, and attributes.
func (s *LogsSuite) TestNewLogSinkCapturesLogs() {
	s.Run("Info at V(0) arrives as SeverityInfo with body and attributes", func() {
		recorder := logtest.NewRecorder()
		sink := NewLogSink("test-svc", "1.0.0", recorder)
		logger := logr.New(sink)

		logger.Info("hello", "key", "val")

		records := allRecords(recorder.Result())
		s.Require().Len(records, 1)
		s.Equal("hello", records[0].Body.AsString())
		s.Equal(log.SeverityInfo, records[0].Severity)
		s.True(hasAttr(records[0], "key", "val"))
	})

	s.Run("Info at V(1) arrives as SeverityDebug", func() {
		recorder := logtest.NewRecorder()
		logger := logr.New(NewLogSink("test-svc", "1.0.0", recorder)).V(1)

		logger.Info("debug-msg")

		records := allRecords(recorder.Result())
		s.Require().Len(records, 1)
		s.Equal(log.SeverityDebug, records[0].Severity)
	})

	s.Run("Error arrives with error severity", func() {
		recorder := logtest.NewRecorder()
		logger := logr.New(NewLogSink("test-svc", "1.0.0", recorder))

		logger.Error(errTest, "broke", "code", 42)

		records := allRecords(recorder.Result())
		s.Require().Len(records, 1)
		s.Equal("broke", records[0].Body.AsString())
		s.GreaterOrEqual(int(records[0].Severity), int(log.SeverityError))
	})

	s.Run("WithValues attaches attributes to subsequent records", func() {
		recorder := logtest.NewRecorder()
		logger := logr.New(NewLogSink("test-svc", "1.0.0", recorder)).WithValues("rid", "abc")

		logger.Info("with-values")

		records := allRecords(recorder.Result())
		s.Require().Len(records, 1)
		s.True(hasAttr(records[0], "rid", "abc"))
	})

	s.Run("multiple calls accumulate records", func() {
		recorder := logtest.NewRecorder()
		logger := logr.New(NewLogSink("test-svc", "1.0.0", recorder))

		logger.Info("one")
		logger.Info("two")
		logger.Info("three")

		s.Len(allRecords(recorder.Result()), 3)
	})
}

// --- helpers -------------------------------------------------------------

type errString string

func (e errString) Error() string { return string(e) }

var errTest = errString("test error")

func allRecords(recording logtest.Recording) []logtest.Record {
	var out []logtest.Record
	for _, recs := range recording {
		out = append(out, recs...)
	}
	return out
}

func hasAttr(r logtest.Record, key attribute.Key, value string) bool {
	for _, a := range r.Attributes {
		if a.Key == key && a.Value.AsString() == value {
			return true
		}
	}
	return false
}

package telemetry

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
)

type OTLPHTTPPathSuite struct {
	suite.Suite
	mu    sync.Mutex
	paths []string
	url   string
}

func (s *OTLPHTTPPathSuite) SetupTest() {
	s.paths = nil
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.paths = append(s.paths, r.URL.Path)
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	s.T().Cleanup(server.Close)
	s.url = server.URL
}

func (s *OTLPHTTPPathSuite) recorded() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

func (s *OTLPHTTPPathSuite) TestTracesUseSignalPathForBaseEndpoint() {
	exporter, err := createExporterWithConfig(s.T().Context(), configtest.Telemetry(s.url, "http/protobuf"))
	s.Require().NoError(err)
	defer func() { _ = exporter.Shutdown(s.T().Context()) }()

	s.Require().NoError(exporter.ExportSpans(s.T().Context(), tracetest.SpanStubs{{Name: "span"}}.Snapshots()))
	s.Equal([]string{"/v1/traces"}, s.recorded())
}

func (s *OTLPHTTPPathSuite) TestLogsUseSignalPathForBaseEndpoint() {
	exporter, err := createLogExporter(s.T().Context(), configtest.Telemetry(s.url, "http/protobuf"))
	s.Require().NoError(err)
	defer func() { _ = exporter.Shutdown(s.T().Context()) }()

	s.Require().NoError(exporter.Export(s.T().Context(), []sdklog.Record{{}}))
	s.Equal([]string{"/v1/logs"}, s.recorded())
}

func (s *OTLPHTTPPathSuite) TestExplicitPathIsKept() {
	exporter, err := createExporterWithConfig(s.T().Context(), configtest.Telemetry(s.url+"/custom/traces", "http/protobuf"))
	s.Require().NoError(err)
	defer func() { _ = exporter.Shutdown(s.T().Context()) }()

	s.Require().NoError(exporter.ExportSpans(s.T().Context(), tracetest.SpanStubs{{Name: "span"}}.Snapshots()))
	s.Equal([]string{"/custom/traces"}, s.recorded())
}

func TestOTLPHTTPPath(t *testing.T) {
	suite.Run(t, new(OTLPHTTPPathSuite))
}

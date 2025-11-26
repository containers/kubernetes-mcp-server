package mcp

import (
	"bytes"
	"flag"
	"regexp"
	"strconv"
	"testing"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/stretchr/testify/suite"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
)

type McpLoggingSuite struct {
	BaseMcpSuite
	klogState klog.State
	logBuffer bytes.Buffer
}

func (s *McpLoggingSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.klogState = klog.CaptureState()
}

func (s *McpLoggingSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	s.klogState.Restore()
}

func (s *McpLoggingSuite) SetLogLevel(level int) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	klog.InitFlags(flags)
	_ = flags.Set("v", strconv.Itoa(level))
	klog.SetLogger(textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(level), textlogger.Output(&s.logBuffer))))
}

func (s *McpLoggingSuite) TestLogsToolCall() {
	s.SetLogLevel(5)
	s.InitMcpClient()
	_, err := s.CallTool("configuration_view", map[string]interface{}{"minified": false})
	s.Require().NoError(err, "call to tool configuration_view failed")

	s.Run("Logs tool name", func() {
		s.Contains(s.logBuffer.String(), "mcp tool call: configuration_view(")
	})
	s.Run("Logs tool call arguments", func() {
		expected := `"mcp tool call: configuration_view\((.+)\)"`
		m := regexp.MustCompile(expected).FindStringSubmatch(s.logBuffer.String())
		s.Len(m, 2, "Expected log entry to contain arguments")
		s.Equal("map[minified:false]", m[1], "Expected log arguments to be 'map[minified:false]'")
	})
}

func (s *McpLoggingSuite) TestLogsToolCallHeaders() {
	s.SetLogLevel(7)
	s.InitMcpClient(transport.WithHTTPHeaders(map[string]string{
		"Accept-Encoding":   "gzip",
		"Authorization":     "Bearer should-not-be-logged",
		"authorization":     "Bearer should-not-be-logged",
		"a-loggable-header": "should-be-logged",
	}))
	_, err := s.CallTool("configuration_view", map[string]interface{}{"minified": false})
	s.Require().NoError(err, "call to tool configuration_view failed")

	s.Run("Logs tool call headers", func() {
		expectedLog := "mcp tool call headers: A-Loggable-Header: should-be-logged"
		s.Contains(s.logBuffer.String(), expectedLog, "Expected log to contain loggable header")
	})
	sensitiveHeaders := []string{
		"Authorization:",
		// TODO: Add more sensitive headers as needed
	}
	s.Run("Does not log sensitive headers", func() {
		for _, header := range sensitiveHeaders {
			s.NotContains(s.logBuffer.String(), header, "Log should not contain sensitive header")
		}
	})
	s.Run("Does not log sensitive header values", func() {
		s.NotContains(s.logBuffer.String(), "should-not-be-logged", "Log should not contain sensitive header value")
	})
}

func TestMcpLogging(t *testing.T) {
	suite.Run(t, new(McpLoggingSuite))
}

type CustomAuthHeadersMiddlewareSuite struct {
	BaseMcpSuite
}

func (s *CustomAuthHeadersMiddlewareSuite) TestParsesAuthHeadersFromHTTPHeaders() {
	caCertBase64 := "dGVzdC1jYS1jZXJ0" // base64 of "test-ca-cert"
	serverURL := "https://k8s.example.com:6443"
	token := "Bearer test-token"

	s.InitMcpClient(transport.WithHTTPHeaders(map[string]string{
		"kubernetes-server":                     serverURL,
		"kubernetes-certificate-authority-data": caCertBase64,
		"kubernetes-authorization":              token,
	}))

	_, err := s.CallTool("configuration_view", map[string]interface{}{"minified": false})
	s.Require().NoError(err, "call to tool configuration_view failed")

	// The middleware should have successfully parsed and added auth headers to context
	// This is validated indirectly by the tool call succeeding
}

func (s *CustomAuthHeadersMiddlewareSuite) TestHeadersAreLowercased() {
	caCertBase64 := "dGVzdC1jYS1jZXJ0" // base64 of "test-ca-cert"
	serverURL := "https://k8s.example.com:6443"
	token := "Bearer test-token"

	// Use uppercase header names
	s.InitMcpClient(transport.WithHTTPHeaders(map[string]string{
		"Kubernetes-Server":                     serverURL,    // uppercase K
		"KUBERNETES-CERTIFICATE-AUTHORITY-DATA": caCertBase64, // all uppercase
		"Kubernetes-Authorization":              token,        // mixed case
	}))

	_, err := s.CallTool("configuration_view", map[string]interface{}{"minified": false})
	s.Require().NoError(err, "call should succeed even with uppercase headers")
}

func (s *CustomAuthHeadersMiddlewareSuite) TestIgnoresInvalidAuthHeadersWhenNotUsingAuthHeadersProvider() {
	// When not using auth-headers provider, invalid custom headers are ignored
	// and the default kubeconfig provider is used instead
	s.InitMcpClient(transport.WithHTTPHeaders(map[string]string{
		"kubernetes-server": "https://k8s.example.com:6443",
		// Missing CA cert and authorization - will be ignored
	}))

	_, err := s.CallTool("configuration_view", map[string]interface{}{"minified": false})
	s.Require().NoError(err, "call should succeed using default kubeconfig provider")
}

func (s *CustomAuthHeadersMiddlewareSuite) TestPassesThroughWithNoHeaders() {
	// No custom headers provided - should work with default kubeconfig
	s.InitMcpClient()

	_, err := s.CallTool("configuration_view", map[string]interface{}{"minified": false})
	s.Require().NoError(err, "call should succeed without custom headers")
}

func TestCustomAuthHeadersMiddleware(t *testing.T) {
	suite.Run(t, new(CustomAuthHeadersMiddlewareSuite))
}

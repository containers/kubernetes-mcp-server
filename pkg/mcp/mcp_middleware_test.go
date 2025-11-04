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

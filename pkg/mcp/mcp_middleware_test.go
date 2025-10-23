package mcp

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client/transport"
)

func TestToolCallLogging(t *testing.T) {
	testCaseWithContext(t, &mcpContext{logLevel: 5}, func(c *mcpContext) {
		_, _ = c.callTool("configuration_view", map[string]interface{}{
			"minified": false,
		})
		t.Run("Logs tool name", func(t *testing.T) {
			expectedLog := "mcp tool call: configuration_view("
			if !strings.Contains(c.logBuffer.String(), expectedLog) {
				t.Errorf("Expected log to contain '%s', got: %s", expectedLog, c.logBuffer.String())
			}
		})
		t.Run("Logs tool call arguments", func(t *testing.T) {
			expected := `"mcp tool call: configuration_view\((.+)\)"`
			m := regexp.MustCompile(expected).FindStringSubmatch(c.logBuffer.String())
			if len(m) != 2 {
				t.Fatalf("Expected log entry to contain arguments, got %s", c.logBuffer.String())
			}
			if m[1] != "map[minified:false]" {
				t.Errorf("Expected log arguments to be 'map[minified:false]', got %s", m[1])
			}
		})
	})
	before := func(c *mcpContext) {
		c.clientOptions = append(c.clientOptions, transport.WithHeaders(map[string]string{
			"Accept-Encoding":   "gzip",
			"Authorization":     "Bearer should-not-be-logged",
			"authorization":     "Bearer should-not-be-logged",
			"a-loggable-header": "should-be-logged",
		}))
	}
	testCaseWithContext(t, &mcpContext{logLevel: 7, before: before}, func(c *mcpContext) {
		_, _ = c.callTool("configuration_view", map[string]interface{}{
			"minified": false,
		})
		t.Run("Logs tool call headers", func(t *testing.T) {
			expectedLog := "mcp tool call headers: A-Loggable-Header: should-be-logged"
			if !strings.Contains(c.logBuffer.String(), expectedLog) {
				t.Errorf("Expected log to contain '%s', got: %s", expectedLog, c.logBuffer.String())
			}
		})
		sensitiveHeaders := []string{
			"Authorization:",
			// TODO: Add more sensitive headers as needed
		}
		t.Run("Does not log sensitive headers", func(t *testing.T) {
			for _, header := range sensitiveHeaders {
				if strings.Contains(c.logBuffer.String(), header) {
					t.Errorf("Log should not contain sensitive header '%s', got: %s", header, c.logBuffer.String())
				}
			}
		})
		t.Run("Does not log sensitive header values", func(t *testing.T) {
			if strings.Contains(c.logBuffer.String(), "should-not-be-logged") {
				t.Errorf("Log should not contain sensitive header value 'should-not-be-logged', got: %s", c.logBuffer.String())
			}
		})
	})
}

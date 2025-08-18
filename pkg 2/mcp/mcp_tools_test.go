package mcp

import (
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"k8s.io/utils/ptr"
	"regexp"
	"strings"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
)

func TestUnrestricted(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		tools, err := c.mcpClient.ListTools(c.ctx, mcp.ListToolsRequest{})
		t.Run("ListTools returns tools", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call ListTools failed %v", err)
			}
		})
		t.Run("Destructive tools ARE NOT read only", func(t *testing.T) {
			for _, tool := range tools.Tools {
				readOnly := ptr.Deref(tool.Annotations.ReadOnlyHint, false)
				destructive := ptr.Deref(tool.Annotations.DestructiveHint, false)
				if readOnly && destructive {
					t.Errorf("Tool %s is read-only and destructive, which is not allowed", tool.Name)
				}
			}
		})
	})
}

func TestReadOnly(t *testing.T) {
	readOnlyServer := func(c *mcpContext) { c.staticConfig = &config.StaticConfig{ReadOnly: true} }
	testCaseWithContext(t, &mcpContext{before: readOnlyServer}, func(c *mcpContext) {
		tools, err := c.mcpClient.ListTools(c.ctx, mcp.ListToolsRequest{})
		t.Run("ListTools returns tools", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call ListTools failed %v", err)
			}
		})
		t.Run("ListTools returns only read-only tools", func(t *testing.T) {
			for _, tool := range tools.Tools {
				if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
					t.Errorf("Tool %s is not read-only but should be", tool.Name)
				}
				if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
					t.Errorf("Tool %s is destructive but should not be in read-only mode", tool.Name)
				}
			}
		})
	})
}

func TestDisableDestructive(t *testing.T) {
	disableDestructiveServer := func(c *mcpContext) { c.staticConfig = &config.StaticConfig{DisableDestructive: true} }
	testCaseWithContext(t, &mcpContext{before: disableDestructiveServer}, func(c *mcpContext) {
		tools, err := c.mcpClient.ListTools(c.ctx, mcp.ListToolsRequest{})
		t.Run("ListTools returns tools", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call ListTools failed %v", err)
			}
		})
		t.Run("ListTools does not return destructive tools", func(t *testing.T) {
			for _, tool := range tools.Tools {
				if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
					t.Errorf("Tool %s is destructive but should not be", tool.Name)
				}
			}
		})
	})
}

func TestEnabledTools(t *testing.T) {
	testCaseWithContext(t, &mcpContext{
		staticConfig: &config.StaticConfig{
			EnabledTools: []string{"namespaces_list", "events_list"},
		},
	}, func(c *mcpContext) {
		tools, err := c.mcpClient.ListTools(c.ctx, mcp.ListToolsRequest{})
		t.Run("ListTools returns tools", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call ListTools failed %v", err)
			}
		})
		t.Run("ListTools returns only explicitly enabled tools", func(t *testing.T) {
			if len(tools.Tools) != 2 {
				t.Fatalf("ListTools should return 2 tools, got %d", len(tools.Tools))
			}
			for _, tool := range tools.Tools {
				if tool.Name != "namespaces_list" && tool.Name != "events_list" {
					t.Errorf("Tool %s is not enabled but should be", tool.Name)
				}
			}
		})
	})
}

func TestDisabledTools(t *testing.T) {
	testCaseWithContext(t, &mcpContext{
		staticConfig: &config.StaticConfig{
			DisabledTools: []string{"namespaces_list", "events_list"},
		},
	}, func(c *mcpContext) {
		tools, err := c.mcpClient.ListTools(c.ctx, mcp.ListToolsRequest{})
		t.Run("ListTools returns tools", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call ListTools failed %v", err)
			}
		})
		t.Run("ListTools does not return disabled tools", func(t *testing.T) {
			for _, tool := range tools.Tools {
				if tool.Name == "namespaces_list" || tool.Name == "events_list" {
					t.Errorf("Tool %s is not disabled but should be", tool.Name)
				}
			}
		})
	})
}

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
			"Authorization",
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

package mcp

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

// McpToolProcessingSuite tests MCP tool processing (isToolApplicable)
type McpToolProcessingSuite struct {
	BaseMcpSuite
}

func (s *McpToolProcessingSuite) TestUnrestricted() {
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("Destructive tools ARE NOT read only", func() {
		for _, tool := range tools.Tools {
			readOnly := tool.Annotations.ReadOnlyHint
			destructive := tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint
			s.Falsef(readOnly && destructive, "Tool %s is read-only and destructive, which is not allowed", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestReadOnly() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		read_only = true
	`), s.Cfg), "Expected to parse read only server config")
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools returns only read-only tools", func() {
		for _, tool := range tools.Tools {
			s.Truef(tool.Annotations.ReadOnlyHint,
				"Tool %s is not read-only but should be", tool.Name)
			s.Falsef(tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint,
				"Tool %s is destructive but should not be in read-only mode", tool.Name)
		}
	})
}

// TestReadOnlyBlocksWriteToolInvocation proves that read_only=true is enforced,
// not just advertised. TestReadOnly above only checks that ListTools omits write
// tools from the menu; a client that already knows a write tool's name (e.g. from
// a previous, non-read-only session, or by guessing) could still try to invoke it
// directly. This calls write tools by name regardless of what ListTools returned,
// and additionally verifies against the real (envtest) cluster that the target
// resource was left untouched -- a ground-truth check independent of the API
// response, so a bug that returns an error while still mutating the cluster would
// still be caught.
//
// Two tools on two different code paths (pods_delete, resources_create_or_update)
// are exercised to show the block is a property of read_only=true itself -- both
// go through the same isToolApplicable filtering at server startup (see mcp.go),
// so a single representative tool would already prove the mechanism works, but
// covering both guards against a future regression that is specific to one
// handler rather than the shared filtering logic.
func (s *McpToolProcessingSuite) TestReadOnlyBlocksWriteToolInvocation() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		read_only = true
	`), s.Cfg), "Expected to parse read only server config")
	s.InitMcpClient()

	kubernetesAdmin := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())

	s.Run("pods_delete", func() {
		s.Run("calling the tool directly is rejected even though it is not in the tool list", func() {
			result, err := s.CallTool("pods_delete", map[string]any{"name": "a-pod-in-default"})
			s.Require().Errorf(err, "expected calling a write tool directly to be rejected in read-only mode, got result: %v", result)
		})

		s.Run("ground truth: the target resource was not mutated", func() {
			pod, err := kubernetesAdmin.CoreV1().Pods("default").Get(s.T().Context(), "a-pod-in-default", metav1.GetOptions{})
			s.Require().NoError(err, "expected pod a-pod-in-default to still exist after the blocked delete attempt")
			s.NotNilf(pod, "expected pod a-pod-in-default to still exist after the blocked delete attempt")
		})
	})

	s.Run("resources_create_or_update", func() {
		const configMapName = "should-not-exist-in-read-only-mode"
		configMapYaml := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: " + configMapName + "\n  namespace: default\n"

		s.Run("calling the tool directly is rejected even though it is not in the tool list", func() {
			result, err := s.CallTool("resources_create_or_update", map[string]any{"resource": configMapYaml})
			s.Require().Errorf(err, "expected calling a write tool directly to be rejected in read-only mode, got result: %v", result)
		})

		s.Run("ground truth: the resource was never created", func() {
			_, err := kubernetesAdmin.CoreV1().ConfigMaps("default").Get(s.T().Context(), configMapName, metav1.GetOptions{})
			s.Truef(errors.IsNotFound(err), "expected ConfigMap %s to not exist after the blocked create attempt, got err: %v", configMapName, err)
		})
	})
}

func (s *McpToolProcessingSuite) TestDisableDestructive() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		disable_destructive = true
	`), s.Cfg), "Expected to parse disable destructive server config")
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools does not return destructive tools", func() {
		for _, tool := range tools.Tools {
			s.Falsef(tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint,
				"Tool %s is destructive but should not be in disable_destructive mode", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestEnabledTools() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		enabled_tools = [ "namespaces_list", "events_list" ]
	`), s.Cfg), "Expected to parse enabled tools server config")
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools returns only explicitly enabled tools", func() {
		s.Len(tools.Tools, 2, "ListTools should return exactly 2 tools")
		for _, tool := range tools.Tools {
			s.Falsef(tool.Name != "namespaces_list" && tool.Name != "events_list",
				"Tool %s is not enabled but should be", tool.Name)
		}
	})
}

func (s *McpToolProcessingSuite) TestDisabledTools() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		disabled_tools = [ "namespaces_list", "events_list" ]
	`), s.Cfg), "Expected to parse disabled tools server config")
	s.InitMcpClient()

	tools, err := s.ListTools()
	s.Require().NotNil(tools)

	s.Run("ListTools returns tools", func() {
		s.NoError(err, "call ListTools failed")
		s.NotNilf(tools, "list tools failed")
	})

	s.Run("ListTools does not return disabled tools", func() {
		for _, tool := range tools.Tools {
			s.Falsef(tool.Name == "namespaces_list" || tool.Name == "events_list",
				"Tool %s is not disabled but should be", tool.Name)
		}
	})
}

func TestMcpToolProcessing(t *testing.T) {
	suite.Run(t, new(McpToolProcessingSuite))
}

package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config/configtest"
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
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		read_only = true
	`)
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

// TestReadOnlyBlocksWriteToolInvocation verifies read_only = true is enforced
func (s *McpToolProcessingSuite) TestReadOnlyBlocksWriteToolInvocation() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		read_only = true
	`)
	s.InitMcpClient()
	kc := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())

	s.Run("pods_delete", func() {
		_, err := kc.CoreV1().Pods("default").Create(s.T().Context(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "a-pod-protected-by-read-only"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "nginx", Image: "nginx"}}},
		}, metav1.CreateOptions{})
		s.Require().NoError(err, "failed to create test pod")
		s.T().Cleanup(func() {
			_ = kc.CoreV1().Pods("default").Delete(context.Background(), "a-pod-protected-by-read-only", metav1.DeleteOptions{})
		})

		_, err = s.CallTool("pods_delete", map[string]any{"name": "a-pod-protected-by-read-only", "namespace": "default"})
		s.Run("is rejected as an unknown tool", func() {
			s.Require().Error(err, "expected pods_delete to be rejected in read-only mode")
			s.Contains(err.Error(), "unknown tool")
		})
		s.Run("does not delete the pod", func() {
			_, getErr := kc.CoreV1().Pods("default").Get(s.T().Context(), "a-pod-protected-by-read-only", metav1.GetOptions{})
			s.NoError(getErr, "pod should still exist after a rejected pods_delete")
		})
	})

	s.Run("resources_create_or_update", func() {
		_, err := s.CallTool("resources_create_or_update", map[string]any{
			"resource": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a-configmap-blocked-by-read-only\n  namespace: default\n",
		})
		s.Run("is rejected as an unknown tool", func() {
			s.Require().Error(err, "expected resources_create_or_update to be rejected in read-only mode")
			s.Contains(err.Error(), "unknown tool")
		})
		s.Run("does not create the resource", func() {
			_, getErr := kc.CoreV1().ConfigMaps("default").Get(s.T().Context(), "a-configmap-blocked-by-read-only", metav1.GetOptions{})
			s.Truef(apierrors.IsNotFound(getErr), "configmap should not exist after a rejected resources_create_or_update, got: %v", getErr)
		})
	})
}

func (s *McpToolProcessingSuite) TestDisableDestructive() {
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		disable_destructive = true
	`)
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
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		enabled_tools = [ "namespaces_list", "events_list" ]
	`)
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
	configtest.OverlayTOML(s.T(), &s.Cfg, `
		disabled_tools = [ "namespaces_list", "events_list" ]
	`)
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

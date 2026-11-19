package mcp

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

// McpBuiltinPromptsSuite exercises the built-in prompts shipped by the core
// toolset end to end through the MCP protocol against envtest. It mirrors the
// manual validation matrix (no arguments, namespace scoped with and without
// events) and the multi-cluster target argument behaviour.
type McpBuiltinPromptsSuite struct {
	BaseMcpSuite
}

const clusterHealthCheckPrompt = "cluster-health-check"

// promptText returns the text of message i, failing the test if it is not TextContent.
func (s *McpBuiltinPromptsSuite) promptText(result *mcp.GetPromptResult, i int) string {
	s.T().Helper()
	s.Require().NotNil(result)
	s.Require().Greater(len(result.Messages), i, "expected at least %d messages", i+1)
	textContent, ok := result.Messages[i].Content.(*mcp.TextContent)
	s.Require().True(ok, "expected TextContent in message %d", i)
	return textContent.Text
}

func (s *McpBuiltinPromptsSuite) findPrompt(name string) *mcp.Prompt {
	s.T().Helper()
	prompts, err := s.ListPrompts()
	s.Require().NoError(err, "call ListPrompts failed")
	for _, p := range prompts.Prompts {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (s *McpBuiltinPromptsSuite) TestClusterHealthCheckListed() {
	s.InitMcpClient()

	prompt := s.findPrompt(clusterHealthCheckPrompt)

	s.Run("built-in prompt is listed with its metadata", func() {
		s.Require().NotNil(prompt, "%s should be listed", clusterHealthCheckPrompt)
		s.Equal("Cluster Health Check", prompt.Title)
		s.NotEmpty(prompt.Description)
	})

	s.Run("both arguments are listed as optional", func() {
		s.Require().NotNil(prompt)
		s.Require().Len(prompt.Arguments, 2, "single-cluster mode exposes exactly namespace and check_events")
		s.Equal("namespace", prompt.Arguments[0].Name)
		s.False(prompt.Arguments[0].Required)
		s.Equal("check_events", prompt.Arguments[1].Name)
		s.False(prompt.Arguments[1].Required)
	})
}

func (s *McpBuiltinPromptsSuite) TestClusterHealthCheckWithoutArguments() {
	s.InitMcpClient()

	result, err := s.GetPrompt(clusterHealthCheckPrompt, nil)

	s.Run("GetPrompt succeeds", func() {
		s.Require().NoError(err, "call GetPrompt failed")
		s.Require().NotNil(result)
		s.Equal("Cluster health diagnostic data gathered successfully", result.Description)
	})

	s.Run("returns a user diagnostic message followed by an assistant message", func() {
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 2)
		s.Equal("user", string(result.Messages[0].Role))
		s.Equal("assistant", string(result.Messages[1].Role))
		s.Contains(s.promptText(result, 0), "# Cluster Health Check Diagnostic Data")
		s.Contains(s.promptText(result, 1), "analyze the cluster health diagnostic data")
	})

	s.Run("scope is cluster-wide", func() {
		text := s.promptText(result, 0)
		s.Contains(text, "**Scope:** All namespaces")
		s.NotContains(text, "**WARNING:**")
	})

	s.Run("pods from every namespace are included", func() {
		text := s.promptText(result, 0)
		s.Contains(text, "## 3. Pods")
		s.Contains(text, "ns-1/a-pod-in-ns-1")
		s.Contains(text, "ns-2/a-pod-in-ns-2")
	})

	s.Run("events are checked by default", func() {
		s.Contains(s.promptText(result, 0), "## 6. Recent Events (Last Hour)")
	})
}

func (s *McpBuiltinPromptsSuite) TestClusterHealthCheckNamespaceScoped() {
	s.InitMcpClient()

	s.Run("namespace with check_events=false", func() {
		result, err := s.GetPrompt(clusterHealthCheckPrompt, map[string]string{
			"namespace":    "ns-1",
			"check_events": "false",
		})
		s.Require().NoError(err, "call GetPrompt failed")
		text := s.promptText(result, 0)

		s.Run("scope is the requested namespace", func() {
			s.Contains(text, "**Scope:** Namespace `ns-1`")
			s.NotContains(text, "**WARNING:**")
		})
		s.Run("only pods from that namespace are included", func() {
			s.Contains(text, "ns-1/a-pod-in-ns-1")
			s.NotContains(text, "ns-2/a-pod-in-ns-2")
		})
		s.Run("events section is omitted", func() {
			s.NotContains(text, "## 6. Recent Events")
		})
	})

	s.Run("namespace with check_events=true", func() {
		result, err := s.GetPrompt(clusterHealthCheckPrompt, map[string]string{
			"namespace":    "ns-1",
			"check_events": "true",
		})
		s.Require().NoError(err, "call GetPrompt failed")
		text := s.promptText(result, 0)

		s.Run("scope is the requested namespace", func() {
			s.Contains(text, "**Scope:** Namespace `ns-1`")
		})
		s.Run("events section is included", func() {
			s.Contains(text, "## 6. Recent Events (Last Hour)")
		})
	})

	s.Run("check_events with a non-boolean value is treated as true", func() {
		result, err := s.GetPrompt(clusterHealthCheckPrompt, map[string]string{
			"namespace":    "ns-1",
			"check_events": "maybe",
		})
		s.Require().NoError(err, "call GetPrompt failed")
		s.Contains(s.promptText(result, 0), "## 6. Recent Events (Last Hour)")
	})
}

func (s *McpBuiltinPromptsSuite) TestClusterHealthCheckUnknownNamespace() {
	s.InitMcpClient()

	result, err := s.GetPrompt(clusterHealthCheckPrompt, map[string]string{
		"namespace": "does-not-exist",
	})

	s.Run("GetPrompt still succeeds", func() {
		s.Require().NoError(err, "unknown namespace must not fail the prompt")
		s.Require().NotNil(result)
	})

	s.Run("warns about the unknown namespace and falls back to cluster-wide", func() {
		text := s.promptText(result, 0)
		s.Contains(text, "**WARNING:** Namespace 'does-not-exist' not found or not accessible. Showing cluster-wide information instead.")
		s.Contains(text, "**Scope:** All namespaces")
		s.Contains(text, "ns-1/a-pod-in-ns-1")
	})
}

// multiClusterKubeconfig writes a copy of the envtest kubeconfig with extra
// (unreachable) contexts so the provider switches to multi-cluster mode, and
// returns the name of the real envtest context.
func (s *McpBuiltinPromptsSuite) multiClusterKubeconfig() string {
	s.T().Helper()
	kubeconfig, err := clientcmd.LoadFromFile(s.Cfg.KubeConfig.Get())
	s.Require().NoError(err, "load envtest kubeconfig")
	realContext := kubeconfig.CurrentContext
	s.Require().NotEmpty(realContext, "envtest kubeconfig should have a current context")
	kubeconfig.Contexts["extra-cluster"] = clientcmdapi.NewContext()
	s.Cfg.KubeConfig.SetForTest(test.KubeconfigFile(s.T(), kubeconfig))
	return realContext
}

func (s *McpBuiltinPromptsSuite) TestClusterHealthCheckMultiCluster() {
	realContext := s.multiClusterKubeconfig()
	s.InitMcpClient()

	s.Run("context argument is added to cluster-aware prompts", func() {
		prompt := s.findPrompt(clusterHealthCheckPrompt)
		s.Require().NotNil(prompt)
		var contextArg *mcp.PromptArgument
		for _, arg := range prompt.Arguments {
			if arg.Name == "context" {
				contextArg = arg
			}
		}
		s.Require().NotNil(contextArg, "context argument should be present in multi-cluster mode")
		s.False(contextArg.Required, "context argument must be optional")
		s.Contains(contextArg.Description, realContext, "description should name the default context")
	})

	s.Run("omitting context uses the default cluster", func() {
		result, err := s.GetPrompt(clusterHealthCheckPrompt, map[string]string{
			"namespace": "ns-1", "check_events": "false",
		})
		s.Require().NoError(err, "call GetPrompt failed")
		s.Contains(s.promptText(result, 0), "ns-1/a-pod-in-ns-1")
	})

	s.Run("explicit context selects the named cluster", func() {
		result, err := s.GetPrompt(clusterHealthCheckPrompt, map[string]string{
			"namespace": "ns-1", "check_events": "false", "context": realContext,
		})
		s.Require().NoError(err, "call GetPrompt failed")
		s.Contains(s.promptText(result, 0), "ns-1/a-pod-in-ns-1")
	})

	s.Run("unknown context returns an error", func() {
		result, err := s.GetPrompt(clusterHealthCheckPrompt, map[string]string{
			"context": "no-such-context",
		})
		s.Error(err, "expected error for unknown context")
		s.Nil(result)
	})
}

func TestMcpBuiltinPromptsSuite(t *testing.T) {
	suite.Run(t, new(McpBuiltinPromptsSuite))
}

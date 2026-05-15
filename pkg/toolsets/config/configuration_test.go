package config

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// The MCP-level integration suite (pkg/mcp/configuration_test.go) cannot
// exercise the empty-kubeconfig branch of contextsList:
//   - a kubeconfig with zero contexts fails provider initialization
//     ("no current-context is set and no contexts are defined"), and
//   - a kubeconfig with a single context filters the tool out entirely via
//     ShouldIncludeTargetListTool because the provider is not multi-target.
// This test covers the otherwise-unreachable empty branch by invoking the
// handler with a minimal ClientConfig that returns a kubeconfig with no
// contexts.

type ContextsListEmptySuite struct {
	suite.Suite
}

func (s *ContextsListEmptySuite) TestContextsListEmpty() {
	params := api.ToolHandlerParams{
		KubernetesClient: emptyContextsKubernetesClient{},
	}

	result, err := contextsList(params)

	s.Require().NoError(err, "contextsList returned a transport error")
	s.Require().NotNil(result, "expected non-nil result for empty kubeconfig")
	s.NoError(result.Error, "expected no tool-level error for empty kubeconfig")
	s.Equal("No contexts found in kubeconfig", result.Content)
	s.Nil(result.StructuredContent, "structured content must be nil when no contexts are present")
}

func TestContextsListEmptySuite(t *testing.T) {
	suite.Run(t, new(ContextsListEmptySuite))
}

// emptyContextsKubernetesClient is a minimal api.KubernetesClient that
// returns a kubeconfig with no contexts. The embedded api.KubernetesClient
// is left nil — contextsList only calls ToRawKubeConfigLoader on it for
// the empty branch, so the other interface methods are never reached.
type emptyContextsKubernetesClient struct {
	api.KubernetesClient // intentionally nil; any unexpected method call will panic loudly.
}

func (emptyContextsKubernetesClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return emptyClientConfig{}
}

// emptyClientConfig is a clientcmd.ClientConfig that returns a kubeconfig
// with no contexts. The non-RawConfig methods are stubbed because the
// configuration_contexts_list handler does not invoke them on the empty
// branch — `clientcmd.ClientConfig` cannot be embedded here because the
// interface has a `ClientConfig()` method that would collide with the
// embedded field's promoted name.
type emptyClientConfig struct{}

var _ clientcmd.ClientConfig = emptyClientConfig{}

func (emptyClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return *clientcmdapi.NewConfig(), nil
}

func (emptyClientConfig) ClientConfig() (*rest.Config, error) { return nil, nil }

func (emptyClientConfig) Namespace() (string, bool, error) { return "", false, nil }

func (emptyClientConfig) ConfigAccess() clientcmd.ConfigAccess { return nil }

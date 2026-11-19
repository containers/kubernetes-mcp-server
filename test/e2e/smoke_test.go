//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

type smokeState struct {
	dep       *serverDeployment
	mcpClient *test.McpClient
}

var smokeTS testState[smokeState]

func TestSmoke(t *testing.T) {
	f := features.New("smoke").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			dep := deployServer(ctx, t, cfg, "smoke", withValues(viewClusterRoleBindingValues()))
			mcpClient := test.NewMcpClient(t, nil, test.WithEndpoint(dep.serverURL+"/mcp"))
			t.Cleanup(mcpClient.Close)
			return smokeTS.set(ctx, &smokeState{dep: dep, mcpClient: mcpClient})
		}).
		Assess("server exposes expected tools", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := smokeTS.get(ctx)

			result, err := s.mcpClient.ListTools()
			require.NoError(t, err)
			names := toolNames(result.Tools)
			require.Greater(t, len(names), 0, "expected at least one tool")
			require.Contains(t, names, "namespaces_list")
			require.Contains(t, names, "pods_list")

			return ctx
		}).
		Assess("namespaces_list returns real cluster data", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := smokeTS.get(ctx)

			output := requireToolCallSuccess(t, s.mcpClient, "namespaces_list", map[string]any{})
			require.NotEmpty(t, output, "expected text content from namespaces_list")

			// The server's own namespace must appear.
			require.Contains(t, output, s.dep.namespace,
				"server namespace %q not in namespaces_list output", s.dep.namespace)

			// Every namespace the K8s API knows about should be present.
			clientset, err := clientsetFromKubeconfig(cfg.KubeconfigFile())
			require.NoError(t, err)
			nsList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			for _, ns := range nsList.Items {
				require.Contains(t, output, ns.Name,
					"namespace %q missing from tool output", ns.Name)
			}

			return ctx
		}).
		Assess("pods_list returns the server pod", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			s := smokeTS.get(ctx)

			output := requireToolCallSuccess(t, s.mcpClient, "pods_list_in_namespace", map[string]any{
				"namespace": s.dep.namespace,
			})
			require.Contains(t, output, s.dep.name,
				"server pod not found in pods_list output for namespace %s", s.dep.namespace)

			return ctx
		}).
		Feature()

	testenv.Test(t, f)
}

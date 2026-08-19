//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

const toolPrefix = "k8s_"

var mcpServerRegistrationGVR = schema.GroupVersionResource{
	Group:    "mcp.kuadrant.io",
	Version:  "v1alpha1",
	Resource: "mcpserverregistrations",
}

func TestKuadrantGatewayTools(t *testing.T) {
	gatewayNS := envOrDefault("GATEWAY_NAMESPACE", "gateway-system")
	gatewaySvc := envOrDefault("GATEWAY_SERVICE", "mcp-gateway-istio")
	gatewayHost := envOrDefault("GATEWAY_HOST", "mcp.gateway.local")

	f := features.New("kuadrant-gateway-tools").
		Assess("tools accessible through Kuadrant MCP Gateway", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			kubeconfig := cfg.KubeconfigFile()
			clientset, err := clientsetFromKubeconfig(kubeconfig)
			require.NoError(t, err)

			restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			require.NoError(t, err)

			// Check if gateway service exists; skip if not.
			_, err = clientset.CoreV1().Services(gatewayNS).Get(ctx, gatewaySvc, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				t.Skipf("Kuadrant MCP Gateway service %s/%s not found — install with 'make kuadrant-setup'", gatewayNS, gatewaySvc)
			}
			require.NoError(t, err, "check gateway service")

			// 1. Deploy the MCP server with read-only config, RBAC, and HTTPRoute.
			dep := deployServer(ctx, t, cfg, "kuadrant-gw", withConfig(`
				read_only = true
			`), withValues(map[string]any{
				"rbac": map[string]any{
					"create": true,
					"extraClusterRoles": []map[string]any{
						{
							"name": "cluster-reader",
							"rules": []map[string]any{
								{
									"apiGroups": []string{""},
									"resources": []string{"namespaces", "nodes"},
									"verbs":     []string{"get", "list", "watch"},
								},
							},
						},
					},
					"extraClusterRoleBindings": []map[string]any{
						{
							"name": "view",
							"roleRef": map[string]any{
								"name":     "view",
								"external": true,
							},
						},
						{
							"name": "cluster-reader",
							"roleRef": map[string]any{
								"name": "cluster-reader",
							},
						},
					},
				},
				"httpRoute": map[string]any{
					"enabled": true,
					"parentRefs": []map[string]any{
						{
							"name":      "mcp-gateway",
							"namespace": gatewayNS,
						},
					},
					"hostnames": []string{"{{ .Release.Name }}.mcp.local"},
					"rules": []map[string]any{
						{
							"matches": []map[string]any{
								{"path": map[string]any{"type": "PathPrefix", "value": "/"}},
							},
						},
					},
				},
			}))

			// 2. Create MCPServerRegistration.
			dynClient, err := dynamic.NewForConfig(restCfg)
			require.NoError(t, err, "create dynamic client")

			regName := dep.name + "-reg"
			reg := &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "mcp.kuadrant.io/v1alpha1",
					"kind":       "MCPServerRegistration",
					"metadata": map[string]any{
						"name":      regName,
						"namespace": dep.namespace,
					},
					"spec": map[string]any{
						"prefix":           toolPrefix,
						"userSpecificList": "Enabled",
						"targetRef": map[string]any{
							"group": "gateway.networking.k8s.io",
							"kind":  "HTTPRoute",
							"name":  dep.name,
						},
					},
				},
			}
			_, err = dynClient.Resource(mcpServerRegistrationGVR).Namespace(dep.namespace).Create(ctx, reg, metav1.CreateOptions{})
			require.NoError(t, err, "create MCPServerRegistration")

			t.Cleanup(func() {
				_ = dynClient.Resource(mcpServerRegistrationGVR).Namespace(dep.namespace).Delete(
					context.Background(), regName, metav1.DeleteOptions{},
				)
			})

			// 3. Wait for registration to become Ready.
			waitForRegistrationReady(ctx, t, dynClient, dep.namespace, regName, 120*time.Second)

			// 4. Port-forward to the gateway and connect MCP.
			gatewayURL, stopGW := portForwardService(ctx, t, restCfg, clientset, gatewayNS, gatewaySvc, serverPort)
			t.Cleanup(stopGW)

			mcpClient := test.NewMcpClient(t, nil,
				test.WithEndpoint(gatewayURL+"/mcp"),
				test.WithHTTPHeaders(map[string]string{"Host": gatewayHost}),
			)
			t.Cleanup(mcpClient.Close)

			// 5. Verify tools with prefix.
			result, err := mcpClient.ListTools()
			require.NoError(t, err, "list tools through gateway")

			names := toolNames(result.Tools)
			var prefixed []string
			for _, tool := range result.Tools {
				if strings.HasPrefix(tool.Name, toolPrefix) {
					prefixed = append(prefixed, tool.Name)
				}
			}
			require.Greater(t, len(prefixed), 0,
				"expected tools with prefix %q, got: %v", toolPrefix, names)

			// 6. Call namespaces_list through the gateway and verify.
			namespacesTool := toolPrefix + "namespaces_list"
			require.Contains(t, names, namespacesTool)

			toolOutput := requireToolCallSuccess(t, mcpClient, namespacesTool, map[string]any{})

			// Cross-reference against direct K8s API.
			nsList, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			require.NoError(t, err, "list namespaces via K8s API")
			for _, ns := range nsList.Items {
				require.Contains(t, toolOutput, ns.Name,
					"namespace %q not found in tool output", ns.Name)
			}

			return ctx
		}).Feature()

	testenv.Test(t, f)
}

func waitForRegistrationReady(ctx context.Context, t *testing.T, dynClient dynamic.Interface, namespace, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		obj, err := dynClient.Resource(mcpServerRegistrationGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			lastErr = err
			time.Sleep(2 * time.Second)
			continue
		}

		conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
		if err != nil {
			require.Fail(t, fmt.Sprintf("MCPServerRegistration %s/%s has invalid status.conditions: %v", namespace, name, err))
		}
		if found {
			for _, c := range conditions {
				cond, ok := c.(map[string]any)
				if !ok {
					continue
				}
				if cond["type"] == "Ready" && cond["status"] == "True" {
					return
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	if lastErr != nil {
		require.NoError(t, lastErr, "MCPServerRegistration %s/%s: last API error before timeout", namespace, name)
	}
	require.Fail(t, fmt.Sprintf("MCPServerRegistration %s/%s not ready within %v", namespace, name, timeout))
}

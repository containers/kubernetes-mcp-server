package code

import (
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/code"
)

// sdkDocumentation provides documentation for LLMs about the JavaScript SDK available in scripts.
// This is included in the tool description to help LLMs understand how to use the API.
const sdkDocumentation = `
## JavaScript SDK

**Note:** Full ES5.1 syntax support, partial ES6. Synchronous execution only (no async/await or Promises).

### Globals
- **k8s** - Kubernetes client (case-insensitive: coreV1, CoreV1, COREV1 all work)
- **ctx** - Request context for cancellation
- **namespace** - Default namespace

### k8s API Clients
- k8s.coreV1() - pods, services, configMaps, secrets, namespaces, nodes, etc.
- k8s.appsV1() - deployments, statefulSets, daemonSets, replicaSets
- k8s.batchV1() - jobs, cronJobs
- k8s.networkingV1() - ingresses, networkPolicies
- k8s.rbacV1() - roles, roleBindings, clusterRoles, clusterRoleBindings
- k8s.metricsV1beta1Client() - pod and node metrics (CPU/memory usage)
- k8s.dynamicClient() - any resource by GVR
- k8s.discoveryClient() - API discovery

### Examples

#### Combine multiple API calls with JavaScript
` + "```javascript" + `
// Get all deployments and their pod counts across namespaces
const deps = k8s.appsV1().deployments("").list(ctx, {});
const result = deps.items.flatMap(d => {
  const pods = k8s.coreV1().pods(d.metadata.namespace).list(ctx, {
    labelSelector: Object.entries(d.spec.selector.matchLabels || {})
      .map(([k,v]) => k+"="+v).join(",")
  });
  return [{
    deployment: d.metadata.name,
    namespace: d.metadata.namespace,
    replicas: d.status.readyReplicas + "/" + d.status.replicas,
    pods: pods.items.map(p => p.metadata.name)
  }];
});
JSON.stringify(result);
` + "```" + `

#### Filter and aggregate
` + "```javascript" + `
const pods = k8s.coreV1().pods("").list(ctx, {});
const unhealthy = pods.items.filter(p =>
  p.status.containerStatuses?.some(c => c.restartCount > 5)
).map(p => ({
  name: p.metadata.name,
  ns: p.metadata.namespace,
  restarts: p.status.containerStatuses.reduce((s,c) => s + c.restartCount, 0)
}));
JSON.stringify(unhealthy);
` + "```" + `

#### Create resources (using standard Kubernetes YAML/JSON structure)
` + "```javascript" + `
const pod = {
  apiVersion: "v1", kind: "Pod",
  metadata: { name: "my-pod", namespace: namespace },
  spec: { containers: [{ name: "nginx", image: "nginx:latest" }] }
};
k8s.coreV1().pods(namespace).create(ctx, pod, {}).metadata.name;
` + "```" + `

#### API introspection
` + "```javascript" + `
// Discover available resources on coreV1
const resources = []; for (const k in k8s.coreV1()) if (typeof k8s.coreV1()[k]==='function') resources.push(k);
// resources: ["configMaps","namespaces","pods","secrets","services",...]

// Discover available operations on pods
const ops = []; for (const k in k8s.coreV1().pods(namespace)) if (typeof k8s.coreV1().pods(namespace)[k]==='function') ops.push(k);
// ops: ["create","delete","get","list","update","watch",...]
` + "```" + `

#### Get pod metrics with resource quantities
` + "```javascript" + `
const metrics = k8s.metricsV1beta1Client();
const podMetrics = metrics.podMetricses("").list(ctx, {});
const result = podMetrics.items.map(function(pm) {
  return {
    name: pm.metadata.name,
    cpu: pm.containers[0].usage.cpu,    // "100m"
    memory: pm.containers[0].usage.memory // "128Mi"
  };
});
JSON.stringify(result);
` + "```" + `

#### Get pod logs
` + "```javascript" + `
const logBytes = k8s.coreV1().pods(namespace).getLogs("my-pod", {container: "main", tailLines: 100}).doRaw(ctx);
var logs = ""; for (var i = 0; i < logBytes.length; i++) logs += String.fromCharCode(logBytes[i]);
logs;
` + "```" + `

### Return Value
Last expression is returned. Use JSON.stringify() for objects.
`

func initEvaluate() []api.ServerTool {
	tools := []api.ServerTool{
		{
			Tool: api.Tool{
				Name: "evaluate_script",
				Description: "Execute a JavaScript script with access to Kubernetes clients. " +
					"Use this tool for complex operations that require multiple API calls, " +
					"data transformation, filtering, or aggregation that would be inefficient " +
					"with individual tool calls. The script runs in a sandboxed environment " +
					"with access only to Kubernetes clients - no file system or network access." +
					"\n\n" + sdkDocumentation,
				InputSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"script": {
							Type:        "string",
							Description: "JavaScript code to execute. The last expression is returned as the result.",
						},
						"timeout": {
							Type:        "integer",
							Description: "Execution timeout in milliseconds (default: 30000, max: 300000)",
						},
					},
					Required: []string{"script"},
				},
				Annotations: api.ToolAnnotations{
					Title:           "Code: Evaluate Script",
					ReadOnlyHint:    ptr.To(false), // Scripts can modify resources
					DestructiveHint: ptr.To(true),  // Scripts can be destructive
					IdempotentHint:  ptr.To(false), // Scripts may not be idempotent
					OpenWorldHint:   ptr.To(true),  // Interacts with Kubernetes cluster
				},
			},
			Handler: evaluateScript,
		},
	}
	return tools
}

func evaluateScript(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	// Extract required script parameter
	script, err := api.RequiredString(params, "script")
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Extract optional timeout parameter
	args := params.GetArguments()
	timeout := code.DefaultTimeout
	if timeoutVal, ok := args["timeout"]; ok {
		timeoutMs, err := api.ParseInt64(timeoutVal)
		if err != nil {
			return api.NewToolCallResult("", fmt.Errorf("failed to parse timeout: %w", err)), nil
		}
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	// Create evaluator with SDK already configured
	evaluator, err := code.NewEvaluator(params.Context, params.KubernetesClient)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	// Execute the script
	result, err := evaluator.Evaluate(script, timeout)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	return api.NewToolCallResult(result, nil), nil
}

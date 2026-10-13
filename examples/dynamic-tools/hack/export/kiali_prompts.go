package main

import (
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/toolsets/dynamic"
)

// kialiCall is one POST to a Kiali MCP endpoint whose raw (JSON) body is kept
// as text, exactly as the in-tree prompts embed it.
func kialiCall(name, endpoint, body string, ignore bool, when string) dynamic.Step {
	return dynamic.Step{Name: name, When: when, IgnoreError: ignore, HTTP: &dynamic.HTTPAction{
		Method:                "POST",
		URL:                   urlSentinel,
		Path:                  "/api/mcp/" + endpoint,
		Headers:               map[string]string{"X-Kubernetes-MCP-Server": "true"},
		Body:                  &dynamic.Body{Expression: "merge(" + body + ", {'mcp_mode': 'true'})"},
		CertificateAuthority:  caSentinel,
		InsecureSkipTLSVerify: true,
		Response:              dynamic.ResponseText,
	}}
}

// kialiData renders a step result the way fetchKialiData does: the body, or a
// "(data unavailable: ...)" placeholder when the call failed.
func kialiData(step string) string {
	return "${type(steps." + step + ") == string ? steps." + step + " : '(data unavailable: ' + steps." + step + ".error + ')'}"
}

const (
	// nsListJSON resolves "all" to every accessible mesh namespace.
	nsListJSON  = "args.namespaces.trim().lowerAscii() == 'all'"
	resolvedNS  = "args.namespaces.trim().lowerAscii() == 'all' ? steps.nsList.namespaces.map(n, n.name).filter(n, n != '').join(',') : args.namespaces"
	listSummary = "## Instructions\n\nSummarize the %s listed above. Highlight any that need attention.\n"
)

func kialiPrompts() map[string][]promptConversion {
	scope := "${has(args.namespace) && args.namespace != '' ? \"namespace '\" + args.namespace + \"'\" : 'all namespaces'}"
	listPrompt := func(resourceType string) []promptConversion {
		return []promptConversion{{
			Steps: []dynamic.Step{kialiCall("data", "list_or_get_resources",
				"merge({'resourceType': '"+resourceType+"'}, has(args.namespace) && args.namespace != '' ? {'namespaces': args.namespace} : {})", false, "")},
			Messages: []dynamic.PromptMessage{{Role: "user", Text: "# List " + resourceType + "\n\n## Scope\n\n" + scope + "\n\n## Data\n\n${steps.data}\n\n" + strings.ReplaceAll(listSummary, "%s", resourceType)}},
		}}
	}
	nsListStep := func(when string) dynamic.Step {
		step := kialiCall("nsList", "list_or_get_resources", "{'resourceType': 'namespace'}", false, when)
		step.HTTP.Response = dynamic.ResponseJSON
		return step
	}
	return map[string][]promptConversion{
		"mesh-health-check": {{
			Steps: []dynamic.Step{kialiCall("status", "get_mesh_status", "{}", false, "")},
			Messages: []dynamic.PromptMessage{
				{Role: "user", Text: "# Mesh Health Check\n\n## Scope\n\nAnalyze the health of ${has(args.namespace) && args.namespace != '' ? \"the '\" + args.namespace + \"' namespace\" : 'the entire mesh'}.\n\n" +
					"## Collected Data\n\n### Mesh Status\n\n${steps.status}\n\n## Instructions\n\nBased on the data above, provide a comprehensive health report covering:\n\n" +
					"1. **Control Plane Status**: Is istiod running and healthy? Report any issues with the Istio control plane.\n" +
					"2. **Data Plane Health**: Which namespaces are healthy, degraded, or unhealthy? Summarize the overall data plane status.\n" +
					"3. **Observability Stack**: Are Prometheus, Grafana, and tracing backends connected and operational?\n" +
					"4. **Issues Found**: List any problems discovered, ordered by severity.\n" +
					"5. **Recommendations**: Suggest actions to resolve any issues found.\n"},
				{Role: "assistant", Text: "I'll analyze the mesh health data and provide a comprehensive assessment."},
			},
		}},
		"istio-config-review": {{
			Steps: []dynamic.Step{
				kialiCall("istio", "manage_istio_config_read", "{'namespace': args.namespace, 'action': 'list'}", true, ""),
				kialiCall("resources", "list_or_get_resources", "{'namespaces': args.namespace, 'resourceType': 'service'}", true, ""),
			},
			Messages: []dynamic.PromptMessage{
				{Role: "user", Text: "# Istio Configuration Review\n\n## Namespace: ${args.namespace}\n\nReview the Istio configuration in this namespace for correctness, best practices, and potential issues.\n\n---\n\n" +
					"## Istio Resources\n\n" + kialiData("istio") + "\n\n---\n\n## Services in Namespace\n\n" + kialiData("resources") + "\n\n---\n\n" +
					"## Review Checklist\n\nAnalyze the configuration above and check for:\n\n" +
					"1. **Validation Errors**: Are there any Istio validation warnings or errors reported?\n" +
					"2. **VirtualService Issues**: Do VirtualServices reference hosts and subsets that actually exist? Are weights valid?\n" +
					"3. **DestinationRule Consistency**: Do DestinationRules define subsets that match actual workload labels?\n" +
					"4. **Traffic Policy**: Are there conflicting traffic policies or missing mTLS settings?\n" +
					"5. **Gateway Configuration**: Are Gateways properly configured with correct hosts and ports?\n" +
					"6. **Best Practices**: Are there any configuration anti-patterns (e.g., overly broad host matches, missing retries)?\n\n" +
					"Provide a summary of findings with severity (critical, warning, info) and recommendations.\n"},
				{Role: "assistant", Text: "I'll review the Istio configuration and check for issues or misconfigurations."},
			},
		}},
		"service-troubleshoot": {{
			Steps: []dynamic.Step{
				kialiCall("logs", "get_logs", "{'namespace': args.namespace, 'name': has(args.workload) && args.workload != '' ? args.workload : args.service}", true, ""),
				kialiCall("istio", "manage_istio_config_read", "{'namespace': args.namespace, 'action': 'list'}", true, ""),
			},
			Messages: []dynamic.PromptMessage{
				{Role: "user", Text: "# Service Troubleshooting Guide\n\n## Target: ${args.service} in namespace ${args.namespace}\n\nUse this guide to diagnose issues with the service. The relevant data has been collected below.\n\n---\n\n" +
					"## Step 1: Workload Logs\n\nReview the workload logs for error messages, stack traces, or unusual patterns:\n\n" + kialiData("logs") + "\n\n---\n\n" +
					"## Step 2: Istio Configuration\n\nReview the Istio configuration (VirtualServices, DestinationRules, etc.) for routing issues, fault injection, or misconfiguration:\n\n" + kialiData("istio") + "\n\n---\n\n" +
					"## Analysis Instructions\n\nBased on the data above, provide:\n\n" +
					"1. **Error Summary**: What errors or issues are visible in the logs?\n" +
					"2. **Istio Impact**: Are there any Istio configurations (fault injection, traffic routing, retries) that could cause or contribute to the errors?\n" +
					"3. **Root Cause**: What is the most likely root cause?\n" +
					"4. **Recommendations**: What actions should be taken to resolve the issue?\n"},
				{Role: "assistant", Text: "I'll analyze the collected data to diagnose the service issues systematically."},
			},
		}},
		"trace-analysis": {{
			Steps: []dynamic.Step{
				kialiCall("traces", "list_traces", "{'namespace': args.namespace, 'serviceName': args.service, 'limit': 20}", true, ""),
				kialiCall("errorTraces", "list_traces", "{'namespace': args.namespace, 'serviceName': args.service, 'errorOnly': true, 'limit': 10}", true, ""),
			},
			Messages: []dynamic.PromptMessage{
				{Role: "user", Text: "# Trace and Latency Investigation\n\n## Target: ${args.service} in namespace ${args.namespace}\n\nAnalyze distributed traces to find latency bottlenecks, error patterns, and performance issues.\n\n---\n\n" +
					"## Recent Traces\n\n" + kialiData("traces") + "\n\n---\n\n## Error Traces\n\n" + kialiData("errorTraces") + "\n\n---\n\n" +
					"## Analysis Instructions\n\nBased on the trace data above, provide:\n\n" +
					"1. **Latency Overview**: What is the typical request duration? Are there outliers with significantly higher latency?\n" +
					"2. **Slowest Services**: Which downstream services or spans contribute the most latency?\n" +
					"3. **Error Patterns**: Are there traces with errors? What services or operations are failing?\n" +
					"4. **Bottlenecks**: Identify any services that appear as common slow points across multiple traces.\n" +
					"5. **Recommendations**: Suggest optimizations such as caching, connection pooling, retry tuning, or timeout adjustments.\n\n" +
					"If a specific trace looks problematic, mention its trace ID so the user can drill into details using the get_trace_details tool.\n"},
				{Role: "assistant", Text: "I'll analyze the trace data to identify latency issues and error patterns."},
			},
		}},
		"traffic-topology": {{
			Steps: []dynamic.Step{
				nsListStep(nsListJSON),
				{Name: "resolved", Expression: resolvedNS},
				{Name: "present", Fail: "steps.resolved == '' ? 'no mesh namespaces found' : ''"},
				kialiCall("graph", "get_mesh_traffic_graph", "{'namespaces': steps.resolved}", false, ""),
			},
			Messages: []dynamic.PromptMessage{
				{Role: "user", Text: "# Traffic Topology Analysis\n\n## Scope\n\nAnalyze traffic topology for namespaces: ${steps.resolved}.\n\n## Collected Data\n\n### Traffic Graph\n\n${steps.graph}\n\n" +
					"## Instructions\n\nBased on the traffic graph data above, provide an analysis covering:\n\n" +
					"1. **Service Dependencies**: Map out which services communicate with each other and the direction of traffic flow.\n" +
					"2. **Traffic Patterns**: Identify high-traffic paths, bottlenecks, or unusual communication patterns.\n" +
					"3. **Health Overview**: Highlight any services or edges showing errors or degraded health.\n" +
					"4. **Observations**: Note any unexpected dependencies, circular calls, or services that appear isolated.\n"},
				{Role: "assistant", Text: "I'll analyze the traffic topology and identify service dependencies and communication patterns."},
			},
		}},
		"mesh-topology": {{
			Steps: []dynamic.Step{
				kialiCall("status", "get_mesh_status", "{}", true, ""),
				nsListStep(""),
				{Name: "resolved", Expression: "steps.nsList.namespaces.map(n, n.name).filter(n, n != '').join(',')"},
				{Name: "present", Fail: "steps.resolved == '' ? 'no mesh namespaces found' : ''"},
				kialiCall("graph", "get_mesh_traffic_graph", "{'namespaces': steps.resolved}", true, ""),
			},
			Messages: []dynamic.PromptMessage{{Role: "user", Text: "# Mesh Topology Overview\n\n## Mesh Status\n\n" + kialiData("status") + "\n\n## Traffic Graph\n\n" + kialiData("graph") + "\n\n" +
				"## Instructions\n\nSummarize the mesh topology covering control plane components, cluster connectivity, and data plane overview.\n"}},
		}},
		"list-istio-config": {{
			Steps:    []dynamic.Step{kialiCall("data", "manage_istio_config_read", "merge({'action': 'list'}, has(args.namespace) && args.namespace != '' ? {'namespace': args.namespace} : {})", false, "")},
			Messages: []dynamic.PromptMessage{{Role: "user", Text: "# List Istio Configuration\n\n## Scope\n\n" + scope + "\n\n## Data\n\n${steps.data}\n\n## Instructions\n\nSummarize the Istio configuration listed above. Highlight any that need attention.\n"}},
		}},
		"mesh-list-applications": listPrompt("app"),
		"mesh-list-namespaces":   listPrompt("namespace"),
		"mesh-list-services":     listPrompt("service"),
		"mesh-list-workloads":    listPrompt("workload"),
	}
}

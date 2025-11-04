package core

import (
	"fmt"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

const (
	// Health check configuration constants
	defaultRestartThreshold = 5
	eventLookbackMinutes    = 30
	maxWarningEvents        = 20
)

// isVerboseEnabled checks if the verbose flag is enabled.
// It accepts "true", "1", "yes", or "y" (case-insensitive) as truthy values.
func isVerboseEnabled(value string) bool {
	switch strings.ToLower(value) {
	case "true", "1", "yes", "y":
		return true
	default:
		return false
	}
}

// initHealthCheckPrompts creates prompts for cluster health diagnostics.
// These prompts guide LLMs to systematically check cluster components using existing tools.
func initHealthCheckPrompts() []api.ServerPrompt {
	return []api.ServerPrompt{
		{
			Name:        "cluster_health_check",
			Description: "Guide for performing comprehensive health check on Kubernetes/OpenShift clusters. Provides step-by-step instructions for examining cluster operators, nodes, pods, workloads, storage, and events to identify issues affecting cluster stability.",
			Arguments: []api.PromptArgument{
				{
					Name:        "verbose",
					Description: "Whether to include detailed diagnostics and resource-level information",
					Required:    false,
				},
				{
					Name:        "namespace",
					Description: "Limit health check to specific namespace (optional, defaults to all namespaces)",
					Required:    false,
				},
			},
			GetMessages: func(arguments map[string]string) []api.PromptMessage {
				verbose := isVerboseEnabled(arguments["verbose"])
				namespace := arguments["namespace"]

				return buildHealthCheckPromptMessages(verbose, namespace)
			},
		},
	}
}

// buildHealthCheckPromptMessages constructs the prompt messages for cluster health checks.
// It adapts the instructions based on verbose mode and namespace filtering.
func buildHealthCheckPromptMessages(verbose bool, namespace string) []api.PromptMessage {
	scopeMsg := "across all namespaces"
	podListInstruction := "- Use pods_list to get all pods"

	if namespace != "" {
		scopeMsg = fmt.Sprintf("in namespace '%s'", namespace)
		podListInstruction = fmt.Sprintf("- Use pods_list_in_namespace with namespace '%s'", namespace)
	}

	verboseMsg := ""
	if verbose {
		verboseMsg = "\n\nFor verbose mode, include additional details such as:\n" +
			"- Specific error messages from conditions\n" +
			"- Resource-level details (CPU/memory pressure types)\n" +
			"- Individual pod and deployment names\n" +
			"- Event messages and timestamps"
	}

	// Construct the event display range dynamically using maxWarningEvents
	eventDisplayRange := fmt.Sprintf("10-%d", maxWarningEvents)

	userMessage := fmt.Sprintf(`Please perform a comprehensive health check on the Kubernetes cluster %s.

Follow these steps systematically:

## 1. Check Cluster-Level Components

### For OpenShift Clusters:
- Use resources_list with apiVersion=config.openshift.io/v1 and kind=ClusterOperator to check cluster operator health
- Look for operators with:
  * Degraded=True (CRITICAL)
  * Available=False (CRITICAL)
  * Progressing=True (WARNING)

### For All Kubernetes Clusters:
- Verify if this is an OpenShift cluster by checking for OpenShift-specific resources
- Note the cluster type in your report

## 2. Check Node Health
- Use resources_list with apiVersion=v1 and kind=Node to examine all nodes
- Check each node for:
  * Ready condition != True (CRITICAL)
  * Unschedulable spec field = true (WARNING)
  * MemoryPressure, DiskPressure, or PIDPressure conditions = True (WARNING)
- Count total nodes and categorize issues

## 3. Check Pod Health
%s
- Identify problematic pods:
  * Phase = Failed or Pending (CRITICAL)
  * Container state waiting with reason:
    - CrashLoopBackOff (CRITICAL)
    - ImagePullBackOff or ErrImagePull (CRITICAL)
  * RestartCount > %d (WARNING - configurable threshold)
- Group issues by type and count occurrences

## 4. Check Workload Controllers
- Use resources_list for each workload type:
  * apiVersion=apps/v1, kind=Deployment
  * apiVersion=apps/v1, kind=StatefulSet
  * apiVersion=apps/v1, kind=DaemonSet
- For each controller, compare:
  * spec.replicas vs status.readyReplicas (Deployment/StatefulSet)
  * status.desiredNumberScheduled vs status.numberReady (DaemonSet)
  * Report mismatches as WARNINGs

## 5. Check Storage
- Use resources_list with apiVersion=v1 and kind=PersistentVolumeClaim
- Identify PVCs not in Bound phase (WARNING)
- Note namespace and PVC name for each issue

## 6. Check Recent Events (Optional)
- Use events_list to get cluster events
- Filter for:
  * Type = Warning
  * Timestamp within last %d minutes
- Limit to %s most recent warnings
- Include event message and involved object%s

## Output Format

Structure your health check report as follows:

`+"```"+`
================================================
Cluster Health Check Report
================================================
Cluster Type: [Kubernetes/OpenShift]
Cluster Version: [if determinable]
Check Time: [current timestamp]
Scope: [all namespaces / specific namespace]

### Cluster Operators (OpenShift only)
[Status with counts and specific issues]

### Node Health
[Status with counts: total, not ready, unschedulable, under pressure]

### Pod Health
[Status with counts: total, failed, crash looping, image pull errors, high restarts]

### Workload Controllers
[Status for Deployments, StatefulSets, DaemonSets]

### Storage
[PVC status: total, bound, pending/other]

### Recent Events
[Warning events from last %d minutes]

================================================
Summary
================================================
Critical Issues: [count]
Warnings: [count]

[Overall assessment: healthy / has warnings / has critical issues]
`+"```"+`

## Health Status Definitions

- **CRITICAL**: Issues requiring immediate attention (e.g., pods failing, nodes not ready, degraded operators)
- **WARNING**: Issues that should be monitored (e.g., high restarts, progressing operators, resource pressure)
- **HEALTHY**: No issues detected

## Important Notes

- Use the existing tools (resources_list, pods_list, events_list, etc.)
- Be efficient: don't call the same tool multiple times unnecessarily
- If a resource type doesn't exist (e.g., ClusterOperator on vanilla K8s), skip it gracefully
- Provide clear, actionable insights in your summary
- Use emojis for visual clarity: ✅ (healthy), ⚠️ (warning), ❌ (critical)

### Common apiVersion Values

When using resources_list, specify the correct apiVersion for each resource type:
- Core resources: apiVersion=v1 (Pod, Service, Node, PersistentVolumeClaim, ConfigMap, Secret, Namespace)
- Apps: apiVersion=apps/v1 (Deployment, StatefulSet, DaemonSet, ReplicaSet)
- Batch: apiVersion=batch/v1 (Job, CronJob)
- RBAC: apiVersion=rbac.authorization.k8s.io/v1 (Role, RoleBinding, ClusterRole, ClusterRoleBinding)
- Networking: apiVersion=networking.k8s.io/v1 (Ingress, NetworkPolicy)
- OpenShift Config: apiVersion=config.openshift.io/v1 (ClusterOperator, ClusterVersion)
- OpenShift Routes: apiVersion=route.openshift.io/v1 (Route)`, scopeMsg, podListInstruction, defaultRestartThreshold, eventLookbackMinutes, eventDisplayRange, verboseMsg, eventLookbackMinutes)

	assistantMessage := `I'll perform a comprehensive cluster health check following the systematic approach outlined. Let me start by gathering information about the cluster components.`

	return []api.PromptMessage{
		{
			Role:    "user",
			Content: userMessage,
		},
		{
			Role:    "assistant",
			Content: assistantMessage,
		},
	}
}

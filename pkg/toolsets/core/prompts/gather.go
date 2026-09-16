package prompts

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
)

// eventTaskName returns a task name used for event gather and analysis
// tasks scoped to a single namespace. The "events/" prefix prevents
// collisions with resource-type task names (e.g. "pods", "deployments").
func eventTaskName(namespace string) string {
	return eventTaskPrefix + namespace
}

// gatherNodeTask returns a Task that lists all cluster nodes.
func gatherNodeTask(params api.PromptHandlerParams) *tasks.Task {
	return &tasks.Task{
		Name: taskNameNodes,
		Run: func(ctx context.Context) (any, error) {
			return params.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		},
	}
}

// gatherNamespaceListTask returns a Task that lists all namespaces.
func gatherNamespaceListTask(params api.PromptHandlerParams) *tasks.Task {
	return &tasks.Task{
		Name: taskNameNamespaceList,
		Run: func(ctx context.Context) (any, error) {
			return params.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		},
	}
}

// gatherNamespaceGetTask returns a Task that gets a single namespace by name.
func gatherNamespaceGetTask(params api.PromptHandlerParams, namespace string) *tasks.Task {
	return &tasks.Task{
		Name: taskNameNamespaceCheck,
		Run: func(ctx context.Context) (any, error) {
			return params.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		},
	}
}

// gatherPodTask returns a Task that lists pods in the given namespace (empty = all).
func gatherPodTask(params api.PromptHandlerParams, namespace string) *tasks.Task {
	return &tasks.Task{
		Name: taskNamePods,
		Run: func(ctx context.Context) (any, error) {
			return params.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		},
	}
}

// gatherDeploymentTask returns a Task that lists deployments in the given namespace (empty = all).
func gatherDeploymentTask(params api.PromptHandlerParams, namespace string) *tasks.Task {
	return &tasks.Task{
		Name: taskNameDeployments,
		Run: func(ctx context.Context) (any, error) {
			return params.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		},
	}
}

// gatherStatefulSetTask returns a Task that lists statefulsets in the given namespace (empty = all).
func gatherStatefulSetTask(params api.PromptHandlerParams, namespace string) *tasks.Task {
	return &tasks.Task{
		Name: taskNameStatefulSets,
		Run: func(ctx context.Context) (any, error) {
			return params.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
		},
	}
}

// gatherDaemonSetTask returns a Task that lists daemonsets in the given namespace (empty = all).
func gatherDaemonSetTask(params api.PromptHandlerParams, namespace string) *tasks.Task {
	return &tasks.Task{
		Name: taskNameDaemonSets,
		Run: func(ctx context.Context) (any, error) {
			return params.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
		},
	}
}

// gatherPVCTask returns a Task that lists PVCs in the given namespace (empty = all).
func gatherPVCTask(params api.PromptHandlerParams, namespace string) *tasks.Task {
	return &tasks.Task{
		Name: taskNamePVCs,
		Run: func(ctx context.Context) (any, error) {
			return params.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
		},
	}
}

// gatherEventTask returns a Task that lists non-normal events in the given namespace.
func gatherEventTask(params api.PromptHandlerParams, namespace string) *tasks.Task {
	return &tasks.Task{
		Name: eventTaskName(namespace),
		Run: func(ctx context.Context) (any, error) {
			return params.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
				FieldSelector: "type!=Normal",
				Limit:         100,
			})
		},
	}
}

// gatherClusterOperatorTask returns a Task that lists OpenShift ClusterOperator resources.
func gatherClusterOperatorTask(params api.PromptHandlerParams) *tasks.Task {
	return &tasks.Task{
		Name: taskNameClusterOperators,
		Run: func(ctx context.Context) (any, error) {
			gvk := &schema.GroupVersionKind{
				Group:   "config.openshift.io",
				Version: "v1",
				Kind:    "ClusterOperator",
			}
			return kubernetes.NewCore(params).ResourcesList(ctx, gvk, "", api.ListOptions{})
		},
	}
}

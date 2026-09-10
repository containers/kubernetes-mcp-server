package prompts

// Task name constants for the health-check gather/analyze pipeline.
// Centralizing the names prevents typos and makes refactoring safer.
const (
	taskNameNodes            = "nodes"
	taskNameNamespaceList    = "namespace-list"
	taskNameNamespaceCheck   = "namespace-check"
	taskNamePods             = "pods"
	taskNameDeployments      = "deployments"
	taskNameStatefulSets     = "statefulsets"
	taskNameDaemonSets       = "daemonsets"
	taskNamePVCs             = "pvcs"
	taskNameClusterOperators = "cluster-operators"
	taskNameEventsGather     = "events-gather"

	// eventTaskPrefix is prepended to event task names to avoid collisions
	// with workload task names (e.g. a namespace literally named "pods").
	eventTaskPrefix = "events/"
)

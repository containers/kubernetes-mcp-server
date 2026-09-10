package prompts

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
)

type RenderSuite struct {
	suite.Suite
}

func (s *RenderSuite) TestRenderNodeHealth() {
	s.Run("nil report returns no nodes message", func() {
		out := renderNodeHealth(nil)
		s.Contains(out, "No nodes found")
	})

	s.Run("empty report returns no nodes message", func() {
		out := renderNodeHealth(&nodeHealthReport{})
		s.Contains(out, "No nodes found")
	})

	s.Run("all healthy nodes renders totals and healthy note", func() {
		out := renderNodeHealth(&nodeHealthReport{
			totalNodes: 3,
			readyNodes: 3,
		})
		s.Contains(out, "Total:** 3")
		s.Contains(out, "Ready:** 3")
		s.Contains(out, "All nodes are healthy")
	})

	s.Run("pressure and issues are rendered", func() {
		out := renderNodeHealth(&nodeHealthReport{
			totalNodes:         2,
			readyNodes:         0,
			nodesUnderPressure: []nodeIssue{{name: "n1", status: "Ready", issues: []string{"MemoryPressure: low"}}},
			nodesWithIssues:    []nodeIssue{{name: "n2", status: "NotReady", issues: []string{"Not ready: kubelet down"}}},
		})
		s.Contains(out, "n1")
		s.Contains(out, "n2")
		s.Contains(out, "MemoryPressure")
		s.Contains(out, "kubelet down")
	})
}

func (s *RenderSuite) TestRenderNamespaceHealthSummary() {
	s.Run("no results renders zeros and no issues", func() {
		out := renderNamespaceHealthSummary(nil, false, 0)
		s.Contains(out, "Total Pods:** 0")
		s.Contains(out, "No issues detected")
	})

	s.Run("task errors are surfaced", func() {
		out := renderNamespaceHealthSummary([]tasks.TaskResult{
			{Name: taskNamePods, Err: errors.New("boom")},
		}, false, 0)
		s.Contains(out, "Failed to analyze pods")
		s.Contains(out, "No namespace health summary available")
	})

	s.Run("compact cluster-wide summary aggregates by namespace", func() {
		results := []tasks.TaskResult{
			{
				Name: taskNamePods,
				Output: &podHealthReport{
					totalPods: 10,
					issues: []podIssue{{
						namespace: "ns1",
						issues:    map[string][]string{"Pod in Pending phase": {"p1", "p2"}},
					}},
				},
			},
			{
				Name:   taskNameDeployments,
				Output: &workloadHealthReport{workloadType: taskNameDeployments, items: []workloadIssue{{namespace: "ns1", unhealthy: map[string]int32{"d1": 2}}}},
			},
			{
				Name:   taskNamePVCs,
				Output: &pvcHealthReport{items: []pvcIssue{{namespace: "ns2", pending: []string{"pvc1"}}}},
			},
		}
		out := renderNamespaceHealthSummary(results, false, 0)
		s.Contains(out, "Total Pods:** 10")
		s.Contains(out, "Pods with Issues:** 2")
		s.Contains(out, "Unhealthy Workloads:** 1")
		s.Contains(out, "PVCs Pending:** 1")
		s.Contains(out, "ns1")
		s.Contains(out, "ns2")
	})

	s.Run("detailed namespace view includes pod and workload names", func() {
		results := []tasks.TaskResult{
			{
				Name: taskNamePods,
				Output: &podHealthReport{
					issues: []podIssue{{
						namespace: "ns1",
						issues:    map[string][]string{"Pod in Pending phase": {"p1"}},
					}},
				},
			},
			{
				Name:   taskNameDeployments,
				Output: &workloadHealthReport{workloadType: taskNameDeployments, items: []workloadIssue{{namespace: "ns1", unhealthy: map[string]int32{"d1": 1}}}},
			},
		}
		out := renderNamespaceHealthSummary(results, true, 0)
		s.Contains(out, "p1")
		s.Contains(out, "d1")
		s.Contains(out, "replicas missing")
	})

	s.Run("namespaces are sorted by issue count descending", func() {
		results := []tasks.TaskResult{
			{
				Name: taskNamePods,
				Output: &podHealthReport{
					issues: []podIssue{
						{namespace: "aa", issues: map[string][]string{"x": {"a"}}},
						{namespace: "bb", issues: map[string][]string{"x": {"b", "c"}}},
					},
				},
			},
		}
		out := renderNamespaceHealthSummary(results, false, 0)
		bbIdx := strings.Index(out, "bb")
		aaIdx := strings.Index(out, "aa")
		s.True(bbIdx < aaIdx, "higher-issue namespace should appear first")
	})

	s.Run("maxLines truncates and shows note", func() {
		results := []tasks.TaskResult{
			{
				Name: taskNamePods,
				Output: &podHealthReport{
					issues: []podIssue{
						{namespace: "ns1", issues: map[string][]string{"x": {"a"}}},
						{namespace: "ns2", issues: map[string][]string{"x": {"b"}}},
						{namespace: "ns3", issues: map[string][]string{"x": {"c"}}},
					},
				},
			},
		}
		out := renderNamespaceHealthSummary(results, false, 2)
		s.Contains(out, "Showing top 2 namespaces")
		s.NotContains(out, "ns3")
	})

	s.Run("deterministic workload order regardless of insertion", func() {
		results := []tasks.TaskResult{
			{
				Name:   taskNameDaemonSets,
				Output: &workloadHealthReport{workloadType: taskNameDaemonSets, items: []workloadIssue{{namespace: "ns1", unhealthy: map[string]int32{"ds1": 1}}}},
			},
			{
				Name:   taskNameStatefulSets,
				Output: &workloadHealthReport{workloadType: taskNameStatefulSets, items: []workloadIssue{{namespace: "ns1", unhealthy: map[string]int32{"sts1": 1}}}},
			},
			{
				Name:   taskNameDeployments,
				Output: &workloadHealthReport{workloadType: taskNameDeployments, items: []workloadIssue{{namespace: "ns1", unhealthy: map[string]int32{"d1": 1}}}},
			},
		}
		out := renderNamespaceHealthSummary(results, false, 0)
		dIdx := strings.Index(out, "deployments:")
		stsIdx := strings.Index(out, "statefulsets:")
		dsIdx := strings.Index(out, "daemonsets:")
		s.True(dIdx < stsIdx && stsIdx < dsIdx, "workloads should render in deployments, statefulsets, daemonsets order")
	})
}

func (s *RenderSuite) TestRenderEvents() {
	s.Run("no results renders zero counts", func() {
		out := renderEvents(nil, 0)
		s.Contains(out, "Warnings:** 0")
		s.Contains(out, "No recent warning events")
	})

	s.Run("warning totals are aggregated", func() {
		results := []tasks.TaskResult{
			{
				Name: eventTaskPrefix + "ns1",
				Output: &eventReport{
					warnings: 2,
					objects: []objectEvents{{
						key: "ns1/Pod/foo",
						entries: []eventEntry{
							{reason: "Failed", count: 5, message: "bad"},
						},
					}},
				},
			},
		}
		out := renderEvents(results, 0)
		s.Contains(out, "Warnings:** 2")
		s.Contains(out, "ns1/Pod/foo")
		s.Contains(out, "Failed")
	})

	s.Run("truncation stops at object group boundary", func() {
		results := []tasks.TaskResult{
			{
				Name: eventTaskPrefix + "ns1",
				Output: &eventReport{
					warnings: 2,
					objects: []objectEvents{
						{key: "ns1/Pod/a", entries: []eventEntry{{reason: "Failed", count: 1, message: "m1"}}},
						{key: "ns1/Pod/b", entries: []eventEntry{{reason: "BackOff", count: 1, message: "m2"}}},
						{key: "ns1/Pod/c", entries: []eventEntry{{reason: "BackOff", count: 1, message: "m3"}}},
					},
				},
			},
		}
		out := renderEvents(results, 4) // header+1 per object -> fits 3 objects, but limit 4 means two objects (1+1 each + header?) Actually renderEvents maxLines counts data lines not header; each object group 1+len(entries)=2 lines. With maxLines=4, 2 objects use 4 lines, 3rd truncated.
		s.Contains(out, "ns1/Pod/a")
		s.Contains(out, "ns1/Pod/b")
		s.NotContains(out, "ns1/Pod/c")
		s.Contains(out, "Event list truncated")
	})

	s.Run("truncation to zero objects shows budget note not no events", func() {
		results := []tasks.TaskResult{
			{
				Name: eventTaskPrefix + "ns1",
				Output: &eventReport{
					warnings: 5,
					objects: []objectEvents{
						{key: "ns1/Pod/a", entries: []eventEntry{{reason: "Failed", count: 1, message: "m1"}, {reason: "BackOff", count: 1, message: "m2"}}},
					},
				},
			},
		}
		out := renderEvents(results, 1)
		s.Contains(out, "Warnings:** 5")
		s.Contains(out, "Event list truncated")
		s.Contains(out, "do not fit in the available event budget")
		s.NotContains(out, "No recent warning events")
	})

	s.Run("task errors are surfaced", func() {
		out := renderEvents([]tasks.TaskResult{
			{Name: eventTaskPrefix + "ns1", Err: errors.New("timeout")},
		}, 0)
		s.Contains(out, "Failed to analyze events for ns1")
		s.Contains(out, "No event details available")
	})
}

func (s *RenderSuite) TestRenderClusterOperators() {
	s.Run("nil report returns no operators message", func() {
		s.Equal("No cluster operators found", renderClusterOperators(nil))
	})

	s.Run("no issues renders healthy note", func() {
		out := renderClusterOperators(&clusterOperatorReport{})
		s.Contains(out, "Operators with Issues:** 0")
		s.Contains(out, "All cluster operators are healthy")
	})

	s.Run("operators with issues are rendered", func() {
		out := renderClusterOperators(&clusterOperatorReport{
			operatorsWithIssues: []operatorIssue{
				{name: "co1", available: "False", degraded: "True", issues: []string{"Not available: broken"}},
			},
		})
		s.Contains(out, "co1")
		s.Contains(out, "False")
		s.Contains(out, "broken")
	})
}

func (s *RenderSuite) TestFormatPromptHeader() {
	collectionTime := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	out := formatPromptHeader("Test Title", collectionTime)
	s.Contains(out, "# Test Title")
	s.Contains(out, "Collection Time:** 2026-06-14T12:00:00Z")
	s.Contains(out, "Elapsed Time:**")
}

func (s *RenderSuite) TestFormatPromptInstructions() {
	out := formatPromptInstructions("cluster")
	s.Contains(out, "## Your Task")
	s.Contains(out, "Analyze the following cluster diagnostic data")
	s.Contains(out, "Overall Health Status")
	s.Contains(out, "Recommendations")
}

func (s *RenderSuite) TestFormatPromptFooter() {
	out := formatPromptFooter("namespace")
	s.Contains(out, "namespace health assessment")
}

func (s *RenderSuite) TestFormatNamespaceLookupError() {
	s.Run("not found", func() {
		err := apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "missing")
		out := formatNamespaceLookupError("missing", err)
		s.Contains(out, "not found")
		s.Contains(out, "verify the namespace name")
	})

	s.Run("forbidden", func() {
		err := apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "secret", errors.New("no"))
		out := formatNamespaceLookupError("secret", err)
		s.Contains(out, "Access denied")
		s.Contains(out, "RBAC")
	})

	s.Run("unauthorized", func() {
		err := apierrors.NewUnauthorized("bad creds")
		out := formatNamespaceLookupError("ns", err)
		s.Contains(out, "Authentication failed")
		s.Contains(out, "credentials")
	})

	s.Run("generic error", func() {
		out := formatNamespaceLookupError("ns", errors.New("network down"))
		s.Contains(out, "Failed to verify namespace")
		s.Contains(out, "cluster may be unreachable")
	})
}

func (s *RenderSuite) TestFormatNamespaceLookupNoData() {
	s.Run("not found", func() {
		err := apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "missing")
		s.Contains(formatNamespaceLookupNoData(err), "namespace not found")
	})

	s.Run("other error", func() {
		s.Contains(formatNamespaceLookupNoData(errors.New("x")), "namespace lookup failed")
	})
}

func (s *RenderSuite) TestPartitionHealthResults() {
	results := []tasks.TaskResult{
		{Name: taskNamePods, Output: &podHealthReport{}},
		{Name: eventTaskPrefix + "ns1", Output: &eventReport{}},
		{Name: taskNameNodes, Output: &nodeHealthReport{}},
	}
	ns, events := partitionHealthResults(results)
	s.Len(ns, 1)
	s.Len(events, 1)
	s.Equal(taskNamePods, ns[0].Name)
	s.Equal(eventTaskPrefix+"ns1", events[0].Name)
}

func (s *RenderSuite) TestFormatClusterHealthPrompt() {
	collectionTime := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	results := []tasks.TaskResult{
		{Name: taskNameNodes, Output: &nodeHealthReport{totalNodes: 1, readyNodes: 1}},
	}

	s.Run("successful namespace list renders normally", func() {
		out := formatClusterHealthPrompt(collectionTime, "test-cluster", 5, results, false, false, nil)
		s.Contains(out, "# Cluster Health Check Diagnostic Data for test-cluster")
		s.Contains(out, "All namespaces for workload data")
		s.Contains(out, "system namespaces for events")
		s.Contains(out, "Total: 5")
		s.Contains(out, "## Nodes")
		s.NotContains(out, "WARNING")
	})

	s.Run("namespace list error surfaces warning", func() {
		out := formatClusterHealthPrompt(collectionTime, "test-cluster", 0, results, true, false, errors.New("forbidden"))
		s.Contains(out, "WARNING:** Failed to list namespaces: forbidden")
		s.Contains(out, "Event gathering was skipped")
		s.Contains(out, "## Nodes")
	})
}

func (s *RenderSuite) TestFormatNamespaceHealthPrompt() {
	collectionTime := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	results := []tasks.TaskResult{
		{Name: taskNamePods, Output: &podHealthReport{totalPods: 3}},
	}

	s.Run("valid namespace renders workload section", func() {
		out := formatNamespaceHealthPrompt(collectionTime, "test-cluster", "default", results, false, nil)
		s.Contains(out, "Namespace Health Check Diagnostic Data")
		s.Contains(out, "Cluster: test-cluster")
		s.Contains(out, "Namespace: default")
		s.Contains(out, "## Workload Health")
		s.Contains(out, "namespace health assessment")
	})

	s.Run("NotFound error renders warning and no data", func() {
		err := apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "missing")
		out := formatNamespaceHealthPrompt(collectionTime, "test-cluster", "missing", nil, false, err)
		s.Contains(out, "WARNING:** Namespace 'missing' not found")
		s.Contains(out, "No diagnostic data available")
		s.NotContains(out, "## Workload Health")
	})

	s.Run("Forbidden error renders warning but continues with workload data", func() {
		err := apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "missing", fmt.Errorf("access denied"))
		out := formatNamespaceHealthPrompt(collectionTime, "test-cluster", "restricted", results, false, err)
		s.Contains(out, "WARNING:** Access denied")
		s.Contains(out, "## Workload Health")
		s.NotContains(out, "No diagnostic data available")
	})
}

func TestRender(t *testing.T) {
	suite.Run(t, new(RenderSuite))
}

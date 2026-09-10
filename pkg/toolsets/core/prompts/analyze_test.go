package prompts

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AnalyzeSuite struct {
	suite.Suite
}

func (s *AnalyzeSuite) TestAnalyzeNodeHealth() {
	s.Run("nil input returns empty report", func() {
		report := analyzeNodeHealth(nil)
		s.NotNil(report)
		s.Equal(0, report.totalNodes)
		s.Equal(0, report.readyNodes)
		s.Empty(report.nodesUnderPressure)
		s.Empty(report.nodesWithIssues)
	})

	s.Run("ready node with no pressure is counted as ready", func() {
		report := analyzeNodeHealth(&v1.NodeList{
			Items: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ready-node"},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{Type: v1.NodeReady, Status: v1.ConditionTrue},
						},
					},
				},
			},
		})
		s.Equal(1, report.totalNodes)
		s.Equal(1, report.readyNodes)
		s.Empty(report.nodesUnderPressure)
		s.Empty(report.nodesWithIssues)
	})

	s.Run("ready node with pressure is categorized as under pressure", func() {
		report := analyzeNodeHealth(&v1.NodeList{
			Items: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pressure-node"},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{Type: v1.NodeReady, Status: v1.ConditionTrue},
							{Type: v1.NodeMemoryPressure, Status: v1.ConditionTrue, Message: "memory low"},
						},
					},
				},
			},
		})
		s.Equal(1, report.totalNodes)
		s.Equal(0, report.readyNodes)
		s.Len(report.nodesUnderPressure, 1)
		s.Equal("pressure-node", report.nodesUnderPressure[0].name)
		s.Equal("Ready", report.nodesUnderPressure[0].status)
	})

	s.Run("not ready node is categorized as having issues", func() {
		report := analyzeNodeHealth(&v1.NodeList{
			Items: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "not-ready-node"},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{Type: v1.NodeReady, Status: v1.ConditionFalse, Message: "kubelet not ready"},
						},
					},
				},
			},
		})
		s.Equal(1, report.totalNodes)
		s.Equal(0, report.readyNodes)
		s.Empty(report.nodesUnderPressure)
		s.Len(report.nodesWithIssues, 1)
		s.Equal("not-ready-node", report.nodesWithIssues[0].name)
	})

	s.Run("node with unknown ready condition is categorized as having issues", func() {
		report := analyzeNodeHealth(&v1.NodeList{
			Items: []v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "unknown-node"},
					Status:     v1.NodeStatus{Conditions: []v1.NodeCondition{}},
				},
			},
		})
		s.Equal(1, report.totalNodes)
		s.Equal(0, report.readyNodes)
		s.Len(report.nodesWithIssues, 1)
		s.Equal("Unknown", report.nodesWithIssues[0].status)
	})
}

func (s *AnalyzeSuite) TestAnalyzePodHealth() {
	s.Run("nil input returns empty report", func() {
		report := analyzePodHealth(nil)
		s.NotNil(report)
		s.Equal(0, report.totalPods)
		s.Empty(report.issues)
	})

	s.Run("healthy running pod produces no issues", func() {
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "default"},
					Status:     v1.PodStatus{Phase: v1.PodRunning},
				},
			},
		})
		s.Equal(1, report.totalPods)
		s.Empty(report.issues)
	})

	s.Run("waiting CrashLoopBackOff is reported", func() {
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "crash", Namespace: "default"},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name: "app",
								State: v1.ContainerState{
									Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
								},
							},
						},
					},
				},
			},
		})
		s.Equal(1, report.totalPods)
		s.Len(report.issues, 1)
		s.Contains(report.issues[0].issues, "Container waiting: CrashLoopBackOff")
	})

	s.Run("terminated with Error is reported", func() {
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "error", Namespace: "default"},
					Status: v1.PodStatus{
						Phase: v1.PodRunning,
						ContainerStatuses: []v1.ContainerStatus{
							{
								Name: "app",
								State: v1.ContainerState{
									Terminated: &v1.ContainerStateTerminated{Reason: "Error"},
								},
							},
						},
					},
				},
			},
		})
		s.Equal(1, report.totalPods)
		s.Len(report.issues, 1)
		s.Contains(report.issues[0].issues, "Container terminated: Error")
	})

	s.Run("pod in pending phase is reported", func() {
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default"},
					Status:     v1.PodStatus{Phase: v1.PodPending},
				},
			},
		})
		s.Equal(1, report.totalPods)
		s.Len(report.issues, 1)
		s.Contains(report.issues[0].issues, "Pod in Pending phase")
	})

	s.Run("pending pod scheduled with init containers stuck is reported as init-stuck", func() {
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "init-stuck", Namespace: "default"},
					Spec: v1.PodSpec{
						NodeName: "node1",
						InitContainers: []v1.Container{
							{Name: "init-1"},
							{Name: "init-2"},
							{Name: "init-3"},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodPending,
						InitContainerStatuses: []v1.ContainerStatus{
							{
								Name:  "init-1",
								Ready: true,
								State: v1.ContainerState{
									Terminated: &v1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0},
								},
							},
							{
								Name: "init-2",
								State: v1.ContainerState{
									Waiting: &v1.ContainerStateWaiting{Reason: "PodInitializing"},
								},
							},
							{
								Name: "init-3",
								State: v1.ContainerState{
									Waiting: &v1.ContainerStateWaiting{Reason: "PodInitializing"},
								},
							},
						},
					},
				},
			},
		})
		s.Equal(1, report.totalPods)
		s.Len(report.issues, 1)
		s.Contains(report.issues[0].issues, "Init containers not ready: 1/3")
	})

	s.Run("pending pod not scheduled falls back to generic Pending", func() {
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "unscheduled", Namespace: "default"},
					Spec: v1.PodSpec{
						InitContainers: []v1.Container{
							{Name: "init-1"},
						},
					},
					Status: v1.PodStatus{Phase: v1.PodPending},
				},
			},
		})
		s.Equal(1, report.totalPods)
		s.Len(report.issues, 1)
		s.Contains(report.issues[0].issues, "Pod in Pending phase")
	})

	s.Run("pending pod with init container error defers to container issue", func() {
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "init-error", Namespace: "default"},
					Spec: v1.PodSpec{
						NodeName: "node1",
						InitContainers: []v1.Container{
							{Name: "init-1"},
						},
					},
					Status: v1.PodStatus{
						Phase: v1.PodPending,
						InitContainerStatuses: []v1.ContainerStatus{
							{
								Name: "init-1",
								State: v1.ContainerState{
									Terminated: &v1.ContainerStateTerminated{Reason: "Error", ExitCode: 1},
								},
							},
						},
					},
				},
			},
		})
		s.Equal(1, report.totalPods)
		s.Len(report.issues, 1)
		s.Contains(report.issues[0].issues, "Init container terminated: Error")
		s.Contains(report.issues[0].issues, "Pod in Pending phase")
		for key := range report.issues[0].issues {
			s.False(strings.HasPrefix(key, "Init containers not ready"), "should not contain init-stuck message when init error exists")
		}
	})

	s.Run("high restart frequency is reported", func() {
		start := time.Now().Add(-30 * time.Minute)
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "restarts", Namespace: "default"},
					Status: v1.PodStatus{
						Phase:     v1.PodRunning,
						StartTime: &metav1.Time{Time: start},
						ContainerStatuses: []v1.ContainerStatus{
							{RestartCount: 3},
						},
					},
				},
			},
		})
		s.Equal(1, report.totalPods)
		s.Len(report.issues, 1)
		s.Contains(report.issues[0].issues, "High restart frequency")
	})

	s.Run("issues are aggregated per namespace", func() {
		report := analyzePodHealth(&v1.PodList{
			Items: []v1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns1"},
					Status:     v1.PodStatus{Phase: v1.PodPending},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns2"},
					Status:     v1.PodStatus{Phase: v1.PodFailed},
				},
			},
		})
		s.Equal(2, report.totalPods)
		s.Len(report.issues, 2)
	})
}

func (s *AnalyzeSuite) TestAnalyzeDeploymentHealth() {
	s.Run("no unavailable replicas returns empty report", func() {
		report := analyzeDeploymentHealth(&appsv1.DeploymentList{
			Items: []appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ready", Namespace: "default"},
					Status:     appsv1.DeploymentStatus{UnavailableReplicas: 0},
				},
			},
		})
		s.Empty(report.items)
	})

	s.Run("unavailable replicas are reported", func() {
		report := analyzeDeploymentHealth(&appsv1.DeploymentList{
			Items: []appsv1.Deployment{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "partial", Namespace: "default"},
					Status:     appsv1.DeploymentStatus{UnavailableReplicas: 2},
				},
			},
		})
		s.Len(report.items, 1)
		s.Equal(int32(2), report.items[0].unhealthy["partial"])
	})
}

func (s *AnalyzeSuite) TestAnalyzeStatefulSetHealth() {
	s.Run("nil replicas defaults to one", func() {
		report := analyzeStatefulSetHealth(&appsv1.StatefulSetList{
			Items: []appsv1.StatefulSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: "default"},
					Status:     appsv1.StatefulSetStatus{ReadyReplicas: 0},
				},
			},
		})
		s.Len(report.items, 1)
		s.Equal(int32(1), report.items[0].unhealthy["sts"])
	})

	s.Run("ready replicas equal spec replicas returns empty report", func() {
		replicas := int32(3)
		report := analyzeStatefulSetHealth(&appsv1.StatefulSetList{
			Items: []appsv1.StatefulSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: "default"},
					Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
					Status:     appsv1.StatefulSetStatus{ReadyReplicas: 3},
				},
			},
		})
		s.Empty(report.items)
	})
}

func (s *AnalyzeSuite) TestAnalyzeDaemonSetHealth() {
	s.Run("zero unavailable returns empty report", func() {
		report := analyzeDaemonSetHealth(&appsv1.DaemonSetList{
			Items: []appsv1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "default"},
					Status:     appsv1.DaemonSetStatus{NumberUnavailable: 0},
				},
			},
		})
		s.Empty(report.items)
	})

	s.Run("unavailable daemons are reported", func() {
		report := analyzeDaemonSetHealth(&appsv1.DaemonSetList{
			Items: []appsv1.DaemonSet{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "default"},
					Status:     appsv1.DaemonSetStatus{NumberUnavailable: 1},
				},
			},
		})
		s.Len(report.items, 1)
		s.Equal(int32(1), report.items[0].unhealthy["ds"])
	})
}

func (s *AnalyzeSuite) TestAnalyzePVCHealth() {
	s.Run("healthy PVC returns empty report", func() {
		report := analyzePVCHealth(&v1.PersistentVolumeClaimList{
			Items: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "bound", Namespace: "default"},
					Status:     v1.PersistentVolumeClaimStatus{Phase: v1.ClaimBound},
				},
			},
		})
		s.Empty(report.items)
	})

	s.Run("pending and lost PVCs are reported", func() {
		report := analyzePVCHealth(&v1.PersistentVolumeClaimList{
			Items: []v1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "pending", Namespace: "default"},
					Status:     v1.PersistentVolumeClaimStatus{Phase: v1.ClaimPending},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "lost", Namespace: "default"},
					Status:     v1.PersistentVolumeClaimStatus{Phase: v1.ClaimLost},
				},
			},
		})
		s.Len(report.items, 1)
		s.Equal([]string{"pending"}, report.items[0].pending)
		s.Equal([]string{"lost"}, report.items[0].lost)
	})
}

func (s *AnalyzeSuite) TestAnalyzeEvents() {
	baseTime := time.Now()

	s.Run("nil input returns empty report", func() {
		report := analyzeEvents("default", nil, baseTime.Add(-time.Hour))
		s.NotNil(report)
		s.Equal(0, report.warnings)
		s.Empty(report.objects)
	})

	s.Run("warning event is counted and rendered", func() {
		report := analyzeEvents("default", &v1.EventList{
			Items: []v1.Event{
				{
					Type:    v1.EventTypeWarning,
					Reason:  "FailedScheduling",
					Message: "0/3 nodes available",
					InvolvedObject: v1.ObjectReference{
						Kind: "Pod",
						Name: "foo",
					},
					LastTimestamp: metav1.Time{Time: baseTime},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Equal(1, report.warnings)
		s.Len(report.objects, 1)
	})

	s.Run("old events outside time window are ignored", func() {
		report := analyzeEvents("default", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "BackOff",
					Message:        "backoff",
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "old"},
					LastTimestamp:  metav1.Time{Time: baseTime.Add(-2 * time.Hour)},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Equal(0, report.warnings)
	})

	s.Run("events for same object in different namespaces do not collide", func() {
		report := analyzeEvents("ns1", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "Failed",
					Message:        "msg1",
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "foo"},
					LastTimestamp:  metav1.Time{Time: baseTime},
				},
			},
		}, baseTime.Add(-time.Hour))
		report2 := analyzeEvents("ns2", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "Failed",
					Message:        "msg2",
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "foo"},
					LastTimestamp:  metav1.Time{Time: baseTime},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Len(report.objects, 1)
		s.Len(report2.objects, 1)
		s.NotEqual(report.objects[0].key, report2.objects[0].key)
	})

	s.Run("series event uses Series.LastObservedTime for time window", func() {
		// Event with stale LastTimestamp outside the window but Series.LastObservedTime inside
		report := analyzeEvents("default", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "BackOff",
					Message:        "back-off restarting",
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "series-pod"},
					LastTimestamp:  metav1.Time{Time: baseTime.Add(-2 * time.Hour)},
					Series: &v1.EventSeries{
						Count:            5,
						LastObservedTime: metav1.MicroTime{Time: baseTime.Add(-10 * time.Minute)},
					},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Equal(1, report.warnings, "event should be included based on Series.LastObservedTime")
		s.Len(report.objects, 1)
		s.Equal(int32(5), report.objects[0].entries[0].count, "count should come from Series.Count")
	})

	s.Run("series event outside time window is excluded", func() {
		report := analyzeEvents("default", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "BackOff",
					Message:        "back-off restarting",
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "old-series"},
					LastTimestamp:  metav1.Time{Time: baseTime.Add(-3 * time.Hour)},
					Series: &v1.EventSeries{
						Count:            10,
						LastObservedTime: metav1.MicroTime{Time: baseTime.Add(-2 * time.Hour)},
					},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Equal(0, report.warnings, "series event outside window should be excluded")
	})

	s.Run("series count is preferred over top-level count", func() {
		report := analyzeEvents("default", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "FailedMount",
					Message:        "mount failed",
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "mount-pod"},
					Count:          1,
					LastTimestamp:  metav1.Time{Time: baseTime},
					Series: &v1.EventSeries{
						Count:            42,
						LastObservedTime: metav1.MicroTime{Time: baseTime},
					},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Len(report.objects, 1)
		s.Equal(int32(42), report.objects[0].entries[0].count)
	})

	s.Run("event without series uses top-level count", func() {
		report := analyzeEvents("default", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "FailedMount",
					Message:        "mount failed",
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "mount-pod"},
					Count:          3,
					LastTimestamp:  metav1.Time{Time: baseTime},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Len(report.objects, 1)
		s.Equal(int32(3), report.objects[0].entries[0].count)
	})

	s.Run("long multi-byte messages are truncated at rune boundary", func() {
		longMessage := "日本語" + string(make([]byte, 300))
		report := analyzeEvents("default", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "Failed",
					Message:        longMessage,
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "foo"},
					LastTimestamp:  metav1.Time{Time: baseTime},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Len(report.objects, 1)
		s.Len(report.objects[0].entries, 1)
		entry := report.objects[0].entries[0]
		s.LessOrEqual(len([]rune(entry.message)), 154) // 150 runes + "..."
		s.Contains(entry.message, "...")
	})

	s.Run("newlines are collapsed before length truncation", func() {
		// 80 single-byte lines of 5 chars each plus "\n" become 80*6-1 = 479 runes.
		message := strings.Repeat("line\n", 80)
		message = strings.TrimSuffix(message, "\n")
		report := analyzeEvents("default", &v1.EventList{
			Items: []v1.Event{
				{
					Type:           v1.EventTypeWarning,
					Reason:         "Failed",
					Message:        message,
					InvolvedObject: v1.ObjectReference{Kind: "Pod", Name: "foo"},
					LastTimestamp:  metav1.Time{Time: baseTime},
				},
			},
		}, baseTime.Add(-time.Hour))
		s.Len(report.objects, 1)
		s.Len(report.objects[0].entries, 1)
		entry := report.objects[0].entries[0]
		s.LessOrEqual(len([]rune(entry.message)), 154)
		s.NotContains(entry.message, "\n")
		s.Contains(entry.message, "...")
	})
}

func (s *AnalyzeSuite) TestAnalyzeClusterOperatorHealth() {
	s.Run("nil input returns empty report", func() {
		report := analyzeClusterOperatorHealth(nil)
		s.NotNil(report)
		s.Empty(report.operatorsWithIssues)
	})

	s.Run("healthy operator is not reported", func() {
		report := analyzeClusterOperatorHealth(unstructuredListFixture(
			map[string]any{
				"metadata": map[string]any{"name": "operator1"},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Available", "status": "True"},
						map[string]any{"type": "Degraded", "status": "False"},
					},
				},
			},
		))
		s.Empty(report.operatorsWithIssues)
	})

	s.Run("unavailable operator is reported", func() {
		report := analyzeClusterOperatorHealth(unstructuredListFixture(
			map[string]any{
				"metadata": map[string]any{"name": "operator1"},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Available", "status": "False", "message": "not ready"},
					},
				},
			},
		))
		s.Len(report.operatorsWithIssues, 1)
		s.Equal("operator1", report.operatorsWithIssues[0].name)
	})

	s.Run("degraded operator is reported", func() {
		report := analyzeClusterOperatorHealth(unstructuredListFixture(
			map[string]any{
				"metadata": map[string]any{"name": "operator1"},
				"status": map[string]any{
					"conditions": []any{
						map[string]any{"type": "Degraded", "status": "True", "message": "degraded"},
					},
				},
			},
		))
		s.Len(report.operatorsWithIssues, 1)
		s.Equal("True", report.operatorsWithIssues[0].degraded)
	})
}

// unstructuredListFixture builds a minimal *unstructured.UnstructuredList that
// satisfies the unstructuredContenter interface used by analyzeClusterOperatorHealth.
func unstructuredListFixture(items ...map[string]any) *unstructuredList {
	return &unstructuredList{items: items}
}

type unstructuredList struct {
	items []map[string]any
}

func (u *unstructuredList) UnstructuredContent() map[string]any {
	list := make([]any, 0, len(u.items))
	for _, item := range u.items {
		list = append(list, item)
	}
	return map[string]any{"items": list}
}

func TestAnalyze(t *testing.T) {
	suite.Run(t, new(AnalyzeSuite))
}

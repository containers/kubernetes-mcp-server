package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

type PromptClusterHealthCheckSuite struct {
	BaseMcpSuite
}

func (s *PromptClusterHealthCheckSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.createClusterHealthCheckTestData()
}

func (s *PromptClusterHealthCheckSuite) createClusterHealthCheckTestData() {
	ctx := s.T().Context()
	client := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())

	// Create an unhealthy deployment in ns-1 so the cluster-wide report has
	// workload issues outside of the default namespace.
	deploy, err := client.AppsV1().Deployments("ns-1").Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-unhealthy-deploy"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](3),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "cluster-unhealthy"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "cluster-unhealthy"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
				},
			},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)

	// envtest does not run the deployment controller, so set the status directly
	// to simulate unavailable replicas.
	deploy.Status.Replicas = 3
	deploy.Status.ReadyReplicas = 1
	deploy.Status.AvailableReplicas = 1
	deploy.Status.UnavailableReplicas = 2
	_, err = client.AppsV1().Deployments("ns-1").UpdateStatus(ctx, deploy, metav1.UpdateOptions{})
	s.Require().NoError(err)

	// Create a warning event in kube-system because cluster-wide event gathering
	// only collects events from default, openshift-*, and *-system namespaces.
	now := metav1.Now()
	_, err = client.CoreV1().Events("kube-system").Create(ctx, &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-health-check-warning-event"},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "cluster-crashing-pod",
			Namespace:  "kube-system",
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "BackOff",
		Message:       "Back-off restarting failed container",
		LastTimestamp: now,
		Count:         5,
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *PromptClusterHealthCheckSuite) TearDownTest() {
	ctx := s.T().Context()
	client := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	_ = client.AppsV1().Deployments("ns-1").Delete(ctx, "cluster-unhealthy-deploy", metav1.DeleteOptions{})
	_ = client.CoreV1().Events("kube-system").Delete(ctx, "cluster-health-check-warning-event", metav1.DeleteOptions{})
	s.BaseMcpSuite.TearDownTest()
}

func (s *PromptClusterHealthCheckSuite) TestPromptArguments() {
	s.InitMcpClient()

	prompts, err := s.ListPrompts()
	s.Require().NoError(err)
	s.Require().NotNil(prompts)

	var healthCheck *mcp.Prompt
	for _, p := range prompts.Prompts {
		if p.Name == "cluster-health-check" {
			healthCheck = p
			break
		}
	}

	s.Run("prompt is registered", func() {
		s.Require().NotNil(healthCheck, "cluster-health-check prompt should be registered")
	})

	s.Run("has correct metadata", func() {
		s.Require().NotNil(healthCheck)
		s.Equal("cluster-health-check", healthCheck.Name)
		s.Contains(healthCheck.Description, "cluster")
	})

	s.Run("has expected arguments", func() {
		s.Require().NotNil(healthCheck)
		s.Require().Len(healthCheck.Arguments, 1, "should have 1 argument")

		s.Equal("check_events", healthCheck.Arguments[0].Name)
		s.NotEmpty(healthCheck.Arguments[0].Description)
		s.False(healthCheck.Arguments[0].Required)
	})
}

func (s *PromptClusterHealthCheckSuite) TestClusterHealthCheck() {
	s.InitMcpClient()

	start := time.Now()
	result, err := s.GetPrompt("cluster-health-check", map[string]string{})
	elapsed := time.Since(start)

	s.Run("completes without error", func() {
		s.NoError(err, "GetPrompt should not return error")
		s.Require().NotNil(result, "result should not be nil")
	})

	s.Run("completes within 5 seconds", func() {
		s.Less(elapsed, 5*time.Second, "cluster health check should complete quickly on envtest")
	})

	s.Run("returns two messages (user + assistant)", func() {
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 2)
		s.Equal("user", string(result.Messages[0].Role))
		s.Equal("assistant", string(result.Messages[1].Role))
	})

	s.Run("output contains expected section headers", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "# Cluster Health Check Diagnostic Data")
		s.Contains(text, "## Nodes")
		s.Contains(text, "## Namespace Health")
		s.Contains(text, "## Recent System Events (Last Hour)")
	})

	s.Run("output shows cluster scope", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "All namespaces for workload data")
		s.Contains(text, "system namespaces for events")
	})

	s.Run("section ordering is nodes, namespace health, events", func() {
		text := s.requireMessageText(result, 0)
		nodeIdx := strings.Index(text, "## Nodes")
		nsHealthIdx := strings.Index(text, "## Namespace Health")
		eventsIdx := strings.Index(text, "## Recent System Events")
		s.GreaterOrEqual(nodeIdx, 0, "Nodes section header not found")
		s.GreaterOrEqual(nsHealthIdx, 0, "Namespace Health section header not found")
		s.GreaterOrEqual(eventsIdx, 0, "Recent System Events section header not found")
		s.Less(nodeIdx, nsHealthIdx, "Nodes section should appear before Namespace Health")
		s.Less(nsHealthIdx, eventsIdx, "Namespace Health section should appear before Events")
	})

	s.Run("assistant message contains analysis prompt", func() {
		text := s.requireMessageText(result, 1)
		s.Contains(text, "I'll analyze the cluster health diagnostic data and provide a comprehensive assessment.")
	})
}

func (s *PromptClusterHealthCheckSuite) TestCheckEventsFalse() {
	s.InitMcpClient()

	result, err := s.GetPrompt("cluster-health-check", map[string]string{
		"check_events": "false",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output does not contain events section", func() {
		text := s.requireMessageText(result, 0)
		s.NotContains(text, "## Recent System Events")
	})

	s.Run("nodes and namespace health sections are still present", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "## Nodes")
		s.Contains(text, "## Namespace Health")
	})
}

func (s *PromptClusterHealthCheckSuite) TestCheckEventsInvalidDefaultsToTrue() {
	s.InitMcpClient()

	result, err := s.GetPrompt("cluster-health-check", map[string]string{
		"check_events": "invalid",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("events section is present because invalid value defaults to true", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "## Recent System Events")
	})
}

func (s *PromptClusterHealthCheckSuite) TestNonOpenShiftCluster() {
	s.InitMcpClient()

	result, err := s.GetPrompt("cluster-health-check", map[string]string{})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output does not contain cluster operators section", func() {
		text := s.requireMessageText(result, 0)
		s.NotContains(text, "## Cluster Operators")
	})
}

func (s *PromptClusterHealthCheckSuite) TestClusterWideWorkloadDetection() {
	s.InitMcpClient()

	result, err := s.GetPrompt("cluster-health-check", map[string]string{
		"check_events": "false",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output reports unhealthy deployment in ns-1", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "- **ns-1** -")
		// Cluster-wide rendering uses compact counts rather than workload names.
		s.Contains(text, "deployments: 1 unhealthy (2 replicas missing)")
	})

	s.Run("summary reflects non-zero unhealthy workloads", func() {
		text := s.requireMessageText(result, 0)
		s.Regexp(`\*\*Unhealthy Workloads:\*\* [1-9]\d*`, text)
	})
}

func (s *PromptClusterHealthCheckSuite) TestClusterWideEventDetection() {
	s.InitMcpClient()

	result, err := s.GetPrompt("cluster-health-check", map[string]string{})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output contains event from kube-system", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "kube-system/Pod/cluster-crashing-pod")
	})

	s.Run("output shows non-zero warning count", func() {
		text := s.requireMessageText(result, 0)
		s.Regexp(`\*\*Warnings:\*\* [1-9]\d*`, text)
	})
}

// TestGracefulEmptySections verifies that sections with no data (no nodes in
// envtest) do not prevent other sections from rendering. It does not cover a
// true partial failure where one gather/analysis task errors while others
// succeed; inducing that reliably against the shared envtest control plane
// without affecting other tests would require complex RBAC setup, so that
// scenario is left for future coverage once the prompt internals expose a
// safer injection point.
func (s *PromptClusterHealthCheckSuite) TestGracefulEmptySections() {
	s.InitMcpClient()

	result, err := s.GetPrompt("cluster-health-check", map[string]string{
		"check_events": "false",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("nodes section renders even with no nodes", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "## Nodes")
		s.Contains(text, "No nodes found")
	})

	s.Run("namespace health section still renders when nodes are empty", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "## Namespace Health")
	})
}

func TestPromptClusterHealthCheckSuite(t *testing.T) {
	suite.Run(t, new(PromptClusterHealthCheckSuite))
}

package mcp

import (
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/containers/kubernetes-mcp-server/internal/test"
)

type PromptNamespaceHealthCheckSuite struct {
	BaseMcpSuite
}

func (s *PromptNamespaceHealthCheckSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.createNamespaceHealthCheckTestData()
}

func (s *PromptNamespaceHealthCheckSuite) createNamespaceHealthCheckTestData() {
	ctx := s.T().Context()
	client := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())

	// Create a Deployment with available replicas
	_, err := client.AppsV1().Deployments("default").Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-healthy-deploy"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ns-healthy"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ns-healthy"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
				},
			},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)

	// Create a StatefulSet
	_, err = client.AppsV1().StatefulSets("default").Create(ctx, &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-healthy-sts"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ns-sts"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ns-sts"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
				},
			},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)

	// Create a DaemonSet
	_, err = client.AppsV1().DaemonSets("default").Create(ctx, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-healthy-ds"},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ns-ds"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ns-ds"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
				},
			},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)

	// Create a PVC
	_, err = client.CoreV1().PersistentVolumeClaims("default").Create(ctx, &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-test-pvc"},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)

	// Create a Warning event with a recent timestamp
	now := metav1.Now()
	_, err = client.CoreV1().Events("default").Create(ctx, &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-health-check-warning-event"},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "ns-crashing-pod",
			Namespace:  "default",
		},
		Type:          corev1.EventTypeWarning,
		Reason:        "BackOff",
		Message:       "Back-off restarting failed container",
		LastTimestamp: now,
		Count:         3,
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *PromptNamespaceHealthCheckSuite) TearDownTest() {
	ctx := s.T().Context()
	client := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	_ = client.AppsV1().Deployments("default").Delete(ctx, "ns-healthy-deploy", metav1.DeleteOptions{})
	_ = client.AppsV1().StatefulSets("default").Delete(ctx, "ns-healthy-sts", metav1.DeleteOptions{})
	_ = client.AppsV1().DaemonSets("default").Delete(ctx, "ns-healthy-ds", metav1.DeleteOptions{})
	// envtest doesn't run the PVC protection controller, so the
	// kubernetes.io/pvc-protection finalizer is never removed autommatically.
	// Clear it before deleting so the PVC is fully removed and doesn't block the next SetupTest.
	if pvc, getErr := client.CoreV1().PersistentVolumeClaims("default").Get(ctx, "ns-test-pvc", metav1.GetOptions{}); getErr == nil {
		pvc.Finalizers = nil
		_, updateErr := client.CoreV1().PersistentVolumeClaims("default").Update(ctx, pvc, metav1.UpdateOptions{})
		s.Require().NoError(updateErr, "failed to clear PVC finalizers")
		s.Require().NoError(client.CoreV1().PersistentVolumeClaims("default").Delete(ctx, "ns-test-pvc", metav1.DeleteOptions{}), "failed to delete PVC")
	}
	_ = client.CoreV1().Events("default").Delete(ctx, "ns-health-check-warning-event", metav1.DeleteOptions{})
	s.BaseMcpSuite.TearDownTest()
}

func (s *PromptNamespaceHealthCheckSuite) TestPromptArguments() {
	s.InitMcpClient()

	prompts, err := s.ListPrompts()
	s.Require().NoError(err)
	s.Require().NotNil(prompts)

	var healthCheck *mcp.Prompt
	for _, p := range prompts.Prompts {
		if p.Name == "namespace-health-check" {
			healthCheck = p
			break
		}
	}

	s.Run("prompt is registered", func() {
		s.Require().NotNil(healthCheck, "namespace-health-check prompt should be registered")
	})

	s.Run("has correct metadata", func() {
		s.Require().NotNil(healthCheck)
		s.Equal("namespace-health-check", healthCheck.Name)
		s.Contains(healthCheck.Description, "workloads")
	})

	s.Run("has expected arguments", func() {
		s.Require().NotNil(healthCheck)
		s.Require().Len(healthCheck.Arguments, 2, "should have 2 arguments")

		s.Equal("namespace", healthCheck.Arguments[0].Name)
		s.NotEmpty(healthCheck.Arguments[0].Description)
		s.True(healthCheck.Arguments[0].Required)

		s.Equal("check_events", healthCheck.Arguments[1].Name)
		s.NotEmpty(healthCheck.Arguments[1].Description)
		s.False(healthCheck.Arguments[1].Required)
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestNamespaceHealthCheck() {
	s.InitMcpClient()

	start := time.Now()
	result, err := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace": "default",
	})
	elapsed := time.Since(start)

	s.Run("completes without error", func() {
		s.NoError(err, "GetPrompt should not return error")
		s.Require().NotNil(result, "result should not be nil")
	})

	s.Run("completes within 5 seconds", func() {
		s.Less(elapsed, 5*time.Second, "health check should complete quickly on envtest")
	})

	s.Run("returns two messages (user + assistant)", func() {
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 2)
		s.Equal("user", string(result.Messages[0].Role))
		s.Equal("assistant", string(result.Messages[1].Role))
	})

	s.Run("output contains expected section headers", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "# Namespace Health Check Diagnostic Data")
		s.Contains(text, "## Workload Health")
		s.Contains(text, "## Recent Events (Last Hour)")
	})

	s.Run("output shows namespace scope", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "Namespace Health Check Diagnostic Data. Cluster:")
		s.Contains(text, "Namespace: default")
	})

	s.Run("assistant message contains analysis prompt", func() {
		text := s.requireMessageText(result, 1)
		s.Contains(text, "I'll analyze the namespace health diagnostic data and provide a comprehensive assessment.")
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestMissingRequiredNamespace() {
	s.InitMcpClient()

	result, err := s.GetPrompt("namespace-health-check", map[string]string{})

	s.Run("returns error when namespace is not provided", func() {
		s.Error(err)
	})

	s.Run("result is nil", func() {
		s.Nil(result)
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestNonExistentNamespace() {
	s.InitMcpClient()

	result, err := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace": "does-not-exist",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output warns namespace not found", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "Namespace 'does-not-exist' not found")
	})

	s.Run("output does not contain workload sections", func() {
		text := s.requireMessageText(result, 0)
		s.NotContains(text, "## Workload Health")
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestCheckEventsFalse() {
	s.InitMcpClient()

	result, err := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace":    "default",
		"check_events": "false",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output does not contain events section", func() {
		text := s.requireMessageText(result, 0)
		s.NotContains(text, "## Recent Events")
	})

	s.Run("workload section is still present", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "## Workload Health")
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestNoNodesSection() {
	s.InitMcpClient()

	result, err := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace": "default",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output does not contain nodes section", func() {
		text := s.requireMessageText(result, 0)
		s.NotContains(text, "## Nodes")
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestNoClusterOperatorsSection() {
	s.InitMcpClient()

	result, err := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace": "default",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output does not contain cluster operators section", func() {
		text := s.requireMessageText(result, 0)
		s.NotContains(text, "## Cluster Operators")
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestUnhealthyWorkloadDetection() {
	ctx := s.T().Context()
	client := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())

	// Create a deployment and set its status to have unavailable replicas
	deploy, err := client.AppsV1().Deployments("default").Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-unhealthy-deploy"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](3),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "ns-unhealthy"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "ns-unhealthy"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
				},
			},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	defer func() {
		_ = client.AppsV1().Deployments("default").Delete(ctx, "ns-unhealthy-deploy", metav1.DeleteOptions{})
	}()

	deploy.Status.Replicas = 3
	deploy.Status.ReadyReplicas = 1
	deploy.Status.AvailableReplicas = 1
	deploy.Status.UnavailableReplicas = 2
	_, err = client.AppsV1().Deployments("default").UpdateStatus(ctx, deploy, metav1.UpdateOptions{})
	s.Require().NoError(err)

	s.InitMcpClient()
	result, promptErr := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace":    "default",
		"check_events": "false",
	})

	s.Run("completes without error", func() {
		s.NoError(promptErr)
		s.Require().NotNil(result)
	})

	s.Run("output reports unhealthy deployment with name and missing replicas", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "ns-unhealthy-deploy (2 replicas missing)")
	})

	s.Run("summary reflects unhealthy workload count", func() {
		text := s.requireMessageText(result, 0)
		s.Regexp(`\*\*Unhealthy Workloads:\*\* [1-9]\d*`, text)
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestPodIssueDetection() {
	ctx := s.T().Context()
	client := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())

	// Create a pod and update its status to simulate CrashLoopBackOff
	pod, err := client.CoreV1().Pods("default").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "ns-crashloop-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c", Image: "nginx"}},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	defer func() {
		_ = client.CoreV1().Pods("default").Delete(ctx, "ns-crashloop-pod", metav1.DeleteOptions{})
	}()

	pod.Status.Phase = corev1.PodRunning
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			Name: "c",
			State: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{
					Reason: "CrashLoopBackOff",
				},
			},
			RestartCount: 5,
		},
	}
	_, err = client.CoreV1().Pods("default").UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	s.Require().NoError(err)

	s.InitMcpClient()
	result, promptErr := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace":    "default",
		"check_events": "false",
	})

	s.Run("completes without error", func() {
		s.NoError(promptErr)
		s.Require().NotNil(result)
	})

	s.Run("output reports CrashLoopBackOff with pod name", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "Container waiting: CrashLoopBackOff")
		s.Contains(text, "ns-crashloop-pod")
	})

	s.Run("summary shows pods with issues", func() {
		text := s.requireMessageText(result, 0)
		s.Regexp(`\*\*Pods with Issues:\*\* [1-9]\d*`, text)
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestPVCPendingDetection() {
	s.InitMcpClient()

	// ns-test-pvc is created in SetupTest and stays Pending in envtest (no provisioner)
	result, err := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace":    "default",
		"check_events": "false",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("summary reports pending PVCs", func() {
		text := s.requireMessageText(result, 0)
		s.Regexp(`\*\*PVCs Pending:\*\* [1-9]\d*`, text)
	})

	s.Run("namespace entry reports pending PVC with name", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "PVC:")
		s.Contains(text, "pending: ns-test-pvc")
	})
}

func (s *PromptNamespaceHealthCheckSuite) TestEventContentDetails() {
	s.InitMcpClient()

	// Warning event for ns-crashing-pod is created in SetupTest
	result, err := s.GetPrompt("namespace-health-check", map[string]string{
		"namespace": "default",
	})

	s.Run("completes without error", func() {
		s.NoError(err)
		s.Require().NotNil(result)
	})

	s.Run("output contains involved object reference", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "Pod/ns-crashing-pod")
	})

	s.Run("output contains event reason and count", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "BackOff (Count: 3)")
	})

	s.Run("output contains event message", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "Back-off restarting failed container")
	})

	s.Run("output shows non-zero warning count", func() {
		text := s.requireMessageText(result, 0)
		s.Contains(text, "**Warnings:**")
		s.Regexp(`\*\*Warnings:\*\* [1-9]\d*`, text)
	})
}

func TestPromptNamespaceHealthCheckSuite(t *testing.T) {
	suite.Run(t, new(PromptNamespaceHealthCheckSuite))
}

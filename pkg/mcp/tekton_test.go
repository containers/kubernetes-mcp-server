package mcp

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var tektonTestAPIs = []schema.GroupVersionResource{
	{Group: "tekton.dev", Version: "v1", Resource: "pipelines"},
	{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"},
	{Group: "tekton.dev", Version: "v1", Resource: "tasks"},
	{Group: "tekton.dev", Version: "v1", Resource: "taskruns"},
	{Group: "pipelinesascode.tekton.dev", Version: "v1alpha1", Resource: "repositories"},
	{Group: "operator.tekton.dev", Version: "v1alpha1", Resource: "tektonconfigs"},
}

var (
	tektonTestPipelineGVR    = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelines"}
	tektonTestPipelineRunGVR = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "pipelineruns"}
	tektonTestTaskRunGVR     = schema.GroupVersionResource{Group: "tekton.dev", Version: "v1", Resource: "taskruns"}
	tektonTestRepositoryGVR  = schema.GroupVersionResource{Group: "pipelinesascode.tekton.dev", Version: "v1alpha1", Resource: "repositories"}
	tektonTestConfigGVR      = schema.GroupVersionResource{Group: "operator.tekton.dev", Version: "v1alpha1", Resource: "tektonconfigs"}
)

type TektonMcpSuite struct {
	BaseMcpSuite
	namespace        string
	tektonConfigName string
	dynamic          dynamic.Interface
}

func (s *TektonMcpSuite) SetupSuite() {
	for _, api := range tektonTestAPIs {
		s.Require().NoError(EnvTestEnableCRD(s.T().Context(), api.Group, api.Version, api.Resource))
	}
}

func (s *TektonMcpSuite) TearDownSuite() {
	for _, api := range tektonTestAPIs {
		s.Require().NoError(EnvTestDisableCRD(s.T().Context(), api.Group, api.Version, api.Resource))
	}
}

func (s *TektonMcpSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.Cfg.Toolsets = append(s.Cfg.Toolsets, "tekton")
	s.namespace = fmt.Sprintf("tekton-mcp-%d", time.Now().UnixNano())
	s.tektonConfigName = s.namespace
	s.dynamic = dynamic.NewForConfigOrDie(test.EnvTestRestConfig())
	_, err := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).CoreV1().Namespaces().Create(s.T().Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: s.namespace}}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.InitMcpClient()
}

func (s *TektonMcpSuite) TearDownTest() {
	_ = s.dynamic.Resource(tektonTestConfigGVR).Delete(s.T().Context(), s.tektonConfigName, metav1.DeleteOptions{})
	_ = kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).CoreV1().Namespaces().Delete(s.T().Context(), s.namespace, metav1.DeleteOptions{})
	s.BaseMcpSuite.TearDownTest()
}

func (s *TektonMcpSuite) TestPipelineRunLifecycle() {
	s.Run("cancel", func() {
		s.createPipelineRun("cancel-me")

		toolResult, err := s.CallTool("tekton_pipelinerun_lifecycle", map[string]interface{}{
			"namespace": s.namespace,
			"name":      "cancel-me",
			"action":    "cancel",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError)

		pipelineRun, err := s.dynamic.Resource(tektonTestPipelineRunGVR).Namespace(s.namespace).Get(s.T().Context(), "cancel-me", metav1.GetOptions{})
		s.Require().NoError(err)
		status, _, _ := unstructured.NestedString(pipelineRun.Object, "spec", "status")
		s.Equal("Cancelled", status)
	})

	s.Run("restart", func() {
		s.createPipelineRun("restart-me")

		toolResult, err := s.CallTool("tekton_pipelinerun_lifecycle", map[string]interface{}{
			"namespace": s.namespace,
			"name":      "restart-me",
			"action":    "restart",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError)

		list, err := s.dynamic.Resource(tektonTestPipelineRunGVR).Namespace(s.namespace).List(s.T().Context(), metav1.ListOptions{})
		s.Require().NoError(err)
		foundRestart := false
		for _, item := range list.Items {
			if item.GetName() != "restart-me" && strings.HasPrefix(item.GetName(), "restart-me-") {
				foundRestart = true
				break
			}
		}
		s.True(foundRestart, "expected restart action to create a new generated PipelineRun")
	})
}

func (s *TektonMcpSuite) TestPipelineRunLogsWithoutTaskRuns() {
	s.createPipelineRun("no-taskruns")

	toolResult, err := s.CallTool("tekton_pipelinerun_logs", map[string]interface{}{
		"namespace": s.namespace,
		"name":      "no-taskruns",
	})
	s.Require().NoError(err)
	s.False(toolResult.IsError)
	s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "No TaskRuns found")
}

func (s *TektonMcpSuite) TestPipelineRunLogsWithoutMatchingPipelineTask() {
	s.createPipelineRun("filtered-run")
	s.createLogTaskRun("compile-run", "filtered-run", "compile", []string{"build"}, nil)

	toolResult, err := s.CallTool("tekton_pipelinerun_logs", map[string]interface{}{
		"namespace": s.namespace,
		"name":      "filtered-run",
		"task":      "deploy",
	})
	s.Require().NoError(err)
	s.False(toolResult.IsError)
	s.Equal(
		fmt.Sprintf("No TaskRuns found for PipelineRun 'filtered-run' in namespace '%s' matching pipeline task 'deploy'", s.namespace),
		toolResult.Content[0].(*mcp.TextContent).Text,
	)
}

func (s *TektonMcpSuite) TestPipelineRunLogFilters() {
	s.createPipelineRun("filtered-run")
	s.createLogTaskRun("compile-run", "filtered-run", "compile", []string{"build", "test"}, []string{"cache"})
	s.createLogTaskRun("deploy-run", "filtered-run", "deploy", []string{"release"}, nil)

	toolResult, err := s.CallTool("tekton_pipelinerun_logs", map[string]interface{}{
		"namespace": s.namespace,
		"name":      "filtered-run",
		"task":      "compile",
		"step":      "build",
	})
	s.Require().NoError(err)
	s.False(toolResult.IsError)
	text := toolResult.Content[0].(*mcp.TextContent).Text
	s.Contains(text, "# TaskRun: compile-run")
	s.Contains(text, "[step: build]")
	s.NotContains(text, "[step: test]")
	s.NotContains(text, "[sidecar: cache]")
	s.NotContains(text, "# TaskRun: deploy-run")
}

func (s *TektonMcpSuite) TestTaskRunLogStepFilter() {
	s.createLogTaskRun("filtered-taskrun", "standalone-run", "compile", []string{"build", "test"}, []string{"cache"})

	s.Run("returns only the selected step", func() {
		toolResult, err := s.CallTool("tekton_taskrun_logs", map[string]interface{}{
			"namespace": s.namespace,
			"name":      "filtered-taskrun",
			"step":      "test",
		})
		s.Require().NoError(err)
		s.False(toolResult.IsError)
		text := toolResult.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "[step: test]")
		s.NotContains(text, "[step: build]")
		s.NotContains(text, "[sidecar: cache]")
	})

	s.Run("returns no logs for an unknown step", func() {
		toolResult, err := s.CallTool("tekton_taskrun_logs", map[string]interface{}{
			"namespace": s.namespace,
			"name":      "filtered-taskrun",
			"step":      "unknown",
		})
		s.Require().NoError(err)
		s.Contains(toolResult.Content[0].(*mcp.TextContent).Text, "No logs available")
	})

	s.Run("preserves all steps and sidecars when no filter is set", func() {
		toolResult, err := s.CallTool("tekton_taskrun_logs", map[string]interface{}{
			"namespace": s.namespace,
			"name":      "filtered-taskrun",
		})
		s.Require().NoError(err)
		text := toolResult.Content[0].(*mcp.TextContent).Text
		s.Contains(text, "[step: build]")
		s.Contains(text, "[step: test]")
		s.Contains(text, "[sidecar: cache]")
	})
}

func (s *TektonMcpSuite) TestPipelineTroubleshootPrompt() {
	s.Run("returns gathered PipelineRun data", func() {
		s.createPipeline("demo-pipeline", "diagnostic-task")
		s.createPipelineRun("broken-run")
		s.createTaskRun("broken-run-task", "broken-run")
		s.createEvent("broken-run-event", "PipelineRun", "broken-run", "BrokenPipelineRun")
		s.createEvent("unrelated-run-event", "PipelineRun", "unrelated-run", "UnrelatedPipelineRun")
		s.createRepository("eval-repo")
		s.createTektonConfig()

		result, err := s.GetPrompt("pipeline-troubleshoot", map[string]string{
			"namespace": s.namespace,
			"name":      "broken-run",
		})
		s.Require().NoError(err)
		s.Require().NotNil(result)
		s.Require().Len(result.Messages, 2)
		text := result.Messages[0].Content.(*mcp.TextContent).Text
		s.Contains(text, "PipelineRun: "+s.namespace+"/broken-run")
		s.Contains(text, "diagnostic-task")
		s.Contains(text, "broken-run-task")
		s.Contains(text, "BrokenPipelineRun")
		s.NotContains(text, "UnrelatedPipelineRun")
		s.Contains(text, "eval-repo")
		s.Contains(text, s.tektonConfigName)
	})

	s.Run("returns an embedded PipelineSpec", func() {
		s.createEmbeddedPipelineRun("embedded-run", "embedded-diagnostic-task")

		result, err := s.GetPrompt("pipeline-troubleshoot", map[string]string{
			"namespace": s.namespace,
			"name":      "embedded-run",
		})
		s.Require().NoError(err)
		text := result.Messages[0].Content.(*mcp.TextContent).Text
		s.Contains(text, "Embedded PipelineSpec")
		s.Contains(text, "embedded-diagnostic-task")
	})

	s.Run("continues when a referenced Pipeline is missing", func() {
		s.createReferencedPipelineRun("missing-pipeline-run", "does-not-exist")

		result, err := s.GetPrompt("pipeline-troubleshoot", map[string]string{
			"namespace": s.namespace,
			"name":      "missing-pipeline-run",
		})
		s.Require().NoError(err)
		text := result.Messages[0].Content.(*mcp.TextContent).Text
		s.Contains(text, "Pipeline definition unavailable")
		s.Contains(text, "does-not-exist")
	})

	s.Run("returns the resolved PipelineSpec for a resolver-based Pipeline", func() {
		s.createResolverPipelineRun("resolver-pipeline-run", "resolved-diagnostic-task")

		result, err := s.GetPrompt("pipeline-troubleshoot", map[string]string{
			"namespace": s.namespace,
			"name":      "resolver-pipeline-run",
		})
		s.Require().NoError(err)
		text := result.Messages[0].Content.(*mcp.TextContent).Text
		s.Contains(text, "Resolved PipelineSpec")
		s.Contains(text, "resolved-diagnostic-task")
	})

	s.Run("returns only failed step logs and warning events", func() {
		s.createPipeline("failed-step-pipeline", "diagnostic-task")
		s.createReferencedPipelineRun("failed-step-run", "failed-step-pipeline")
		s.createLogTaskRunWithStepStates("failed-step-taskrun", "failed-step-run", "diagnostic-task", []interface{}{
			map[string]interface{}{
				"name":      "failed",
				"container": "step-failed",
				"terminated": map[string]interface{}{
					"exitCode": int64(1),
					"reason":   "Error",
				},
			},
			map[string]interface{}{
				"name":      "passed",
				"container": "step-passed",
				"terminated": map[string]interface{}{
					"exitCode": int64(0),
					"reason":   "Completed",
				},
			},
			map[string]interface{}{
				"name":      "errored",
				"container": "step-errored",
				"waiting": map[string]interface{}{
					"reason": "ErrImagePull",
				},
			},
			map[string]interface{}{
				"name":      "starting",
				"container": "step-starting",
				"waiting": map[string]interface{}{
					"reason": "ContainerCreating",
				},
			},
		}, []string{"cache"})
		s.createEvent("failed-step-warning", "PipelineRun", "failed-step-run", "WarningEvent")
		s.createEventWithType("failed-step-normal", "PipelineRun", "failed-step-run", "NormalEvent", corev1.EventTypeNormal)

		result, err := s.GetPrompt("pipeline-troubleshoot", map[string]string{
			"namespace": s.namespace,
			"name":      "failed-step-run",
		})
		s.Require().NoError(err)
		text := result.Messages[0].Content.(*mcp.TextContent).Text
		s.Contains(text, "## PipelineRun Status")
		s.Contains(text, "## TaskRun Status")
		s.Contains(text, "## Failed Step Logs")
		s.Contains(text, "## Troubleshooting Analysis")
		s.Contains(text, "## Fix Suggestions")
		s.Contains(text, "[step: failed]")
		s.Contains(text, "[step: errored]")
		s.NotContains(text, "[step: passed]")
		s.NotContains(text, "[step: starting]")
		s.NotContains(text, "[sidecar: cache]")
		s.Contains(text, "WarningEvent")
		s.NotContains(text, "NormalEvent")
	})

	s.Run("requires namespace", func() {
		result, err := s.GetPrompt("pipeline-troubleshoot", map[string]string{"name": "broken-run"})
		s.Error(err)
		s.Nil(result)
	})
}

func (s *TektonMcpSuite) createPipeline(name, pipelineTaskName string) {
	_, err := s.dynamic.Resource(tektonTestPipelineGVR).Namespace(s.namespace).Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "tekton.dev/v1",
		"kind":       "Pipeline",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": s.namespace,
		},
		"spec": map[string]interface{}{
			"tasks": []interface{}{map[string]interface{}{
				"name":    pipelineTaskName,
				"taskRef": map[string]interface{}{"name": "demo-task"},
			}},
		},
	}}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *TektonMcpSuite) createPipelineRun(name string) {
	s.createReferencedPipelineRun(name, "demo-pipeline")
}

func (s *TektonMcpSuite) createReferencedPipelineRun(name, pipelineName string) {
	_, err := s.dynamic.Resource(tektonTestPipelineRunGVR).Namespace(s.namespace).Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": s.namespace,
		},
		"spec": map[string]interface{}{
			"pipelineRef": map[string]interface{}{"name": pipelineName},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{map[string]interface{}{
				"type":   "Succeeded",
				"status": "False",
				"reason": "Failed",
			}},
		},
	}}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *TektonMcpSuite) createResolverPipelineRun(name, pipelineTaskName string) {
	_, err := s.dynamic.Resource(tektonTestPipelineRunGVR).Namespace(s.namespace).Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": s.namespace,
		},
		"spec": map[string]interface{}{
			"pipelineRef": map[string]interface{}{"resolver": "git"},
		},
		"status": map[string]interface{}{
			"pipelineSpec": map[string]interface{}{
				"tasks": []interface{}{map[string]interface{}{
					"name":    pipelineTaskName,
					"taskRef": map[string]interface{}{"name": "demo-task"},
				}},
			},
		},
	}}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *TektonMcpSuite) createEmbeddedPipelineRun(name, pipelineTaskName string) {
	_, err := s.dynamic.Resource(tektonTestPipelineRunGVR).Namespace(s.namespace).Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": s.namespace,
		},
		"spec": map[string]interface{}{
			"pipelineSpec": map[string]interface{}{
				"tasks": []interface{}{map[string]interface{}{
					"name":    pipelineTaskName,
					"taskRef": map[string]interface{}{"name": "demo-task"},
				}},
			},
		},
	}}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *TektonMcpSuite) createTaskRun(name, pipelineRun string) {
	_, err := s.dynamic.Resource(tektonTestTaskRunGVR).Namespace(s.namespace).Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "tekton.dev/v1",
		"kind":       "TaskRun",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": s.namespace,
			"labels": map[string]interface{}{
				"tekton.dev/pipelineRun": pipelineRun,
			},
		},
		"spec": map[string]interface{}{
			"taskRef": map[string]interface{}{"name": "demo-task"},
		},
	}}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *TektonMcpSuite) createLogTaskRun(name, pipelineRun, pipelineTask string, steps, sidecars []string) {
	stepStates := make([]interface{}, 0, len(steps))
	for _, step := range steps {
		stepStates = append(stepStates, map[string]interface{}{"name": step, "container": "step-" + step})
	}
	s.createLogTaskRunWithStepStates(name, pipelineRun, pipelineTask, stepStates, sidecars)
}

func (s *TektonMcpSuite) createLogTaskRunWithStepStates(name, pipelineRun, pipelineTask string, stepStates []interface{}, sidecars []string) {
	sidecarStates := make([]interface{}, 0, len(sidecars))
	for _, sidecar := range sidecars {
		sidecarStates = append(sidecarStates, map[string]interface{}{"name": sidecar, "container": "sidecar-" + sidecar})
	}

	_, err := s.dynamic.Resource(tektonTestTaskRunGVR).Namespace(s.namespace).Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "tekton.dev/v1",
		"kind":       "TaskRun",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": s.namespace,
			"labels": map[string]interface{}{
				"tekton.dev/pipelineRun":  pipelineRun,
				"tekton.dev/pipelineTask": pipelineTask,
			},
		},
		"spec": map[string]interface{}{
			"taskRef": map[string]interface{}{"name": "demo-task"},
		},
		"status": map[string]interface{}{
			"podName":  "pod-" + name,
			"steps":    stepStates,
			"sidecars": sidecarStates,
		},
	}}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *TektonMcpSuite) createEvent(name, involvedKind, involvedName, reason string) {
	s.createEventWithType(name, involvedKind, involvedName, reason, corev1.EventTypeWarning)
}

func (s *TektonMcpSuite) createEventWithType(name, involvedKind, involvedName, reason, eventType string) {
	now := metav1.Now()
	_, err := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig()).CoreV1().Events(s.namespace).Create(s.T().Context(), &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      involvedKind,
			Name:      involvedName,
			Namespace: s.namespace,
		},
		Type:           eventType,
		Reason:         reason,
		Message:        reason,
		FirstTimestamp: now,
		LastTimestamp:  now,
		Source:         corev1.EventSource{Component: "tekton-test"},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *TektonMcpSuite) createRepository(name string) {
	_, err := s.dynamic.Resource(tektonTestRepositoryGVR).Namespace(s.namespace).Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "pipelinesascode.tekton.dev/v1alpha1",
		"kind":       "Repository",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": s.namespace,
		},
		"spec": map[string]interface{}{
			"url": "https://github.com/example/repo",
		},
	}}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *TektonMcpSuite) createTektonConfig() {
	_, err := s.dynamic.Resource(tektonTestConfigGVR).Create(s.T().Context(), &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "operator.tekton.dev/v1alpha1",
		"kind":       "TektonConfig",
		"metadata": map[string]interface{}{
			"name": s.tektonConfigName,
		},
		"spec": map[string]interface{}{
			"profile": "all",
		},
	}}, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func TestTektonMcp(t *testing.T) {
	suite.Run(t, new(TektonMcpSuite))
}

package tekton

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/clientcmd"
	metricsv1beta1 "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
)

type RetentionSuite struct {
	suite.Suite
}

func TestRetention(t *testing.T) {
	suite.Run(t, new(RetentionSuite))
}

func (s *RetentionSuite) TestStepLogsStopBeforeTheNextContainer() {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/log") {
			hits.Add(1)
			_, _ = w.Write([]byte(strings.Repeat("x", 200)))
			return
		}
		http.NotFound(w, r)
	}))
	s.T().Cleanup(srv.Close)

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	s.Require().NoError(err)
	tr := &tektonv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run"},
		Status: tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				PodName: "pod",
				Steps: []tektonv1.StepState{
					{Name: "one", Container: "one"},
					{Name: "two", Container: "two"},
					{Name: "three", Container: "three"},
				},
			},
		},
	}
	b := api.NewBuilderWithBudget(40)
	collectTaskRunLogsWithClient(s.T().Context(), coreClient{Interface: cs}, b, "ns", tr, "", 10)
	s.Equal(int32(1), hits.Load())
	s.True(b.Truncated())
	s.Equal(1, strings.Count(b.String(), api.ResultTruncationNotice))
}

func (s *RetentionSuite) TestContainerLogReadStopsAtTheBudget() {
	const budget = int64(40)
	var written atomic.Int32
	var once sync.Once
	done := make(chan struct{})
	cfg := &rest.Config{
		Host: "https://apiserver",
		Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/log") {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("missing")),
					Header:     make(http.Header),
					Request:    req,
				}, nil
			}
			pr, pw := io.Pipe()
			go func() {
				defer once.Do(func() { close(done) })
				defer func() { _ = pw.Close() }()
				chunk := bytes.Repeat([]byte("x"), 1024)
				for range 100 {
					n, err := pw.Write(chunk)
					written.Add(int32(n))
					if err != nil {
						return
					}
				}
			}()
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       pipeBody{pr},
				Header:     make(http.Header),
				Request:    req,
			}, nil
		}),
	}
	cs, err := kubernetes.NewForConfig(cfg)
	s.Require().NoError(err)
	tr := &tektonv1.TaskRun{
		ObjectMeta: metav1.ObjectMeta{Name: "run"},
		Status: tektonv1.TaskRunStatus{
			TaskRunStatusFields: tektonv1.TaskRunStatusFields{
				PodName: "pod",
				Steps: []tektonv1.StepState{
					{Name: "one", Container: "one"},
					{Name: "two", Container: "two"},
				},
			},
		},
	}
	b := api.NewBuilderWithBudget(budget)
	collectTaskRunLogsWithClient(s.T().Context(), coreClient{Interface: cs}, b, "ns", tr, "", 10)
	<-done
	s.LessOrEqual(written.Load(), int32(budget))
	s.True(b.Truncated())
	s.LessOrEqual(int64(b.Len()), budget+int64(len(api.ResultTruncationNotice)))
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type pipeBody struct{ *io.PipeReader }

func (b pipeBody) Close() error { return b.PipeReader.Close() }

func (s *RetentionSuite) TestAppendTaskBlockStopsTheScratch() {
	dst := api.NewBuilderWithBudget(80)
	reads := 0
	kept := appendTaskBlock(dst, "# Task\n", "", func(inner *api.BuilderWithBudget) {
		for _, name := range []string{"a", "b", "c"} {
			if inner.Truncated() {
				return
			}
			reads++
			inner.WriteString(name + strings.Repeat("x", 200))
		}
	})
	s.False(kept)
	s.Equal(1, reads)
	s.True(dst.Truncated())
	s.Equal(1, strings.Count(dst.String(), api.ResultTruncationNotice))
}

func (s *RetentionSuite) TestPipelineEventsStopBeforeTheNextTarget() {
	var lists atomic.Int32
	cs := fakeclient.NewSimpleClientset()
	cs.PrependReactor("list", "events", func(clienttesting.Action) (bool, runtime.Object, error) {
		lists.Add(1)
		return true, &corev1.EventList{Items: []corev1.Event{{
			ObjectMeta:     metav1.ObjectMeta{Name: "warn", Namespace: "ns", UID: "u"},
			Type:           corev1.EventTypeWarning,
			Reason:         "Failed",
			Message:        strings.Repeat("m", 400),
			InvolvedObject: corev1.ObjectReference{Kind: "PipelineRun", Name: "pr"},
		}}}, nil
	})
	params := api.PromptHandlerParams{
		Context:          context.Background(),
		KubernetesClient: coreClient{Interface: cs},
	}
	taskRuns := []tektonv1.TaskRun{
		{ObjectMeta: metav1.ObjectMeta{Name: "t1"}, Status: tektonv1.TaskRunStatus{TaskRunStatusFields: tektonv1.TaskRunStatusFields{PodName: "p1"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "t2"}, Status: tektonv1.TaskRunStatus{TaskRunStatusFields: tektonv1.TaskRunStatusFields{PodName: "p2"}}},
	}
	b := api.NewBuilderWithBudget(32)
	fetchPipelineRunEventsForPrompt(params, "ns", "pr", taskRuns, b)
	s.Equal(int32(1), lists.Load())
	s.True(b.Truncated())
	s.Contains(b.String(), api.ResultTruncationNotice)
}

// coreClient exposes a clientset to retention tests. Those tests call CoreV1 only.
type coreClient struct {
	kubernetes.Interface
}

func (coreClient) NamespaceOrDefault(ns string) string { return ns }
func (coreClient) ToRESTConfig() (*rest.Config, error) {
	return nil, errCoreClientUnused
}
func (coreClient) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return nil, errCoreClientUnused
}
func (coreClient) ToRESTMapper() (meta.RESTMapper, error) { return nil, errCoreClientUnused }
func (coreClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return nil
}
func (coreClient) RESTConfig() *rest.Config                            { return nil }
func (coreClient) RESTMapper() meta.ResettableRESTMapper               { return nil }
func (coreClient) DiscoveryClient() discovery.CachedDiscoveryInterface { return nil }
func (coreClient) RawDiscoveryClient() discovery.DiscoveryInterface    { return nil }
func (coreClient) DynamicClient() dynamic.Interface                    { return nil }
func (coreClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

var errCoreClientUnused = errors.New("unused")

func (s *RetentionSuite) TestPipelinePrefixStopsBeforeTheNextList() {
	s.Run("does not list TaskRuns once the PipelineRun section fills the budget", func() {
		counts := map[string]int{}
		params := pipelinePrefixParams(s.pipelinePrefixObjects(""), counts)
		b := api.NewBuilderWithBudget(200)
		writePipelineTroubleshootGuide(params, "ns", "run", b)

		s.Contains(b.String(), api.ResultTruncationNotice)
		s.NotContains(b.String(), "## TaskRun Status")
		s.Equal(0, counts["taskruns"])
		s.Equal(0, counts["repositories"])
		s.Equal(0, counts["tektonconfigs"])
	})

	s.Run("does not list Pipeline-as-Code repositories once the TaskRun section fills the budget", func() {
		counts := map[string]int{}
		probe := api.NewBuilderWithBudget(0)
		writePipelineTroubleshootGuide(pipelinePrefixParams(s.pipelinePrefixObjects(""), nil), "ns", "run", probe)
		second := strings.Index(probe.String(), "### TaskRun: tr-b")
		s.Require().Greater(second, 0)

		params := pipelinePrefixParams(s.pipelinePrefixObjects(strings.Repeat("x", 8000)), counts)
		b := api.NewBuilderWithBudget(int64(second))
		writePipelineTroubleshootGuide(params, "ns", "run", b)

		s.Contains(b.String(), api.ResultTruncationNotice)
		s.NotContains(b.String(), "### TaskRun: tr-b")
		s.Equal(1, counts["taskruns"])
		s.Equal(0, counts["repositories"])
		s.Equal(0, counts["tektonconfigs"])
	})

	s.Run("does not list TektonConfigs once the Pipeline-as-Code section fills the budget", func() {
		counts := map[string]int{}
		probe := api.NewBuilderWithBudget(0)
		writePipelineTroubleshootGuide(pipelinePrefixParams(s.pipelinePrefixObjects(""), nil), "ns", "run", probe)
		heading := strings.Index(probe.String(), "## TektonConfig")
		s.Require().Greater(heading, 0)

		params := pipelinePrefixParams(s.pipelinePrefixObjects(""), counts)
		b := api.NewBuilderWithBudget(int64(heading))
		writePipelineTroubleshootGuide(params, "ns", "run", b)

		s.Contains(b.String(), api.ResultTruncationNotice)
		s.Equal(1, counts["repositories"])
		s.Equal(0, counts["tektonconfigs"])
	})
}

func (s *RetentionSuite) pipelinePrefixObjects(taskMessage string) []runtime.Object {
	pipelineRun := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.dev/v1",
		"kind":       "PipelineRun",
		"metadata":   map[string]any{"name": "run", "namespace": "ns"},
	}}
	taskRun := func(name string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "tekton.dev/v1",
			"kind":       "TaskRun",
			"metadata": map[string]any{
				"name":      name,
				"namespace": "ns",
				"labels":    map[string]any{"tekton.dev/pipelineRun": "run"},
			},
			"status": map[string]any{
				"conditions": []any{map[string]any{
					"type": "Succeeded", "status": "True", "reason": "Succeeded", "message": taskMessage,
				}},
			},
		}}
	}
	repo := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "pipelinesascode.tekton.dev/v1alpha1",
		"kind":       "Repository",
		"metadata":   map[string]any{"name": "repo", "namespace": "ns"},
	}}
	config := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "operator.tekton.dev/v1alpha1",
		"kind":       "TektonConfig",
		"metadata":   map[string]any{"name": "config"},
	}}
	return []runtime.Object{pipelineRun, taskRun("tr-a"), taskRun("tr-b"), repo, config}
}

func pipelinePrefixParams(objects []runtime.Object, counts map[string]int) api.PromptHandlerParams {
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		pipelineRunGVR:   "PipelineRunList",
		pipelineGVR:      "PipelineList",
		taskRunGVR:       "TaskRunList",
		pacRepositoryGVR: "RepositoryList",
		tektonConfigGVR:  "TektonConfigList",
	}, objects...)
	if counts != nil {
		dyn.PrependReactor("*", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
			if action.GetVerb() == "list" || action.GetVerb() == "get" {
				counts[action.GetResource().Resource]++
			}
			return false, nil, nil
		})
	}
	return api.PromptHandlerParams{
		Context: context.Background(),
		KubernetesClient: pipelinePrefixClient{
			coreClient: coreClient{Interface: fakeclient.NewSimpleClientset()},
			dyn:        dyn,
		},
	}
}

type pipelinePrefixClient struct {
	coreClient
	dyn dynamic.Interface
}

func (c pipelinePrefixClient) DynamicClient() dynamic.Interface { return c.dyn }

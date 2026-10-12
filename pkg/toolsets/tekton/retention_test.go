package tekton

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
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
func (coreClient) DynamicClient() dynamic.Interface                    { return nil }
func (coreClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

var errCoreClientUnused = errors.New("unused")

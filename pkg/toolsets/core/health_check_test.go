package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
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

type ClusterHealthCheckSuite struct {
	suite.Suite
}

func (s *ClusterHealthCheckSuite) TestPromptIsRegistered() {
	s.Run("cluster-health-check prompt is registered via GetPrompts", func() {
		// Create a new instance of the core toolset
		toolset := &Toolset{}

		// Get prompts from the toolset
		prompts := toolset.GetPrompts(s.T().Context(), api.ToolsetContext{})

		s.Require().NotNil(prompts, "GetPrompts should not return nil")
		s.Require().NotEmpty(prompts, "GetPrompts should return at least one prompt")

		// Find the cluster-health-check prompt
		var foundHealthCheck bool
		for _, prompt := range prompts {
			if prompt.Prompt.Name == "cluster-health-check" {
				foundHealthCheck = true

				// Verify prompt metadata
				s.Equal("cluster-health-check", prompt.Prompt.Name)
				s.Equal("Cluster Health Check", prompt.Prompt.Title)
				s.Contains(prompt.Prompt.Description, "comprehensive health assessment")

				// Verify arguments
				s.Require().Len(prompt.Prompt.Arguments, 2, "should have 2 arguments")

				// Check namespace argument
				s.Equal("namespace", prompt.Prompt.Arguments[0].Name)
				s.NotEmpty(prompt.Prompt.Arguments[0].Description)
				s.False(prompt.Prompt.Arguments[0].Required)

				// Check check_events argument
				s.Equal("check_events", prompt.Prompt.Arguments[1].Name)
				s.NotEmpty(prompt.Prompt.Arguments[1].Description)
				s.False(prompt.Prompt.Arguments[1].Required)

				// Verify handler is set
				s.NotNil(prompt.Handler, "handler should be set")

				break
			}
		}

		s.True(foundHealthCheck, "cluster-health-check prompt should be registered")
	})
}

func (s *ClusterHealthCheckSuite) TestPromptRBACMetadata() {
	prompt := (&Toolset{}).GetPrompts(s.T().Context(), api.ToolsetContext{})[0]

	s.Equal(api.RBACBounded(
		api.RBACRequirement{
			Verbs:  []string{"list"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "nodes"}},
		},
		api.RBACRequirement{
			Verbs:  []string{"get", "list"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "namespaces"}},
		},
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "pods"}},
			Namespace: &api.RBACNamespace{Argument: "namespace"},
		},
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "apps", Resource: "deployments"}},
			Namespace: &api.RBACNamespace{Argument: "namespace"},
		},
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "apps", Resource: "statefulsets"}},
			Namespace: &api.RBACNamespace{Argument: "namespace"},
		},
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{APIGroup: "apps", Resource: "daemonsets"}},
			Namespace: &api.RBACNamespace{Argument: "namespace"},
		},
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "persistentvolumeclaims"}},
			Namespace: &api.RBACNamespace{Argument: "namespace"},
		},
		api.RBACRequirement{
			Verbs: []string{"list"},
			Target: api.RBACTarget{Resource: &api.RBACResourceTarget{
				APIGroup: "config.openshift.io",
				Resource: "clusteroperators",
			}},
		},
		api.RBACRequirement{
			Verbs:     []string{"list"},
			Target:    api.RBACTarget{Resource: &api.RBACResourceTarget{Resource: "events"}},
			Namespace: &api.RBACNamespace{Argument: "namespace"},
		},
	), prompt.RBAC)
}

func (s *ClusterHealthCheckSuite) TestEventLinesStopAtTwenty() {
	var listed atomic.Int32
	var namespaces []runtime.Object
	namespaces = append(namespaces,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "openshift-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "openshift-b"}},
	)
	now := metav1.NewTime(time.Now())
	for i := range 25 {
		namespaces = append(namespaces, &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("e-%d", i), Namespace: "default"},
			Type:       corev1.EventTypeWarning,
			Reason:     "BackOff",
			Message:    "boom",
			Count:      1,
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod", Name: "p", Namespace: "default",
			},
			LastTimestamp: now,
		})
	}
	cs := fakeclient.NewSimpleClientset(namespaces...)
	cs.PrependReactor("list", "events", func(action clienttesting.Action) (bool, runtime.Object, error) {
		listed.Add(1)
		s.Equal("default", action.GetNamespace())
		return false, nil, nil
	})
	text, err := gatherEventDiagnostics(api.PromptHandlerParams{
		Context:          context.Background(),
		KubernetesClient: healthCoreClient{Interface: cs},
	}, "")
	s.Require().NoError(err)
	s.Equal(int32(1), listed.Load())
	s.Contains(text, "**Warnings:** 25")
	s.Equal(healthCheckMaxEvents, strings.Count(text, "\n- **"))
}

func (s *ClusterHealthCheckSuite) TestIssueLinesStopWhenTheSectionBudgetIsFull() {
	var nodeLists atomic.Int32
	long := strings.Repeat("x", 4000)
	node := func(name string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
				Type:    corev1.NodeDiskPressure,
				Status:  corev1.ConditionTrue,
				Message: long,
			}}},
		}
	}
	cs := fakeclient.NewSimpleClientset(
		node("n1"),
		node("n2"),
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)
	cs.PrependReactor("list", "nodes", func(clienttesting.Action) (bool, runtime.Object, error) {
		nodeLists.Add(1)
		return false, nil, nil
	})

	params := api.PromptHandlerParams{
		Context:          context.Background(),
		KubernetesClient: healthCoreClient{Interface: cs},
	}
	// Room for the totals line and the start of one issue block. The second
	// node's block is not formatted once that scratch is full.
	b := api.NewBuilderWithBudget(220)
	s.Require().NoError(gatherNodeDiagnostics(params, b))
	text := b.String()
	s.Equal(int32(1), nodeLists.Load())
	s.Contains(text, api.ResultTruncationNotice)
	s.NotContains(text, long)
	names := 0
	if strings.Contains(text, "n1") {
		names++
	}
	if strings.Contains(text, "n2") {
		names++
	}
	s.Equal(1, names)
}

func (s *ClusterHealthCheckSuite) TestLaterSectionsStopWhenTheBudgetIsFull() {
	var nodeLists atomic.Int32
	cs := fakeclient.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)
	cs.PrependReactor("list", "nodes", func(clienttesting.Action) (bool, runtime.Object, error) {
		nodeLists.Add(1)
		return false, nil, nil
	})

	params := api.PromptHandlerParams{
		KubernetesClient: healthCoreClient{Interface: cs},
	}
	chrome := api.NewBuilderWithBudget(0)
	writeHealthCheckPreamble(chrome, &clusterDiagnostics{
		CollectionTime:  time.Unix(0, 0).UTC(),
		TotalNamespaces: 1,
	})
	header := "## 1. Nodes\n\n"
	limit := int64(len(healthCheckAssistantText) + chrome.Len() + len(header))
	params.Context = api.WithResultBudget(context.Background(), limit)

	text := buildHealthCheckPrompt(params, "", "", false)
	s.Equal(int32(0), nodeLists.Load())
	s.NotContains(text, header)
}

func TestClusterHealthCheckSuite(t *testing.T) {
	suite.Run(t, new(ClusterHealthCheckSuite))
}

// healthCoreClient exposes a clientset to the health-check event test.
type healthCoreClient struct {
	kubernetes.Interface
}

func (healthCoreClient) NamespaceOrDefault(ns string) string { return ns }
func (healthCoreClient) ToRESTConfig() (*rest.Config, error) {
	return nil, errHealthCoreUnused
}
func (healthCoreClient) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return nil, errHealthCoreUnused
}
func (healthCoreClient) ToRESTMapper() (meta.RESTMapper, error) { return nil, errHealthCoreUnused }
func (healthCoreClient) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return nil
}
func (healthCoreClient) RESTConfig() *rest.Config                            { return nil }
func (healthCoreClient) RESTMapper() meta.ResettableRESTMapper               { return nil }
func (healthCoreClient) DiscoveryClient() discovery.CachedDiscoveryInterface { return nil }
func (healthCoreClient) RawDiscoveryClient() discovery.DiscoveryInterface    { return nil }
func (healthCoreClient) DynamicClient() dynamic.Interface                    { return nil }
func (healthCoreClient) MetricsV1beta1Client() *metricsv1beta1.MetricsV1beta1Client {
	return nil
}

var errHealthCoreUnused = errors.New("unused")

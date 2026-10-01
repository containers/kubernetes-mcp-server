package kubernetes_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type ImpersonationSuite struct {
	suite.Suite
}

func (s *ImpersonationSuite) TestClientPathsPreserveIdentityAndBackendAuthentication() {
	server := test.NewMockServer()
	s.T().Cleanup(server.Close)
	var mu sync.Mutex
	var received []http.Header
	server.Handle(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, r.Header.Clone())
	}))
	discovery := test.NewDiscoveryClientHandler(metav1.APIResourceList{
		GroupVersion: "metrics.k8s.io/v1beta1",
		APIResources: []metav1.APIResource{{Name: "pods", Kind: "PodMetrics", Namespaced: true, Verbs: metav1.Verbs{"list"}}},
	})
	discovery.APIResourceLists[0].APIResources = append(discovery.APIResourceLists[0].APIResources,
		metav1.APIResource{Name: "secrets", Kind: "Secret", Namespaced: true, Verbs: metav1.Verbs{"list"}})
	server.Handle(discovery)
	server.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/namespaces/alice/pods":
			test.WriteObject(w, &corev1.PodList{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"}, Items: []corev1.Pod{}})
		case "/api/v1/namespaces/alice/pods/example":
			test.WriteObject(w, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "example"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}}})
		case "/api/v1/namespaces/alice/pods/example/log":
			_, _ = io.WriteString(w, "application log")
		case "/api/v1/namespaces/alice/secrets":
			test.WriteObject(w, &corev1.SecretList{Items: []corev1.Secret{}})
		case "/version":
			_, _ = io.WriteString(w, `{"major":"1","minor":"35","gitVersion":"v1.35.0"}`)
		case "/apis/metrics.k8s.io/v1beta1/namespaces/alice/pods":
			_, _ = io.WriteString(w, `{"apiVersion":"metrics.k8s.io/v1beta1","kind":"PodMetricsList","items":[]}`)
		case "/api/v1/namespaces/alice/pods/example/exec":
			if r.URL.Query().Get("command") == "websocket" {
				upgrader := websocket.Upgrader{Subprotocols: []string{"v5.channel.k8s.io"}}
				connection, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = connection.Close() }()
				_ = connection.WriteMessage(websocket.BinaryMessage, append([]byte{1}, []byte("executed")...))
				_ = connection.WriteMessage(websocket.BinaryMessage, append([]byte{3}, []byte(`{"status":"Success"}`)...))
				_ = connection.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if r.Method == http.MethodGet {
				http.Error(w, "use SPDY", http.StatusBadRequest)
				return
			}
			streams, err := test.CreateHTTPStreams(w, r, &test.StreamOptions{Stdout: io.Discard, Stderr: io.Discard})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer func() { _ = streams.Close() }()
			_, _ = io.WriteString(streams.StdoutStream, "executed")
		}
	}))
	cfg := config.BaseDefault()
	cfg.ClusterAuthMode.SetForTest("impersonation")
	backend := rest.CopyConfig(server.Config())
	backend.BearerToken = "backend-credential"
	backend.Impersonate = rest.ImpersonationConfig{UserName: "old-user", Groups: []string{"old-admin-group"}, UID: "old-uid", Extra: map[string][]string{"old": {"value"}}}
	raw := server.Kubeconfig()
	raw.AuthInfos["fake"].Token = "backend-credential"
	manager, err := kubernetes.NewManager(s.T().Context(), cfg, backend, clientcmd.NewDefaultClientConfig(*raw, nil))
	s.Require().NoError(err)
	s.T().Cleanup(manager.Close)
	ctx, cancel := context.WithTimeout(s.T().Context(), 30*time.Second)
	s.T().Cleanup(cancel)
	ctx = context.WithValue(ctx, kubernetes.OAuthAuthorizationHeader, "Bearer caller-token-never-forwarded")
	ctx = kubernetes.WithImpersonationIdentity(ctx, kubernetes.ImpersonationIdentity{UserName: "alice", Groups: []string{"readers"}})
	client, err := manager.Derived(ctx)
	s.Require().NoError(err)
	core := kubernetes.NewCore(client)
	cases := []struct {
		name string
		run  func() error
	}{
		{"typed client", func() error { _, err := client.CoreV1().Pods("alice").List(ctx, metav1.ListOptions{}); return err }},
		{"dynamic client", func() error {
			_, err := client.DynamicClient().Resource(schema.GroupVersionResource{Version: "v1", Resource: "pods"}).Namespace("alice").List(ctx, metav1.ListOptions{})
			return err
		}},
		{"discovery without request context", func() error {
			client.DiscoveryClient().Invalidate()
			_, err := client.DiscoveryClient().ServerGroups()
			return err
		}},
		{"metrics", func() error {
			_, err := client.MetricsV1beta1Client().PodMetricses("alice").List(ctx, metav1.ListOptions{})
			return err
		}},
		{"logs", func() error {
			output, err := core.PodsLog(ctx, "alice", "example", "main", false, 10)
			s.Equal("application log", output)
			return err
		}},
		{"helm", func() error { _, err := helm.NewHelm(client, nil).List(ctx, "alice", false); return err }},
		{"websocket exec", func() error {
			output, _, err := core.PodsExec(ctx, "alice", "example", "main", []string{"websocket"})
			s.Equal("executed", output)
			return err
		}},
		{"SPDY fallback exec", func() error {
			output, _, err := core.PodsExec(ctx, "alice", "example", "main", []string{"spdy"})
			s.Equal("executed", output)
			return err
		}},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			mu.Lock()
			received = nil
			mu.Unlock()
			s.Require().NoError(tc.run())
			mu.Lock()
			defer mu.Unlock()
			s.Require().NotEmpty(received, "exercise an actual backend request")
			for _, headers := range received {
				s.Equal("Bearer backend-credential", headers.Get("Authorization"))
				s.Equal("alice", headers.Get("Impersonate-User"))
				s.Equal([]string{"readers"}, headers.Values("Impersonate-Group"))
				s.Empty(headers.Get("Impersonate-Uid"))
				for key := range headers {
					s.False(strings.HasPrefix(strings.ToLower(key), "impersonate-extra-"))
				}
			}
		})
	}
	s.Run("configuration output excludes backend credentials", func() {
		raw, err := client.ToRawKubeConfigLoader().RawConfig()
		s.Require().NoError(err)
		serialized, err := clientcmd.Write(raw)
		s.Require().NoError(err)
		s.NotContains(string(serialized), "backend-credential")
	})
}

func (s *ImpersonationSuite) TestMissingIdentityNeverFallsBackToBackendCredentials() {
	for _, authorization := range []string{"", "Bearer caller-token"} {
		s.Run(authorization, func() {
			cfg := config.BaseDefault()
			cfg.ClusterAuthMode.SetForTest("impersonation")
			manager, err := kubernetes.NewManager(s.T().Context(), cfg,
				&rest.Config{Host: "https://unused.example", BearerToken: "backend-token"},
				clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), nil))
			s.Require().NoError(err)
			s.T().Cleanup(manager.Close)
			ctx := context.WithValue(s.T().Context(), kubernetes.OAuthAuthorizationHeader, authorization)
			_, err = manager.Derived(ctx)
			s.Error(err, "a missing proxy identity must never yield a backend client")
		})
	}
}

func TestImpersonation(t *testing.T) {
	suite.Run(t, new(ImpersonationSuite))
}

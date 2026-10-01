package kubernetes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// TestImpersonationExecPluginHelper runs this test executable as a portable credential plugin.
func TestImpersonationExecPluginHelper(t *testing.T) {
	tokenFile := os.Getenv("KUBERNETES_MCP_EXEC_TEST_TOKEN_FILE")
	if tokenFile == "" {
		return
	}
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		os.Exit(1)
	}
	credential := clientauthenticationv1.ExecCredential{
		TypeMeta: metav1.TypeMeta{APIVersion: "client.authentication.k8s.io/v1", Kind: "ExecCredential"},
		Status: &clientauthenticationv1.ExecCredentialStatus{
			Token: string(token), ExpirationTimestamp: &metav1.Time{Time: time.Now().Add(time.Hour)},
		},
	}
	if err := json.NewEncoder(os.Stdout).Encode(credential); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

type ImpersonationExecSuite struct{ suite.Suite }

func (s *ImpersonationExecSuite) TestConcurrentClientsAndCredentialRefresh() {
	server := test.NewMockServer()
	s.T().Cleanup(server.Close)
	server.Handle(test.NewDiscoveryClientHandler())
	var expectedToken atomic.Value
	expectedToken.Store("first-backend-token")
	server.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/alice/pods" {
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+expectedToken.Load().(string) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Impersonate-User") != "alice" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		test.WriteObject(w, &corev1.PodList{Items: []corev1.Pod{}})
	}))
	plugin, err := os.Executable()
	s.Require().NoError(err)
	tokenFile := filepath.Join(s.T().TempDir(), "exec-token")
	s.Require().NoError(os.WriteFile(tokenFile, []byte(expectedToken.Load().(string)), 0600))
	backend := rest.CopyConfig(server.Config())
	backend.ExecProvider = &clientcmdapi.ExecConfig{
		Command: plugin, Args: []string{"-test.run=^TestImpersonationExecPluginHelper$"},
		APIVersion: "client.authentication.k8s.io/v1", InteractiveMode: clientcmdapi.NeverExecInteractiveMode,
		ProvideClusterInfo: true,
		Config:             &runtime.Unknown{Raw: []byte(`{"audience":"cluster"}`)},
		Env: []clientcmdapi.ExecEnvVar{
			{Name: "KUBERNETES_MCP_EXEC_TEST_TOKEN_FILE", Value: tokenFile},
			{Name: "GORACE", Value: "atexit_sleep_ms=0"},
		},
	}
	cfg := config.BaseDefault()
	cfg.ClusterAuthMode.SetForTest(config.ClusterAuthImpersonation)
	manager, err := kubernetes.NewManager(s.T().Context(), cfg, backend, clientcmd.NewDefaultClientConfig(*server.Kubeconfig(), nil))
	s.Require().NoError(err)
	s.T().Cleanup(manager.Close)
	ctx := kubernetes.WithImpersonationIdentity(s.T().Context(), kubernetes.ImpersonationIdentity{UserName: "alice"})
	start := make(chan struct{})
	results := make(chan error, 12)
	for range 12 {
		go func() {
			<-start
			requestCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			client, err := manager.Derived(requestCtx)
			if err == nil {
				_, err = client.CoreV1().Pods("alice").List(requestCtx, metav1.ListOptions{})
			}
			results <- err
		}()
	}
	close(start)
	for range 12 {
		s.Require().NoError(<-results)
	}
	client, err := manager.Derived(ctx)
	s.Require().NoError(err)
	expectedToken.Store("renewed-backend-token")
	s.Require().NoError(os.WriteFile(tokenFile, []byte(expectedToken.Load().(string)), 0600))
	_, err = client.CoreV1().Pods("alice").List(ctx, metav1.ListOptions{})
	s.True(apierrors.IsUnauthorized(err), "a rejected backend credential must trigger plugin refresh: %v", err)
	_, err = client.CoreV1().Pods("alice").List(ctx, metav1.ListOptions{})
	s.NoError(err, "renewed exec credentials must retain the impersonated identity")
}

func TestImpersonationExec(t *testing.T) {
	suite.Run(t, new(ImpersonationExecSuite))
}

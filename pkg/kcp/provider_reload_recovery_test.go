package kcp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes/watcher"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

type ProviderReloadRecoveryTestSuite struct {
	suite.Suite
	dynamicClient dynamic.Interface
	provider      *kcpClusterProvider
	attempts      <-chan error
}

func (s *ProviderReloadRecoveryTestSuite) SetupTest() {
	s.dynamicClient = dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{WorkspaceGVR: "WorkspaceList"},
	)

	cfg := config.New()
	cfg.KubeConfig.SetForTest(filepath.Join(s.T().TempDir(), "missing-kubeconfig"))
	s.provider = &kcpClusterProvider{
		cfg:      cfg,
		managers: map[string]*kubernetes.Manager{},
	}
	s.provider.SetReset(s.provider.reset)
	s.Require().NoError(s.provider.Replace(
		NewWorkspaceWatcher(s.T().Context(), s.dynamicClient, "root", 100*time.Millisecond, 10*time.Millisecond),
		watcher.NewClusterStateForTargets(
			s.T().Context(),
			func(context.Context) map[string]discovery.CachedDiscoveryInterface { return nil },
			time.Hour,
			time.Hour,
		),
	))
	s.T().Cleanup(s.provider.Close)

	attempts := make(chan error, 16)
	s.attempts = attempts
	s.provider.WatchTargets(s.T().Context(), kubernetes.NewMcpReloader(func(fn func() error) error {
		err := fn()
		attempts <- err
		return err
	}, nil))
}

func (s *ProviderReloadRecoveryTestSuite) TestWorkspaceWatcherContinuesAfterResetFailure() {
	s.createWorkspace("first")
	s.Require().Error(s.nextReloadAttempt())

	s.createWorkspace("second")
	s.Require().Error(s.nextReloadAttempt())
}

func (s *ProviderReloadRecoveryTestSuite) createWorkspace(name string) {
	workspace := &unstructured.Unstructured{}
	workspace.SetAPIVersion(WorkspaceGVR.Group + "/" + WorkspaceGVR.Version)
	workspace.SetKind("Workspace")
	workspace.SetName(name)
	_, err := s.dynamicClient.Resource(WorkspaceGVR).Create(s.T().Context(), workspace, metav1.CreateOptions{})
	s.Require().NoError(err)
}

func (s *ProviderReloadRecoveryTestSuite) nextReloadAttempt() error {
	select {
	case err := <-s.attempts:
		return err
	case <-time.After(5 * time.Second):
		s.T().Fatal("timeout waiting for reload attempt")
		return nil
	}
}

func TestProviderReloadRecovery(t *testing.T) {
	suite.Run(t, new(ProviderReloadRecoveryTestSuite))
}

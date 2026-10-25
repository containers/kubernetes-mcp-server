package mcp

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func TestMain(m *testing.M) {
	// Initialize the envtest environment once for this test package. Go runs each
	// package's tests as a separate process, so this TestMain and the resulting
	// envtest instance are scoped to the mcp package alone (see
	// internal/test/envtest.go); other packages initialize their own.
	test.EnvTest()

	// Create test data specific to mcp tests
	ctx := context.Background()
	restoreAuth(ctx)
	createTestData(ctx)

	// Run all tests
	code := m.Run()

	// Tear down
	_ = test.StopEnvTest()
	os.Exit(code)
}

func restoreAuth(ctx context.Context) {
	kubernetesAdmin := kubernetes.NewForConfigOrDie(test.EnvTest().Config)
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-all"},
		Rules: []rbacv1.PolicyRule{{
			Verbs:     []string{"*"},
			APIGroups: []string{"*"},
			Resources: []string{"*"},
		}},
	}
	if _, err := kubernetesAdmin.RbacV1().ClusterRoles().Update(ctx, role, metav1.UpdateOptions{}); apierrors.IsNotFound(err) {
		_, _ = kubernetesAdmin.RbacV1().ClusterRoles().Create(ctx, role, metav1.CreateOptions{})
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-all"},
		Subjects:   []rbacv1.Subject{{Kind: "Group", Name: "test:users"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "allow-all"},
	}
	if _, err := kubernetesAdmin.RbacV1().ClusterRoleBindings().Update(ctx, binding, metav1.UpdateOptions{}); apierrors.IsNotFound(err) {
		binding.RoleRef.APIGroup = "rbac.authorization.k8s.io"
		_, _ = kubernetesAdmin.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
	}
}

// revokeAllowAll deletes the test user's cluster role and waits until the
// apiserver authorizer agrees. The authorizer watches RBAC and is briefly
// stale after the delete; on windows-latest that window was long enough for
// namespaces_list to succeed and the forbidden assertion to fail.
func revokeAllowAll(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	admin := kubernetes.NewForConfigOrDie(test.EnvTest().Config)
	user := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		err := admin.RbacV1().ClusterRoles().Delete(ctx, "allow-all", metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			last = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		_, err = user.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
		if apierrors.IsForbidden(err) {
			return
		}
		last = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("test user can still list namespaces after deleting allow-all: %v", last)
}

func createTestData(ctx context.Context) {
	kubernetesAdmin := kubernetes.NewForConfigOrDie(test.EnvTestRestConfig())
	// Namespaces
	_, _ = kubernetesAdmin.CoreV1().Namespaces().
		Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-1"}}, metav1.CreateOptions{})
	_, _ = kubernetesAdmin.CoreV1().Namespaces().
		Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-2"}}, metav1.CreateOptions{})
	_, _ = kubernetesAdmin.CoreV1().Namespaces().
		Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-to-delete"}}, metav1.CreateOptions{})
	_, _ = kubernetesAdmin.CoreV1().Pods("default").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "a-pod-in-default",
			Labels: map[string]string{"app": "nginx"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}, metav1.CreateOptions{})
	// Pods for listing
	_, _ = kubernetesAdmin.CoreV1().Pods("ns-1").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "a-pod-in-ns-1",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}, metav1.CreateOptions{})
	_, _ = kubernetesAdmin.CoreV1().Pods("ns-2").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "a-pod-in-ns-2",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}, metav1.CreateOptions{})
	_, _ = kubernetesAdmin.CoreV1().ConfigMaps("default").
		Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "a-configmap-to-delete"}}, metav1.CreateOptions{})
}

type BaseMcpSuite struct {
	suite.Suite
	*test.McpClient
	mcpServer *Server
	provider  internalk8s.Provider
	Cfg       *config.Config
}

func (s *BaseMcpSuite) SetupTest() {
	s.Cfg = config.BaseDefault()
	s.Cfg.ListOutput.SetForTest("yaml")
	s.Cfg.KubeConfig.SetForTest(test.EnvTestKubeconfigFile(s.T()))
	test.ApplyEnvtestClientLimits(s.Cfg)
}

func (s *BaseMcpSuite) TearDownTest() {
	if s.McpClient != nil {
		s.Close()
	}
	if s.mcpServer != nil {
		s.mcpServer.Close()
	}
	if s.provider != nil {
		s.provider.Close()
	}
}

func (s *BaseMcpSuite) InitMcpClient(options ...test.McpClientOption) {
	var err error
	s.provider, err = internalk8s.NewProvider(s.T().Context(), s.Cfg)
	s.Require().NoError(err, "Expected no error creating k8s provider")
	s.mcpServer, err = NewServer(s.T().Context(), Configuration{Config: s.Cfg}, s.provider)
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP(), options...)
}

// StartCapturingLogNotifications begins capturing log notifications.
// Must be called BEFORE the tool call that triggers the notification.
// This method sets the logging level to debug to ensure all log messages are received.
func (s *BaseMcpSuite) StartCapturingLogNotifications() *test.NotificationCapture {
	// Set logging level to debug to receive all log messages
	err := s.SetLoggingLevel(mcp.LoggingLevel("debug")) //nolint:staticcheck // MCP logging deprecated (SEP-2577)
	s.Require().NoError(err, "failed to set logging level")

	return s.StartCapturingNotifications()
}

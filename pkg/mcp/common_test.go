package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

func TestMain(m *testing.M) {
	// Initialize shared envtest environment once for all test packages.
	// This runs in the mcp package but envtest is shared via internal/test
	// so kubernetes, http, watcher packages all use the same instance.
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
	// Authorization
	_, _ = kubernetesAdmin.RbacV1().ClusterRoles().Update(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-all"},
		Rules: []rbacv1.PolicyRule{{
			Verbs:     []string{"*"},
			APIGroups: []string{"*"},
			Resources: []string{"*"},
		}},
	}, metav1.UpdateOptions{})
	_, _ = kubernetesAdmin.RbacV1().ClusterRoleBindings().Update(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-all"},
		Subjects:   []rbacv1.Subject{{Kind: "Group", Name: "test:users"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "allow-all"},
	}, metav1.UpdateOptions{})
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
	Cfg       *config.StaticConfig
}

func (s *BaseMcpSuite) SetupTest() {
	s.Cfg = config.BaseDefault()
	s.Cfg.ListOutput = "yaml"
	s.Cfg.KubeConfig = test.EnvTestKubeconfigFile(s.T())
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
	s.mcpServer, err = NewServer(s.T().Context(), Configuration{StaticConfig: s.Cfg}, s.provider)
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP(), options...)
}

// StartCapturingLogNotifications begins capturing log notifications.
// Must be called BEFORE the tool call that triggers the notification.
// This method sets the logging level to debug to ensure all log messages are received.
func (s *BaseMcpSuite) StartCapturingLogNotifications() *test.NotificationCapture {
	// Set logging level to debug to receive all log messages
	err := s.SetLoggingLevel(mcp.LoggingLevel("debug"))
	s.Require().NoError(err, "failed to set logging level")

	return s.StartCapturingNotifications()
}

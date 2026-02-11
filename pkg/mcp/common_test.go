package mcp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1spec "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/remote"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/store"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/workflows"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
)

// envTest has an expensive setup, so we only want to do it once per entire test run.
var envTest *envtest.Environment
var envTestRestConfig *rest.Config
var envTestUser = envtest.User{Name: "test-user", Groups: []string{"test:users"}}

func TestMain(m *testing.M) {
	// Set up
	_ = os.Setenv("KUBECONFIG", "/dev/null")     // Avoid interference from existing kubeconfig
	_ = os.Setenv("KUBERNETES_SERVICE_HOST", "") // Avoid interference from in-cluster config
	_ = os.Setenv("KUBERNETES_SERVICE_PORT", "") // Avoid interference from in-cluster config
	// Set high rate limits to avoid client-side throttling in tests
	_ = os.Setenv("KUBE_CLIENT_QPS", "1000")
	_ = os.Setenv("KUBE_CLIENT_BURST", "2000")
	//// Enable control plane output to see API server logs
	//_ = os.Setenv("KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT", "true")
	envTestDir, err := store.DefaultStoreDir()
	if err != nil {
		panic(err)
	}
	envTestEnv := &env.Env{
		FS:  afero.Afero{Fs: afero.NewOsFs()},
		Out: os.Stdout,
		Client: &remote.HTTPClient{
			IndexURL: remote.DefaultIndexURL,
		},
		Platform: versions.PlatformItem{
			Platform: versions.Platform{
				OS:   runtime.GOOS,
				Arch: runtime.GOARCH,
			},
		},
		Version: versions.AnyVersion,
		Store:   store.NewAt(envTestDir),
	}
	envTestEnv.CheckCoherence()
	workflows.Use{}.Do(envTestEnv)
	versionDir := envTestEnv.Platform.BaseName(*envTestEnv.Version.AsConcrete())
	envTest = &envtest.Environment{
		BinaryAssetsDirectory: filepath.Join(envTestDir, "k8s", versionDir),
		CRDs: []*apiextensionsv1spec.CustomResourceDefinition{
			// OpenShift
			CRD("project.openshift.io", "v1", "projects", "Project", "project", false),
			CRD("route.openshift.io", "v1", "routes", "Route", "route", true),
			// Kubevirt
			CRD("kubevirt.io", "v1", "virtualmachines", "VirtualMachine", "virtualmachine", true),
			CRD("cdi.kubevirt.io", "v1beta1", "datasources", "DataSource", "datasource", true),
			CRD("instancetype.kubevirt.io", "v1beta1", "virtualmachineclusterinstancetypes", "VirtualMachineClusterInstancetype", "virtualmachineclusterinstancetype", false),
			CRD("instancetype.kubevirt.io", "v1beta1", "virtualmachineinstancetypes", "VirtualMachineInstancetype", "virtualmachineinstancetype", true),
			CRD("instancetype.kubevirt.io", "v1beta1", "virtualmachineclusterpreferences", "VirtualMachineClusterPreference", "virtualmachineclusterpreference", false),
			CRD("instancetype.kubevirt.io", "v1beta1", "virtualmachinepreferences", "VirtualMachinePreference", "virtualmachinepreference", true),
		},
	}
	// Configure API server for faster CRD establishment and test performance
	envTest.ControlPlane.GetAPIServer().Configure().
		// Increase concurrent request limits for faster parallel operations
		Set("max-requests-inflight", "1000").
		Set("max-mutating-requests-inflight", "500").
		// Speed up namespace cleanup with more workers
		Set("delete-collection-workers", "10") //.
	// Enable verbose logging for debugging
	//Set("v", "9")

	adminSystemMasterBaseConfig, _ := envTest.Start()
	au := test.Must(envTest.AddUser(envTestUser, adminSystemMasterBaseConfig))
	envTestRestConfig = au.Config()
	envTest.KubeConfig = test.Must(au.KubeConfig())

	//Create test data as administrator
	ctx := context.Background()
	restoreAuth(ctx)
	createTestData(ctx)

	// Test!
	code := m.Run()

	// Tear down
	if envTest != nil {
		_ = envTest.Stop()
	}
	os.Exit(code)
}

func restoreAuth(ctx context.Context) {
	kubernetesAdmin := kubernetes.NewForConfigOrDie(envTest.Config)
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
		Subjects:   []rbacv1.Subject{{Kind: "Group", Name: envTestUser.Groups[0]}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "allow-all"},
	}, metav1.UpdateOptions{})
}

func createTestData(ctx context.Context) {
	kubernetesAdmin := kubernetes.NewForConfigOrDie(envTestRestConfig)
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
	Cfg       *config.StaticConfig
}

func (s *BaseMcpSuite) SetupTest() {
	s.Cfg = config.Default()
	s.Cfg.ListOutput = "yaml"
	s.Cfg.KubeConfig = filepath.Join(s.T().TempDir(), "config")
	s.Require().NoError(os.WriteFile(s.Cfg.KubeConfig, envTest.KubeConfig, 0600), "Expected to write kubeconfig")
}

func (s *BaseMcpSuite) TearDownTest() {
	if s.McpClient != nil {
		s.Close()
	}
	if s.mcpServer != nil {
		s.mcpServer.Close()
	}
}

func (s *BaseMcpSuite) InitMcpClient(options ...test.McpClientOption) {
	provider, err := internalk8s.NewProvider(s.Cfg)
	s.Require().NoError(err, "Expected no error creating k8s provider")
	s.mcpServer, err = NewServer(Configuration{StaticConfig: s.Cfg}, provider)
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP(), options...)
}

// StartCapturingNotifications begins capturing all MCP notifications.
// Must be called BEFORE the operation that triggers the notification.
func (s *BaseMcpSuite) StartCapturingNotifications() *test.NotificationCapture {
	return s.McpClient.StartCapturingNotifications()
}

// StartCapturingLogNotifications begins capturing log notifications.
// Must be called BEFORE the tool call that triggers the notification.
// This method sets the logging level to debug to ensure all log messages are received.
func (s *BaseMcpSuite) StartCapturingLogNotifications() *test.NotificationCapture {
	// Set logging level to debug to receive all log messages
	err := s.SetLevel(s.T().Context(), mcp.SetLevelRequest{
		Params: mcp.SetLevelParams{
			Level: mcp.LoggingLevelDebug,
		},
	})
	s.Require().NoError(err, "failed to set logging level")

	return s.StartCapturingNotifications()
}

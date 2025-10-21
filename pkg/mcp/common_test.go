package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1spec "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	toolswatch "k8s.io/client-go/tools/watch"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/remote"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/store"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/workflows"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
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
	}
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

func (s *BaseMcpSuite) InitMcpClient(options ...transport.StreamableHTTPCOption) {
	var err error
	s.mcpServer, err = NewServer(Configuration{StaticConfig: s.Cfg})
	s.Require().NoError(err, "Expected no error creating MCP server")
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP(), options...)
}

// EnvTestInOpenShift sets up the kubernetes environment to seem to be running OpenShift
func EnvTestInOpenShift(ctx context.Context) error {
	crdTemplate := `
          {
            "apiVersion": "apiextensions.k8s.io/v1",
            "kind": "CustomResourceDefinition",
            "metadata": {"name": "%s"},
            "spec": {
              "group": "%s",
              "versions": [{
                "name": "v1","served": true,"storage": true,
                "schema": {"openAPIV3Schema": {"type": "object","x-kubernetes-preserve-unknown-fields": true}}
              }],
              "scope": "%s",
              "names": {"plural": "%s","singular": "%s","kind": "%s"}
            }
          }`
	tasks, _ := errgroup.WithContext(ctx)
	tasks.Go(func() error {
		return EnvTestCrdApply(ctx, fmt.Sprintf(crdTemplate, "projects.project.openshift.io", "project.openshift.io",
			"Cluster", "projects", "project", "Project"))
	})
	tasks.Go(func() error {
		return EnvTestCrdApply(ctx, fmt.Sprintf(crdTemplate, "routes.route.openshift.io", "route.openshift.io",
			"Namespaced", "routes", "route", "Route"))
	})
	return tasks.Wait()
}

// EnvTestInOpenShiftClear clears the kubernetes environment so it no longer seems to be running OpenShift
func EnvTestInOpenShiftClear(ctx context.Context) error {
	tasks, _ := errgroup.WithContext(ctx)
	tasks.Go(func() error { return EnvTestCrdDelete(ctx, "projects.project.openshift.io") })
	tasks.Go(func() error { return EnvTestCrdDelete(ctx, "routes.route.openshift.io") })
	return tasks.Wait()
}

// EnvTestCrdWaitUntilReady waits for a CRD to be established
func EnvTestCrdWaitUntilReady(ctx context.Context, name string) error {
	apiExtensionClient := apiextensionsv1.NewForConfigOrDie(envTestRestConfig)
	watcher, err := apiExtensionClient.CustomResourceDefinitions().Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + name,
	})
	if err != nil {
		return fmt.Errorf("unable to watch CRDs: %w", err)
	}
	_, err = toolswatch.UntilWithoutRetry(ctx, watcher, func(event watch.Event) (bool, error) {
		for _, c := range event.Object.(*apiextensionsv1spec.CustomResourceDefinition).Status.Conditions {
			if c.Type == apiextensionsv1spec.Established && c.Status == apiextensionsv1spec.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to wait for CRD: %w", err)
	}
	return nil
}

// EnvTestCrdApply creates a CRD from the provided resource string and waits for it to be established
func EnvTestCrdApply(ctx context.Context, resource string) error {
	apiExtensionsV1Client := apiextensionsv1.NewForConfigOrDie(envTestRestConfig)
	var crd = &apiextensionsv1spec.CustomResourceDefinition{}
	err := json.Unmarshal([]byte(resource), crd)
	if err != nil {
		return fmt.Errorf("failed to create CRD %v", err)
	}
	_, err = apiExtensionsV1Client.CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create CRD %v", err)
	}
	return EnvTestCrdWaitUntilReady(ctx, crd.Name)
}

// crdDelete deletes a CRD by name and waits for it to be removed
func EnvTestCrdDelete(ctx context.Context, name string) error {
	apiExtensionsV1Client := apiextensionsv1.NewForConfigOrDie(envTestRestConfig)
	err := apiExtensionsV1Client.CustomResourceDefinitions().Delete(ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: ptr.To(int64(0)),
	})
	iteration := 0
	for iteration < 100 {
		if _, derr := apiExtensionsV1Client.CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{}); derr != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
		iteration++
	}
	if err != nil {
		return errors.Wrap(err, "failed to delete CRD")
	}
	return nil
}

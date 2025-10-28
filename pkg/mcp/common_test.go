package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	toolswatch "k8s.io/client-go/tools/watch"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/textlogger"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/remote"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/store"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/workflows"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
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

type mcpContext struct {
	toolsets   []string
	listOutput output.Output
	logLevel   int

	staticConfig  *config.StaticConfig
	clientOptions []transport.ClientOption
	before        func(*mcpContext)
	after         func(*mcpContext)
	ctx           context.Context
	tempDir       string
	cancel        context.CancelFunc
	mcpServer     *Server
	mcpHttpServer *httptest.Server
	mcpClient     *client.Client
	klogState     klog.State
	logBuffer     bytes.Buffer
}

func (c *mcpContext) beforeEach(t *testing.T) {
	var err error
	c.ctx, c.cancel = context.WithCancel(t.Context())
	c.tempDir = t.TempDir()
	c.withKubeConfig(nil)
	if c.staticConfig == nil {
		c.staticConfig = config.Default()
		// Default to use YAML output for lists (previously the default)
		c.staticConfig.ListOutput = "yaml"
	}
	if c.toolsets != nil {
		c.staticConfig.Toolsets = c.toolsets

	}
	if c.listOutput != nil {
		c.staticConfig.ListOutput = c.listOutput.GetName()
	}
	if c.before != nil {
		c.before(c)
	}
	// Set up logging
	c.klogState = klog.CaptureState()
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	klog.InitFlags(flags)
	_ = flags.Set("v", strconv.Itoa(c.logLevel))
	klog.SetLogger(textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(c.logLevel), textlogger.Output(&c.logBuffer))))
	// MCP Server
	if c.mcpServer, err = NewServer(Configuration{StaticConfig: c.staticConfig}); err != nil {
		t.Fatal(err)
		return
	}
	c.mcpHttpServer = server.NewTestServer(c.mcpServer.server, server.WithSSEContextFunc(contextFunc))
	if c.mcpClient, err = client.NewSSEMCPClient(c.mcpHttpServer.URL+"/sse", c.clientOptions...); err != nil {
		t.Fatal(err)
		return
	}
	// MCP Client
	if err = c.mcpClient.Start(c.ctx); err != nil {
		t.Fatal(err)
		return
	}
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{Name: "test", Version: "1.33.7"}
	_, err = c.mcpClient.Initialize(c.ctx, initRequest)
	if err != nil {
		t.Fatal(err)
		return
	}
}

func (c *mcpContext) afterEach() {
	if c.after != nil {
		c.after(c)
	}
	c.cancel()
	c.mcpServer.Close()
	_ = c.mcpClient.Close()
	c.mcpHttpServer.Close()
	c.klogState.Restore()
}

func testCase(t *testing.T, test func(c *mcpContext)) {
	testCaseWithContext(t, &mcpContext{}, test)
}

func testCaseWithContext(t *testing.T, mcpCtx *mcpContext, test func(c *mcpContext)) {
	mcpCtx.beforeEach(t)
	defer mcpCtx.afterEach()
	test(mcpCtx)
}

// withKubeConfig sets up a fake kubeconfig in the temp directory based on the provided rest.Config
func (c *mcpContext) withKubeConfig(rc *rest.Config) *clientcmdapi.Config {
	fakeConfig := clientcmdapi.NewConfig()
	fakeConfig.Clusters["fake"] = clientcmdapi.NewCluster()
	fakeConfig.Clusters["fake"].Server = "https://127.0.0.1:6443"
	fakeConfig.Clusters["additional-cluster"] = clientcmdapi.NewCluster()
	fakeConfig.AuthInfos["fake"] = clientcmdapi.NewAuthInfo()
	fakeConfig.AuthInfos["additional-auth"] = clientcmdapi.NewAuthInfo()
	if rc != nil {
		fakeConfig.Clusters["fake"].Server = rc.Host
		fakeConfig.Clusters["fake"].CertificateAuthorityData = rc.CAData
		fakeConfig.AuthInfos["fake"].ClientKeyData = rc.KeyData
		fakeConfig.AuthInfos["fake"].ClientCertificateData = rc.CertData
	}
	fakeConfig.Contexts["fake-context"] = clientcmdapi.NewContext()
	fakeConfig.Contexts["fake-context"].Cluster = "fake"
	fakeConfig.Contexts["fake-context"].AuthInfo = "fake"
	fakeConfig.Contexts["additional-context"] = clientcmdapi.NewContext()
	fakeConfig.Contexts["additional-context"].Cluster = "additional-cluster"
	fakeConfig.Contexts["additional-context"].AuthInfo = "additional-auth"
	fakeConfig.CurrentContext = "fake-context"
	kubeConfig := filepath.Join(c.tempDir, "config")
	_ = clientcmd.WriteToFile(*fakeConfig, kubeConfig)
	_ = os.Setenv("KUBECONFIG", kubeConfig)
	if c.mcpServer != nil {
		if err := c.mcpServer.reloadKubernetesClusterProvider(); err != nil {
			panic(err)
		}
	}
	return fakeConfig
}

// withEnvTest sets up the environment for kubeconfig to be used with envTest
func (c *mcpContext) withEnvTest() {
	c.withKubeConfig(envTestRestConfig)
}

// inOpenShift sets up the kubernetes environment to seem to be running OpenShift
func inOpenShift(c *mcpContext) {
	c.withEnvTest()
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
	tasks, _ := errgroup.WithContext(c.ctx)
	tasks.Go(func() error {
		return c.crdApply(fmt.Sprintf(crdTemplate, "projects.project.openshift.io", "project.openshift.io",
			"Cluster", "projects", "project", "Project"))
	})
	tasks.Go(func() error {
		return c.crdApply(fmt.Sprintf(crdTemplate, "routes.route.openshift.io", "route.openshift.io",
			"Namespaced", "routes", "route", "Route"))
	})
	if err := tasks.Wait(); err != nil {
		panic(err)
	}
}

// inOpenShiftClear clears the kubernetes environment so it no longer seems to be running OpenShift
func inOpenShiftClear(c *mcpContext) {
	tasks, _ := errgroup.WithContext(c.ctx)
	tasks.Go(func() error { return c.crdDelete("projects.project.openshift.io") })
	tasks.Go(func() error { return c.crdDelete("routes.route.openshift.io") })
	if err := tasks.Wait(); err != nil {
		panic(err)
	}
}

// newKubernetesClient creates a new Kubernetes client with the envTest kubeconfig
func (c *mcpContext) newKubernetesClient() *kubernetes.Clientset {
	return kubernetes.NewForConfigOrDie(envTestRestConfig)
}

// newApiExtensionsClient creates a new ApiExtensions client with the envTest kubeconfig
func (c *mcpContext) newApiExtensionsClient() *apiextensionsv1.ApiextensionsV1Client {
	return apiextensionsv1.NewForConfigOrDie(envTestRestConfig)
}

// crdApply creates a CRD from the provided resource string and waits for it to be established
func (c *mcpContext) crdApply(resource string) error {
	apiExtensionsV1Client := c.newApiExtensionsClient()
	var crd = &apiextensionsv1spec.CustomResourceDefinition{}
	err := json.Unmarshal([]byte(resource), crd)
	if err != nil {
		return fmt.Errorf("failed to create CRD %v", err)
	}
	_, err = apiExtensionsV1Client.CustomResourceDefinitions().Create(c.ctx, crd, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create CRD %v", err)
	}
	c.crdWaitUntilReady(crd.Name)
	return nil
}

// crdDelete deletes a CRD by name and waits for it to be removed
func (c *mcpContext) crdDelete(name string) error {
	apiExtensionsV1Client := c.newApiExtensionsClient()
	err := apiExtensionsV1Client.CustomResourceDefinitions().Delete(c.ctx, name, metav1.DeleteOptions{
		GracePeriodSeconds: ptr.To(int64(0)),
	})
	iteration := 0
	for iteration < 100 {
		if _, derr := apiExtensionsV1Client.CustomResourceDefinitions().Get(c.ctx, name, metav1.GetOptions{}); derr != nil {
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

// crdWaitUntilReady waits for a CRD to be established
func (c *mcpContext) crdWaitUntilReady(name string) {
	watcher, err := c.newApiExtensionsClient().CustomResourceDefinitions().Watch(c.ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + name,
	})
	if err != nil {
		panic(fmt.Errorf("failed to watch CRD %v", err))
	}
	_, err = toolswatch.UntilWithoutRetry(c.ctx, watcher, func(event watch.Event) (bool, error) {
		for _, c := range event.Object.(*apiextensionsv1spec.CustomResourceDefinition).Status.Conditions {
			if c.Type == apiextensionsv1spec.Established && c.Status == apiextensionsv1spec.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		panic(fmt.Errorf("failed to wait for CRD %v", err))
	}
}

// callTool helper function to call a tool by name with arguments
func (c *mcpContext) callTool(name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
	callToolRequest := mcp.CallToolRequest{}
	callToolRequest.Params.Name = name
	callToolRequest.Params.Arguments = args
	return c.mcpClient.CallTool(c.ctx, callToolRequest)
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
	s.McpClient = test.NewMcpClient(s.T(), s.mcpServer.ServeHTTP(nil), options...)
}

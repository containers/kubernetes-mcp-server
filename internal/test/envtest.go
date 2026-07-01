package test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1spec "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/env"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/remote"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/store"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
	"sigs.k8s.io/controller-runtime/tools/setup-envtest/workflows"
)

var (
	envTestOnce       sync.Once
	envTestInstance   *envtest.Environment
	envTestRestConfig *rest.Config
	envTestUser       = envtest.User{Name: "test-user", Groups: []string{"test:users"}}
)

// EnvTest returns a shared envtest.Environment instance, initializing it on first call.
// The environment includes CRDs for OpenShift (Project, Route) and KubeVirt resources.
// Each test package process gets its own envtest instance with isolated etcd data directory.
func EnvTest() *envtest.Environment {
	envTestOnce.Do(func() {
		// Set up environment variables
		_ = os.Setenv("KUBECONFIG", "/dev/null")     // Avoid interference from existing kubeconfig
		_ = os.Setenv("KUBERNETES_SERVICE_HOST", "") // Avoid interference from in-cluster config
		_ = os.Setenv("KUBERNETES_SERVICE_PORT", "") // Avoid interference from in-cluster config
		// Set high rate limits to avoid client-side throttling in tests
		_ = os.Setenv("KUBE_CLIENT_QPS", "1000")
		_ = os.Setenv("KUBE_CLIENT_BURST", "2000")

		// Use a unique etcd data directory per process to avoid conflicts when
		// test packages run in parallel
		etcdDataDir, err := os.MkdirTemp("", "envtest-etcd-*")
		if err != nil {
			panic(err)
		}

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

		envTestInstance = &envtest.Environment{
			BinaryAssetsDirectory: filepath.Join(envTestDir, "k8s", versionDir),
			// Use random ports and isolated etcd data dir to avoid conflicts when test packages run in parallel
			ControlPlane: envtest.ControlPlane{
				APIServer: &envtest.APIServer{},
				Etcd: &envtest.Etcd{
					DataDir: etcdDataDir,
				},
			},
			CRDs: []*apiextensionsv1spec.CustomResourceDefinition{
				// OpenShift
				CRD("project.openshift.io", "v1", "projects", "Project", "project", false),
				CRD("route.openshift.io", "v1", "routes", "Route", "route", true),
				// Kubevirt
				CRD("kubevirt.io", "v1", "virtualmachines", "VirtualMachine", "virtualmachine", true),
				CRD("kubevirt.io", "v1", "virtualmachineinstances", "VirtualMachineInstance", "virtualmachineinstance", true),
				CRD("clone.kubevirt.io", "v1beta1", "virtualmachineclones", "VirtualMachineClone", "virtualmachineclone", true),
				CRD("cdi.kubevirt.io", "v1beta1", "datasources", "DataSource", "datasource", true),
				CRD("instancetype.kubevirt.io", "v1beta1", "virtualmachineclusterinstancetypes", "VirtualMachineClusterInstancetype", "virtualmachineclusterinstancetype", false),
				CRD("instancetype.kubevirt.io", "v1beta1", "virtualmachineinstancetypes", "VirtualMachineInstancetype", "virtualmachineinstancetype", true),
				CRD("instancetype.kubevirt.io", "v1beta1", "virtualmachineclusterpreferences", "VirtualMachineClusterPreference", "virtualmachineclusterpreference", false),
				CRD("instancetype.kubevirt.io", "v1beta1", "virtualmachinepreferences", "VirtualMachinePreference", "virtualmachinepreference", true),
			},
		}

		// Configure API server for faster CRD establishment and test performance
		envTestInstance.ControlPlane.GetAPIServer().Configure().
			Set("max-requests-inflight", "1000").
			Set("max-mutating-requests-inflight", "500").
			Set("delete-collection-workers", "10")

		adminSystemMasterBaseConfig, err := envTestInstance.Start()
		if err != nil {
			panic(err)
		}

		au, err := envTestInstance.AddUser(envTestUser, adminSystemMasterBaseConfig)
		if err != nil {
			panic(err)
		}
		envTestRestConfig = au.Config()
		envTestInstance.KubeConfig, err = au.KubeConfig()
		if err != nil {
			panic(err)
		}

		// Create test data as administrator
		kc := Must(kubernetes.NewForConfig(adminSystemMasterBaseConfig))
		createTestData(kc)
	})
	return envTestInstance
}

// EnvTestRestConfig returns the rest.Config for the shared envtest environment.
func EnvTestRestConfig() *rest.Config {
	EnvTest() // Ensure initialized
	return envTestRestConfig
}

// EnvTestKubeconfigFile returns a kubeconfig file path for the shared envtest environment.
func EnvTestKubeconfigFile(t interface {
	TempDir() string
	Fatal(...interface{})
}) string {
	env := EnvTest()
	kubeconfigPath := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, env.KubeConfig, 0600); err != nil {
		t.Fatal(err)
	}
	return kubeconfigPath
}

// EnvTestUser returns the test user for the shared envtest environment.
func EnvTestUser() *envtest.User {
	EnvTest() // Ensure initialized
	return &envTestUser
}

// StopEnvTest stops the shared envtest environment if it was started.
// This should typically be called from TestMain after all tests complete.
func StopEnvTest() error {
	if envTestInstance != nil {
		err := envTestInstance.Stop()
		// Null the instance/config so any stray EnvTest() call after Stop() fails
		// loudly instead of handing out a stopped environment. This does NOT re-arm
		// envTestOnce, so EnvTest() cannot reinitialize within the same process; it
		// is safe only because StopEnvTest runs from TestMain after m.Run(), and each
		// test package runs as its own process.
		envTestInstance = nil
		envTestRestConfig = nil
		return err
	}
	return nil
}

// CRD creates a CustomResourceDefinition for testing.
func CRD(group, version, plural, kind, singular string, namespaced bool) *apiextensionsv1spec.CustomResourceDefinition {
	scope := apiextensionsv1spec.ClusterScoped
	if namespaced {
		scope = apiextensionsv1spec.NamespaceScoped
	}
	return &apiextensionsv1spec.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: plural + "." + group},
		Spec: apiextensionsv1spec.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1spec.CustomResourceDefinitionNames{
				Plural:   plural,
				Kind:     kind,
				Singular: singular,
			},
			Versions: []apiextensionsv1spec.CustomResourceDefinitionVersion{
				{
					Name:    version,
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1spec.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1spec.JSONSchemaProps{
							Type:                   "object",
							XPreserveUnknownFields: ptr.To(true),
						},
					},
				},
			},
			Scope: scope,
		},
	}
}

func createTestData(kc kubernetes.Interface) {
	ctx := context.Background()

	// Create namespaces
	for _, ns := range []string{"default", "kube-system", "kube-public", "openshift"} {
		_, _ = kc.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}, metav1.CreateOptions{})
	}

	// Create a cluster role for the test user
	// Named "allow-all" to match what mcp tests expect for RBAC test manipulation
	_, _ = kc.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-all"},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
		},
	}, metav1.CreateOptions{})

	// Bind the role to the test user's group
	// The binding is to "test:users" group (which envTestUser belongs to)
	_, _ = kc.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-all"},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "allow-all",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "Group",
				Name: "test:users",
			},
		},
	}, metav1.CreateOptions{})
}

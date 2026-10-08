package kubernetes_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/helm"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/stretchr/testify/suite"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientset "k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const impersonationReadGroup = "mcp-test:readers"

var impersonationUsers = []string{"mcp-test:alice", "mcp-test:bob"}
var impersonationNamespaces = []string{"mcp-test-alice", "mcp-test-bob"}

type ImpersonationIntegrationSuite struct {
	suite.Suite
	manager *kubernetes.Manager
	admin   *clientset.Clientset
	bot     *clientset.Clientset
}

func (s *ImpersonationIntegrationSuite) SetupSuite() {
	environment := test.EnvTest()
	ctx := s.T().Context()
	admin, err := clientset.NewForConfig(environment.Config)
	s.Require().NoError(err)
	s.admin = admin
	botUser, err := environment.AddUser(envtest.User{Name: "mcp-test:bot"}, environment.Config)
	s.Require().NoError(err)
	s.bot, err = clientset.NewForConfig(botUser.Config())
	s.Require().NoError(err)

	// The backend may only impersonate enrolled users, and has no workload access itself.
	_, err = admin.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "mcp-test-impersonate"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"users"}, ResourceNames: impersonationUsers, Verbs: []string{"impersonate"}},
			{APIGroups: []string{""}, Resources: []string{"groups"}, ResourceNames: []string{impersonationReadGroup}, Verbs: []string{"impersonate"}},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.bindBackend("mcp-test-impersonate", "mcp-test:bot")
	_, err = admin.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "mcp-test-read"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"configmaps", "nodes", "namespaces"}, Verbs: []string{"get", "list", "watch"}},
		},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	_, err = admin.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "mcp-test-read"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "mcp-test-read"},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "Group", Name: impersonationReadGroup}},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	for index, namespace := range impersonationNamespaces {
		_, err = admin.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		s.Require().NoError(err)
		_, err = admin.RbacV1().Roles(namespace).Create(ctx, &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "owner"},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}}},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)
		_, err = admin.RbacV1().RoleBindings(namespace).Create(ctx, &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "owner"},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: "owner"},
			Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "User", Name: impersonationUsers[index]}},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)
		_, err = admin.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "private"}}, metav1.CreateOptions{})
		s.Require().NoError(err)
	}
	botKubeconfig, err := botUser.KubeConfig()
	s.Require().NoError(err)
	path := filepath.Join(s.T().TempDir(), "bot-kubeconfig")
	s.Require().NoError(os.WriteFile(path, botKubeconfig, 0600))
	cfg := config.BaseDefault()
	cfg.KubeConfig.SetForTest(path)
	cfg.ClusterAuthMode.SetForTest("impersonation")
	test.ApplyEnvtestClientLimits(cfg)
	s.manager, err = kubernetes.NewKubeconfigManager(ctx, cfg, "")
	s.Require().NoError(err)

	// Wait for RBAC propagation before testing denials so cache timing cannot mask a defect.
	s.Require().Eventually(func() bool {
		for index, user := range impersonationUsers {
			client, err := s.manager.Derived(kubernetes.WithImpersonationIdentity(ctx, kubernetes.ImpersonationIdentity{UserName: user, Groups: []string{impersonationReadGroup}}))
			if err != nil {
				return false
			}
			allowed, err := kubernetes.CanI(ctx, client.AuthorizationV1(), &schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, impersonationNamespaces[index], "", "create")
			if err != nil || !allowed {
				return false
			}
		}
		return true
	}, 10*time.Second, 100*time.Millisecond)
}

func (s *ImpersonationIntegrationSuite) TearDownSuite() {
	if s.manager != nil {
		s.manager.Close()
	}
}

func (s *ImpersonationIntegrationSuite) bindBackend(name, user string) *rbacv1.ClusterRoleBinding {
	binding, err := s.admin.RbacV1().ClusterRoleBindings().Create(s.T().Context(), &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "mcp-test-impersonate"},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "User", Name: user}},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	return binding
}

func (s *ImpersonationIntegrationSuite) authorizeBackend(binding *rbacv1.ClusterRoleBinding, user string) {
	binding.Subjects[0].Name = user
	_, err := s.admin.RbacV1().ClusterRoleBindings().Update(s.T().Context(), binding, metav1.UpdateOptions{})
	s.Require().NoError(err)
}

func (s *ImpersonationIntegrationSuite) client(user string, groups ...string) *kubernetes.Kubernetes {
	ctx := kubernetes.WithImpersonationIdentity(s.T().Context(), kubernetes.ImpersonationIdentity{UserName: user, Groups: groups})
	ctx = context.WithValue(ctx, kubernetes.OAuthAuthorizationHeader, "Bearer not-a-kubernetes-token")
	client, err := s.manager.Derived(ctx)
	s.Require().NoError(err)
	return client
}

func (s *ImpersonationIntegrationSuite) TestBackendCannotAccessWorkloadsWithoutImpersonation() {
	_, err := s.bot.CoreV1().ConfigMaps(impersonationNamespaces[0]).List(s.T().Context(), metav1.ListOptions{})
	s.True(apierrors.IsForbidden(err), "backend must not have workload access: %v", err)
}

func (s *ImpersonationIntegrationSuite) TestBackgroundDiscoveryWithoutCallerIdentity() {
	ctx := s.T().Context()
	provider, err := kubernetes.NewProvider(ctx, s.manager.Config())
	s.Require().NoError(err)
	defer provider.Close()
	s.True(provider.AnyTargetHasGVKs(ctx, []schema.GroupVersionKind{{Version: "v1", Kind: "Pod"}}))
	s.False(provider.AnyTargetHasGVKs(ctx, []schema.GroupVersionKind{{Group: "absent.example.com", Version: "v1", Kind: "Absent"}}),
		"background discovery must distinguish missing APIs without a caller identity")
	_, err = provider.GetDerivedKubernetes(ctx, provider.GetDefaultTarget())
	s.ErrorContains(err, "identity required", "background discovery must not enable anonymous tool calls")
}

func (s *ImpersonationIntegrationSuite) TestNamespaceAndClusterAuthorization() {
	ctx := s.T().Context()
	client := s.client(impersonationUsers[0], impersonationReadGroup)
	s.Run("TLS-authenticated backend becomes requested user", func() {
		review, err := client.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
		s.Require().NoError(err)
		s.Equal(impersonationUsers[0], review.Status.UserInfo.Username)
		s.ElementsMatch([]string{impersonationReadGroup, "system:authenticated"}, review.Status.UserInfo.Groups)
	})
	s.Run("owner creates and reads a resource", func() {
		created, err := client.CoreV1().ConfigMaps(impersonationNamespaces[0]).Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "owned"}, Data: map[string]string{"owner": impersonationUsers[0]},
		}, metav1.CreateOptions{})
		s.Require().NoError(err)
		read, err := client.CoreV1().ConfigMaps(impersonationNamespaces[0]).Get(ctx, created.Name, metav1.GetOptions{})
		s.Require().NoError(err)
		s.Equal(created.Data, read.Data)
	})
	s.Run("other namespace mutation is forbidden through dynamic client", func() {
		_, err := client.DynamicClient().Resource(schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}).Namespace(impersonationNamespaces[1]).Create(ctx,
			&unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "forbidden"}}}, metav1.CreateOptions{})
		s.True(apierrors.IsForbidden(err), "cross-namespace create must be forbidden: %v", err)
	})
	s.Run("cluster mutation is forbidden", func() {
		_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "forbidden"}}, metav1.CreateOptions{})
		s.True(apierrors.IsForbidden(err), "namespace creation must be forbidden: %v", err)
	})
	s.Run("approved cluster read is allowed", func() {
		_, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		s.NoError(err)
	})
	s.Run("other namespace secret read is forbidden", func() {
		_, err := client.CoreV1().Secrets(impersonationNamespaces[1]).Get(ctx, "private", metav1.GetOptions{})
		s.True(apierrors.IsForbidden(err), "cross-namespace secret read must be forbidden: %v", err)
	})
	s.Run("missing read group cannot inherit another request's groups", func() {
		_, err := s.client(impersonationUsers[0]).CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		s.True(apierrors.IsForbidden(err), "omitted group must not carry over from another client: %v", err)
	})
}

func (s *ImpersonationIntegrationSuite) TestDiscoveryAndAuthorizationReviews() {
	client := s.client(impersonationUsers[0], impersonationReadGroup)
	s.Run("discovery works without request context", func() {
		resources, err := client.DiscoveryClient().ServerResourcesForGroupVersion("v1")
		s.Require().NoError(err)
		s.NotEmpty(resources.APIResources)
	})
	for index, namespace := range impersonationNamespaces {
		s.Run(namespace, func() {
			allowed, err := kubernetes.CanI(s.T().Context(), client.AuthorizationV1(), &schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, namespace, "", "create")
			s.Require().NoError(err)
			s.Equal(index == 0, allowed)
		})
	}
}

func (s *ImpersonationIntegrationSuite) TestBackendImpersonationAllowlist() {
	for _, identity := range []kubernetes.ImpersonationIdentity{
		{UserName: "mcp-test:mallory", Groups: []string{impersonationReadGroup}},
		{UserName: impersonationUsers[0], Groups: []string{"system:masters"}},
	} {
		s.Run(fmt.Sprintf("%s/%v", identity.UserName, identity.Groups), func() {
			_, err := s.client(identity.UserName, identity.Groups...).AuthenticationV1().SelfSubjectReviews().Create(s.T().Context(), &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
			s.True(apierrors.IsForbidden(err), "backend must not impersonate an unapproved identity: %v", err)
		})
	}
}

func (s *ImpersonationIntegrationSuite) TestConcurrentUsersRemainIsolated() {
	ctx := s.T().Context()
	results := make(chan error, 16)
	var workers sync.WaitGroup
	for index, user := range impersonationUsers {
		for request := range 8 {
			workers.Go(func() {
				requestCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				client, err := s.manager.Derived(kubernetes.WithImpersonationIdentity(requestCtx, kubernetes.ImpersonationIdentity{UserName: user, Groups: []string{impersonationReadGroup}}))
				if err == nil {
					var review *authenticationv1.SelfSubjectReview
					review, err = client.AuthenticationV1().SelfSubjectReviews().Create(requestCtx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
					if err == nil && review.Status.UserInfo.Username != user {
						err = fmt.Errorf("request for %q reached API as %q", user, review.Status.UserInfo.Username)
					}
				}
				if err == nil {
					_, err = client.CoreV1().ConfigMaps(impersonationNamespaces[index]).Create(requestCtx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("parallel-%d", request)}}, metav1.CreateOptions{})
				}
				if err == nil {
					_, denied := client.CoreV1().Secrets(impersonationNamespaces[1-index]).Get(requestCtx, "private", metav1.GetOptions{})
					if !apierrors.IsForbidden(denied) {
						err = fmt.Errorf("cross-user request was not forbidden: %v", denied)
					}
				}
				results <- err
			})
		}
	}
	workers.Wait()
	close(results)
	for err := range results {
		s.NoError(err)
	}
}

func (s *ImpersonationIntegrationSuite) TestHelmUsesTheSameIdentity() {
	ctx := s.T().Context()
	h := helm.NewHelm(s.client(impersonationUsers[0], impersonationReadGroup), &helm.Config{StorageDriver: "secret"})
	chart := s.T().TempDir()
	s.Require().NoError(os.Mkdir(filepath.Join(chart, "templates"), 0700))
	s.Require().NoError(os.WriteFile(filepath.Join(chart, "Chart.yaml"), []byte("apiVersion: v2\nname: impersonation-test\nversion: 0.1.0\n"), 0600))
	s.Require().NoError(os.WriteFile(filepath.Join(chart, "templates", "configmap.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: helm-owned\n  namespace: {{ .Release.Namespace }}\n"), 0600))
	s.Run("install in own namespace", func() {
		_, err := h.Install(ctx, chart, nil, "impersonation-test", impersonationNamespaces[0])
		s.NoError(err)
	})
	s.Run("list in own namespace", func() {
		releases, err := h.List(ctx, impersonationNamespaces[0], false)
		s.Require().NoError(err)
		s.Contains(releases, "impersonation-test")
	})
	s.Run("list other namespace is forbidden", func() {
		_, err := h.List(ctx, impersonationNamespaces[1], false)
		s.ErrorContains(err, "forbidden")
	})
	s.Run("install other namespace is forbidden", func() {
		_, err := h.Install(ctx, chart, nil, "impersonation-test", impersonationNamespaces[1])
		s.ErrorContains(err, "forbidden")
	})
}

func (s *ImpersonationIntegrationSuite) TestKubeconfigCertificateRotation() {
	ctx, cancel := context.WithCancel(s.T().Context())
	defer cancel()
	environment := test.EnvTest()
	oldBot, err := environment.AddUser(envtest.User{Name: "mcp-test:rotation-old"}, environment.Config)
	s.Require().NoError(err)
	newBot, err := environment.AddUser(envtest.User{Name: "mcp-test:rotation-new"}, environment.Config)
	s.Require().NoError(err)
	binding := s.bindBackend("mcp-test-rotation", "mcp-test:rotation-old")
	oldKubeconfig, err := oldBot.KubeConfig()
	s.Require().NoError(err)
	newKubeconfig, err := newBot.KubeConfig()
	s.Require().NoError(err)
	path := filepath.Join(s.T().TempDir(), "rotating-kubeconfig")
	s.Require().NoError(os.WriteFile(path, oldKubeconfig, 0600))
	cfg := config.BaseDefault()
	cfg.KubeConfig.SetForTest(path)
	cfg.ClusterAuthMode.SetForTest("impersonation")
	cfg.KubeconfigDebounceWindow.SetForTest(10 * time.Millisecond)
	test.ApplyEnvtestClientLimits(cfg)
	provider, err := kubernetes.NewProvider(ctx, cfg)
	s.Require().NoError(err)
	defer provider.Close()
	identityCtx := kubernetes.WithImpersonationIdentity(ctx, kubernetes.ImpersonationIdentity{UserName: impersonationUsers[0], Groups: []string{impersonationReadGroup}})
	oldClient, err := provider.GetDerivedKubernetes(identityCtx, provider.GetDefaultTarget())
	s.Require().NoError(err)
	s.Require().Eventually(func() bool {
		_, err := oldClient.CoreV1().ConfigMaps(impersonationNamespaces[0]).List(ctx, metav1.ListOptions{})
		return err == nil
	}, 10*time.Second, 100*time.Millisecond)

	// Changing the backend subject lets the API prove that requests use the replacement certificate.
	s.authorizeBackend(binding, "mcp-test:rotation-new")
	s.Require().Eventually(func() bool {
		_, err := oldClient.CoreV1().ConfigMaps(impersonationNamespaces[0]).List(ctx, metav1.ListOptions{})
		return apierrors.IsForbidden(err)
	}, 10*time.Second, 100*time.Millisecond)

	reloaded := make(chan struct{}, 1)
	provider.WatchTargets(ctx, kubernetes.McpReloaderFromCallback(func() error {
		select {
		case reloaded <- struct{}{}:
		default:
		}
		return nil
	}))
	s.Require().NoError(os.WriteFile(path, newKubeconfig, 0600))
	select {
	case <-reloaded:
	case <-time.After(10 * time.Second):
		s.T().Fatal("provider did not reload rotated kubeconfig")
	}
	newClient, err := provider.GetDerivedKubernetes(identityCtx, provider.GetDefaultTarget())
	s.Require().NoError(err)
	review, err := newClient.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	s.Require().NoError(err)
	s.Equal(impersonationUsers[0], review.Status.UserInfo.Username)
	_, err = newClient.CoreV1().ConfigMaps(impersonationNamespaces[0]).List(ctx, metav1.ListOptions{})
	s.Require().NoError(err)
	_, err = newClient.CoreV1().Secrets(impersonationNamespaces[1]).Get(ctx, "private", metav1.GetOptions{})
	s.True(apierrors.IsForbidden(err), "credential renewal must preserve caller RBAC: %v", err)
}

func TestImpersonationIntegration(t *testing.T) {
	suite.Run(t, new(ImpersonationIntegrationSuite))
}

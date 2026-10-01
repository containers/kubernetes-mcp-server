package kubernetes_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	authenticationv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func (s *ImpersonationIntegrationSuite) TestBackendCertificateFilesRotateWithoutKubeconfigReload() {
	environment := test.EnvTest()
	ctx := s.T().Context()
	admin, err := clientset.NewForConfig(environment.Config)
	s.Require().NoError(err)
	oldBot, err := environment.AddUser(envtest.User{Name: "mcp-test:file-old"}, environment.Config)
	s.Require().NoError(err)
	newBot, err := environment.AddUser(envtest.User{Name: "mcp-test:file-new"}, environment.Config)
	s.Require().NoError(err)
	binding, err := admin.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "mcp-test-file-rotation"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "mcp-test-impersonate"},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: "User", Name: "mcp-test:file-old"}},
	}, metav1.CreateOptions{})
	s.Require().NoError(err)
	path := s.T().TempDir()
	certFile, keyFile := filepath.Join(path, "client.crt"), filepath.Join(path, "client.key")
	s.Require().NoError(os.WriteFile(certFile, oldBot.Config().CertData, 0600))
	s.Require().NoError(os.WriteFile(keyFile, oldBot.Config().KeyData, 0600))
	backend := rest.CopyConfig(oldBot.Config())
	backend.CertData, backend.KeyData = nil, nil
	backend.CertFile, backend.KeyFile = certFile, keyFile
	cfg := config.BaseDefault()
	cfg.ClusterAuthMode.SetForTest("impersonation")
	manager, err := kubernetes.NewManager(ctx, cfg, backend, clientcmd.NewDefaultClientConfig(*clientcmdapi.NewConfig(), nil))
	s.Require().NoError(err)
	defer manager.Close()
	request := func() (*authenticationv1.SelfSubjectReview, error) {
		// Each tool call has its own client lifetime, allowing renewal on new connections.
		requestCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		requestCtx = kubernetes.WithImpersonationIdentity(requestCtx, kubernetes.ImpersonationIdentity{UserName: impersonationUsers[0], Groups: []string{impersonationReadGroup}})
		client, err := manager.Derived(requestCtx)
		if err != nil {
			return nil, err
		}
		return client.AuthenticationV1().SelfSubjectReviews().Create(requestCtx, &authenticationv1.SelfSubjectReview{}, metav1.CreateOptions{})
	}
	s.Require().Eventually(func() bool { _, err := request(); return err == nil }, 10*time.Second, 100*time.Millisecond)
	binding.Subjects[0].Name = "mcp-test:file-new"
	_, err = admin.RbacV1().ClusterRoleBindings().Update(ctx, binding, metav1.UpdateOptions{})
	s.Require().NoError(err)
	s.Require().Eventually(func() bool { _, err := request(); return apierrors.IsForbidden(err) }, 10*time.Second, 100*time.Millisecond)
	s.Require().NoError(os.WriteFile(certFile, newBot.Config().CertData, 0600))
	s.Require().NoError(os.WriteFile(keyFile, newBot.Config().KeyData, 0600))
	// No manager rebuild or kubeconfig event: only the external credential files change.
	s.Require().Eventually(func() bool {
		review, err := request()
		return err == nil && review.Status.UserInfo.Username == impersonationUsers[0]
	}, 10*time.Second, 100*time.Millisecond)
}

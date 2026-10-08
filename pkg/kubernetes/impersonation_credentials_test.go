package kubernetes_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	authenticationv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func (s *ImpersonationIntegrationSuite) TestBackendCertificateFilesRotateWithoutKubeconfigReload() {
	environment := test.EnvTest()
	ctx := s.T().Context()
	oldBot, err := environment.AddUser(envtest.User{Name: "mcp-test:file-old"}, environment.Config)
	s.Require().NoError(err)
	newBot, err := environment.AddUser(envtest.User{Name: "mcp-test:file-new"}, environment.Config)
	s.Require().NoError(err)
	binding := s.bindBackend("mcp-test-file-rotation", "mcp-test:file-old")
	path := s.T().TempDir()
	certFile, keyFile := filepath.Join(path, "client.crt"), filepath.Join(path, "client.key")
	s.Require().NoError(os.WriteFile(certFile, oldBot.Config().CertData, 0600))
	s.Require().NoError(os.WriteFile(keyFile, oldBot.Config().KeyData, 0600))
	backend := rest.CopyConfig(oldBot.Config())
	backend.CertData, backend.KeyData = nil, nil
	backend.CertFile, backend.KeyFile = certFile, keyFile
	manager := newImpersonationManager(s.T(), backend, clientcmdapi.NewConfig())
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
	s.authorizeBackend(binding, "mcp-test:file-new")
	s.Require().Eventually(func() bool { _, err := request(); return apierrors.IsForbidden(err) }, 10*time.Second, 100*time.Millisecond)
	s.Require().NoError(os.WriteFile(certFile, newBot.Config().CertData, 0600))
	s.Require().NoError(os.WriteFile(keyFile, newBot.Config().KeyData, 0600))
	// No manager rebuild or kubeconfig event: only the external credential files change.
	s.Require().Eventually(func() bool {
		review, err := request()
		return err == nil && review.Status.UserInfo.Username == impersonationUsers[0]
	}, 10*time.Second, 100*time.Millisecond)
}

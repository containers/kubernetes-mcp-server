package code

import (
	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	"github.com/stretchr/testify/suite"
	"k8s.io/client-go/tools/clientcmd"
)

// BaseEvaluatorSuite provides common setup for evaluator tests.
// It uses MockServer for lightweight Kubernetes API mocking.
type BaseEvaluatorSuite struct {
	suite.Suite
	mockServer *test.MockServer
	kubernetes *kubernetes.Kubernetes
}

func (s *BaseEvaluatorSuite) SetupTest() {
	s.mockServer = test.NewMockServer()
	s.mockServer.Handle(test.NewDiscoveryClientHandler())
	clientConfig := clientcmd.NewDefaultClientConfig(
		*s.mockServer.Kubeconfig(),
		&clientcmd.ConfigOverrides{},
	)
	var err error
	s.kubernetes, err = kubernetes.NewKubernetes(
		config.Default(),
		clientConfig,
		s.mockServer.Config(),
	)
	s.Require().NoError(err, "Expected no error creating kubernetes client")
}

func (s *BaseEvaluatorSuite) TearDownTest() {
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

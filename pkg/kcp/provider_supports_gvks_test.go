package kcp

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type ProviderSupportsGVKsTestSuite struct {
	suite.Suite
}

func (s *ProviderSupportsGVKsTestSuite) TestSupportsGVKsNoopReturnsTrue() {
	provider := &kcpClusterProvider{}

	s.True(provider.SupportsGVKs(nil))
	s.True(provider.SupportsGVKs([]schema.GroupVersionKind{}))
	s.True(provider.SupportsGVKs([]schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "Deployment"},
	}))
}

func TestProviderSupportsGVKs(t *testing.T) {
	suite.Run(t, new(ProviderSupportsGVKsTestSuite))
}

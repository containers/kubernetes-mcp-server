package kiali

import (
	"context"
	"errors"
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type OpenShiftSuite struct {
	suite.Suite
}

type mockInspector struct {
	available        bool
	discoveryErr     error
	lastGroupVersion string
}

func (m *mockInspector) Discovery() api.AggregateDiscovery       { return m }
func (m *mockInspector) Unstructured() api.AggregateUnstructured { return nil }
func (m *mockInspector) GetTargets(context.Context) ([]string, error) {
	return []string{""}, nil
}
func (m *mockInspector) GetDefaultTarget() string       { return "" }
func (m *mockInspector) GetTargetParameterName() string { return "" }
func (m *mockInspector) IsMultiTarget() bool            { return false }
func (m *mockInspector) ServerResourcesForGroupVersion(ctx context.Context, groupVersion string) api.Results[*metav1.APIResourceList] {
	m.lastGroupVersion = groupVersion
	return api.NewResults(ctx, m, func(context.Context, string) (*metav1.APIResourceList, error) {
		list := &metav1.APIResourceList{}
		if m.available {
			list.APIResources = []metav1.APIResource{{Kind: openshiftProjectGVK.Kind}}
		}
		return list, m.discoveryErr
	})
}

func (s *OpenShiftSuite) TestIsOpenShiftFromInspector() {
	s.Run("returns false without inspector", func() {
		s.False(IsOpenShiftFromInspector(s.T().Context(), nil))
	})

	s.Run("delegates to cluster discovery", func() {
		inspector := &mockInspector{available: true}
		s.True(IsOpenShiftFromInspector(s.T().Context(), inspector))
		s.Equal(openshiftProjectGVK.GroupVersion().String(), inspector.lastGroupVersion)
	})

	s.Run("returns false when project kind is absent", func() {
		s.False(IsOpenShiftFromInspector(s.T().Context(), &mockInspector{}))
	})

	s.Run("returns false when discovery confirms absence", func() {
		inspector := &mockInspector{discoveryErr: apierrors.NewNotFound(schema.GroupResource{Group: openshiftProjectGVK.Group, Resource: "projects"}, "")}
		s.False(IsOpenShiftFromInspector(s.T().Context(), inspector))
	})

	s.Run("retains OpenShift requirements when discovery fails", func() {
		inspector := &mockInspector{discoveryErr: errors.New("discovery unavailable")}
		s.True(IsOpenShiftFromInspector(s.T().Context(), inspector))
	})
}

func TestOpenShift(t *testing.T) {
	suite.Run(t, new(OpenShiftSuite))
}

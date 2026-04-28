package mcp

import (
	"testing"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/stretchr/testify/suite"
)

type ResourceFilterSuite struct {
	suite.Suite
}

func (s *ResourceFilterSuite) TestResourceFilterType() {
	s.Run("can be used as function", func() {
		var filter ResourceFilter = func(resource api.ServerResource) bool {
			return resource.Resource.Name == "included"
		}
		s.Run("returns true for included resource", func() {
			r := api.ServerResource{Resource: api.Resource{Name: "included"}}
			s.True(filter(r))
		})
		s.Run("returns false for excluded resource", func() {
			r := api.ServerResource{Resource: api.Resource{Name: "excluded"}}
			s.False(filter(r))
		})
	})
}

func (s *ResourceFilterSuite) TestCompositeResourceFilter() {
	s.Run("returns true with no filters", func() {
		filter := CompositeResourceFilter()
		r := api.ServerResource{Resource: api.Resource{Name: "any"}}
		s.True(filter(r))
	})
	s.Run("returns true if all filters return true", func() {
		filter := CompositeResourceFilter(
			func(_ api.ServerResource) bool { return true },
			func(_ api.ServerResource) bool { return true },
		)
		r := api.ServerResource{Resource: api.Resource{Name: "test"}}
		s.True(filter(r))
	})
	s.Run("returns false if any filter returns false", func() {
		filter := CompositeResourceFilter(
			func(_ api.ServerResource) bool { return true },
			func(_ api.ServerResource) bool { return false },
		)
		r := api.ServerResource{Resource: api.Resource{Name: "test"}}
		s.False(filter(r))
	})
	s.Run("short-circuits on first false", func() {
		called := false
		filter := CompositeResourceFilter(
			func(_ api.ServerResource) bool { return false },
			func(_ api.ServerResource) bool { called = true; return true },
		)
		r := api.ServerResource{Resource: api.Resource{Name: "test"}}
		s.False(filter(r))
		s.False(called)
	})
}

func (s *ResourceFilterSuite) TestResourceMutatorType() {
	s.Run("can be used as function", func() {
		var mutator ResourceMutator = func(resource api.ServerResource) api.ServerResource {
			resource.Resource.Name = "mutated-" + resource.Resource.Name
			return resource
		}
		r := api.ServerResource{Resource: api.Resource{Name: "original"}}
		result := mutator(r)
		s.Equal("mutated-original", result.Resource.Name)
	})
}

func (s *ResourceFilterSuite) TestComposeResourceMutators() {
	s.Run("identity with no mutators", func() {
		mutator := ComposeResourceMutators()
		r := api.ServerResource{Resource: api.Resource{Name: "unchanged"}}
		result := mutator(r)
		s.Equal("unchanged", result.Resource.Name)
	})
	s.Run("applies single mutator", func() {
		mutator := ComposeResourceMutators(
			func(r api.ServerResource) api.ServerResource {
				r.Resource.Description = "added"
				return r
			},
		)
		r := api.ServerResource{Resource: api.Resource{Name: "test"}}
		result := mutator(r)
		s.Equal("added", result.Resource.Description)
	})
	s.Run("chains mutators in order", func() {
		mutator := ComposeResourceMutators(
			func(r api.ServerResource) api.ServerResource {
				r.Resource.Name = r.Resource.Name + "-first"
				return r
			},
			func(r api.ServerResource) api.ServerResource {
				r.Resource.Name = r.Resource.Name + "-second"
				return r
			},
		)
		r := api.ServerResource{Resource: api.Resource{Name: "start"}}
		result := mutator(r)
		s.Equal("start-first-second", result.Resource.Name)
	})
}

type ResourceTemplateFilterSuite struct {
	suite.Suite
}

func (s *ResourceTemplateFilterSuite) TestResourceTemplateFilterType() {
	s.Run("can be used as function", func() {
		var filter ResourceTemplateFilter = func(t api.ServerResourceTemplate) bool {
			return t.ResourceTemplate.Name == "included"
		}
		s.Run("returns true for included template", func() {
			t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "included"}}
			s.True(filter(t))
		})
		s.Run("returns false for excluded template", func() {
			t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "excluded"}}
			s.False(filter(t))
		})
	})
}

func (s *ResourceTemplateFilterSuite) TestCompositeResourceTemplateFilter() {
	s.Run("returns true with no filters", func() {
		filter := CompositeResourceTemplateFilter()
		t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "any"}}
		s.True(filter(t))
	})
	s.Run("returns true if all filters return true", func() {
		filter := CompositeResourceTemplateFilter(
			func(_ api.ServerResourceTemplate) bool { return true },
			func(_ api.ServerResourceTemplate) bool { return true },
		)
		t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "test"}}
		s.True(filter(t))
	})
	s.Run("returns false if any filter returns false", func() {
		filter := CompositeResourceTemplateFilter(
			func(_ api.ServerResourceTemplate) bool { return true },
			func(_ api.ServerResourceTemplate) bool { return false },
		)
		t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "test"}}
		s.False(filter(t))
	})
	s.Run("short-circuits on first false", func() {
		called := false
		filter := CompositeResourceTemplateFilter(
			func(_ api.ServerResourceTemplate) bool { return false },
			func(_ api.ServerResourceTemplate) bool { called = true; return true },
		)
		t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "test"}}
		s.False(filter(t))
		s.False(called)
	})
}

func (s *ResourceTemplateFilterSuite) TestResourceTemplateMutatorType() {
	s.Run("can be used as function", func() {
		var mutator ResourceTemplateMutator = func(t api.ServerResourceTemplate) api.ServerResourceTemplate {
			t.ResourceTemplate.Name = "mutated-" + t.ResourceTemplate.Name
			return t
		}
		t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "original"}}
		result := mutator(t)
		s.Equal("mutated-original", result.ResourceTemplate.Name)
	})
}

func (s *ResourceTemplateFilterSuite) TestComposeResourceTemplateMutators() {
	s.Run("identity with no mutators", func() {
		mutator := ComposeResourceTemplateMutators()
		t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "unchanged"}}
		result := mutator(t)
		s.Equal("unchanged", result.ResourceTemplate.Name)
	})
	s.Run("applies single mutator", func() {
		mutator := ComposeResourceTemplateMutators(
			func(t api.ServerResourceTemplate) api.ServerResourceTemplate {
				t.ResourceTemplate.Description = "added"
				return t
			},
		)
		t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "test"}}
		result := mutator(t)
		s.Equal("added", result.ResourceTemplate.Description)
	})
	s.Run("chains mutators in order", func() {
		mutator := ComposeResourceTemplateMutators(
			func(t api.ServerResourceTemplate) api.ServerResourceTemplate {
				t.ResourceTemplate.Name = t.ResourceTemplate.Name + "-first"
				return t
			},
			func(t api.ServerResourceTemplate) api.ServerResourceTemplate {
				t.ResourceTemplate.Name = t.ResourceTemplate.Name + "-second"
				return t
			},
		)
		t := api.ServerResourceTemplate{ResourceTemplate: api.ResourceTemplate{Name: "start"}}
		result := mutator(t)
		s.Equal("start-first-second", result.ResourceTemplate.Name)
	})
}

func TestResourceFilter(t *testing.T) {
	suite.Run(t, new(ResourceFilterSuite))
}

func TestResourceTemplateFilter(t *testing.T) {
	suite.Run(t, new(ResourceTemplateFilterSuite))
}

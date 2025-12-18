package core

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ClusterHealthCheckSuite struct {
	suite.Suite
}

func (s *ClusterHealthCheckSuite) TestPromptIsRegistered() {
	s.Run("cluster-health-check prompt is registered via GetPrompts", func() {
		// Create a new instance of the core toolset
		toolset := &Toolset{}

		// Get prompts from the toolset
		prompts := toolset.GetPrompts()

		s.Require().NotNil(prompts, "GetPrompts should not return nil")
		s.Require().NotEmpty(prompts, "GetPrompts should return at least one prompt")

		// Find the cluster-health-check prompt
		var foundHealthCheck bool
		for _, prompt := range prompts {
			if prompt.Prompt.Name == "cluster-health-check" {
				foundHealthCheck = true

				// Verify prompt metadata
				s.Equal("cluster-health-check", prompt.Prompt.Name)
				s.Equal("Cluster Health Check", prompt.Prompt.Title)
				s.Contains(prompt.Prompt.Description, "comprehensive health assessment")

				// Verify arguments
				s.Require().Len(prompt.Prompt.Arguments, 2, "should have 2 arguments")

				// Check namespace argument
				s.Equal("namespace", prompt.Prompt.Arguments[0].Name)
				s.NotEmpty(prompt.Prompt.Arguments[0].Description)
				s.False(prompt.Prompt.Arguments[0].Required)

				// Check check_events argument
				s.Equal("check_events", prompt.Prompt.Arguments[1].Name)
				s.NotEmpty(prompt.Prompt.Arguments[1].Description)
				s.False(prompt.Prompt.Arguments[1].Required)

				// Verify handler is set
				s.NotNil(prompt.Handler, "handler should be set")

				break
			}
		}

		s.True(foundHealthCheck, "cluster-health-check prompt should be registered")
	})
}

func TestClusterHealthCheckSuite(t *testing.T) {
	suite.Run(t, new(ClusterHealthCheckSuite))
}

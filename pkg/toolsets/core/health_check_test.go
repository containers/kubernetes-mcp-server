package core

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/containers/kubernetes-mcp-server/pkg/prompts"
)

type ClusterHealthCheckSuite struct {
	suite.Suite
}

func (s *ClusterHealthCheckSuite) TestPromptIsRegistered() {
	s.Run("cluster-health-check prompt is registered", func() {
		configPrompts := prompts.ConfigPrompts()

		var foundHealthCheck bool
		for _, prompt := range configPrompts {
			if prompt.Prompt.Name == "cluster-health-check" {
				foundHealthCheck = true

				// Verify prompt metadata
				s.Equal("cluster-health-check", prompt.Prompt.Name)
				s.Equal("Cluster Health Check", prompt.Prompt.Title)
				s.Contains(prompt.Prompt.Description, "comprehensive health assessment")

				// Verify arguments
				s.Require().Len(prompt.Prompt.Arguments, 3, "should have 3 arguments")

				// Check namespace argument
				s.Equal("namespace", prompt.Prompt.Arguments[0].Name)
				s.False(prompt.Prompt.Arguments[0].Required)

				// Check verbose argument
				s.Equal("verbose", prompt.Prompt.Arguments[1].Name)
				s.False(prompt.Prompt.Arguments[1].Required)

				// Check check_events argument
				s.Equal("check_events", prompt.Prompt.Arguments[2].Name)
				s.False(prompt.Prompt.Arguments[2].Required)

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

package mcp

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/internal/test"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
)

type WatchKubeConfigSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
}

func (s *WatchKubeConfigSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	s.T().Setenv("KUBECONFIG_DEBOUNCE_WINDOW_MS", "10")
	s.mockServer = test.NewMockServer()
	s.Require().NoError(toml.Unmarshal([]byte(`
		[[prompts]]
		name = "test-prompt"
		title = "Test Prompt"
		description = "A test prompt for testing"

		[[prompts.arguments]]
		name = "test_arg"
		description = "A test argument"
		required = true
		
		[[prompts.messages]]
		role = "user"
		content = "Test message with {{test_arg}}"
	`), s.Cfg), "Expected to parse prompts config")
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
}

func (s *WatchKubeConfigSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *WatchKubeConfigSuite) WriteKubeconfig() {
	f, _ := os.OpenFile(s.Cfg.KubeConfig, os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("\n")
	_ = f.Close()
}

func (s *WatchKubeConfigSuite) TestNotifiesToolsChange() {
	// Given
	s.InitMcpClient()
	// When
	s.WriteKubeconfig()
	notification := s.WaitForNotification(5*time.Second, "notifications/tools/list_changed")
	// Then
	s.NotNil(notification, "WatchKubeConfig did not notify")
	s.Equal("notifications/tools/list_changed", notification.Method, "WatchKubeConfig did not notify tools change")
}

func (s *WatchKubeConfigSuite) TestNotifiesPromptsChange() {
	// Given
	s.InitMcpClient()
	// When
	s.WriteKubeconfig()
	notification := s.WaitForNotification(5*time.Second, "notifications/prompts/list_changed")
	// Then
	s.NotNil(notification, "WatchKubeConfig did not notify")
	s.Equal("notifications/prompts/list_changed", notification.Method, "WatchKubeConfig did not notify prompts change")
}

func (s *WatchKubeConfigSuite) TestNotifiesToolsChangeMultipleTimes() {
	// Given
	s.InitMcpClient()
	// When
	for i := 0; i < 3; i++ {
		s.WriteKubeconfig()
		notification := s.WaitForNotification(5*time.Second, "notifications/tools/list_changed")
		// Then
		s.NotNil(notification, "WatchKubeConfig did not notify on iteration %d", i)
		s.Equalf("notifications/tools/list_changed", notification.Method, "WatchKubeConfig did not notify tools change on iteration %d", i)
	}
}

func (s *WatchKubeConfigSuite) TestNotifiesPromptsChangeMultipleTimes() {
	// Given
	s.InitMcpClient()
	// When
	for i := 0; i < 3; i++ {
		s.WriteKubeconfig()
		notification := s.WaitForNotification(5*time.Second, "notifications/prompts/list_changed")
		// Then
		s.NotNil(notification, "WatchKubeConfig did not notify on iteration %d", i)
		s.Equalf("notifications/prompts/list_changed", notification.Method, "WatchKubeConfig did not notify prompts change on iteration %d", i)
	}
}

func (s *WatchKubeConfigSuite) TestClearsNoLongerAvailableTools() {
	s.mockServer.Handle(&test.InOpenShiftHandler{})
	s.InitMcpClient()

	s.Run("OpenShift tool is available", func() {
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Require().NoError(err, "call ListTools failed")
		s.Require().NotNil(tools, "list tools failed")
		var found bool
		for _, tool := range tools.Tools {
			if tool.Name == "projects_list" {
				found = true
				break
			}
		}
		s.Truef(found, "expected OpenShift tool to be available")
	})

	s.Run("OpenShift tool is removed after kubeconfig change", func() {
		// Reload Config without OpenShift
		s.mockServer.ResetHandlers()
		s.WriteKubeconfig()
		s.WaitForNotification(5*time.Second, "notifications/tools/list_changed")

		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Require().NoError(err, "call ListTools failed")
		s.Require().NotNil(tools, "list tools failed")
		for _, tool := range tools.Tools {
			s.Require().Falsef(tool.Name == "projects_list", "expected OpenShift tool to be removed")
		}
	})
}

func TestWatchKubeConfig(t *testing.T) {
	suite.Run(t, new(WatchKubeConfigSuite))
}

type WatchClusterStateSuite struct {
	BaseMcpSuite
	mockServer *test.MockServer
	handler    *test.DiscoveryClientHandler
}

func (s *WatchClusterStateSuite) SetupTest() {
	s.BaseMcpSuite.SetupTest()
	// Configure fast polling for tests
	s.T().Setenv("CLUSTER_STATE_POLL_INTERVAL_MS", "50")
	s.T().Setenv("CLUSTER_STATE_DEBOUNCE_WINDOW_MS", "10")
	s.mockServer = test.NewMockServer()
	s.handler = &test.DiscoveryClientHandler{}
	s.mockServer.Handle(s.handler)
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
}

func (s *WatchClusterStateSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *WatchClusterStateSuite) AddAPIGroup(groupName string) {
	s.handler.Groups = append(s.handler.Groups, groupName)
}

func (s *WatchClusterStateSuite) TestNotifiesToolsChangeOnAPIGroupAddition() {
	// Given - Initialize with basic API groups
	s.InitMcpClient()

	// When - Add a new API group to simulate cluster state change
	s.AddAPIGroup(`{"name":"custom.example.com","versions":[{"groupVersion":"custom.example.com/v1","version":"v1"}],"preferredVersion":{"groupVersion":"custom.example.com/v1","version":"v1"}}`)

	notification := s.WaitForNotification(5*time.Second, "notifications/tools/list_changed")

	// Then
	s.NotNil(notification, "cluster state watcher did not notify")
	s.Equal("notifications/tools/list_changed", notification.Method, "cluster state watcher did not notify tools change")
}

func (s *WatchClusterStateSuite) TestNotifiesToolsChangeMultipleTimes() {
	// Given - Initialize with basic API groups
	s.InitMcpClient()

	// When - Add multiple API groups to simulate cluster state changes
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("custom-%d", i)
		s.AddAPIGroup(`{"name":"` + name + `.example.com","versions":[{"groupVersion":"` + name + `.example.com/v1","version":"v1"}],"preferredVersion":{"groupVersion":"` + name + `.example.com/v1","version":"v1"}}`)
		notification := s.WaitForNotification(5*time.Second, "notifications/tools/list_changed")
		s.NotNil(notification, "cluster state watcher did not notify on iteration %d", i)
		s.Equalf("notifications/tools/list_changed", notification.Method, "cluster state watcher did not notify tools change on iteration %d", i)
	}
}

func (s *WatchClusterStateSuite) TestDetectsOpenShiftClusterStateChange() {
	s.InitMcpClient()

	s.Run("OpenShift tool is not available initially", func() {
		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Require().NoError(err, "call ListTools failed")
		s.Require().NotNil(tools, "list tools failed")
		for _, tool := range tools.Tools {
			s.Require().Falsef(tool.Name == "projects_list", "expected OpenShift tool to not be available initially")
		}
	})

	s.Run("OpenShift tool is added after cluster becomes OpenShift", func() {
		// Simulate cluster becoming OpenShift by adding OpenShift API groups
		s.mockServer.ResetHandlers()
		s.mockServer.Handle(&test.InOpenShiftHandler{})

		notification := s.WaitForNotification(5*time.Second, "notifications/tools/list_changed")
		s.NotNil(notification, "cluster state watcher did not notify")

		tools, err := s.ListTools(s.T().Context(), mcp.ListToolsRequest{})
		s.Require().NoError(err, "call ListTools failed")
		s.Require().NotNil(tools, "list tools failed")

		var found bool
		for _, tool := range tools.Tools {
			if tool.Name == "projects_list" {
				found = true
				break
			}
		}
		s.Truef(found, "expected OpenShift tool to be available after cluster state change")
	})
}

func TestWatchClusterState(t *testing.T) {
	suite.Run(t, new(WatchClusterStateSuite))
}

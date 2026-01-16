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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// serverSettleDelay is the time to wait after receiving a notification for the server
// to finish updating its internal state. The MCP server sends notifications before
// completing async updates to tools/prompts, so tests that verify server state after
// a notification need to wait for those updates to complete.
const serverSettleDelay = 100 * time.Millisecond

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
	capture := s.StartCapturingNotifications()
	// When
	s.WriteKubeconfig()
	notification := capture.RequireNotification(s.T(), 5*time.Second, "notifications/tools/list_changed")
	// Then
	s.NotNil(notification, "WatchKubeConfig did not notify")
}

func (s *WatchKubeConfigSuite) TestNotifiesPromptsChange() {
	// Given
	s.InitMcpClient()
	capture := s.StartCapturingNotifications()
	// When
	s.WriteKubeconfig()
	notification := capture.RequireNotification(s.T(), 5*time.Second, "notifications/prompts/list_changed")
	// Then
	s.NotNil(notification, "WatchKubeConfig did not notify")
}

func (s *WatchKubeConfigSuite) TestNotifiesToolsChangeMultipleTimes() {
	// Given
	s.InitMcpClient()
	// When
	for i := 0; i < 3; i++ {
		capture := s.StartCapturingNotifications()
		s.WriteKubeconfig()
		notification := capture.RequireNotification(s.T(), 5*time.Second, "notifications/tools/list_changed")
		// Then
		s.NotNil(notification, "WatchKubeConfig did not notify on iteration %d", i)
	}
}

func (s *WatchKubeConfigSuite) TestNotifiesPromptsChangeMultipleTimes() {
	// Given
	s.InitMcpClient()
	// When
	for i := 0; i < 3; i++ {
		capture := s.StartCapturingNotifications()
		s.WriteKubeconfig()
		notification := capture.RequireNotification(s.T(), 5*time.Second, "notifications/prompts/list_changed")
		// Then
		s.NotNil(notification, "WatchKubeConfig did not notify on iteration %d", i)
	}
}

func (s *WatchKubeConfigSuite) TestClearsNoLongerAvailableTools() {
	s.mockServer.Handle(test.NewInOpenShiftHandler())
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
		capture := s.StartCapturingNotifications()

		// Reload Config without OpenShift
		s.mockServer.ResetHandlers()
		s.WriteKubeconfig()
		capture.RequireNotification(s.T(), 5*time.Second, "notifications/tools/list_changed")
		time.Sleep(serverSettleDelay)

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
	s.handler = test.NewDiscoveryClientHandler()
	s.mockServer.Handle(s.handler)
	s.Cfg.KubeConfig = s.mockServer.KubeconfigFile(s.T())
}

func (s *WatchClusterStateSuite) TearDownTest() {
	s.BaseMcpSuite.TearDownTest()
	if s.mockServer != nil {
		s.mockServer.Close()
	}
}

func (s *WatchClusterStateSuite) AddAPIGroup(groupVersion string) {
	s.handler.AddAPIResourceList(metav1.APIResourceList{GroupVersion: groupVersion})
}

func (s *WatchClusterStateSuite) TestNotifiesToolsChangeOnAPIGroupAddition() {
	// Given - Initialize with basic API groups
	s.InitMcpClient()
	capture := s.StartCapturingNotifications()

	// When - Add a new API group to simulate cluster state change
	s.AddAPIGroup("custom.example.com/v1")

	notification := capture.RequireNotification(s.T(), 5*time.Second, "notifications/tools/list_changed")

	// Then
	s.NotNil(notification, "cluster state watcher did not notify")
}

func (s *WatchClusterStateSuite) TestNotifiesToolsChangeMultipleTimes() {
	// Given - Initialize with basic API groups
	s.InitMcpClient()

	// When - Add multiple API groups to simulate cluster state changes
	for i := 0; i < 3; i++ {
		capture := s.StartCapturingNotifications()
		s.AddAPIGroup(fmt.Sprintf("custom-%d.example.com/v1", i))
		notification := capture.RequireNotification(s.T(), 5*time.Second, "notifications/tools/list_changed")
		s.NotNil(notification, "cluster state watcher did not notify on iteration %d", i)
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
		capture := s.StartCapturingNotifications()

		// Simulate cluster becoming OpenShift by adding OpenShift API groups
		s.mockServer.ResetHandlers()
		s.mockServer.Handle(test.NewInOpenShiftHandler())

		capture.RequireNotification(s.T(), 5*time.Second, "notifications/tools/list_changed")
		time.Sleep(serverSettleDelay)

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

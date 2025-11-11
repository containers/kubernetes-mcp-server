package mcp

import (
	"context"
	"os"
	"testing"
	"time"

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
	s.mockServer = test.NewMockServer()
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

// WaitForNotification waits for an MCP server notification or fails the test after a timeout
func (s *WatchKubeConfigSuite) WaitForNotification() *mcp.JSONRPCNotification {
	withTimeout, cancel := context.WithTimeout(s.T().Context(), 5*time.Second)
	defer cancel()
	var notification *mcp.JSONRPCNotification
	s.OnNotification(func(n mcp.JSONRPCNotification) {
		notification = &n
	})
	for notification == nil {
		select {
		case <-withTimeout.Done():
			s.FailNow("timeout waiting for WatchKubeConfig notification")
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	return notification
}

func (s *WatchKubeConfigSuite) TestNotifiesToolsChange() {
	// Given
	s.InitMcpClient()
	// When
	s.WriteKubeconfig()
	notification := s.WaitForNotification()
	// Then
	s.NotNil(notification, "WatchKubeConfig did not notify")
	s.Equal("notifications/tools/list_changed", notification.Method, "WatchKubeConfig did not notify tools change")
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
		s.WaitForNotification()

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

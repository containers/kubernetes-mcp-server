package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

type EventsSuite struct {
	BaseMcpSuite
}

func (s *EventsSuite) TestEventsList() {
	s.InitMcpClient()
	s.Run("events_list (no events)", func() {
		toolResult, err := s.CallTool("events_list", map[string]interface{}{})
		s.Run("no error", func() {
			s.Nilf(err, "call tool failed %v", err)
			s.Falsef(toolResult.IsError, "call tool failed")
		})
		s.Run("returns no events message", func() {
			s.Equal("# No events found", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("events_list (with events)", func() {
		client := kubernetes.NewForConfigOrDie(envTestRestConfig)
		for _, ns := range []string{"default", "ns-1"} {
			_, eventCreateErr := client.CoreV1().Events(ns).Create(s.T().Context(), &v1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name: "an-event-in-" + ns,
				},
				InvolvedObject: v1.ObjectReference{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       "a-pod",
					Namespace:  ns,
				},
				Type:    "Normal",
				Message: "The event message",
			}, metav1.CreateOptions{})
			s.Require().NoError(eventCreateErr, "failed to create event in namespace %s", ns)
		}
		s.Run("events_list()", func() {
			toolResult, err := s.CallTool("events_list", map[string]interface{}{})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			s.Run("has yaml comment indicating output format", func() {
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# The following events (YAML format) were found:\n"), "unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
			})
			var decoded []v1.Event
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
			s.Run("has yaml content", func() {
				s.Nilf(err, "unmarshal failed %v", err)
			})
			s.Run("returns all events", func() {
				s.YAMLEqf(""+
					"- InvolvedObject:\n"+
					"    Kind: Pod\n"+
					"    Name: a-pod\n"+
					"    apiVersion: v1\n"+
					"  Message: The event message\n"+
					"  Namespace: default\n"+
					"  Reason: \"\"\n"+
					"  Timestamp: 0001-01-01 00:00:00 +0000 UTC\n"+
					"  Type: Normal\n"+
					"- InvolvedObject:\n"+
					"    Kind: Pod\n"+
					"    Name: a-pod\n"+
					"    apiVersion: v1\n"+
					"  Message: The event message\n"+
					"  Namespace: ns-1\n"+
					"  Reason: \"\"\n"+
					"  Timestamp: 0001-01-01 00:00:00 +0000 UTC\n"+
					"  Type: Normal\n",
					toolResult.Content[0].(mcp.TextContent).Text,
					"unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)

			})
		})
		s.Run("events_list(namespace=ns-1)", func() {
			toolResult, err := s.CallTool("events_list", map[string]interface{}{
				"namespace": "ns-1",
			})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			s.Run("has yaml comment indicating output format", func() {
				s.Truef(strings.HasPrefix(toolResult.Content[0].(mcp.TextContent).Text, "# The following events (YAML format) were found:\n"), "unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
			})
			var decoded []v1.Event
			err = yaml.Unmarshal([]byte(toolResult.Content[0].(mcp.TextContent).Text), &decoded)
			s.Run("has yaml content", func() {
				s.Nilf(err, "unmarshal failed %v", err)
			})
			s.Run("returns events from namespace", func() {
				s.YAMLEqf(""+
					"- InvolvedObject:\n"+
					"    Kind: Pod\n"+
					"    Name: a-pod\n"+
					"    apiVersion: v1\n"+
					"  Message: The event message\n"+
					"  Namespace: ns-1\n"+
					"  Reason: \"\"\n"+
					"  Timestamp: 0001-01-01 00:00:00 +0000 UTC\n"+
					"  Type: Normal\n",
					toolResult.Content[0].(mcp.TextContent).Text,
					"unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
			})
		})
	})
}

func (s *EventsSuite) TestEventsListDenied() {
	s.Require().NoError(toml.Unmarshal([]byte(`
		denied_resources = [ { version = "v1", kind = "Event" } ]
	`), s.Cfg), "Expected to parse denied resources config")
	s.InitMcpClient()
	s.Run("events_list (denied)", func() {
		toolResult, err := s.CallTool("events_list", map[string]interface{}{})
		s.Require().NotNil(toolResult, "toolResult should not be nil")
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			msg := toolResult.Content[0].(mcp.TextContent).Text
			s.Contains(msg, "resource not allowed:")
			expectedMessage := "failed to list events in all namespaces:(.+:)? resource not allowed: /v1, Kind=Event"
			s.Regexpf(expectedMessage, msg,
				"expected descriptive error '%s', got %v", expectedMessage, msg)
		})
	})
}

func (s *EventsSuite) TestEventsListForbidden() {
	s.InitMcpClient()
	defer restoreAuth(s.T().Context())
	client := kubernetes.NewForConfigOrDie(envTestRestConfig)
	// Remove all permissions - user will have forbidden access
	_ = client.RbacV1().ClusterRoles().Delete(s.T().Context(), "allow-all", metav1.DeleteOptions{})

	s.Run("events_list (forbidden)", func() {
		capture := s.StartCapturingLogNotifications()
		toolResult, _ := s.CallTool("events_list", map[string]interface{}{})
		s.Run("returns error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Contains(toolResult.Content[0].(mcp.TextContent).Text, "forbidden",
				"error message should indicate forbidden")
		})
		s.Run("sends log notification", func() {
			logNotification := capture.RequireLogNotification(s.T(), 2*time.Second)
			s.Equal("error", logNotification.Level, "forbidden errors should log at error level")
			s.Contains(logNotification.Data, "Permission denied", "log message should indicate permission denied")
		})
	})
}

func TestEvents(t *testing.T) {
	suite.Run(t, new(EventsSuite))
}

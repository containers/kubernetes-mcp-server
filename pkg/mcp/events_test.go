package mcp

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
			s.Equal("No events found", toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
	s.Run("events_list (with events)", func() {
		client := kubernetes.NewForConfigOrDie(envTestRestConfig)
		for _, ns := range []string{"default", "ns-1"} {
			_, _ = client.CoreV1().Events(ns).Create(s.T().Context(), &v1.Event{
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
		}
		s.Run("events_list()", func() {
			toolResult, err := s.CallTool("events_list", map[string]interface{}{})
			s.Run("no error", func() {
				s.Nilf(err, "call tool failed %v", err)
				s.Falsef(toolResult.IsError, "call tool failed")
			})
			s.Run("returns all events", func() {
				s.Equalf("The following events (YAML format) were found:\n"+
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
			s.Run("returns events from namespace", func() {
				s.Equalf("The following events (YAML format) were found:\n"+
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
		s.Run("has error", func() {
			s.Truef(toolResult.IsError, "call tool should fail")
			s.Nilf(err, "call tool should not return error object")
		})
		s.Run("describes denial", func() {
			expectedMessage := "failed to list events in all namespaces: resource not allowed: /v1, Kind=Event"
			s.Equalf(expectedMessage, toolResult.Content[0].(mcp.TextContent).Text,
				"expected descriptive error '%s', got %v", expectedMessage, toolResult.Content[0].(mcp.TextContent).Text)
		})
	})
}

func TestEvents(t *testing.T) {
	suite.Run(t, new(EventsSuite))
}

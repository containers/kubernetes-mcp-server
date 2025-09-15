package mcp

import (
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	"github.com/mark3labs/mcp-go/mcp"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestEventsList(t *testing.T) {
	testCase(t, func(c *mcpContext) {
		c.withEnvTest()
		toolResult, err := c.callTool("events_list", map[string]interface{}{})
		t.Run("events_list with no events returns OK", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if toolResult.IsError {
				t.Fatalf("call tool failed")
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "No events found" {
				t.Fatalf("unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
			}
		})
		client := c.newKubernetesClient()
		for _, ns := range []string{"default", "ns-1"} {
			_, _ = client.CoreV1().Events(ns).Create(c.ctx, &v1.Event{
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
		toolResult, err = c.callTool("events_list", map[string]interface{}{})
		t.Run("events_list with events returns all OK", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if toolResult.IsError {
				t.Fatalf("call tool failed")
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "The following events (YAML format) were found:\n"+
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
				"  Type: Normal\n" {
				t.Fatalf("unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
			}
		})
		toolResult, err = c.callTool("events_list", map[string]interface{}{
			"namespace": "ns-1",
		})
		t.Run("events_list in namespace with events returns from namespace OK", func(t *testing.T) {
			if err != nil {
				t.Fatalf("call tool failed %v", err)
			}
			if toolResult.IsError {
				t.Fatalf("call tool failed")
			}
			if toolResult.Content[0].(mcp.TextContent).Text != "The following events (YAML format) were found:\n"+
				"- InvolvedObject:\n"+
				"    Kind: Pod\n"+
				"    Name: a-pod\n"+
				"    apiVersion: v1\n"+
				"  Message: The event message\n"+
				"  Namespace: ns-1\n"+
				"  Reason: \"\"\n"+
				"  Timestamp: 0001-01-01 00:00:00 +0000 UTC\n"+
				"  Type: Normal\n" {
				t.Fatalf("unexpected result %v", toolResult.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}

func TestEventsListDenied(t *testing.T) {
	deniedResourcesServer := &config.StaticConfig{DeniedResources: []config.GroupVersionKind{{Version: "v1", Kind: "Event"}}}
	testCaseWithContext(t, &mcpContext{staticConfig: deniedResourcesServer}, func(c *mcpContext) {
		c.withEnvTest()
		eventList, _ := c.callTool("events_list", map[string]interface{}{})
		t.Run("events_list has error", func(t *testing.T) {
			if !eventList.IsError {
				t.Fatalf("call tool should fail")
			}
		})
		t.Run("events_list describes denial", func(t *testing.T) {
			expectedMessage := "failed to list events in all namespaces: resource not allowed: /v1, Kind=Event"
			if eventList.Content[0].(mcp.TextContent).Text != expectedMessage {
				t.Fatalf("expected descriptive error '%s', got %v", expectedMessage, eventList.Content[0].(mcp.TextContent).Text)
			}
		})
	})
}

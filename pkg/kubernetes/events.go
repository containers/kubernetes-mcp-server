package kubernetes

import (
	"context"
	"strings"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (c *Core) EventsList(ctx context.Context, namespace string) ([]map[string]any, error) {
	var eventMap []map[string]any
	raw, err := c.ResourcesList(ctx, &schema.GroupVersionKind{
		Group: "", Version: "v1", Kind: "Event",
	}, namespace, api.ListOptions{})
	if err != nil {
		return eventMap, err
	}
	unstructuredList := raw.(*unstructured.UnstructuredList)
	if len(unstructuredList.Items) == 0 {
		return eventMap, nil
	}
	for _, item := range unstructuredList.Items {
		event := &v1.Event{}
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, event); err != nil {
			return eventMap, err
		}
		timestamp := event.EventTime.Time
		if timestamp.IsZero() && event.Series != nil {
			timestamp = event.Series.LastObservedTime.Time
		} else if timestamp.IsZero() && event.Count > 1 {
			timestamp = event.LastTimestamp.Time
		} else if timestamp.IsZero() {
			timestamp = event.FirstTimestamp.Time
		}
		eventMap = append(eventMap, map[string]any{
			"Namespace": event.Namespace,
			"Timestamp": timestamp.String(),
			"Type":      event.Type,
			"Reason":    event.Reason,
			"InvolvedObject": map[string]string{
				"apiVersion": event.InvolvedObject.APIVersion,
				"Kind":       event.InvolvedObject.Kind,
				"Name":       event.InvolvedObject.Name,
			},
			"Message": strings.TrimSpace(event.Message),
		})
	}
	return eventMap, nil
}

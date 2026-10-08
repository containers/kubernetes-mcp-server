package core

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// tableAppStructured converts Kubernetes lists into compact, predictable rows
// for the Pod, Namespace, and Project tools, whose status is expressed as a
// phase. This is independent from list_output so the app receives usable rows
// when the tool's text result is YAML. Generic resources_list results do not
// use this helper: they retain API-server table cells or full YAML objects.
func tableAppStructured(ret runtime.Unstructured, structured any) map[string]any {
	columns := []string{"Name", "Namespace", "Status", "Age", "Labels", "apiVersion", "kind"}
	list, ok := ret.(*unstructured.UnstructuredList)
	if !ok {
		if rows, ok := structured.([]map[string]any); ok {
			return map[string]any{"columns": columns, "items": rows}
		}
		return map[string]any{"columns": columns, "items": []map[string]any{}}
	}
	rows := make([]map[string]any, 0, len(list.Items))
	for _, item := range list.Items {
		row := map[string]any{
			"Name":       item.GetName(),
			"Namespace":  item.GetNamespace(),
			"Age":        appAge(item.GetCreationTimestamp().Time),
			"Labels":     appLabels(item.GetLabels()),
			"apiVersion": item.GetAPIVersion(),
			"kind":       item.GetKind(),
		}
		if phase, found, _ := unstructured.NestedString(item.Object, "status", "phase"); found {
			row["Status"] = phase
		}
		if row["Namespace"] == "" {
			delete(row, "Namespace")
		}
		rows = append(rows, row)
	}
	return map[string]any{"columns": columns, "items": rows}
}

func appLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+labels[key])
	}
	return strings.Join(values, ",")
}

func appAge(created time.Time) string {
	if created.IsZero() {
		return ""
	}
	d := time.Since(created)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

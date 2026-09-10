package prompts

import (
	"fmt"
	"strconv"

	"github.com/containers/kubernetes-mcp-server/pkg/tasks"
)

// extractTaskResult returns the first TaskResult for the given task name.
func extractTaskResult(results []tasks.TaskResult, name string) (tasks.TaskResult, bool) {
	for _, r := range results {
		if r.Name == name {
			return r, true
		}
	}
	return tasks.TaskResult{}, false
}

// extractTaskOutput is a generic helper that extracts a typed output from
// a slice of TaskResults by task name. Returns the zero value if not found
// or if the output type doesn't match.
func extractTaskOutput[T any](results []tasks.TaskResult, name string) (T, bool) {
	var zero T
	if r, ok := extractTaskResult(results, name); ok {
		if r.Err == nil && r.Output != nil {
			if v, ok := r.Output.(T); ok {
				return v, true
			}
		}
	}
	return zero, false
}

// extractRequiredTaskOutput is the strict variant of extractTaskOutput.
// It returns an error when the named task result is missing, failed, nil,
// or not assignable to the requested type.
func extractRequiredTaskOutput[T any](results []tasks.TaskResult, name string) (T, error) {
	var zero T
	r, ok := extractTaskResult(results, name)
	if !ok {
		return zero, fmt.Errorf("missing gather result for %s", name)
	}
	if r.Err != nil {
		return zero, fmt.Errorf("failed to gather %s: %w", name, r.Err)
	}
	if r.Output == nil {
		return zero, fmt.Errorf("gather result for %s returned no output", name)
	}
	v, ok := r.Output.(T)
	if !ok {
		return zero, fmt.Errorf("unexpected gather output type for %s: %T", name, r.Output)
	}
	return v, nil
}

// extractRequiredTaskOutputAny returns the raw output for a named task result.
// It returns an error when the task result is missing, failed, or has nil
// output.
func extractRequiredTaskOutputAny(results []tasks.TaskResult, name string) (any, error) {
	r, ok := extractTaskResult(results, name)
	if !ok {
		return nil, fmt.Errorf("missing gather result for %s", name)
	}
	if r.Err != nil {
		return nil, fmt.Errorf("failed to gather %s: %w", name, r.Err)
	}
	if r.Output == nil {
		return nil, fmt.Errorf("gather result for %s returned no output", name)
	}
	return r.Output, nil
}

// parseCheckEvents parses the check_events prompt argument.
// It defaults to true when the argument is missing or cannot be parsed as a bool.
func parseCheckEvents(value string) bool {
	if value == "" {
		return true
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return true
	}
	return b
}

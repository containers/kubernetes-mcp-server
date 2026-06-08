package logs

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"

	"github.com/rhobs/obs-mcp/pkg/logs/discovery"
	"github.com/rhobs/obs-mcp/pkg/logs/loki"
	"github.com/rhobs/obs-mcp/pkg/prometheus"
	"github.com/rhobs/obs-mcp/pkg/tools"
)

const (
	defaultQueryLookback = 15 * time.Minute
	defaultQueryLimit    = 100
	maxQueryLimit        = 1000
)

func LabelNamesHandler(params ToolParams) (LabelNamesOutput, error) {
	client, err := params.getLokiClient()
	if err != nil {
		return LabelNamesOutput{}, err
	}

	input := buildLabelNamesInput(params.arguments)
	start, end, err := parseDefaultTimeRange(input.Start, input.End)
	if err != nil {
		return LabelNamesOutput{}, err
	}

	labels, err := client.LabelNames(params.context, start, end)
	if err != nil {
		return LabelNamesOutput{}, fmt.Errorf("failed to list Loki label names: %w", err)
	}
	return LabelNamesOutput{Labels: labels}, nil
}

func LabelValuesHandler(params ToolParams) (LabelValuesOutput, error) {
	client, err := params.getLokiClient()
	if err != nil {
		return LabelValuesOutput{}, err
	}

	input := buildLabelValuesInput(params.arguments)
	if input.Label == "" {
		return LabelValuesOutput{}, fmt.Errorf("label parameter is required and must be a string")
	}

	start, end, err := parseDefaultTimeRange(input.Start, input.End)
	if err != nil {
		return LabelValuesOutput{}, err
	}

	values, err := client.LabelValues(params.context, input.Label, start, end)
	if err != nil {
		return LabelValuesOutput{}, fmt.Errorf("failed to list Loki label values: %w", err)
	}
	return LabelValuesOutput{Values: values}, nil
}

func QueryRangeHandler(params ToolParams) (QueryRangeOutput, error) {
	client, err := params.getLokiClient()
	if err != nil {
		return QueryRangeOutput{}, err
	}

	input := buildQueryRangeInput(params.arguments)
	if input.Query == "" {
		return QueryRangeOutput{}, fmt.Errorf("query parameter is required and must be a string")
	}

	start, end, err := parseQueryTimeRange(input)
	if err != nil {
		return QueryRangeOutput{}, err
	}

	direction := input.Direction
	if direction == "" {
		direction = "backward"
	}
	if direction != "backward" && direction != "forward" {
		return QueryRangeOutput{}, fmt.Errorf("direction must be either backward or forward")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	if limit > maxQueryLimit {
		limit = maxQueryLimit
	}

	result, err := client.QueryRange(params.context, loki.QueryRangeInput{
		Query:     input.Query,
		Start:     start,
		End:       end,
		Limit:     limit,
		Direction: direction,
	})
	if err != nil {
		return QueryRangeOutput{}, fmt.Errorf("failed to execute Loki query_range: %w", err)
	}

	return QueryRangeOutput{
		ResultType: result.ResultType,
		Streams:    result.Streams,
	}, nil
}

func ListInstancesHandler(params ToolParams) (ListInstancesOutput, error) {
	instances, err := discovery.ListInstances(params.context, params.dynamicClient, params.useRoute())
	if err != nil {
		return ListInstancesOutput{}, err
	}

	output := make([]LokiInstance, 0, len(instances))
	for _, instance := range instances {
		output = append(output, LokiInstance{
			LokiNamespace: instance.Namespace,
			LokiName:      instance.Name,
			Status:        instance.Status,
			URL:           instance.GetURL(),
		})
	}
	return ListInstancesOutput{Instances: output}, nil
}

func buildLabelNamesInput(args map[string]any) LabelNamesInput {
	return LabelNamesInput{
		LokiNamespace: tools.GetString(args, "lokiNamespace", ""),
		LokiName:      tools.GetString(args, "lokiName", ""),
		Tenant:        tools.GetString(args, "tenant", ""),
		Start:         tools.GetString(args, "start", ""),
		End:           tools.GetString(args, "end", ""),
	}
}

func buildLabelValuesInput(args map[string]any) LabelValuesInput {
	return LabelValuesInput{
		LokiNamespace: tools.GetString(args, "lokiNamespace", ""),
		LokiName:      tools.GetString(args, "lokiName", ""),
		Tenant:        tools.GetString(args, "tenant", ""),
		Label:         tools.GetString(args, "label", ""),
		Start:         tools.GetString(args, "start", ""),
		End:           tools.GetString(args, "end", ""),
	}
}

func buildQueryRangeInput(args map[string]any) QueryRangeInput {
	return QueryRangeInput{
		LokiNamespace: tools.GetString(args, "lokiNamespace", ""),
		LokiName:      tools.GetString(args, "lokiName", ""),
		Tenant:        tools.GetString(args, "tenant", ""),
		Query:         tools.GetString(args, "query", ""),
		Start:         tools.GetString(args, "start", ""),
		End:           tools.GetString(args, "end", ""),
		Duration:      tools.GetString(args, "duration", ""),
		Direction:     tools.GetString(args, "direction", ""),
		Limit:         tools.GetInt(args, "limit", defaultQueryLimit),
	}
}

func parseDefaultTimeRange(start, end string) (startTime, endTime time.Time, err error) {
	if start == "" && end == "" {
		endTime = time.Now()
		startTime = endTime.Add(-defaultQueryLookback)
		return startTime, endTime, nil
	}
	if (start == "") != (end == "") {
		return time.Time{}, time.Time{}, fmt.Errorf("both start and end must be provided together")
	}

	startTime, err = prometheus.ParseTimestamp(start)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start time format: %w", err)
	}
	endTime, err = prometheus.ParseTimestamp(end)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end time format: %w", err)
	}
	if startTime.After(endTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("start must be before or equal to end")
	}
	return startTime, endTime, nil
}

func parseQueryTimeRange(input QueryRangeInput) (start, end time.Time, err error) {
	if input.Start != "" || input.End != "" {
		return parseDefaultTimeRange(input.Start, input.End)
	}

	duration := defaultQueryLookback
	if input.Duration != "" {
		d, parseErr := model.ParseDuration(input.Duration)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid duration format: %w", parseErr)
		}
		duration = time.Duration(d)
		if duration <= 0 {
			return time.Time{}, time.Time{}, fmt.Errorf("duration must be positive")
		}
	}

	end = time.Now()
	start = end.Add(-duration)
	return start, end, nil
}

func (params ToolParams) getLokiClient() (loki.Loader, error) {
	url, err := params.resolveLokiURL()
	if err != nil {
		return nil, err
	}
	tenant := tools.GetString(params.arguments, "tenant", "")
	return params.newLokiLoader(url, tenant)
}

func (params ToolParams) resolveLokiURL() (string, error) {
	namespace := tools.GetString(params.arguments, "lokiNamespace", "")
	name := tools.GetString(params.arguments, "lokiName", "")
	if namespace != "" || name != "" {
		if namespace == "" || name == "" {
			return "", fmt.Errorf("both lokiNamespace and lokiName must be provided together")
		}
		instances, err := discovery.ListInstances(params.context, params.dynamicClient, params.useRoute())
		if err != nil {
			return "", err
		}
		instance, err := discovery.FindInstanceByName(instances, namespace, name)
		if err != nil {
			return "", err
		}
		return instance.GetURL(), nil
	}

	if params.config != nil && params.config.LokiURL != "" {
		return params.config.LokiURL, nil
	}
	return "", fmt.Errorf("loki URL not configured; set loki_url/--loki-url/LOKI_URL or provide lokiNamespace and lokiName")
}

func (params ToolParams) useRoute() bool {
	if params.config == nil {
		return false
	}
	return params.config.UseRoute
}

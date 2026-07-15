package logs

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/prometheus/common/model"

	"github.com/rhobs/obs-mcp/pkg/auth"
	"github.com/rhobs/obs-mcp/pkg/logs/discovery"
	"github.com/rhobs/obs-mcp/pkg/logs/loki"
	"github.com/rhobs/obs-mcp/pkg/prometheus"
)

const (
	defaultQueryLookback = 15 * time.Minute
	defaultQueryLimit    = 100
	maxQueryLimit        = 1000
)

func labelNamesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	startStr := p.OptionalString("start", "")
	endStr := p.OptionalString("end", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list label names: %w", err)), nil
	}

	start, end, err := parseDefaultTimeRange(startStr, endStr)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	client, err := getLokiClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	labels, err := client.LabelNames(params.Context, start, end)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list Loki label names: %w", err)), nil
	}

	return api.NewToolCallResultStructured(LabelNamesOutput{Labels: labels}, nil), nil
}

func labelValuesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	label := p.RequiredString("label")
	startStr := p.OptionalString("start", "")
	endStr := p.OptionalString("end", "")
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list label values: %w", err)), nil
	}

	if label == "" {
		return api.NewToolCallResult("", fmt.Errorf("label parameter is required and must be a string")), nil
	}

	start, end, err := parseDefaultTimeRange(startStr, endStr)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	client, err := getLokiClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	values, err := client.LabelValues(params.Context, label, start, end)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to list Loki label values: %w", err)), nil
	}

	return api.NewToolCallResultStructured(LabelValuesOutput{Values: values}, nil), nil
}

func queryRangeHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	p := api.WrapParams(params)
	query := p.RequiredString("query")
	startStr := p.OptionalString("start", "")
	endStr := p.OptionalString("end", "")
	duration := p.OptionalString("duration", "")
	direction := p.OptionalString("direction", "")
	limit := int(p.OptionalInt64("limit", int64(defaultQueryLimit)))
	if err := p.Err(); err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to execute query range: %w", err)), nil
	}

	if query == "" {
		return api.NewToolCallResult("", fmt.Errorf("query parameter is required and must be a string")), nil
	}

	start, end, err := parseQueryTimeRange(startStr, endStr, duration)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	if direction == "" {
		direction = "backward"
	}
	if direction != "backward" && direction != "forward" {
		return api.NewToolCallResult("", fmt.Errorf("direction must be either backward or forward")), nil
	}

	if limit <= 0 {
		limit = defaultQueryLimit
	}
	if limit > maxQueryLimit {
		limit = maxQueryLimit
	}

	client, err := getLokiClient(params)
	if err != nil {
		return api.NewToolCallResult("", err), nil
	}

	result, err := client.QueryRange(params.Context, loki.QueryRangeInput{
		Query:     query,
		Start:     start,
		End:       end,
		Limit:     limit,
		Direction: direction,
	})
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to execute Loki query_range: %w", err)), nil
	}

	return api.NewToolCallResultStructured(QueryRangeOutput{
		ResultType: result.ResultType,
		Streams:    result.Streams,
	}, nil), nil
}

func listInstancesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	cfg := GetConfig(params)
	instances, err := discovery.ListInstances(params.Context, params.DynamicClient(), cfg.UseRoute)
	if err != nil {
		return api.NewToolCallResult("", err), nil
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

	result := ListInstancesOutput{Instances: output}
	return api.NewToolCallResultStructured(result, nil), nil
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

func parseQueryTimeRange(startStr, endStr, durationStr string) (start, end time.Time, err error) {
	if startStr != "" || endStr != "" {
		return parseDefaultTimeRange(startStr, endStr)
	}

	dur := defaultQueryLookback
	if durationStr != "" {
		d, parseErr := model.ParseDuration(durationStr)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid duration format: %w", parseErr)
		}
		dur = time.Duration(d)
		if dur <= 0 {
			return time.Time{}, time.Time{}, fmt.Errorf("duration must be positive")
		}
	}

	end = time.Now()
	start = end.Add(-dur)
	return start, end, nil
}

func getLokiClient(params api.ToolHandlerParams) (loki.Loader, error) {
	cfg := GetConfig(params)

	url, err := resolveLokiURL(params)
	if err != nil {
		return nil, err
	}

	tenant := api.WrapParams(params).OptionalString("tenant", "")

	tls := strings.HasPrefix(url, "https://")
	rt, err := auth.BuildRoundTripper(params.Context, params.RESTConfig(), cfg.GetAuthMode(), tls, cfg.Insecure)
	if err != nil {
		return nil, fmt.Errorf("failed to create round tripper: %w", err)
	}

	httpClient := &http.Client{
		Timeout:   loki.RequestTimeout,
		Transport: rt,
	}
	return loki.NewHTTPLoader(httpClient, url, tenant), nil
}

func resolveLokiURL(params api.ToolHandlerParams) (string, error) {
	cfg := GetConfig(params)
	if cfg != nil && cfg.LokiURL != "" {
		return cfg.LokiURL, nil
	}

	p := api.WrapParams(params)
	namespace := p.OptionalString("lokiNamespace", "")
	name := p.OptionalString("lokiName", "")

	if namespace != "" || name != "" {
		if namespace == "" || name == "" {
			return "", fmt.Errorf("both lokiNamespace and lokiName must be provided together")
		}
		if err := p.Err(); err != nil {
			return "", err
		}

		instances, err := discovery.ListInstances(params.Context, params.DynamicClient(), cfg.UseRoute)
		if err != nil {
			return "", err
		}
		instance, err := discovery.FindInstanceByName(instances, namespace, name)
		if err != nil {
			return "", err
		}
		return instance.GetURL(), nil
	}

	return "", errors.New("loki URL not configured; set loki_url/--loki-url/LOKI_URL or provide lokiNamespace and lokiName")
}

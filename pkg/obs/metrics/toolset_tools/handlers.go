package toolset_tools

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"

	tools "github.com/rhobs/obs-mcp/pkg/metrics"
)

// ListMetricsHandler handles the listing of available Prometheus metrics.
func ListMetricsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return tools.ListMetricsHandler(params.Context, promClient, tools.BuildListMetricsInput(params.GetArguments())).ToToolsetResult()
}

// ExecuteInstantQueryHandler handles the execution of Prometheus instant queries.
func ExecuteInstantQueryHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return tools.ExecuteInstantQueryHandler(params.Context, promClient, tools.BuildInstantQueryInput(params.GetArguments())).ToToolsetResult()
}

// ExecuteRangeQueryHandler handles the execution of Prometheus range queries.
func ExecuteRangeQueryHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	cfg := getConfig(params)
	return tools.ExecuteRangeQueryHandler(params.Context, promClient, tools.BuildRangeQueryInput(params.GetArguments()), cfg.RangeQueryFullResponse).ToToolsetResult()
}

// ShowTimeseriesHandler handles the show_timeseries tool.
func ShowTimeseriesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return tools.ShowTimeseriesHandler(params.Context, promClient, tools.BuildShowTimeseriesInput(params.GetArguments())).ToToolsetResult()
}

// GetLabelNamesHandler handles the retrieval of label names.
func GetLabelNamesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return tools.GetLabelNamesHandler(params.Context, promClient, tools.BuildLabelNamesInput(params.GetArguments())).ToToolsetResult()
}

// GetLabelValuesHandler handles the retrieval of label values.
func GetLabelValuesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return tools.GetLabelValuesHandler(params.Context, promClient, tools.BuildLabelValuesInput(params.GetArguments())).ToToolsetResult()
}

// GetSeriesHandler handles the retrieval of time series.
func GetSeriesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return tools.GetSeriesHandler(params.Context, promClient, tools.BuildSeriesInput(params.GetArguments())).ToToolsetResult()
}

// GetAlertsHandler handles the retrieval of alerts from Alertmanager.
func GetAlertsHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}

	return tools.GetAlertsHandler(params.Context, amClient, tools.BuildAlertsInput(params.GetArguments())).ToToolsetResult()
}

// GetSilencesHandler handles the retrieval of silences from Alertmanager.
func GetSilencesHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}

	return tools.GetSilencesHandler(params.Context, amClient, tools.BuildSilencesInput(params.GetArguments())).ToToolsetResult()
}

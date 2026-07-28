package metrics

import (
	"fmt"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

func ListMetricsToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return ListMetricsHandler(params.Context, promClient, BuildListMetricsInput(params.GetArguments())).ToToolsetResult()
}

func ExecuteInstantQueryToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return ExecuteInstantQueryHandler(params.Context, promClient, BuildInstantQueryInput(params.GetArguments())).ToToolsetResult()
}

func ExecuteRangeQueryToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	cfg := getConfig(params)
	return ExecuteRangeQueryHandler(params.Context, promClient, BuildRangeQueryInput(params.GetArguments()), cfg.RangeQueryFullResponse).ToToolsetResult()
}

func ShowTimeseriesToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return ShowTimeseriesHandler(params.Context, promClient, BuildShowTimeseriesInput(params.GetArguments())).ToToolsetResult()
}

func GetLabelNamesToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return GetLabelNamesHandler(params.Context, promClient, BuildLabelNamesInput(params.GetArguments())).ToToolsetResult()
}

func GetLabelValuesToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return GetLabelValuesHandler(params.Context, promClient, BuildLabelValuesInput(params.GetArguments())).ToToolsetResult()
}

func GetSeriesToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	promClient, err := getPromClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Prometheus client: %w", err)), nil
	}

	return GetSeriesHandler(params.Context, promClient, BuildSeriesInput(params.GetArguments())).ToToolsetResult()
}

func GetAlertsToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}

	return GetAlertsHandler(params.Context, amClient, BuildAlertsInput(params.GetArguments())).ToToolsetResult()
}

func GetSilencesToolsetHandler(params api.ToolHandlerParams) (*api.ToolCallResult, error) {
	amClient, err := getAlertmanagerClient(params)
	if err != nil {
		return api.NewToolCallResult("", fmt.Errorf("failed to create Alertmanager client: %w", err)), nil
	}

	return GetSilencesHandler(params.Context, amClient, BuildSilencesInput(params.GetArguments())).ToToolsetResult()
}

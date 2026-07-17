package toolset_tools

import (
	"github.com/containers/kubernetes-mcp-server/pkg/api"

	tools "github.com/rhobs/obs-mcp/pkg/metrics"
)

// InitListMetrics creates the list_metrics tool.
func InitListMetrics() []api.ServerTool {
	return []api.ServerTool{
		tools.ListMetrics.ToServerTool(ListMetricsHandler),
	}
}

// InitExecuteInstantQuery creates the execute_instant_query tool.
func InitExecuteInstantQuery() []api.ServerTool {
	return []api.ServerTool{
		tools.ExecuteInstantQuery.ToServerTool(ExecuteInstantQueryHandler),
	}
}

// InitExecuteRangeQuery creates the execute_range_query tool.
func InitExecuteRangeQuery() []api.ServerTool {
	return []api.ServerTool{
		tools.ExecuteRangeQuery.ToServerTool(ExecuteRangeQueryHandler),
	}
}

// InitShowTimeseries creates the show_timeseries tool.
func InitShowTimeseries() []api.ServerTool {
	return []api.ServerTool{
		tools.ShowTimeseries.ToServerTool(ShowTimeseriesHandler),
	}
}

// InitGetLabelNames creates the get_label_names tool.
func InitGetLabelNames() []api.ServerTool {
	return []api.ServerTool{
		tools.GetLabelNames.ToServerTool(GetLabelNamesHandler),
	}
}

// InitGetLabelValues creates the get_label_values tool.
func InitGetLabelValues() []api.ServerTool {
	return []api.ServerTool{
		tools.GetLabelValues.ToServerTool(GetLabelValuesHandler),
	}
}

// InitGetSeries creates the get_series tool.
func InitGetSeries() []api.ServerTool {
	return []api.ServerTool{
		tools.GetSeries.ToServerTool(GetSeriesHandler),
	}
}

// InitGetAlerts creates the get_alerts tool.
func InitGetAlerts() []api.ServerTool {
	return []api.ServerTool{
		tools.GetAlerts.ToServerTool(GetAlertsHandler),
	}
}

// InitGetSilences creates the get_silences tool.
func InitGetSilences() []api.ServerTool {
	return []api.ServerTool{
		tools.GetSilences.ToServerTool(GetSilencesHandler),
	}
}

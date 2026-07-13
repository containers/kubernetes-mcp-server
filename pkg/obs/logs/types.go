package logs

import (
	"github.com/rhobs/obs-mcp/pkg/logs/loki"
	"github.com/rhobs/obs-mcp/pkg/tools"
)

type LabelNamesOutput struct {
	Labels []string `json:"labels"`
}

var labelNamesOutputSchema = tools.MustSchema[LabelNamesOutput]()

type LabelValuesOutput struct {
	Values []string `json:"values"`
}

var labelValuesOutputSchema = tools.MustSchema[LabelValuesOutput]()

type ListInstancesOutput struct {
	Instances []LokiInstance `json:"instances"`
}

var listInstancesOutputSchema = tools.MustSchema[ListInstancesOutput]()

type LokiInstance struct {
	LokiNamespace string `json:"lokiNamespace"`
	LokiName      string `json:"lokiName"`
	Status        string `json:"status"`
	URL           string `json:"url"`
}

type QueryRangeOutput struct {
	ResultType string        `json:"resultType"`
	Streams    []loki.Stream `json:"streams"`
}

var queryRangeOutputSchema = tools.MustSchema[QueryRangeOutput]()

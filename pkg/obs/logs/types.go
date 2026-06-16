package logs

import "github.com/rhobs/obs-mcp/pkg/logs/loki"

type LabelNamesInput struct {
	LokiNamespace string `json:"lokiNamespace,omitempty"`
	LokiName      string `json:"lokiName,omitempty"`
	Tenant        string `json:"tenant,omitempty"`
	Start         string `json:"start,omitempty"`
	End           string `json:"end,omitempty"`
}

type LabelNamesOutput struct {
	Labels []string `json:"labels"`
}

type LabelValuesInput struct {
	LokiNamespace string `json:"lokiNamespace,omitempty"`
	LokiName      string `json:"lokiName,omitempty"`
	Tenant        string `json:"tenant,omitempty"`
	Label         string `json:"label"`
	Start         string `json:"start,omitempty"`
	End           string `json:"end,omitempty"`
}

type LabelValuesOutput struct {
	Values []string `json:"values"`
}

type QueryRangeInput struct {
	LokiNamespace string `json:"lokiNamespace,omitempty"`
	LokiName      string `json:"lokiName,omitempty"`
	Tenant        string `json:"tenant,omitempty"`
	Query         string `json:"query"`
	Start         string `json:"start,omitempty"`
	End           string `json:"end,omitempty"`
	Duration      string `json:"duration,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	Direction     string `json:"direction,omitempty"`
}

type ListInstancesOutput struct {
	Instances []LokiInstance `json:"instances"`
}

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

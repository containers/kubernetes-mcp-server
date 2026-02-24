package kiali

// ServicesListResponse is the raw response from Kiali /api/services.
// Used only for unmarshaling; transform to ServicesByCluster for output.
type ServicesListResponse struct {
	Cluster     string              `json:"cluster"`
	Services    []ServiceListItem   `json:"services"`
	Validations ServicesValidations `json:"validations"`
}

// ServiceListItem is one service entry in the services list API response.
type ServiceListItem struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	Labels       map[string]string `json:"labels"`
	Health       ServiceListHealth `json:"health"`
	IstioRefs    []IstioRef        `json:"istioReferences"`
	AppLabel     bool              `json:"appLabel"`
	VersionLabel bool              `json:"versionLabel"`
}

// ServiceListHealth holds health status from the services list API (health.status.status).
type ServiceListHealth struct {
	Status ServiceHealthStatus `json:"status"`
}

// ServiceHealthStatus holds the status string (e.g. "NA", "HEALTHY", "DEGRADED").
type ServiceHealthStatus struct {
	Status string `json:"status"`
}

// IstioRef is an Istio resource reference (Gateway, VirtualService, etc.).
type IstioRef struct {
	ObjectGVK struct {
		Group string `json:"Group"`
		Kind  string `json:"Kind"`
	} `json:"objectGVK"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`
}

// ServicesValidations holds per-service validation; key is "name.namespace".
type ServicesValidations struct {
	Service map[string]ServiceValidation `json:"service"`
}

// ServiceValidation holds whether the service config is valid and any validation checks.
type ServiceValidation struct {
	Valid  bool    `json:"valid"`
	Checks []Check `json:"checks"`
}

// Check is a single validation check (e.g. KIA0601).
type Check struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	Path     string `json:"path"`
}

// ServiceSummary is the simplified service info returned to callers (e.g. MCP tools).
// Health comes from health.status.status; Configuration is "True" when valid, or comma-separated check codes (e.g. "KIA0601, KIA0602") when invalid; Details from istioReferences; Labels as key=value or "None".
type ServiceSummary struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Health        string `json:"health"`
	Configuration string `json:"configuration"` // "True" when valid; when invalid, "Code(message)" per check, e.g. "KIA0601(msg1), KIA0602(msg2)"
	Details       string `json:"details"`       // e.g. "bookinfo(VS), bookinfo-gateway(GW)" from istioReferences
	Labels        string `json:"labels"`        // key=value pairs or "None"
}

// ServicesByCluster groups service summaries by cluster name.
type ServicesByCluster map[string][]ServiceSummary

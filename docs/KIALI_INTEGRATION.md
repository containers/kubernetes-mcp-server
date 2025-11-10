## Kiali integration

This server can expose Kiali tools so assistants can query mesh information (e.g., mesh status/graph).

### Enable the Kiali toolset

Enable the Kiali tools via the server TOML configuration file.

Config (TOML):

```toml
toolsets = ["core", "kiali"]

[toolset_configs.kiali]
url = "https://kiali.example"
# insecure = true  # optional: allow insecure TLS (not recommended in production)
# certificate_authority = """-----BEGIN CERTIFICATE-----
# MIID...
# -----END CERTIFICATE-----"""
# When url is https and insecure is false, certificate_authority is required.
```

When the `kiali` toolset is enabled, a Kiali toolset configuration is required via `[toolset_configs.kiali]`. If missing or invalid, the server will refuse to start.

### How authentication works

- The server uses your existing Kubernetes credentials (from kubeconfig or in-cluster) to set a bearer token for Kiali calls.
- If you pass an HTTP Authorization header to the MCP HTTP endpoint, that is not required for Kiali; Kiali calls use the server's configured token.

### Available tools (initial)

<details>

<summary>kiali</summary>

- **graph** - Check the status of my mesh by querying Kiali graph
  - `namespace` (`string`) - Optional single namespace to include in the graph (alternative to namespaces)
  - `namespaces` (`string`) - Optional comma-separated list of namespaces to include in the graph

- **mesh_status** - Get the status of mesh components including Istio, Kiali, Grafana, Prometheus and their interactions, versions, and health status

- **istio_config** - Get all Istio configuration objects in the mesh including their full YAML resources and details

- **istio_object_details** - Get detailed information about a specific Istio object including validation and help information
  - `group` (`string`) **(required)** - API group of the Istio object (e.g., 'networking.istio.io', 'gateway.networking.k8s.io')
  - `kind` (`string`) **(required)** - Kind of the Istio object (e.g., 'DestinationRule', 'VirtualService', 'HTTPRoute', 'Gateway')
  - `name` (`string`) **(required)** - Name of the Istio object
  - `namespace` (`string`) **(required)** - Namespace containing the Istio object
  - `version` (`string`) **(required)** - API version of the Istio object (e.g., 'v1', 'v1beta1')

- **istio_object_patch** - Modify an existing Istio object using PATCH method. The JSON patch data will be applied to the existing object.
  - `group` (`string`) **(required)** - API group of the Istio object (e.g., 'networking.istio.io', 'gateway.networking.k8s.io')
  - `json_patch` (`string`) **(required)** - JSON patch data to apply to the object
  - `kind` (`string`) **(required)** - Kind of the Istio object (e.g., 'DestinationRule', 'VirtualService', 'HTTPRoute', 'Gateway')
  - `name` (`string`) **(required)** - Name of the Istio object
  - `namespace` (`string`) **(required)** - Namespace containing the Istio object
  - `version` (`string`) **(required)** - API version of the Istio object (e.g., 'v1', 'v1beta1')

- **istio_object_create** - Create a new Istio object using POST method. The JSON data will be used to create the new object.
  - `group` (`string`) **(required)** - API group of the Istio object (e.g., 'networking.istio.io', 'gateway.networking.k8s.io')
  - `json_data` (`string`) **(required)** - JSON data for the new object
  - `kind` (`string`) **(required)** - Kind of the Istio object (e.g., 'DestinationRule', 'VirtualService', 'HTTPRoute', 'Gateway')
  - `namespace` (`string`) **(required)** - Namespace where the Istio object will be created
  - `version` (`string`) **(required)** - API version of the Istio object (e.g., 'v1', 'v1beta1')

- **istio_object_delete** - Delete an existing Istio object using DELETE method.
  - `group` (`string`) **(required)** - API group of the Istio object (e.g., 'networking.istio.io', 'gateway.networking.k8s.io')
  - `kind` (`string`) **(required)** - Kind of the Istio object (e.g., 'DestinationRule', 'VirtualService', 'HTTPRoute', 'Gateway')
  - `name` (`string`) **(required)** - Name of the Istio object
  - `namespace` (`string`) **(required)** - Namespace containing the Istio object
  - `version` (`string`) **(required)** - API version of the Istio object (e.g., 'v1', 'v1beta1')

- **validations_list** - List all the validations in the current cluster from all namespaces
  - `namespace` (`string`) - Optional single namespace to retrieve validations from (alternative to namespaces)
  - `namespaces` (`string`) - Optional comma-separated list of namespaces to retrieve validations from

- **namespaces** - Get all namespaces in the mesh that the user has access to

- **services_list** - Get all services in the mesh across specified namespaces with health and Istio resource information
  - `namespaces` (`string`) - Comma-separated list of namespaces to get services from (e.g. 'bookinfo' or 'bookinfo,default'). If not provided, will list services from all accessible namespaces

- **service_details** - Get detailed information for a specific service in a namespace, including validation, health status, and configuration
  - `namespace` (`string`) **(required)** - Namespace containing the service
  - `service` (`string`) **(required)** - Name of the service to get details for

- **service_metrics** - Get metrics for a specific service in a namespace. Supports filtering by time range, direction (inbound/outbound), reporter, and other query parameters
  - `byLabels` (`string`) - Comma-separated list of labels to group metrics by (e.g., 'source_workload,destination_service'). Optional
  - `direction` (`string`) - Traffic direction: 'inbound' or 'outbound'. Optional, defaults to 'outbound'
  - `duration` (`string`) - Duration of the query period in seconds (e.g., '1800' for 30 minutes). Optional, defaults to 1800 seconds
  - `namespace` (`string`) **(required)** - Namespace containing the service
  - `quantiles` (`string`) - Comma-separated list of quantiles for histogram metrics (e.g., '0.5,0.95,0.99'). Optional
  - `rateInterval` (`string`) - Rate interval for metrics (e.g., '1m', '5m'). Optional, defaults to '1m'
  - `reporter` (`string`) - Metrics reporter: 'source', 'destination', or 'both'. Optional, defaults to 'source'
  - `requestProtocol` (`string`) - Filter by request protocol (e.g., 'http', 'grpc', 'tcp'). Optional
  - `service` (`string`) **(required)** - Name of the service to get metrics for
  - `step` (`string`) - Step between data points in seconds (e.g., '15'). Optional, defaults to 15 seconds

- **workloads_list** - Get all workloads in the mesh across specified namespaces with health and Istio resource information
  - `namespaces` (`string`) - Comma-separated list of namespaces to get workloads from (e.g. 'bookinfo' or 'bookinfo,default'). If not provided, will list workloads from all accessible namespaces

- **workload_details** - Get detailed information for a specific workload in a namespace, including validation, health status, and configuration
  - `namespace` (`string`) **(required)** - Namespace containing the workload
  - `workload` (`string`) **(required)** - Name of the workload to get details for

- **workload_metrics** - Get metrics for a specific workload in a namespace. Supports filtering by time range, direction (inbound/outbound), reporter, and other query parameters
  - `byLabels` (`string`) - Comma-separated list of labels to group metrics by (e.g., 'source_workload,destination_service'). Optional
  - `direction` (`string`) - Traffic direction: 'inbound' or 'outbound'. Optional, defaults to 'outbound'
  - `duration` (`string`) - Duration of the query period in seconds (e.g., '1800' for 30 minutes). Optional, defaults to 1800 seconds
  - `namespace` (`string`) **(required)** - Namespace containing the workload
  - `quantiles` (`string`) - Comma-separated list of quantiles for histogram metrics (e.g., '0.5,0.95,0.99'). Optional
  - `rateInterval` (`string`) - Rate interval for metrics (e.g., '1m', '5m'). Optional, defaults to '1m'
  - `reporter` (`string`) - Metrics reporter: 'source', 'destination', or 'both'. Optional, defaults to 'source'
  - `requestProtocol` (`string`) - Filter by request protocol (e.g., 'http', 'grpc', 'tcp'). Optional
  - `step` (`string`) - Step between data points in seconds (e.g., '15'). Optional, defaults to 15 seconds
  - `workload` (`string`) **(required)** - Name of the workload to get metrics for

- **health** - Get health status for apps, workloads, and services across specified namespaces in the mesh. Returns health information including error rates and status for the requested resource type
  - `namespaces` (`string`) - Comma-separated list of namespaces to get health from (e.g. 'bookinfo' or 'bookinfo,default'). If not provided, returns health for all accessible namespaces
  - `queryTime` (`string`) - Unix timestamp (in seconds) for the prometheus query. If not provided, uses current time. Optional
  - `rateInterval` (`string`) - Rate interval for fetching error rate (e.g., '10m', '5m', '1h'). Default: '10m'
  - `type` (`string`) - Type of health to retrieve: 'app', 'service', or 'workload'. Default: 'app'

- **workload_logs** - Get logs for a specific workload's pods in a namespace. Only requires namespace and workload name - automatically discovers pods and containers. Optionally filter by container name, time range, and other parameters. Container is auto-detected if not specified.
  - `container` (`string`) - Optional container name to filter logs. If not provided, automatically detects and uses the main application container (excludes istio-proxy and istio-init)
  - `namespace` (`string`) **(required)** - Namespace containing the workload
  - `since` (`string`) - Time duration to fetch logs from (e.g., '5m', '1h', '30s'). If not provided, returns recent logs
  - `tail` (`integer`) - Number of lines to retrieve from the end of logs (default: 100)
  - `workload` (`string`) **(required)** - Name of the workload to get logs for

- **app_traces** - Get distributed tracing data for a specific app in a namespace. Returns trace information including spans, duration, and error details for troubleshooting and performance analysis.
  - `app` (`string`) **(required)** - Name of the app to get traces for
  - `clusterName` (`string`) - Cluster name for multi-cluster environments (optional)
  - `endMicros` (`string`) - End time for traces in microseconds since epoch (optional)
  - `limit` (`integer`) - Maximum number of traces to return (default: 100)
  - `minDuration` (`integer`) - Minimum trace duration in microseconds (optional)
  - `namespace` (`string`) **(required)** - Namespace containing the app
  - `startMicros` (`string`) - Start time for traces in microseconds since epoch (optional)
  - `tags` (`string`) - JSON string of tags to filter traces (optional)

- **service_traces** - Get distributed tracing data for a specific service in a namespace. Returns trace information including spans, duration, and error details for troubleshooting and performance analysis.
  - `clusterName` (`string`) - Cluster name for multi-cluster environments (optional)
  - `endMicros` (`string`) - End time for traces in microseconds since epoch (optional)
  - `limit` (`integer`) - Maximum number of traces to return (default: 100)
  - `minDuration` (`integer`) - Minimum trace duration in microseconds (optional)
  - `namespace` (`string`) **(required)** - Namespace containing the service
  - `service` (`string`) **(required)** - Name of the service to get traces for
  - `startMicros` (`string`) - Start time for traces in microseconds since epoch (optional)
  - `tags` (`string`) - JSON string of tags to filter traces (optional)

- **workload_traces** - Get distributed tracing data for a specific workload in a namespace. Returns trace information including spans, duration, and error details for troubleshooting and performance analysis.
  - `clusterName` (`string`) - Cluster name for multi-cluster environments (optional)
  - `endMicros` (`string`) - End time for traces in microseconds since epoch (optional)
  - `limit` (`integer`) - Maximum number of traces to return (default: 100)
  - `minDuration` (`integer`) - Minimum trace duration in microseconds (optional)
  - `namespace` (`string`) **(required)** - Namespace containing the workload
  - `startMicros` (`string`) - Start time for traces in microseconds since epoch (optional)
  - `tags` (`string`) - JSON string of tags to filter traces (optional)
  - `workload` (`string`) **(required)** - Name of the workload to get traces for

</details>

### Troubleshooting

- Missing Kiali configuration when `kiali` toolset is enabled → set `[toolset_configs.kiali].url` in the config TOML.
- Invalid URL → ensure `[toolset_configs.kiali].url` is a valid `http(s)://host` URL.
- TLS certificate validation:
  - If `[toolset_configs.kiali].url` uses HTTPS and `[toolset_configs.kiali].insecure` is false, you must set `[toolset_configs.kiali].certificate_authority` with the PEM-encoded certificate(s) used by the Kiali server. This field expects inline PEM content, not a file path. You may concatenate multiple PEM blocks to include an intermediate chain.
  - For non-production environments you can set `[toolset_configs.kiali].insecure = true` to skip certificate verification.



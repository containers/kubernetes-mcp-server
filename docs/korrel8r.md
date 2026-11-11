# Korrel8r (`observability/korrel8r`)

The opt-in Korrel8r toolset correlates Kubernetes resources with logs, metrics,
alerts, traces, and other domains supported by a separate [Korrel8r](https://korrel8r.github.io/korrel8r/)
REST service. The upstream Korrel8r MCP module owns the tool schemas and
handlers; this server prefixes their names with `korrel8r_`. The service, not
this MCP server, owns the correlation rules, stores, and traversal costs.

## Correlation

Use `korrel8r_list_domains` and `korrel8r_help` to explore available data.
Start from a query for an actual resource, explore correlations with
`korrel8r_create_neighbors_graph` or `korrel8r_create_goals_graph`, then
inspect returned queries with `korrel8r_get_objects`. Only report relationships
and signals evidenced by responses; graph contents depend on the Korrel8r
service and its stores. Calls report an error when the endpoint is missing or unreachable.

## Configuration and authentication

```toml
toolsets = ["core", "config", "observability/korrel8r"]

[toolset_configs."observability/korrel8r"]
url = "https://korrel8r.example.com"
# auth_mode = "header"  # default: forward this MCP caller's bearer token
# auth_mode = "kubeconfig" # instead forward the kubeconfig/ServiceAccount bearer token
# timeout = "2m"         # per-call backstop; change if traversals take longer
```

`url` must be an HTTP(S) base URL, optionally with a path; userinfo, query,
and fragment are unsupported. The backend endpoint is global, not selected by
Kubernetes context. Each request carries the caller's bearer token; `header`
forwards the bearer token from the MCP request, while `kubeconfig` forwards the
bearer token (`token` or `tokenFile`) of the configured kubeconfig or the
in-cluster ServiceAccount. Only bearer tokens are forwarded: kubeconfigs that
authenticate with client certificates or exec plugins (common for minikube,
kind and cloud providers) send no credentials, so the backend sees anonymous
requests. The Korrel8r service decides what that identity may access. With
`kubeconfig`, all callers share that identity.

The HTTP client defaults to a 2-minute timeout; set `timeout` above the
Korrel8r service's traversal timeout if it is configured to return partial
results. `max_backend_response_bytes` caps each REST response (default 4 MiB;
`0` disables the cap), including reads by Korrel8r's own client.
`max_result_bytes` bounds the completed MCP result. Increase these limits
only when your service returns larger results; use constrained queries to
avoid large graph or object responses. TLS certificate verification uses the
system pool plus the Kubernetes REST config CA and mounted service CA.
Private backend certificates require a trusted CA (for example via
`SSL_CERT_FILE`); insecure TLS is not supported. If `require_tls = true`,
HTTP endpoints are rejected.

For HTTPS backends, use an endpoint reachable from the MCP server with a
trusted certificate for its hostname. Forwarding TLS to `https://localhost`
does not verify a certificate issued for a Service DNS name; do not disable
TLS verification to work around this. In-cluster installations can use
service DNS directly.

## Optional OpenShift console integration

On OpenShift, `korrel8r_get_console` and `korrel8r_show_in_console` can interact
with an OpenShift console connected as the same user to **the same Korrel8r
instance**. These tools use Korrel8r's REST session endpoints; the console
integration and UI are not provided by this toolset. A shared `kubeconfig`
identity cannot keep separate users' console sessions. Without a connected
console, these two tools return an error. Correlation works independently.

## Troubleshooting

| Symptom | Check |
|---|---|
| Endpoint not configured | Set `[toolset_configs."observability/korrel8r"].url` before calling Korrel8r tools. |
| Unauthorized or no data | Check the caller's token, or that the kubeconfig has a bearer token for `auth_mode = "kubeconfig"`; then backend RBAC and service stores. |
| Unknown CA | Trust the configured backend's CA; keep hostname verification enabled. |
| Response too large | Reduce query volume or adjust `max_backend_response_bytes` and `max_result_bytes`. |
| Timeout | Check Korrel8r service health/traversal timeout and the configured per-call `timeout`. |
| OpenShift console tools return an error | If using the optional console integration, connect the console as the same user to the same Korrel8r service. |

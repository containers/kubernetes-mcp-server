## Kiali integration

This server can expose Kiali tools so assistants can query mesh information (e.g., mesh status/graph).

### Enable the Kiali toolset

Enable the Kiali tools via the server TOML configuration file.

Config (TOML):

```toml
toolsets = ["core", "kiali"]

[toolset_configs.kiali]
url = "https://kiali.example" # Endpoint/route to reach Kiali console
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

### Troubleshooting

- Missing Kiali configuration when `kiali` toolset is enabled → set `[toolset_configs.kiali].url` in the config TOML.
- Invalid URL → ensure `[toolset_configs.kiali].url` is a valid `http(s)://host` URL.
- TLS certificate validation:
  - If `[toolset_configs.kiali].url` uses HTTPS and `[toolset_configs.kiali].insecure` is false, you must set `[toolset_configs.kiali].certificate_authority` with the PEM-encoded certificate(s) used by the Kiali server. This field expects inline PEM content, not a file path. You may concatenate multiple PEM blocks to include an intermediate chain.
  - For non-production environments you can set `[toolset_configs.kiali].insecure = true` to skip certificate verification.


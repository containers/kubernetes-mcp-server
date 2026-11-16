# Alert management

Enable the opt-in `observability/alert-management` toolset to list alerts and
list, preview, create, update, or delete alert rules through the OpenShift
monitoring-plugin management API. These operations expose rule provenance,
GitOps previews, and platform-rule overrides that generic Kubernetes resource
CRUD does not provide.

```toml
toolsets = ["observability/metrics", "observability/alert-management"]
read_only = true

[toolset_configs."observability/alert-management"]
auth_mode = "header"
management_api_url = "https://monitoring-plugin.openshift-monitoring.svc:9443"
```

Use `auth_mode = "kubeconfig"` to authenticate with the Kubernetes client token.
HTTPS requires a trusted certificate and matching hostname. System CAs,
Kubernetes REST configuration CA data/files, and the mounted OpenShift service
CA are combined. Insecure TLS is rejected; bearer tokens are never sent over
plaintext HTTP. HTTPS-to-HTTP redirects are rejected.

`read_only = true` exposes only list and preview tools. To register write tools,
set `read_only = false`; `disable_destructive = true` additionally hides rule
update/delete and silence deletion. The backend still enforces caller RBAC.
The existing `observability/metrics` toolset also provides `create_silence`,
`update_silence`, and `delete_silence` alongside `get_silences`.

List first, identify the exact rule IDs, preview changes, and obtain explicit
user confirmation before writes. For GitOps-owned rules, use each preview
resource's `desiredObject` to edit the source repository. For operator-owned
rules, silence notifications rather than repeatedly attempting edits. A malformed
preview resource is reported as an error rather than discarded.

The monitoring-plugin build must expose `/api/v1/alerting/rules/preview` for the
preview-before-write workflow. Keep mutation evals opt-in. Read and preview eval
workflows live under `evals/tasks/obs`; backend E2E tests remain in `rhobs/obs-mcp`
until its E2E migration is complete.

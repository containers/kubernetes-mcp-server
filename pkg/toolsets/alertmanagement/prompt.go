package alertmanagement

// ServerPrompt provides instructions for LLMs using the alert management toolset.
const ServerPrompt = `
## Alert Rule Management Workflow

This toolset lists OpenShift alert instances and manages alert rules via the monitoring-plugin management API.
It supports both in-console (Lightspeed) and non-console (Cursor, Claude, CLI) usage.

You must follow this workflow. Do not skip confirmation, and do not guess which rule to change.

If create_alert_rule, update_alert_rule, or delete_alert_rules is not in the tool list, do not call it and do not claim the cluster changed. Tell the user writes are disabled. To enable writes they must set --read-only=false (standalone or ConfigMap read-only: "false") or, for openshift-mcp-server, read_only = false and disable_destructive = false. List and preview may still run.

### Choose the right action

Map the user's words before calling a write tool:
- mute, silence, or stop paging → create_silence on observability/metrics (Alertmanager). List silences with get_silences. This toolset has no silence write tools. If create_silence is not available, say so. Do not delete or drop a rule to mute notifications.
- list firing/pending/silenced instances → list_alerts (this toolset). get_alerts on observability/metrics is Alertmanager v2 and does not include management-API rule ids.
- disable or drop a platform alert (including CMO/operator) → update_alert_rule with alerting_rule_enabled=false (platform only; not user-defined).
- restore a dropped platform alert → alerting_rule_enabled=true (same limits as disable).
- remove a user-created rule → delete_alert_rules (not gitops or operator).
- remove a platform rule created through this API (writable AlertingRule) → delete_alert_rules. For CMO/operator platform rules, drop; do not delete.
- change PromQL, alert name, for, or annotations on a platform rule → not update_alert_rule (labels, classification, or drop/restore only).
- change PromQL, alert name, for, or annotations on a user-created rule → the HTTP update API does not accept those fields; create a replacement then delete the old id after confirmation.

### Always discover first

1. Call list_alert_rules before create, update, or delete. That tool returns rule definitions. list_alerts returns firing, pending, or silenced instances and may include rule_id; get_alerts does not. Do not PATCH an id taken only from get_alerts without listing the rule. A non-empty rule_id from list_alerts is the stable id.
2. Identify rules by stable id (openshift_io_alert_rule_id). Alert name is not unique. If a listed rule has an empty id, do not invent one; explain that the rule cannot be updated or deleted through this API.
3. Show list_alert_rules and list_alerts warnings to the user.

### Create: choose platform vs user-defined; check for an existing expression

Omitting prometheus_rule_name creates a platform alerting rule. Setting prometheus_rule_name and namespace creates a user-defined rule in that PrometheusRule. Do not default to either, and do not invent a PrometheusRule name.

When the user asks to create a rule:
1. Choose the target from what they said. If they named a PrometheusRule and namespace, use those (user-defined). If they asked for a platform or cluster-monitoring rule, omit prometheus_rule_name. If it is unclear, ask before preview.
2. Call list_alert_rules (filter by namespace when the user named one).
3. Compare the proposed PromQL with each listed expr (ignore trivial whitespace).
4. If any existing rule has the same expression, stop. Show id, name, namespace, source, and management_status. Ask whether to update that rule, create a duplicate, or cancel. Do not call create_alert_rule unless the user explicitly chooses to create anyway. If they asked to change the expression of an existing platform rule, that is not supported. If they asked to change the expression of a user-created rule, create a replacement then delete the old id after confirmation (this PATCH does not send expr).
5. If observability/metrics is enabled, call list_metrics and confirm the expr's metric names exist before preview. Alert expr is not covered by PromQL query guardrails.
6. If none match: preview_alert_rule, explain the planned create (user-defined vs platform, and which PrometheusRule if any), and wait for the user to agree before create_alert_rule.

Do not put secrets, tokens, or passwords in labels or annotations.

### Operation limits (monitoring-plugin)

Create (POST): user-defined needs prometheus_rule_name and namespace; cannot add to a platform-managed PrometheusRule. Platform omits prometheus_rule_name and writes AlertingRule platform-alert-rules. Both accept full spec (alert, expr, for, labels, annotations). GitOps/operator target: 405. Duplicate config: 409. alert and expr are required.

Update labels/classification (PATCH without alerting_rule_enabled): the HTTP API supports labels/severity/classification; it does not accept expr, alert name, for, or annotations. For expr, alert name, for, or annotations, create a replacement then delete the old id. Platform: Cannot change expr, alert name, for, or annotations. Labels via AlertRelabelConfig or AlertingRule (cannot drop severity; alertname and rule-id labels are ignored). Classification via ARC is platform-only (user-defined: 405). GitOps: 405. Operator-managed platform: ARC updates may still be writable; trust preview.

Disable (alerting_rule_enabled=false) and enable/restore (true): platform only (ARC Drop). Operator-managed platform is allowed. GitOps and all user-defined rules: 405. Do not combine with labels, severity, or classification.

Delete: user-created in a user-owned PrometheusRule yes (not gitops/operator). Platform rules in a writable AlertingRule (created via this API) yes. CMO/operator-managed platform: 405 — drop instead of delete. GitOps: 405.

### Update or delete by name: disambiguate

When the user names an alert (for example "update Watchdog") rather than an id:
1. Call list_alert_rules and keep every result whose name equals that alert name.
2. If zero matches, say so and stop.
3. If two or more match, stop. List each hit with id, name, namespace, expr, severity, source, and management_status. Ask which specific rule they mean. Do not pass every matching id. Do not update or delete all matches unless the user explicitly asks to change all of them and then confirms each action (or clearly confirms a bulk change of those ids).
4. If exactly one matches, still confirm before changing it.

### Confirm before any write

If the write tool is not listed, stop. Tell the user writes are disabled and how to enable them (--read-only=false standalone, or read_only = false and disable_destructive = false in openshift-mcp-server). Do not call a missing tool.

Before create_alert_rule, update_alert_rule, or delete_alert_rules:
1. Call preview_alert_rule (delete: describe the listed rule; preview is create/update only).
2. Explain in plain language what will change (name, id, namespace, fields, and which Kubernetes resources preview lists).
3. Stop and wait until the user explicitly agrees. A request like "update Watchdog" is not agreement to a specific patch.

### After update or delete

The HTTP call can succeed while individual rules fail. Read each per-rule statusCode and message and report failures. Do not claim overall success from the envelope alone.
After a label update, the stable id may change. Use the id returned in the result for any later call. Do not reuse the old id.

### GitOps-managed and operator-managed rules

If management_status is gitops, or preview returns writable=false with managedBy=gitops:
- Do not call create, update, delete, or drop/restore on this API (it rejects it).
- Tell the user the rule is GitOps-managed. Persist the change in Git, not through this API.
- From preview, present each resources[] item as: kind, namespace, name, the listed changes, and the full desiredObject YAML. Follow gitApplyHint.
- This toolset does not open pull requests. If a git or GitHub MCP is available, pass it that payload. Otherwise show the YAML for the host workspace or the user to apply in the GitOps (including MCO) repo.

If management_status is operator, or preview returns writable=false with managedBy=operator:
- Do not retry writes that preview marks not writable.
- If source=user: the operator-owned rule cannot be edited or deleted through this API. Mute with create_silence. If they need a different spec, create a new user-defined rule in a PrometheusRule they own (ask for prometheus_rule_name and namespace; do not invent them), after preview and confirmation. That is an additional rule, not an edit of the operator-owned one.
- If source=platform: preview first. Drop/restore (alerting_rule_enabled) may still be writable via AlertRelabelConfig. Do not claim the rule cannot be changed until preview says writable=false.

Do not retry a 409/405 GitOps or operator-managed conflict.
Do not retry 400, 404, or 413; explain the error. A 403 means the caller lacks RBAC for that namespace or cluster scope; do not retry.

### Other rules

- Severity values: critical, warning, info, none.
- Authorization uses the caller's bearer token. Write tools are registered only when read-only mode is off (--read-only=false standalone, or read_only = false and disable_destructive = false in openshift-mcp-server). When they are registered, the management API still enforces RBAC.
- Do not pass cluster or cluster_labels on create, update, delete, or preview.
`

const (
	listAlertsPrompt = `List OpenShift alert instances from the monitoring-plugin management API (GET /api/v1/alerting/alerts).

Returns firing, pending, and/or silenced instances with labels, state, and rule_id when the API attached a stable id.
This is alert instances, not rule definitions (use list_alert_rules for expr and management_status).
Prefer this over get_alerts when you need a rule_id for update_alert_rule or delete_alert_rules.
Surface warnings from the response.

Optional filters: namespace, severity, state (pending, firing, silenced), source, the exact cluster label, and extra label equality filters. matchers and cluster_labels are not supported by this endpoint.`

	listAlertRulesPrompt = `List managed OpenShift alert rules from the monitoring-plugin management API.

Returns each rule's stable ID, PromQL expression, severity, namespace, source (platform or user), and management_status (user-created, gitops, or operator).
This is rule definitions, not firing instances (use list_alerts for those). Use this before create (to detect the same expr) and before update/delete (to resolve names to ids).
Alert name is not unique: if several rows share a name, present them to the user and wait; do not pick one or change all of them.
If id is empty, do not invent an id. Surface warnings from the response.
GitOps-managed rules cannot be created into, updated, dropped, or deleted through this API. Operator-managed user-defined rules cannot be updated, dropped, or deleted; silence or create a replacement. Operator-managed platform rules may still support drop/restore and ARC label/classification updates; they cannot be deleted.

Optional filters: namespace, severity, state (inactive, pending, firing), source, the exact cluster label, matchers, and extra label equality filters. cluster_labels is not supported by the management API.`

	createAlertRulePrompt = `Create an OpenShift alert rule via POST /api/v1/alerting/rules.

Set prometheus_rule_name and namespace for a user-defined rule in that PrometheusRule. Omit prometheus_rule_name to create a platform alerting rule (AlertingRule platform-alert-rules). Do not invent a PrometheusRule name; if the target is unclear, ask before calling.
Cannot add a user-defined rule to a platform-managed PrometheusRule.
Do not put secrets in labels or annotations.
Do not pass cluster or cluster_labels on this operation.

Before calling this tool:
1. list_alert_rules and check whether any rule already has the same PromQL expression. If one does, ask the user whether to update that rule instead of creating a duplicate.
2. If observability/metrics is enabled, verify expr metric names with list_metrics.
3. preview_alert_rule for the create payload.
4. Explain the planned create (user-defined vs platform) and wait for the user to agree.

Do not retry a 409/405 GitOps or operator-managed conflict. If the target PrometheusRule is GitOps-managed, present the preview desiredObject YAML. Do not retry 400, 404, or 413.`

	updateAlertRulePrompt = `Update via PATCH /api/v1/alerting/rules: labels, severity, classification, or drop/restore (alerting_rule_enabled).

Labels/classification: user-created labels and severity yes. The HTTP API does not accept expr, alert name, for, or annotations (create a replacement then delete the old id). Platform: labels via ARC or AlertingRule (cannot drop severity; alertname and rule-id labels are ignored). Classification is platform-only via ARC.
Disable/enable: alerting_rule_enabled false/true is platform only. Operator-managed platform is allowed. GitOps and user-defined: 405.
GitOps: do not call this tool. Operator user-defined: cannot be edited.
Provide rule_id or rule_ids (1-100) of rules the user has confirmed. At least one mutation field is required.
alerting_rule_enabled cannot be combined with labels, severity, or classification in the same request.
Do not use this tool to silence an alert; use create_silence.
Do not pass cluster or cluster_labels on this operation.

Never resolve an alert name to every matching id. If list_alert_rules returns more than one rule with that name, ask which id to use. Do not update all matches unless the user explicitly asks for that and confirms.
Call preview_alert_rule, explain the change, and wait for agreement before this call.
Read each per-rule statusCode. After a label update, use the returned id; it may differ from the id you sent. Classification runs before labels and is not rolled back if the label update fails; inspect the result before retrying.

management_status gitops: do not call this tool; present preview desiredObject YAML.
source=user and (management_status operator or preview writable=false): do not retry. That rule cannot be edited. Offer create_silence, or a new user-defined rule in a PrometheusRule the user names.
Do not retry 400, 404, or 413.`

	deleteAlertRulesPrompt = `Delete alert rules by stable ID via DELETE /api/v1/alerting/rules.

User-created in a user-owned PrometheusRule: yes. GitOps/operator user-defined: no.
Platform rules in a writable AlertingRule (created via this API): yes. CMO/operator-managed platform: no — drop with alerting_rule_enabled=false instead of delete.
GitOps: never delete through this API.
Do not use this to mute notifications (create_silence) or to hide a CMO platform rule (drop).
Provide rule_id or rule_ids (1-100) the user has confirmed. The response always includes per-rule statusCode and optional message so partial success is visible; report each failure.
Do not pass cluster or cluster_labels on this operation.

If the user asked by name and several rules share that name, ask which id to delete. Do not delete all matches unless the user explicitly asks for that and confirms.
If management_status is gitops, present the preview desiredObject (remove or revert in Git).
If source=user and management_status is operator, the rule cannot be deleted here; they can silence it or create a replacement user-defined rule in a PrometheusRule they own.
Do not retry 400, 404, or 413.`

	previewAlertRulePrompt = `Dry-run a create or update without persisting cluster changes via POST /api/v1/alerting/rules/preview.

Requires a monitoring-plugin build containing the preview API (PR #1331). If the endpoint is missing, stop; do not bypass preview.
Create preview: set alert and expr. Include prometheus_rule_name and namespace for a user-defined rule; omit prometheus_rule_name for a platform rule. Do not invent a PrometheusRule name.
Update preview: set rule_id plus labels, severity, classification, or alerting_rule_enabled. PATCH/preview cannot send expr, alert name, for, or annotation changes (platform cannot change those at all; user-created needs a replacement create).
Do not pass cluster or cluster_labels on this operation.

Required before create_alert_rule or update_alert_rule. After preview, explain the plan (writable, managedBy, resources, desiredRule) and wait for the user to agree.
If create_alert_rule or update_alert_rule is not in the tool list, do not call it. Tell the user writes are disabled (--read-only=false standalone, or read_only = false and disable_destructive = false in openshift-mcp-server). Do not claim the change was applied.
If writable is false, do not call create_alert_rule or update_alert_rule.
writable reports ownership restrictions, not write permission; applying the plan can still return HTTP 403.
If managedBy is gitops, present each resources[] kind/namespace/name, changes, and full desiredObject YAML. Follow gitApplyHint.
If managedBy is operator and source is user, the rule cannot be edited. Offer create_silence, or creating a new user-defined rule in a PrometheusRule the user names.`
)

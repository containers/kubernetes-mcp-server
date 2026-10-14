{{/*
CEL fragments shared by the Loki tools. Each takes the CEL expression of its
input as the argument and expands to one CEL expression.
*/}}

{{/* loki.seconds: a Prometheus-style duration string (15m, 1h, 2d, 1w, 1y) in seconds. */}}
{{- define "loki.seconds" -}}
(int({{ . }}.substring(0, size({{ . }}) - 1)) * {'s': 1, 'm': 60, 'h': 3600, 'd': 86400, 'w': 604800, 'y': 31536000}[{{ . }}.substring(size({{ . }}) - 1)])
{{- end -}}

{{/* loki.timeValid: whether a time string is RFC3339, Unix seconds, NOW or NOW±duration. */}}
{{- define "loki.timeValid" -}}
({{ . }}.matches('^(?i)NOW([+-][0-9]+[smhdwy])?$') || {{ . }}.matches('^[0-9]+$') || {{ . }}.matches('^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'))
{{- end -}}

{{/* loki.time: a time string as Unix seconds. */}}
{{- define "loki.time" -}}
({{ . }}.upperAscii() == 'NOW' ? int(now) : {{ . }}.upperAscii().startsWith('NOW-') ? int(now) - {{ include "loki.seconds" (printf "%s.substring(4)" .) }} : {{ . }}.upperAscii().startsWith('NOW+') ? int(now) + {{ include "loki.seconds" (printf "%s.substring(4)" .) }} : {{ . }}.matches('^[0-9]+$') ? int({{ . }}) : int(timestamp({{ . }})))
{{- end -}}

{{/* loki.nanos: Unix seconds (CEL int) as the nanosecond string Loki expects. */}}
{{- define "loki.nanos" -}}
(string(int({{ . }})) + '000000000')
{{- end -}}

{{/* loki.status: the type of the first True condition of a LokiStack, or ''. */}}
{{- define "loki.status" -}}
((has({{ . }}.status) && has({{ . }}.status.conditions) ? {{ . }}.status.conditions.filter(c, c.status == 'True') : []).size() > 0 ? (has({{ . }}.status) && has({{ . }}.status.conditions) ? {{ . }}.status.conditions.filter(c, c.status == 'True') : [])[0].type : '')
{{- end -}}

{{/*
loki.gatewayURL: the base URL of a LokiStack's gateway, as the in-tree
resolver computed it. Takes a dict with "stack" (CEL expression of the
LokiStack object) and "routes" (CEL expression of the Routes of its
namespace, or an empty list when Routes are not used).
*/}}
{{- define "loki.gatewayURL" -}}
{{- $s := .stack -}}{{- $r := .routes -}}
({{ $r }}.filter(r, r.metadata.name == {{ $s }}.metadata.name && has(r.spec.host) && r.spec.host != '').size() > 0 ? 'https://' + {{ $r }}.filter(r, r.metadata.name == {{ $s }}.metadata.name && has(r.spec.host) && r.spec.host != '')[0].spec.host + '/api/logs/v1' : {{ $r }}.filter(r, r.metadata.name == {{ $s }}.metadata.name + '-gateway-http' && has(r.spec.host) && r.spec.host != '').size() > 0 ? 'https://' + {{ $r }}.filter(r, r.metadata.name == {{ $s }}.metadata.name + '-gateway-http' && has(r.spec.host) && r.spec.host != '')[0].spec.host + '/api/logs/v1' : {{ $r }}.filter(r, has(r.spec.to) && has(r.spec.to.name) && r.spec.to.name == {{ $s }}.metadata.name + '-gateway-http' && has(r.spec.host) && r.spec.host != '').size() > 0 ? 'https://' + {{ $r }}.filter(r, has(r.spec.to) && has(r.spec.to.name) && r.spec.to.name == {{ $s }}.metadata.name + '-gateway-http' && has(r.spec.host) && r.spec.host != '')[0].spec.host + '/api/logs/v1' : (has({{ $s }}.spec) && has({{ $s }}.spec.tenants) && has({{ $s }}.spec.tenants.mode) && {{ $s }}.spec.tenants.mode.startsWith('openshift-')) ? 'https://' + {{ $s }}.metadata.name + '-gateway-http.' + {{ $s }}.metadata.namespace + '.svc:8080/api/logs/v1' : 'http://' + {{ $s }}.metadata.name + '-gateway-http.' + {{ $s }}.metadata.namespace + '.svc:8080')
{{- end -}}

{{/*
loki.resolve: the steps that turn lokiNamespace/lokiName (or the fixed url)
into steps.base, the Loki base URL. Emitted as list items at column 0; the
caller indents them with nindent.
*/}}
{{- define "loki.resolve" -}}
{{- if .Values.url }}
- name: base
  expression: {{ printf "'%s'" .Values.url | quote }}
{{- else }}
- name: arguments
  fail: >-
    !(has(args.lokiNamespace) && args.lokiNamespace != '' && has(args.lokiName) && args.lokiName != '')
    ? ((has(args.lokiNamespace) && args.lokiNamespace != '') || (has(args.lokiName) && args.lokiName != '')
       ? 'both lokiNamespace and lokiName must be provided together'
       : 'loki URL not configured; install the chart with --set url=... or provide lokiNamespace and lokiName')
    : ''
- name: stack
  kubernetes:
    verb: get
    apiVersion: loki.grafana.com/v1
    kind: LokiStack
    namespace: {argument: lokiNamespace}
    name: {argument: lokiName}
{{- if .Values.useRoute }}
- name: routes
  kubernetes:
    verb: list
    apiVersion: route.openshift.io/v1
    kind: Route
    namespace: {argument: lokiNamespace}
- name: base
  expression: {{ include "loki.gatewayURL" (dict "stack" "steps.stack" "routes" "steps.routes.items") | quote }}
{{- else }}
- name: base
  expression: {{ include "loki.gatewayURL" (dict "stack" "steps.stack" "routes" "[]") | quote }}
{{- end }}
{{- end }}
{{- end -}}

{{/* loki.tenantPath: the tenant path segment of a gateway URL (gateway modes serve /api/logs/v1/<tenant>/loki/...). */}}
{{- define "loki.tenantPath" -}}
((has(args.tenant) && args.tenant != '' && steps.base.endsWith('/api/logs/v1')) ? '/' + args.tenant : '')
{{- end -}}

{{/* loki.tenantCheck: a fail expression rejecting a tenant that could not be a path segment. */}}
{{- define "loki.tenantCheck" -}}
(has(args.tenant) && args.tenant != '' && !(args.tenant.matches('^[A-Za-z0-9_.-]+$') && args.tenant != '.' && args.tenant != '..')) ? 'tenant must be a Loki tenant ID (letters, digits, -, _ and .)' : ''
{{- end -}}

{{/* loki.http: the http action fields shared by every Loki request. */}}
{{- define "loki.http" -}}
urlExpression: steps.base
certificateAuthority: {{ .Values.certificateAuthority | quote }}
insecureSkipTLSVerify: {{ .Values.insecureSkipTLSVerify }}
headers:
  X-Scope-OrgID: {argument: tenant}
{{- end -}}

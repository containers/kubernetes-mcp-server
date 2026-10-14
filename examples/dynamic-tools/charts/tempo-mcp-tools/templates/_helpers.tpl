{{/*
CEL fragments shared by the Tempo tools. Each takes the CEL expression of its
input as the argument and expands to one CEL expression.
*/}}

{{/* tempo.seconds: a Prometheus-style duration string (15m, 1h, 2d, 1w, 1y) in seconds. */}}
{{- define "tempo.seconds" -}}
(int({{ . }}.substring(0, size({{ . }}) - 1)) * {'s': 1, 'm': 60, 'h': 3600, 'd': 86400, 'w': 604800, 'y': 31536000}[{{ . }}.substring(size({{ . }}) - 1)])
{{- end -}}

{{/* tempo.timeValid: whether a time string is RFC3339, Unix seconds, NOW or NOW±duration. */}}
{{- define "tempo.timeValid" -}}
({{ . }}.matches('^(?i)NOW([+-][0-9]+[smhdwy])?$') || {{ . }}.matches('^[0-9]+$') || {{ . }}.matches('^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'))
{{- end -}}

{{/* tempo.time: a time string as Unix seconds. */}}
{{- define "tempo.time" -}}
({{ . }}.upperAscii() == 'NOW' ? int(now) : {{ . }}.upperAscii().startsWith('NOW-') ? int(now) - {{ include "tempo.seconds" (printf "%s.substring(4)" .) }} : {{ . }}.upperAscii().startsWith('NOW+') ? int(now) + {{ include "tempo.seconds" (printf "%s.substring(4)" .) }} : {{ . }}.matches('^[0-9]+$') ? int({{ . }}) : int(timestamp({{ . }})))
{{- end -}}

{{/* tempo.optionalTime: an optional time argument as Unix seconds, or null when absent. */}}
{{- define "tempo.optionalTime" -}}
((has(args.{{ . }}) && args.{{ . }} != '') ? dyn({{ include "tempo.time" (printf "args.%s" .) }}) : dyn(null))
{{- end -}}

{{/* tempo.timeCheck: the fail expression validating the optional start and end arguments. */}}
{{- define "tempo.timeCheck" -}}
(has(args.start) && args.start != '' && !{{ include "tempo.timeValid" "args.start" }}) ? 'invalid start time: timestamp must be RFC3339 format, Unix timestamp, NOW, or relative time (NOW±duration)' : (has(args.end) && args.end != '' && !{{ include "tempo.timeValid" "args.end" }}) ? 'invalid end time: timestamp must be RFC3339 format, Unix timestamp, NOW, or relative time (NOW±duration)' : (has(args.start) && args.start != '' && has(args.end) && args.end != '' && {{ include "tempo.time" "args.start" }} > {{ include "tempo.time" "args.end" }}) ? 'invalid time range: start is after end' : ''
{{- end -}}

{{/* tempo.status: the type of the first True condition of an instance, or ''. */}}
{{- define "tempo.status" -}}
((has({{ . }}.status) && has({{ . }}.status.conditions) ? {{ . }}.status.conditions.filter(c, c.status == 'True') : []).size() > 0 ? (has({{ . }}.status) && has({{ . }}.status.conditions) ? {{ . }}.status.conditions.filter(c, c.status == 'True') : [])[0].type : '')
{{- end -}}

{{/* tempo.stackInstance: the instance summary of a TempoStack object. */}}
{{- define "tempo.stackInstance" -}}
{'kind': 'TempoStack', 'tempoNamespace': {{ . }}.metadata.namespace, 'tempoName': {{ . }}.metadata.name, 'multitenancy': (has({{ . }}.spec) && has({{ . }}.spec.tenants) && has({{ . }}.spec.tenants.authentication) && size({{ . }}.spec.tenants.authentication) > 0), 'tenants': ((has({{ . }}.spec) && has({{ . }}.spec.tenants) && has({{ . }}.spec.tenants.authentication)) ? {{ . }}.spec.tenants.authentication.map(a, a.tenantName) : []), 'status': {{ include "tempo.status" . }}, 'service': ((has({{ . }}.spec) && has({{ . }}.spec.tenants) && has({{ . }}.spec.tenants.authentication) && size({{ . }}.spec.tenants.authentication) > 0) ? 'tempo-' + {{ . }}.metadata.name + '-gateway' : 'tempo-' + {{ . }}.metadata.name + '-query-frontend')}
{{- end -}}

{{/* tempo.monolithicInstance: the instance summary of a TempoMonolithic object. */}}
{{- define "tempo.monolithicInstance" -}}
{'kind': 'TempoMonolithic', 'tempoNamespace': {{ . }}.metadata.namespace, 'tempoName': {{ . }}.metadata.name, 'multitenancy': (has({{ . }}.spec) && has({{ . }}.spec.multitenancy) && has({{ . }}.spec.multitenancy.enabled) && {{ . }}.spec.multitenancy.enabled && has({{ . }}.spec.multitenancy.authentication) && size({{ . }}.spec.multitenancy.authentication) > 0), 'tenants': ((has({{ . }}.spec) && has({{ . }}.spec.multitenancy) && has({{ . }}.spec.multitenancy.authentication)) ? {{ . }}.spec.multitenancy.authentication.map(a, a.tenantName) : []), 'status': {{ include "tempo.status" . }}, 'service': ((has({{ . }}.spec) && has({{ . }}.spec.multitenancy) && has({{ . }}.spec.multitenancy.enabled) && {{ . }}.spec.multitenancy.enabled && has({{ . }}.spec.multitenancy.authentication) && size({{ . }}.spec.multitenancy.authentication) > 0) ? 'tempo-' + {{ . }}.metadata.name + '-gateway' : 'tempo-' + {{ . }}.metadata.name)}
{{- end -}}

{{/* tempo.serviceURL: the in-cluster base URL of steps.instance (gateway over https, query frontend over http). */}}
{{- define "tempo.serviceURL" -}}
(steps.instance.multitenancy ? 'https://' + steps.instance.service + '.' + steps.instance.tempoNamespace + '.svc:8080' : 'http://' + steps.instance.service + '.' + steps.instance.tempoNamespace + '.svc:3200')
{{- end -}}

{{/*
tempo.resolve: the steps that turn tempoNamespace/tempoName/tenant (or the
fixed url) into steps.instance and steps.base. Emitted as list items at
column 0; the caller indents them with nindent.
*/}}
{{- define "tempo.resolve" -}}
- name: arguments
  fail: >-
    !(has(args.tempoNamespace) && args.tempoNamespace != '') ? 'tempoNamespace parameter must not be empty'
    : !(has(args.tempoName) && args.tempoName != '') ? 'tempoName parameter must not be empty' : ''
{{- if .Values.url }}
- name: instance
  expression: "{'multitenancy': false, 'tenants': []}"
- name: base
  expression: {{ printf "'%s'" .Values.url | quote }}
{{- else }}
- name: stack
  ignoreError: true
  kubernetes:
    verb: get
    apiVersion: tempo.grafana.com/v1alpha1
    kind: TempoStack
    namespace: {argument: tempoNamespace}
    name: {argument: tempoName}
- name: monolithic
  ignoreError: true
  when: "!has(steps.stack.metadata)"
  kubernetes:
    verb: get
    apiVersion: tempo.grafana.com/v1alpha1
    kind: TempoMonolithic
    namespace: {argument: tempoNamespace}
    name: {argument: tempoName}
- name: found
  fail: >-
    (!has(steps.stack.metadata) && (steps.monolithic == null || !has(steps.monolithic.metadata)))
    ? "instance '" + args.tempoName + "' in namespace '" + args.tempoNamespace + "' not found" : ''
- name: instance
  expression: >-
    has(steps.stack.metadata) ? {{ include "tempo.stackInstance" "steps.stack" }} : {{ include "tempo.monolithicInstance" "steps.monolithic" }}
- name: tenant
  fail: >-
    (steps.instance.multitenancy && !(has(args.tenant) && args.tenant != '')) ? 'tenant parameter must not be empty for multi-tenant instance'
    : (steps.instance.multitenancy && !(args.tenant in steps.instance.tenants))
      ? "tenant '" + args.tenant + "' does not exist for instance '" + args.tempoName + "' in namespace '" + args.tempoNamespace + "'" : ''
{{- if .Values.useRoute }}
- name: route
  ignoreError: true
  kubernetes:
    verb: get
    apiVersion: route.openshift.io/v1
    kind: Route
    namespace: {expression: steps.instance.tempoNamespace}
    name: {expression: steps.instance.service}
- name: base
  expression: >-
    (has(steps.route.spec) && has(steps.route.spec.host) && steps.route.spec.host != '') ? 'https://' + steps.route.spec.host : {{ include "tempo.serviceURL" . }}
{{- else }}
- name: base
  expression: {{ include "tempo.serviceURL" . | quote }}
{{- end }}
{{- end }}
{{- end -}}

{{/* tempo.prefix: the tenant path prefix of a multi-tenant gateway (the tenant was checked against the instance's tenants). */}}
{{- define "tempo.prefix" -}}
(steps.instance.multitenancy ? '/api/traces/v1/' + args.tenant + '/tempo' : '')
{{- end -}}

{{/* tempo.http: the http action fields shared by every Tempo request. */}}
{{- define "tempo.http" -}}
urlExpression: steps.base
certificateAuthority: {{ .Values.certificateAuthority | quote }}
insecureSkipTLSVerify: {{ .Values.insecureSkipTLSVerify }}
method: GET
headers:
  Accept: application/vnd.grafana.llm
{{- end -}}

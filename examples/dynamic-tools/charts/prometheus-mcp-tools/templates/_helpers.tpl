{{/*
CEL fragments shared by the Prometheus tools. Each takes the CEL expression
of its input as the argument and expands to one CEL expression.
*/}}

{{/* prom.seconds: a Prometheus-style duration string (15m, 1h, 2d, 1w, 1y) in seconds. */}}
{{- define "prom.seconds" -}}
(int({{ . }}.substring(0, size({{ . }}) - 1)) * {'s': 1, 'm': 60, 'h': 3600, 'd': 86400, 'w': 604800, 'y': 31536000}[{{ . }}.substring(size({{ . }}) - 1)])
{{- end -}}

{{/* prom.timeValid: whether a time string is RFC3339, Unix seconds, NOW or NOW±duration. */}}
{{- define "prom.timeValid" -}}
({{ . }}.matches('^(?i)NOW([+-][0-9]+[smhdwy])?$') || {{ . }}.matches('^[0-9]+$') || {{ . }}.matches('^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'))
{{- end -}}

{{/* prom.time: a time string as Unix seconds. */}}
{{- define "prom.time" -}}
({{ . }}.upperAscii() == 'NOW' ? int(now) : {{ . }}.upperAscii().startsWith('NOW-') ? int(now) - {{ include "prom.seconds" (printf "%s.substring(4)" .) }} : {{ . }}.upperAscii().startsWith('NOW+') ? int(now) + {{ include "prom.seconds" (printf "%s.substring(4)" .) }} : {{ . }}.matches('^[0-9]+$') ? int({{ . }}) : int(timestamp({{ . }})))
{{- end -}}

{{/* prom.timeFormat: the error suffix for an invalid time argument. */}}
{{- define "prom.timeFormat" -}}
timestamp must be RFC3339 format, Unix timestamp, NOW, or relative time (NOW±duration)
{{- end -}}

{{/* prom.rangeCheck: fail expression for the optional start/end arguments of discovery tools. */}}
{{- define "prom.rangeCheck" -}}
(has(args.start) && args.start != '' && !{{ include "prom.timeValid" "args.start" }}) ? 'invalid start time format: {{ include "prom.timeFormat" . }}' : (has(args.end) && args.end != '' && !{{ include "prom.timeValid" "args.end" }}) ? 'invalid end time format: {{ include "prom.timeFormat" . }}' : ''
{{- end -}}

{{/* prom.range: the {start, end} window of a discovery tool: the given times, or the last hour. */}}
{{- define "prom.range" -}}
((has(args.start) && args.start != '') || (has(args.end) && args.end != '')) ? {'start': (has(args.start) && args.start != '') ? dyn({{ include "prom.time" "args.start" }}) : dyn(null), 'end': (has(args.end) && args.end != '') ? dyn({{ include "prom.time" "args.end" }}) : dyn(null)} : {'start': dyn(int(now) - 3600), 'end': dyn(int(now))}
{{- end -}}

{{/* prom.http: the http action fields shared by every Prometheus request. */}}
{{- define "prom.http" -}}
url: {{ .Values.url | quote }}
certificateAuthority: {{ .Values.certificateAuthority | quote }}
insecureSkipTLSVerify: {{ .Values.insecureSkipTLSVerify }}
method: GET
{{- end -}}

{{/* prom.alertmanagerHttp: the http action fields shared by every Alertmanager request. */}}
{{- define "prom.alertmanagerHttp" -}}
url: {{ .Values.alertmanagerUrl | quote }}
certificateAuthority: {{ .Values.certificateAuthority | quote }}
insecureSkipTLSVerify: {{ .Values.insecureSkipTLSVerify }}
method: GET
{{- end -}}

{{/* prom.rangeQueryArgs: the input schema properties shared by execute_range_query and show_timeseries. */}}
{{- define "prom.rangeQueryArgs" -}}
query:
  type: string
  description: PromQL query string using metric names verified via list_metrics
step:
  type: string
  description: "Query resolution step width (e.g., '15s', '1m', '1h'). Choose based on time range: shorter ranges use smaller steps."
  pattern: ^\d+[smhdwy]$
start:
  type: string
  description: Start time as RFC3339 or Unix timestamp (optional)
end:
  type: string
  description: End time as RFC3339 or Unix timestamp (optional). Use `NOW` for current time.
duration:
  type: string
  description: Duration to look back from now (e.g., '1h', '30m', '1d', '2w') (optional)
  pattern: ^\d+[smhdwy]$
{{- end -}}

{{/*
prom.rangeQuerySteps: the steps shared by execute_range_query and
show_timeseries: argument checks, the time window, the query_range request
(steps.result) and the per-series statistics (steps.summary). Emitted as list
items at column 0; the caller indents them with nindent.
*/}}
{{- define "prom.rangeQuerySteps" -}}
- name: check
  fail: >-
    !(has(args.query) && args.query != '') ? 'query parameter is required and must be a string'
    : !(has(args.step) && args.step != '') ? 'step parameter is required and must be a string'
    : !args.step.matches('^[0-9]+(ms|[smhdwy])$') ? 'invalid step format: expected <number><unit> with unit ms, s, m, h, d, w or y'
    : ((has(args.start) && args.start != '') != (has(args.end) && args.end != '')) ? 'both start and end must be provided together'
    : (has(args.start) && args.start != '' && !{{ include "prom.timeValid" "args.start" }}) ? 'invalid start time format: {{ include "prom.timeFormat" . }}'
    : (has(args.end) && args.end != '' && !{{ include "prom.timeValid" "args.end" }}) ? 'invalid end time format: {{ include "prom.timeFormat" . }}'
    : (has(args.duration) && args.duration != '' && !args.duration.matches('^[0-9]+[smhdwy]$')) ? 'invalid duration format: expected <number><unit> with unit s, m, h, d, w or y'
    : ''
- name: range
  expression: >-
    (has(args.start) && args.start != '')
    ? {'start': {{ include "prom.time" "args.start" }}, 'end': {{ include "prom.time" "args.end" }}}
    : {'start': int(now) - ((has(args.duration) && args.duration != '') ? {{ include "prom.seconds" "args.duration" }} : 3600), 'end': int(now)}
- name: result
  http:
    {{- include "prom.http" . | nindent 4 }}
    path: /api/v1/query_range
    query:
      query: {argument: query}
      step: {argument: step}
      start: {expression: steps.range.start}
      end: {expression: steps.range.end}
- name: series
  expression: >-
    (has(steps.result.data) && has(steps.result.data.resultType) && steps.result.data.resultType == 'matrix')
    ? steps.result.data.result.map(s, {'metric': s.metric, 'values': s.values, 'finite': s.values.map(v, double(v[1])).filter(x, math.isFinite(x))})
    : []
- name: summary
  expression: >-
    steps.series.map(s, {
      'series': s.metric,
      'count': size(s.values),
      'firstTimestamp': size(s.values) > 0 ? double(s.values[0][0]) : 0.0,
      'lastTimestamp': size(s.values) > 0 ? double(s.values[size(s.values) - 1][0]) : 0.0,
      'firstValue': size(s.values) > 0 ? double(s.values[0][1]) : 0.0,
      'lastValue': size(s.values) > 0 ? double(s.values[size(s.values) - 1][1]) : 0.0,
      'delta': size(s.values) > 0 ? double(s.values[size(s.values) - 1][1]) - double(s.values[0][1]) : 0.0,
      'min': size(s.finite) > 0 ? math.least(s.finite) : 0.0,
      'max': size(s.finite) > 0 ? math.greatest(s.finite) : 0.0,
      'avg': size(s.finite) > 0 ? sum(s.finite) / double(size(s.finite)) : 0.0,
      'hasNaN': s.values.exists(v, math.isNaN(double(v[1]))),
      'hasInf': s.values.exists(v, math.isInf(double(v[1]))),
      'nonFiniteCount': size(s.values) - size(s.finite)
    })
{{- end -}}

{{/* prom.rangeQueryOutput: the execute_range_query result: every sample, or per-series statistics. */}}
{{- define "prom.rangeQueryOutput" -}}
{{- if .Values.rangeQueryFullResponse -}}
{'resultType': steps.result.data.resultType, 'result': steps.series.map(s, {'metric': s.metric, 'values': s.values}), 'warnings': has(steps.result.warnings) ? steps.result.warnings : []}
{{- else -}}
{'resultType': steps.result.data.resultType, 'summary': steps.summary, 'warnings': has(steps.result.warnings) ? steps.result.warnings : []}
{{- end -}}
{{- end -}}

{{/*
CEL fragments shared by the Prometheus tools. Each takes the CEL expression
of its input as the argument and expands to one CEL expression.
*/}}

{{/* prom.range: the {start, end} window of a discovery tool: the given times, or the last hour. */}}
{{- define "prom.range" -}}
((has(args.start) && args.start != '') || (has(args.end) && args.end != '')) ? {'start': (has(args.start) && args.start != '') ? dyn(time(args.start)) : dyn(null), 'end': (has(args.end) && args.end != '') ? dyn(time(args.end)) : dyn(null)} : {'start': dyn(int(now) - 3600), 'end': dyn(int(now))}
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

{{/* prom.rangeQueryArgs: the input schema properties of prom_chart (prom_query declares its own). */}}
{{- define "prom.rangeQueryArgs" -}}
query:
  type: string
  description: PromQL query string using metric names verified via prom_metrics
step:
  type: string
  description: "Query resolution step width (e.g., '15s', '1m', '1h'). Choose based on time range: shorter ranges use smaller steps."
  pattern: ^\d+[smhdwy]$
start:
  type: string
end:
  type: string
duration:
  type: string
  pattern: ^\d+[smhdwy]$
{{- end -}}

{{/*
prom.rangeQuerySteps: the steps of prom_chart (prom_query guards its own with when): argument checks, the time window, the query_range request
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
    : (has(args.duration) && args.duration != '' && !args.duration.matches('^[0-9]+[smhdwy]$')) ? 'invalid duration format: expected <number><unit> with unit s, m, h, d, w or y'
    : ''
- name: range
  expression: >-
    (has(args.start) && args.start != '')
    ? {'start': time(args.start), 'end': time(args.end)}
    : {'start': int(now) - ((has(args.duration) && args.duration != '') ? seconds(args.duration) : 3600), 'end': int(now)}
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
    {{ include "prom.seriesSummary" "steps.series" }}
{{- end -}}

{{/* prom.seriesSummary: per-series statistics of the series list given as the argument. */}}
{{- define "prom.seriesSummary" -}}
{{ . }}.map(s, { 'series': s.metric, 'count': size(s.values), 'firstTimestamp': size(s.values) > 0 ? double(s.values[0][0]) : 0.0, 'lastTimestamp': size(s.values) > 0 ? double(s.values[size(s.values) - 1][0]) : 0.0, 'firstValue': size(s.values) > 0 ? double(s.values[0][1]) : 0.0, 'lastValue': size(s.values) > 0 ? double(s.values[size(s.values) - 1][1]) : 0.0, 'delta': size(s.values) > 0 ? double(s.values[size(s.values) - 1][1]) - double(s.values[0][1]) : 0.0, 'min': size(s.finite) > 0 ? math.least(s.finite) : 0.0, 'max': size(s.finite) > 0 ? math.greatest(s.finite) : 0.0, 'avg': size(s.finite) > 0 ? sum(s.finite) / double(size(s.finite)) : 0.0, 'hasNaN': s.values.exists(v, math.isNaN(double(v[1]))), 'hasInf': s.values.exists(v, math.isInf(double(v[1]))), 'nonFiniteCount': size(s.values) - size(s.finite) })
{{- end -}}

{{/* prom.rangeQueryOutput: the range result of prom_query: every sample, or per-series statistics. */}}
{{- define "prom.rangeQueryOutput" -}}
{{- if .Values.rangeQueryFullResponse -}}
{'resultType': steps.result.data.resultType, 'result': steps.series.map(s, {'metric': s.metric, 'values': s.values}), 'warnings': has(steps.result.warnings) ? steps.result.warnings : []}
{{- else -}}
{'resultType': steps.result.data.resultType, 'summary': steps.summary, 'warnings': has(steps.result.warnings) ? steps.result.warnings : []}
{{- end -}}
{{- end -}}

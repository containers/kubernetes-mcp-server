{{/* netobserv.http: the http action fields shared by every NetObserv console plugin request. */}}
{{- define "netobserv.http" -}}
url: {{ .Values.url | quote }}
certificateAuthority: {{ .Values.certificateAuthority | quote }}
insecureSkipTLSVerify: {{ .Values.insecureSkipTLSVerify }}
method: GET
response: text
{{- end -}}

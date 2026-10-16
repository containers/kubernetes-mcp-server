{{/* kiali.http: the http action fields shared by every Kiali request: a POST to the Kiali server with the MCP marker header. */}}
{{- define "kiali.http" -}}
url: {{ .Values.url | quote }}
certificateAuthority: {{ .Values.certificateAuthority | quote }}
insecureSkipTLSVerify: {{ .Values.insecureSkipTLSVerify }}
method: POST
headers:
  X-Kubernetes-MCP-Server: "true"
response: text
{{- end -}}

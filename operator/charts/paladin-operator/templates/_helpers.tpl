 {{- define "checksums" -}}
operatorConfig: {{ include (printf "%s/operator/configmap.yaml" $.Template.BasePath) . | replace .Chart.AppVersion "" | sha256sum }}
{{- end -}}

{{- define "paladin-operator.paladinNodeCount" -}}
{{- $paladinCount := .Values.nodeCount | int }}
{{- $paladinCount }}
{{- end -}}

{{- define "paladin-operator.besuNodeCount" -}}
{{- $besuCount := .Values.besuNodeCount | int }}
{{- if eq $besuCount 0 }}
{{-   $besuCount = 1 }}
{{- end }}
{{- $besuCount }}
{{- end -}}


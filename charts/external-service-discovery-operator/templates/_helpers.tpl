{{- define "external-service-discovery-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- define "external-service-discovery-operator.fullname" -}}
{{- if .Values.fullnameOverride }}{{ .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}{{- else }}{{- $name := default .Chart.Name .Values.nameOverride }}{{- if contains $name .Release.Name }}{{ .Release.Name | trunc 63 | trimSuffix "-" }}{{- else }}{{ printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}{{- end }}{{- end }}
{{- end }}
{{- define "external-service-discovery-operator.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "external-service-discovery-operator.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
{{- define "external-service-discovery-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "external-service-discovery-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: controller-manager
{{- end }}
{{- define "external-service-discovery-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}{{ default (include "external-service-discovery-operator.fullname" .) .Values.serviceAccount.name }}{{- else }}{{ required "serviceAccount.name is required when serviceAccount.create is false" .Values.serviceAccount.name }}{{- end }}
{{- end }}

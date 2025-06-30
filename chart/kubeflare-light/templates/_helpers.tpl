{{/*
Expand the name of the chart.
*/}}
{{- define "kubeflare-light.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kubeflare-light.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kubeflare-light.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kubeflare-light.labels" -}}
helm.sh/chart: {{ include "kubeflare-light.chart" . }}
{{ include "kubeflare-light.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kubeflare-light.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubeflare-light.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kubeflare-light.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kubeflare-light.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the secret to use for Cloudflare API token
*/}}
{{- define "kubeflare-light.secretName" -}}
{{- if .Values.cloudflare.existingSecret }}
{{- .Values.cloudflare.existingSecret }}
{{- else }}
{{- include "kubeflare-light.fullname" . }}-cloudflare
{{- end }}
{{- end }}

{{/*
Create the name of the zones configmap
*/}}
{{- define "kubeflare-light.zonesConfigMapName" -}}
{{- default (printf "%s-zones" (include "kubeflare-light.fullname" .)) .Values.zones.configMapName }}
{{- end }}
{{/*
Expand the name of the chart.
*/}}
{{- define "blockchain-explorer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "blockchain-explorer.fullname" -}}
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
Chart label helpers
*/}}
{{- define "blockchain-explorer.labels" -}}
helm.sh/chart: {{ include "blockchain-explorer.chart" . }}
{{ include "blockchain-explorer.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "blockchain-explorer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "blockchain-explorer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "blockchain-explorer.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Redis host/port for the app container.
*/}}
{{- define "blockchain-explorer.redisHost" -}}
{{- if .Values.redis.deployInCluster.enabled -}}
{{- printf "%s-redis" (include "blockchain-explorer.fullname" .) }}
{{- else -}}
{{- .Values.redis.external.host }}
{{- end }}
{{- end }}

{{- define "blockchain-explorer.redisPort" -}}
{{- if .Values.redis.deployInCluster.enabled -}}
6379
{{- else -}}
{{ .Values.redis.external.port }}
{{- end }}
{{- end }}

{{/*
Image reference (digest preferred when set).
*/}}
{{- define "blockchain-explorer.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
{{- end }}

{{- define "blockchain-explorer.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "blockchain-explorer.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Expand the name of the chart.
*/}}
{{- define "things.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "things.fullname" -}}
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
{{- define "things.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "things.labels" -}}
helm.sh/chart: {{ include "things.chart" . }}
{{ include "things.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "things.selectorLabels" -}}
app.kubernetes.io/name: {{ include "things.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
DB Init Container Images
*/}}
{{- define "things.dbInitJob.container" }}
{{- $registry := default .Values.postgresql.image.registry .Values.dbInitJob.image.registry -}}
{{- $repository := default .Values.postgresql.image.repository .Values.dbInitJob.image.repository -}}
{{- $tag := default .Values.postgresql.image.tag .Values.dbInitJob.image.tag -}}
{{ printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}
{{- define "things.dbInitJob.initContainer" }}
{{- $registry := default .Values.postgresql.image.registry .Values.dbInitJob.initContainer.image.registry -}}
{{- $repository := default .Values.postgresql.image.repository .Values.dbInitJob.initContainer.image.repository -}}
{{- $tag := default .Values.postgresql.image.tag .Values.dbInitJob.initContainer.image.tag -}}
{{ printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}
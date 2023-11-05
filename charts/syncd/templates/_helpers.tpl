{{/*
Expand the name of the chart.
*/}}
{{- define "syncd.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "syncd.fullname" -}}
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

{{- define "syncd.serverFullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 56 | trimSuffix "-" | printf "%s-server" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s-%s" .Release.Name $name | trunc 56 | trimSuffix "-" | printf "%s-server" }}
{{- end }}
{{- end }}

{{- define "syncd.clientFullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 56 | trimSuffix "-" | printf "%s-client" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s-%s" .Release.Name $name | trunc 56 | trimSuffix "-" | printf "%s-client" }}
{{- end }}
{{- end }}

{{- define "syncd.pushFullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 57 | trimSuffix "-" | printf "%s-push" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s-%s" .Release.Name $name | trunc 57 | trimSuffix "-" | printf "%s-push" }}
{{- end }}
{{- end }}

{{- define "syncd.pullFullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 57 | trimSuffix "-" | printf "%s-pull" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s-%s" .Release.Name $name | trunc 57 | trimSuffix "-" | printf "%s-pull" }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "syncd.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "syncd.labels" -}}
helm.sh/chart: {{ include "syncd.chart" . }}
{{ include "syncd.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Server labels
*/}}
{{- define "syncd.serverLabels" -}}
{{- include "syncd.labels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Client labels
*/}}
{{- define "syncd.clientLabels" -}}
{{- include "syncd.labels" . }}
app.kubernetes.io/component: client
{{- end }}

{{/*
Push labels
*/}}
{{- define "syncd.pushLabels" -}}
{{- include "syncd.labels" . }}
app.kubernetes.io/component: push-client
{{- end }}

{{/*
Pull labels
*/}}
{{- define "syncd.pullLabels" -}}
{{- include "syncd.labels" . }}
app.kubernetes.io/component: pull-client
{{- end }}

{{/*
Selector labels
*/}}
{{- define "syncd.selectorLabels" -}}
app.kubernetes.io/name: {{ include "syncd.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Server selector labels
*/}}
{{- define "syncd.serverSelectorLabels" -}}
{{- include "syncd.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Push selector labels
*/}}
{{- define "syncd.pushSelectorLabels" -}}
{{- include "syncd.selectorLabels" . }}
app.kubernetes.io/component: push-client
{{- end }}

{{/*
Pull selector labels
*/}}
{{- define "syncd.pullSelectorLabels" -}}
{{- include "syncd.selectorLabels" . }}
app.kubernetes.io/component: pull-client
{{- end }}

{{- define "syncd.needsServerSecret" -}}
{{ false }}
{{- end }}

{{- define "syncd.preSharedKey" -}}
preSharedKey:
{{- if or .fromExistingSecret (not (empty .value)) }}
  fromEnv: SYNCD_AUTH_PRESHARED_KEY
{{- else -}}
 {}
{{- end }}
{{- end }}

{{- define "syncd.tlsConfig" -}}
tls:
  {{- if not .enabled }} {}
  {{- else }}
  {{- if or .fromExistingSecret (not (empty .values.certificate )) }}
  crt: /config/tls/tls.crt
  key: /config/tls/tls.key
  {{- end }}
  {{- if .trustedCAs.fromExistingConfigMap }}
  {{- with .trustedCAs.existingConfigMap.keys }}
  ca:
  {{- range . }}
    - /config/tls/roots/{{ . }}
  {{- end }}
  {{- end }}
  {{- else if not (empty .trustedCAs.values) }}
  ca:
    {{- range $k, $v := .trustedCAs.values }}
    - /config/tls/roots/{{ $k }}
    {{- end }}
  {{- end }}
  {{- end }}
{{- end }}

{{- define "syncd.graphConfig" }}
graph:
  source:
    postgres:
      syncTable:
        name: {{ printf "%s.%s" .Values.graph.source.postgres.schema .Values.graph.source.postgres.syncTable }}
      sequenceTable:
        name: {{ printf "%s.%s" .Values.graph.source.postgres.schema .Values.graph.source.postgres.sequenceTable }}
      connection:
        fromEnv: SYNCD_POSTGRESQL_CONN
  models:
    {{- with .Values.graph.models }}
    {{- toYaml . | nindent 4 }}
    {{- else -}}
    []
    {{- end }}
{{- end }}
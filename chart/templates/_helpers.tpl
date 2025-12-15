{{/*
Expand the name of the chart.
*/}}
{{- define "policy-bot.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "policy-bot.fullname" -}}
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
{{- define "policy-bot.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "policy-bot.labels" -}}
helm.sh/chart: {{ include "policy-bot.chart" . }}
{{ include "policy-bot.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "policy-bot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "policy-bot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "policy-bot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "policy-bot.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the secret to use
*/}}
{{- define "policy-bot.secretName" -}}
{{- if .Values.secrets.create }}
{{- include "policy-bot.fullname" . }}
{{- else }}
{{- required "A valid .Values.secrets.existingSecret is required when secrets.create is false" .Values.secrets.existingSecret }}
{{- end }}
{{- end }}

{{/*
Create the name of the configmap
*/}}
{{- define "policy-bot.configMapName" -}}
{{- include "policy-bot.fullname" . }}-config
{{- end }}

{{/*
Return the public URL
*/}}
{{- define "policy-bot.publicUrl" -}}
{{- if .Values.config.server.publicUrl }}
{{- .Values.config.server.publicUrl }}
{{- else if .Values.ingress.enabled }}
{{- $host := (index .Values.ingress.hosts 0).host }}
{{- $hasCertManager := or (index .Values.ingress.annotations "cert-manager.io/cluster-issuer") (index .Values.ingress.annotations "cert-manager.io/issuer") }}
{{- if or .Values.ingress.tls.enabled $hasCertManager }}
{{- printf "https://%s" $host }}
{{- else }}
{{- printf "http://%s" $host }}
{{- end }}
{{- else }}
{{- printf "http://localhost:%d" (int .Values.config.server.port) }}
{{- end }}
{{- end }}

{{/*
Return the TLS secret name
*/}}
{{- define "policy-bot.tlsSecretName" -}}
{{- if .Values.ingress.tls.existingSecret }}
{{- .Values.ingress.tls.existingSecret }}
{{- else }}
{{- include "policy-bot.fullname" . }}-tls
{{- end }}
{{- end }}

{{/*
Return the image name
*/}}
{{- define "policy-bot.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}

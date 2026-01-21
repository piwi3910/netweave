{{/*
Expand the name of the chart.
*/}}
{{- define "netweave.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "netweave.fullname" -}}
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
{{- define "netweave.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "netweave.labels" -}}
helm.sh/chart: {{ include "netweave.chart" . }}
{{ include "netweave.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "netweave.selectorLabels" -}}
app.kubernetes.io/name: {{ include "netweave.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "netweave.serviceAccountName" -}}
{{- if .Values.gateway.serviceAccount.create }}
{{- default (include "netweave.fullname" .) .Values.gateway.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.gateway.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the proper gateway image name
*/}}
{{- define "netweave.gatewayImage" -}}
{{- $registryName := .Values.gateway.image.registry -}}
{{- $repositoryName := .Values.gateway.image.repository -}}
{{- $tag := .Values.gateway.image.tag | default .Chart.AppVersion -}}
{{- if .Values.global.imageRegistry }}
    {{- $registryName = .Values.global.imageRegistry -}}
{{- end }}
{{- if $registryName }}
{{- printf "%s/%s:%s" $registryName $repositoryName $tag -}}
{{- else }}
{{- printf "%s:%s" $repositoryName $tag -}}
{{- end }}
{{- end }}

{{/*
Return the PostgreSQL hostname
*/}}
{{- define "netweave.postgresql.host" -}}
{{- if .Values.postgresql.enabled }}
    {{- printf "%s-postgresql" (include "netweave.fullname" .) -}}
{{- else }}
    {{- .Values.keycloak.externalDatabase.host -}}
{{- end }}
{{- end }}

{{/*
Return the PostgreSQL port
*/}}
{{- define "netweave.postgresql.port" -}}
{{- if .Values.postgresql.enabled }}
    {{- print "5432" -}}
{{- else }}
    {{- .Values.keycloak.externalDatabase.port -}}
{{- end }}
{{- end }}

{{/*
Return the PostgreSQL database name
*/}}
{{- define "netweave.postgresql.database" -}}
{{- if .Values.postgresql.enabled }}
    {{- .Values.postgresql.auth.database -}}
{{- else }}
    {{- .Values.keycloak.externalDatabase.database -}}
{{- end }}
{{- end }}

{{/*
Return the PostgreSQL username
*/}}
{{- define "netweave.postgresql.username" -}}
{{- if .Values.postgresql.enabled }}
    {{- .Values.postgresql.auth.username -}}
{{- else }}
    {{- .Values.keycloak.externalDatabase.user -}}
{{- end }}
{{- end }}

{{/*
Return the PostgreSQL secret name
*/}}
{{- define "netweave.postgresql.secretName" -}}
{{- if .Values.postgresql.enabled }}
    {{- printf "%s-postgresql" (include "netweave.fullname" .) -}}
{{- else }}
    {{- printf "%s-secret" (include "netweave.fullname" .) -}}
{{- end }}
{{- end }}

{{/*
Return the Redis hostname
*/}}
{{- define "netweave.redis.host" -}}
{{- if .Values.redis.enabled }}
    {{- printf "%s-redis-master" (include "netweave.fullname" .) -}}
{{- else }}
    {{- required "Redis host is required when redis.enabled is false" .Values.redis.externalHost -}}
{{- end }}
{{- end }}

{{/*
Return the Redis port
*/}}
{{- define "netweave.redis.port" -}}
{{- if .Values.redis.enabled }}
    {{- print "6379" -}}
{{- else }}
    {{- .Values.redis.externalPort | default "6379" -}}
{{- end }}
{{- end }}

{{/*
Return the Redis connection string
*/}}
{{- define "netweave.redis.connectionString" -}}
{{- $host := include "netweave.redis.host" . -}}
{{- $port := include "netweave.redis.port" . -}}
{{- printf "redis://%s:%s/0" $host $port -}}
{{- end }}

{{/*
Return the Keycloak URL
*/}}
{{- define "netweave.keycloakURL" -}}
{{- if .Values.keycloak.enabled }}
    {{- printf "http://%s-keycloak:80" (include "netweave.fullname" .) -}}
{{- else }}
    {{- required "Keycloak URL is required when keycloak.enabled is false" .Values.keycloak.externalURL -}}
{{- end }}
{{- end }}

{{/*
Return the Vault URL
*/}}
{{- define "netweave.vaultURL" -}}
{{- if .Values.vault.enabled }}
    {{- printf "http://vault.%s.svc.cluster.local:8200" .Values.vault.namespace -}}
{{- else }}
    {{- required "Vault URL is required when vault.enabled is false" .Values.vault.externalURL -}}
{{- end }}
{{- end }}

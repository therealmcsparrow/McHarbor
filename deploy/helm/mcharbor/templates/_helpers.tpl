{{/*
Copyright (c) 2026 McSparrow. All rights reserved.
McHarbor is licensed under the McHarbor License. See LICENSE for details.
*/}}

{{- define "mcharbor.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "mcharbor.fullname" -}}
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

{{- define "mcharbor.headlessName" -}}
{{- printf "%s-nodes" (include "mcharbor.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "mcharbor.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "mcharbor.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mcharbor.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "mcharbor.labels" -}}
helm.sh/chart: {{ include "mcharbor.chart" . }}
{{ include "mcharbor.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: server
{{- end }}

{{- define "mcharbor.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "mcharbor.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "mcharbor.image" -}}
{{- printf "%s:%s" .Values.image.repository (.Values.image.tag | default .Chart.AppVersion) }}
{{- end }}

{{- define "mcharbor.scheme" -}}
{{- if .Values.tls.enabled }}https{{ else }}http{{ end }}
{{- end }}

{{- define "mcharbor.ha" -}}
{{- if eq .Values.mode "active-passive" }}true{{ end }}
{{- end }}

{{- define "mcharbor.externalDB" -}}
{{- if .Values.externalDatabase.existingSecret }}true{{ end }}
{{- end }}

{{- define "mcharbor.chartSecretWanted" -}}
{{- if and (not .Values.secrets.existingSecret) (or .Values.secrets.mcharborSecret .Values.secrets.encryptionKey .Values.secrets.backupKey) }}true{{ end }}
{{- end }}

{{- define "mcharbor.secretName" -}}
{{- if .Values.secrets.existingSecret }}
{{- .Values.secrets.existingSecret }}
{{- else }}
{{- include "mcharbor.fullname" . }}
{{- end }}
{{- end }}

{{- define "mcharbor.secretsConfigured" -}}
{{- if or .Values.secrets.existingSecret (include "mcharbor.chartSecretWanted" .) }}true{{ end }}
{{- end }}

{{- define "mcharbor.allowedOrigins" -}}
{{- if .Values.config.allowedOrigins }}
{{- .Values.config.allowedOrigins }}
{{- else if .Values.ingress.enabled }}
{{- $origins := list }}
{{- range .Values.ingress.hosts }}
{{- $origins = append $origins (printf "https://%s" .host) }}
{{- end }}
{{- join "," $origins }}
{{- end }}
{{- end }}

{{- define "mcharbor.validate" -}}
{{- if not (has .Values.mode (list "single" "active-passive")) }}
{{- fail (printf "mode must be \"single\" or \"active-passive\", got %q" .Values.mode) }}
{{- end }}
{{- if include "mcharbor.ha" . }}
{{- if not .Values.externalDatabase.existingSecret }}
{{- fail "mode=active-passive needs an external PostgreSQL, MySQL, MariaDB or SQL Server database: set externalDatabase.existingSecret to a Secret holding the DSN under externalDatabase.dsnKey. SQLite cannot be shared between nodes." }}
{{- end }}
{{- if not (has .Values.externalDatabase.driver (list "postgres" "mysql" "mssql")) }}
{{- fail "mode=active-passive supports externalDatabase.driver=postgres, mysql or mssql (MySQL 8.4 and MariaDB 11 use mysql); the active lock and failover are validated on those backends" }}
{{- end }}
{{- if lt (int .Values.replicaCount) 2 }}
{{- fail "mode=active-passive needs replicaCount >= 2" }}
{{- end }}
{{- if not .Values.secrets.existingSecret }}
{{- if not (and .Values.secrets.mcharborSecret .Values.secrets.encryptionKey .Values.secrets.backupKey) }}
{{- fail "mode=active-passive needs the same MCHARBOR_SECRET, ENCRYPTION_KEY and backup key on every node: set secrets.existingSecret, or all of secrets.mcharborSecret, secrets.encryptionKey and secrets.backupKey" }}
{{- end }}
{{- end }}
{{- if .Values.docker.socket.enabled }}
{{- fail "docker.socket.enabled is not supported with mode=active-passive: each pod would manage a different node's Docker daemon. Run the McHarbor agent on the Docker hosts instead." }}
{{- end }}
{{- if not (has .Values.ha.pluginStore (list "database")) }}
{{- if not (or (hasPrefix "shared-fs:" .Values.ha.pluginStore) (hasPrefix "object:" .Values.ha.pluginStore)) }}
{{- fail "ha.pluginStore must be database, shared-fs:/path or object:<storage location id>" }}
{{- end }}
{{- end }}
{{- end }}
{{- if and (eq .Values.mode "single") (not .Values.persistence.enabled) (not .Values.externalDatabase.existingSecret) }}
{{- fail "mode=single with persistence.enabled=false and no external database would lose the SQLite database on every restart" }}
{{- end }}
{{- if and .Values.externalDatabase.existingSecret (not (has .Values.externalDatabase.driver (list "postgres" "mysql" "mssql"))) }}
{{- fail "externalDatabase.driver must be postgres, mysql or mssql" }}
{{- end }}
{{- if .Values.secrets.encryptionKey }}
{{- if ne (len (b64dec .Values.secrets.encryptionKey)) 32 }}
{{- fail "secrets.encryptionKey must be base64 of exactly 32 bytes (openssl rand -base64 32)" }}
{{- end }}
{{- end }}
{{- if .Values.docker.socket.enabled }}
{{- if not .Values.nodeSelector }}
{{- fail "docker.socket.enabled mounts one node's Docker socket: set nodeSelector to pin the pod to that node, or McHarbor silently manages whichever node it was rescheduled to" }}
{{- end }}
{{- if eq (toString .Values.docker.socket.groupId) "" }}
{{- fail "docker.socket.enabled needs docker.socket.groupId: the group that owns the socket on the node (stat -c %g /var/run/docker.sock)" }}
{{- end }}
{{- end }}
{{- end }}

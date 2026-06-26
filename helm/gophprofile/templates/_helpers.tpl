{{/* Chart/name helpers — standard Helm boilerplate. */}}

{{- define "gophprofile.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "gophprofile.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "gophprofile.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "gophprofile.labels" -}}
helm.sh/chart: {{ include "gophprofile.chart" . }}
{{ include "gophprofile.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "gophprofile.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gophprofile.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "gophprofile.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "gophprofile.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
Name of the secret the Deployments read env from. Either the one we render
or an existing one the operator points us at.
*/}}
{{- define "gophprofile.secretName" -}}
{{- if .Values.secret.create -}}
{{- include "gophprofile.fullname" . -}}
{{- else -}}
{{- required "secret.existingSecret is required when secret.create is false" .Values.secret.existingSecret -}}
{{- end -}}
{{- end -}}

{{/* In-cluster service hosts for the bundled subcharts. */}}
{{- define "gophprofile.postgresHost" -}}
{{- printf "%s-postgresql" .Release.Name -}}
{{- end -}}

{{- define "gophprofile.minioHost" -}}
{{- printf "%s-minio" .Release.Name -}}
{{- end -}}

{{- define "gophprofile.rabbitmqHost" -}}
{{- printf "%s-rabbitmq" .Release.Name -}}
{{- end -}}

{{/* Computed secret values. Bundled subchart creds when enabled, else external*. */}}
{{- define "gophprofile.postgresDSN" -}}
{{- if .Values.postgresql.enabled -}}
{{- $a := .Values.postgresql.auth -}}
{{- $ssl := .Values.postgresql.sslMode | default "disable" -}}
{{- printf "postgres://%s:%s@%s:5432/%s?sslmode=%s" $a.username $a.password (include "gophprofile.postgresHost" .) $a.database $ssl -}}
{{- else -}}
{{- required "externalPostgres.dsn is required when postgresql.enabled is false" .Values.externalPostgres.dsn -}}
{{- end -}}
{{- end -}}

{{/*
Migration init containers for the server and worker pods: copy-migrations pulls
the SQL out of the app image into a shared volume, then migrate runs it. As an
initContainer the app waits for the schema, so no pod serves an empty DB. Retries
on its own until Postgres is up; safe to run from several pods at once (migrate
takes an advisory lock). Needs a `migrations` emptyDir volume on the pod.
*/}}
{{- define "gophprofile.migrationInitContainers" -}}
- name: copy-migrations
  image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
  imagePullPolicy: {{ .Values.image.pullPolicy }}
  command: ["sh", "-c", "cp -r /app/migrations/* /migrations/"]
  securityContext:
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop: [ALL]
  volumeMounts:
    - name: migrations
      mountPath: /migrations
- name: migrate
  image: "{{ .Values.migrations.image }}:{{ .Values.migrations.tag }}"
  imagePullPolicy: {{ .Values.migrations.pullPolicy }}
  args: ["-path=/migrations", "-database=$(POSTGRES_DSN)", "up"]
  env:
    - name: POSTGRES_DSN
      valueFrom:
        secretKeyRef:
          name: {{ include "gophprofile.secretName" . }}
          key: POSTGRES_DSN
  securityContext:
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop: [ALL]
  volumeMounts:
    - name: migrations
      mountPath: /migrations
{{- end -}}

{{- define "gophprofile.minioAccessKey" -}}
{{- if .Values.minio.enabled -}}
{{- .Values.minio.auth.rootUser -}}
{{- else -}}
{{- required "externalMinio.accessKey is required when minio.enabled is false" .Values.externalMinio.accessKey -}}
{{- end -}}
{{- end -}}

{{- define "gophprofile.minioSecretKey" -}}
{{- if .Values.minio.enabled -}}
{{- .Values.minio.auth.rootPassword -}}
{{- else -}}
{{- required "externalMinio.secretKey is required when minio.enabled is false" .Values.externalMinio.secretKey -}}
{{- end -}}
{{- end -}}

{{- define "gophprofile.minioEndpoint" -}}
{{- if .Values.minio.enabled -}}
{{- printf "%s:9000" (include "gophprofile.minioHost" .) -}}
{{- else -}}
{{- required "externalMinio.endpoint is required when minio.enabled is false" .Values.externalMinio.endpoint -}}
{{- end -}}
{{- end -}}

{{- define "gophprofile.rabbitmqURL" -}}
{{- if .Values.rabbitmq.enabled -}}
{{- $a := .Values.rabbitmq.auth -}}
{{- printf "amqp://%s:%s@%s:5672/" $a.username $a.password (include "gophprofile.rabbitmqHost" .) -}}
{{- else -}}
{{- required "externalRabbitmq.url is required when rabbitmq.enabled is false" .Values.externalRabbitmq.url -}}
{{- end -}}
{{- end -}}

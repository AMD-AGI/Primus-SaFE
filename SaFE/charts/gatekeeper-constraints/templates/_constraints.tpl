#
# Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#

{{- define "constraints.ownership" -}}
app.kubernetes.io/managed-by: Helm
{{- end -}}

{{/*
Curl image for the constraint apply hook. A SaFE proxy registry replaces the upstream name.
*/}}
{{- define "constraints.curlRepository" -}}
{{- if .Values.global.proxy_image_registry -}}
{{- printf "%s/primussafe/curl" .Values.global.proxy_image_registry -}}
{{- else -}}
{{- .Values.image.repository -}}
{{- end -}}
{{- end -}}

{{- define "constraints.membership" -}}
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: SandboxWorkloadMembership
metadata:
  name: sandbox-workload-membership
  labels:
    {{- include "constraints.ownership" . | nindent 4 }}
  annotations:
    meta.helm.sh/release-name: {{ .Release.Name | quote }}
    meta.helm.sh/release-namespace: {{ .Release.Namespace | quote }}
spec:
  enforcementAction: {{ .Values.enforcementAction | quote }}
  match:
    kinds:
      - apiGroups: ["amd.com"]
        kinds: ["Workload"]
  parameters:
    sandboxCreator: {{ .Values.sandboxCreator | quote }}
{{- end -}}

{{- define "constraints.attestation" -}}
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: SandboxWorkloadAttestation
metadata:
  name: sandbox-workload-attestation
  labels:
    {{- include "constraints.ownership" . | nindent 4 }}
  annotations:
    meta.helm.sh/release-name: {{ .Release.Name | quote }}
    meta.helm.sh/release-namespace: {{ .Release.Namespace | quote }}
spec:
  enforcementAction: {{ .Values.enforcementAction | quote }}
  match:
    kinds:
      - apiGroups: ["amd.com"]
        kinds: ["Workload"]
  parameters:
    sandboxCreator: {{ .Values.sandboxCreator | quote }}
    jwks: {{ .Values.jwks | quote }}
{{- end -}}

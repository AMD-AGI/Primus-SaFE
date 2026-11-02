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
{{- if .Values.image.repository -}}
{{- .Values.image.repository -}}
{{- else if and .Values.global .Values.global.proxy_image_registry -}}
{{- printf "%s/primussafe/curl" .Values.global.proxy_image_registry -}}
{{- else -}}
curlimages/curl
{{- end -}}
{{- end -}}

{{/* deny requires a JWKS document. */}}
{{- define "constraints.requireJwksForDeny" -}}
{{- if and (eq .Values.enforcementAction "deny") (or (not .Values.jwks) (not .Values.identityJwks) (not .Values.identityIssuer)) -}}
{{- fail "jwks, identityJwks, and identityIssuer must be set before enforcementAction is deny" -}}
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

{{- define "constraints.identityAttestation" -}}
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: SandboxWorkloadIdentity
metadata:
  name: sandbox-workload-identity
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
    jwks: {{ .Values.identityJwks | quote }}
    issuer: {{ .Values.identityIssuer | quote }}
{{- end -}}

{{- define "constraints.syncConfig" -}}
apiVersion: config.gatekeeper.sh/v1alpha1
kind: Config
metadata:
  name: config
  namespace: {{ .Release.Namespace }}
spec:
  sync:
    syncOnly:
      - group: amd.com
        version: v1
        kind: User
{{- end -}}

{{- define "constraints.membershipTemplate" -}}
apiVersion: templates.gatekeeper.sh/v1
kind: ConstraintTemplate
metadata:
  name: sandboxworkloadmembership
spec:
  crd:
    spec:
      names:
        kind: SandboxWorkloadMembership
      validation:
        openAPIV3Schema:
          type: object
          properties:
            sandboxCreator:
              type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
{{ .Files.Get "rego/membership.rego" | indent 8 }}
{{- end -}}

{{- define "constraints.attestationTemplate" -}}
apiVersion: templates.gatekeeper.sh/v1
kind: ConstraintTemplate
metadata:
  name: sandboxworkloadattestation
spec:
  crd:
    spec:
      names:
        kind: SandboxWorkloadAttestation
      validation:
        openAPIV3Schema:
          type: object
          properties:
            sandboxCreator:
              type: string
            jwks:
              type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
{{ .Files.Get "rego/attestation.rego" | indent 8 }}
{{- end -}}

{{- define "constraints.identityAttestationTemplate" -}}
apiVersion: templates.gatekeeper.sh/v1
kind: ConstraintTemplate
metadata:
  name: sandboxworkloadidentity
spec:
  crd:
    spec:
      names:
        kind: SandboxWorkloadIdentity
      validation:
        openAPIV3Schema:
          type: object
          properties:
            sandboxCreator:
              type: string
            jwks:
              type: string
            issuer:
              type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
{{ .Files.Get "rego/identity_attestation.rego" | indent 8 }}
{{- end -}}

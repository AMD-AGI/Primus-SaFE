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

{{/* identity.jwks, identity.issuer, and identity.actor are set together. deny requires the grant jwks or the identity provider. */}}
{{- define "constraints.requireJwksForDeny" -}}
{{- $id := .Values.identity -}}
{{- $idSet := and $id.jwks $id.issuer $id.actor -}}
{{- if and (or $id.jwks $id.issuer $id.actor) (not $idSet) -}}
{{- fail "identity.jwks, identity.issuer, and identity.actor must be set together" -}}
{{- end -}}
{{- if and (eq .Values.enforcementAction "deny") (not .Values.jwks) (not $idSet) -}}
{{- fail "jwks or the identity provider must be set before enforcementAction is deny" -}}
{{- end -}}
{{- end -}}

{{/* Constraints are kept when a render omits them; the delete hook removes them. */}}
{{- define "constraints.constraintAnnotations" -}}
meta.helm.sh/release-name: {{ .Release.Name | quote }}
meta.helm.sh/release-namespace: {{ .Release.Namespace | quote }}
helm.sh/resource-policy: keep
{{- end -}}

{{- define "constraints.membership" -}}
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: SandboxWorkloadMembership
metadata:
  name: sandbox-workload-membership
  labels:
    {{- include "constraints.ownership" . | nindent 4 }}
  annotations:
    {{- include "constraints.constraintAnnotations" . | nindent 4 }}
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
    {{- include "constraints.constraintAnnotations" . | nindent 4 }}
spec:
  enforcementAction: {{ .Values.enforcementAction | quote }}
  match:
    kinds:
      - apiGroups: ["amd.com"]
        kinds: ["Workload"]
  parameters:
    sandboxCreator: {{ .Values.sandboxCreator | quote }}
    jwks: {{ .Values.jwks | quote }}
    identity:
      jwks: {{ .Values.identity.jwks | quote }}
      issuer: {{ .Values.identity.issuer | quote }}
      audience: {{ .Values.identity.audience | quote }}
      tokenType: {{ .Values.identity.tokenType | quote }}
      actor: {{ .Values.identity.actor | quote }}
      detailType: {{ .Values.identity.detailType | quote }}
      algorithm: {{ .Values.identity.algorithm | quote }}
{{- end -}}

{{- define "constraints.syncConfig" -}}
apiVersion: config.gatekeeper.sh/v1alpha1
kind: Config
metadata:
  name: config
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "constraints.ownership" . | nindent 4 }}
  annotations:
    meta.helm.sh/release-name: {{ .Release.Name | quote }}
    meta.helm.sh/release-namespace: {{ .Release.Namespace | quote }}
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
  labels:
    {{- include "constraints.ownership" . | nindent 4 }}
  annotations:
    meta.helm.sh/release-name: {{ .Release.Name | quote }}
    meta.helm.sh/release-namespace: {{ .Release.Namespace | quote }}
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
  labels:
    {{- include "constraints.ownership" . | nindent 4 }}
  annotations:
    meta.helm.sh/release-name: {{ .Release.Name | quote }}
    meta.helm.sh/release-namespace: {{ .Release.Namespace | quote }}
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
            identity:
              type: object
              properties:
                jwks:
                  type: string
                issuer:
                  type: string
                audience:
                  type: string
                tokenType:
                  type: string
                actor:
                  type: string
                detailType:
                  type: string
                algorithm:
                  type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
{{ .Files.Get "rego/attestation.rego" | indent 8 }}
{{- end -}}

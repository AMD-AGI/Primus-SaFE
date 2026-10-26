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
        package sandboxworkloadmembership

        violation[{"msg": msg}] {
          input.review.userInfo.username == input.parameters.sandboxCreator
          user_id := object.get(object.get(input.review.object.metadata, "labels", {}), "primus-safe.user.id", "")
          workspace := object.get(input.review.object.spec, "workspace", "")
          not member(user_id, workspace)
          msg := sprintf("user %q is not a member of workspace %q", [user_id, workspace])
        }

        member(user_id, workspace) {
          user_id != ""
          workspace != ""
          listed(user_id, "workspaces")[_] == workspace
        }

        member(user_id, workspace) {
          user_id != ""
          workspace != ""
          listed(user_id, "workspace")[_] == workspace
        }

        listed(user_id, key) = items {
          user := data.inventory.cluster["amd.com/v1"]["User"][user_id]
          spec := object.get(user, "spec", {})
          resources := object.get(spec, "resources", {})
          items := object.get(resources, key, [])
        }
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
        package sandboxworkloadattestation

        violation[{"msg": msg}] {
          input.review.userInfo.username == input.parameters.sandboxCreator
          not valid_attestation
          msg := "grant attestation is missing, invalid, expired or does not match the workload"
        }

        valid_attestation {
          obj := input.review.object
          token := object.get(object.get(obj.metadata, "annotations", {}), "agent-sandbox.io/grant-attestation", "")
          token != ""
          [ok, _, claims] := io.jwt.decode_verify(token, {
            "cert": input.parameters.jwks,
            "alg": "ES256",
            "iss": "primus-claw",
            "aud": "agent-sandbox-workload",
            "time": time.now_ns(),
          })
          ok
          claims.jti == object.get(object.get(obj.metadata, "labels", {}), "agent-sandbox.io/grant-id", "")
          claims.sub == object.get(object.get(obj.metadata, "labels", {}), "primus-safe.user.id", "")
          claims.workspaces[_] == obj.spec.workspace
          claims.kinds[_] == obj.spec.groupVersionKind.kind
          gpu_count(obj) <= claims.maxGpusPerSandbox
        }

        gpu_count(obj) = n {
          cards := [c | resource := obj.spec.resources[_]; c := resource_gpus(resource)]
          n := sum(cards)
        }

        resource_gpus(resource) = n {
          object.get(resource, "gpu", "") != ""
          n := to_number(resource.gpu) * replica(resource)
        }

        replica(resource) = n {
          n := to_number(object.get(resource, "replica", 1))
        }
{{- end -}}

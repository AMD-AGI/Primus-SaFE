package sandboxworkloadattestation

violation[{"msg": msg}] {
  input.review.userInfo.username == input.parameters.sandboxCreator
  not attestation_satisfied
  msg := "attestation is missing, invalid, expired or does not match the workload"
}

grant_annotation := "agent-sandbox.io/grant-attestation"

identity_annotation := "agent-sandbox.io/identity-attestation"

# annotation reads one Workload annotation, or an empty string when it is unset.
annotation(name) = v {
  v := object.get(object.get(input.review.object.metadata, "annotations", {}), name, "")
}

grant_token := annotation(grant_annotation)

identity_token := annotation(identity_annotation)

# attestation_satisfied holds when the Workload carries at least one attestation and every attestation it carries verifies.
attestation_satisfied {
  has_attestation
  grant_ok
  identity_ok
}

has_attestation {
  grant_token != ""
}

has_attestation {
  identity_token != ""
}

grant_ok {
  grant_token == ""
}

grant_ok {
  valid_grant
}

identity_ok {
  identity_token == ""
}

identity_ok {
  valid_identity
}

# verify_time is the time a token is checked at. An UPDATE that keeps the token admitted with the old object is checked
# at the token expiry, so a later update is not refused because the token has since expired. Any other request is
# checked at the current time.
verify_time(name, token) = t {
  input.review.operation == "UPDATE"
  old := object.get(object.get(object.get(input.review, "oldObject", {}), "metadata", {}), "annotations", {})
  object.get(old, name, "") == token
  [_, payload, _] := io.jwt.decode(token)
  t := (payload.exp - 1) * 1000000000
} else = t {
  t := time.now_ns()
}

# valid_grant checks the Claw grant JWT against this Workload.
valid_grant {
  obj := input.review.object
  [ok, _, claims] := io.jwt.decode_verify(grant_token, {
    "cert": input.parameters.jwks,
    "alg": "ES256",
    "iss": "primus-claw",
    "aud": "agent-sandbox-workload",
    "time": verify_time(grant_annotation, grant_token),
  })
  ok
  object.get(claims, "exp", 0) > 0
  claims.jti == object.get(object.get(obj.metadata, "labels", {}), "agent-sandbox.io/grant-id", "")
  claims.sub == object.get(object.get(obj.metadata, "labels", {}), "primus-safe.user.id", "")
  claims.workspaces[_] == obj.spec.workspace
  claims.kinds[_] == obj.spec.groupVersionKind.kind
  gpu_count(obj) <= claims.maxGpusPerSandbox
}

# valid_identity checks the identity provider assertion against this Workload: signature and algorithm, typ, iss,
# aud, exp, the user label as sub, the control plane as act.sub, and a workload authorization detail that matches.
valid_identity {
  obj := input.review.object
  identity := object.get(input.parameters, "identity", {})
  [header, _, _] := io.jwt.decode(identity_token)
  header.typ == identity.tokenType
  [ok, _, claims] := io.jwt.decode_verify(identity_token, {
    "cert": identity.jwks,
    "alg": object.get(identity, "algorithm", "ES256"),
    "iss": identity.issuer,
    "aud": identity.audience,
    "time": verify_time(identity_annotation, identity_token),
  })
  ok
  object.get(claims, "exp", 0) > 0
  claims.sub == object.get(object.get(obj.metadata, "labels", {}), "primus-safe.user.id", "")
  claims.act.sub == identity.actor
  matching_detail(claims, obj, identity.detailType)
}

# matching_detail holds when one authorization detail of the given type names this Workload.
matching_detail(claims, obj, detail_type) {
  detail := object.get(claims, "authorization_details", [])[_]
  detail.type == detail_type
  detail.workspace == obj.spec.workspace
  detail.kind == "Sandbox"
  detail.kind == obj.spec.groupVersionKind.kind
  detail.name == obj.metadata.name
  is_number(detail.gpus)
  detail.gpus == gpu_count(obj)
}

# gpu_count sums gpu times replica across workload resources.
gpu_count(obj) = n {
  cards := [resource_gpus(r) | r := object.get(obj.spec, "resources", [])[_]]
  n := sum(cards)
}

# resource_gpus returns the gpu count times replica, or zero when gpu is unset.
resource_gpus(resource) = n {
  object.get(resource, "gpu", "") != ""
  n := to_number(resource.gpu) * replica(resource)
}

resource_gpus(resource) = 0 {
  object.get(resource, "gpu", "") == ""
}

# replica reads the resource replica, defaulting to one.
replica(resource) = n {
  n := to_number(object.get(resource, "replica", 1))
}

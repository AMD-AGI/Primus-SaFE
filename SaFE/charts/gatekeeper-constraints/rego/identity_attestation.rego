package sandboxworkloadidentity

violation[{"msg": msg}] {
  input.review.userInfo.username == input.parameters.sandboxCreator
  not identity_satisfied
  msg := "identity assertion is missing, invalid, expired or does not match the workload"
}

# identity_token reads the identity provider assertion. An empty value means this Workload uses the Claw grant path.
identity_token = t {
  t := object.get(object.get(input.review.object.metadata, "annotations", {}), "agent-sandbox.io/identity-attestation", "")
}

# grant_token reads the Claw grant JWT. Presence is enough here; the grant constraint verifies it.
grant_token = t {
  t := object.get(object.get(input.review.object.metadata, "annotations", {}), "agent-sandbox.io/grant-attestation", "")
}

# identity_satisfied accepts a valid identity assertion, or no identity assertion when a Claw grant is present.
# An identity assertion that is present must verify. An invalid token is not excused by the other annotation.
identity_satisfied {
  valid_identity
}

identity_satisfied {
  identity_token == ""
  grant_token != ""
}

# valid_identity checks the identity provider assertion against this Workload: signature, typ, iss, aud, exp,
# the user label as sub, the control plane as act.sub, and a workload authorization detail that matches.
valid_identity {
  obj := input.review.object
  token := identity_token
  token != ""
  [header, _, _] := io.jwt.decode(token)
  header.typ == input.parameters.tokenType
  [ok, _, claims] := io.jwt.decode_verify(token, {
    "cert": input.parameters.jwks,
    "iss": input.parameters.issuer,
    "aud": input.parameters.audience,
    "time": time.now_ns(),
  })
  ok
  object.get(claims, "exp", 0) > 0
  claims.sub == object.get(object.get(obj.metadata, "labels", {}), "primus-safe.user.id", "")
  claims.act.sub == input.parameters.actor
  matching_detail(claims, obj)
}

# matching_detail holds when one authorization detail of the configured type names this Workload.
matching_detail(claims, obj) {
  detail := object.get(claims, "authorization_details", [])[_]
  detail.type == input.parameters.detailType
  detail.workspace == obj.spec.workspace
  detail.kind == "Sandbox"
  detail.kind == obj.spec.groupVersionKind.kind
  detail.name == obj.metadata.name
  is_number(detail.gpus)
  detail.gpus == gpu_count(obj)
}

# gpu_count sums gpu times replica across workload resources.
gpu_count(obj) = n {
  cards := [resource_gpus(object.get(obj.spec, "resources", [])[_])]
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

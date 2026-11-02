package sandboxworkloadidentity

violation[{"msg": msg}] {
  input.review.userInfo.username == input.parameters.sandboxCreator
  not identity_satisfied
  msg := "identity attestation is missing, invalid, expired or does not match the workload"
}

# identity_token reads the primus-identity JWT. An empty value means this Workload uses the Claw grant path.
identity_token = t {
  t := object.get(object.get(input.review.object.metadata, "annotations", {}), "agent-sandbox.io/identity-attestation", "")
}

# grant_token reads the Claw grant JWT. Presence is enough here; the grant constraint verifies it.
grant_token = t {
  t := object.get(object.get(input.review.object.metadata, "annotations", {}), "agent-sandbox.io/grant-attestation", "")
}

# identity_satisfied accepts a valid identity attestation, or no identity attestation when a Claw grant is present.
# An identity attestation that is present must verify. An invalid token is not excused by the other annotation.
identity_satisfied {
  valid_identity
}

identity_satisfied {
  identity_token == ""
  grant_token != ""
}

# valid_identity checks the primus-identity sandbox JWT against this Workload.
valid_identity {
  obj := input.review.object
  token := object.get(object.get(obj.metadata, "annotations", {}), "agent-sandbox.io/identity-attestation", "")
  token != ""
  [ok, _, claims] := io.jwt.decode_verify(token, {
    "cert": input.parameters.jwks,
    "alg": "ES256",
    "iss": input.parameters.issuer,
    "aud": "agent-sandbox-identity",
    "time": time.now_ns(),
  })
  ok
  object.get(claims, "exp", 0) > 0
  claims.sub == object.get(object.get(obj.metadata, "labels", {}), "primus-safe.user.id", "")
  claims.workspace == obj.spec.workspace
  claims.kind == "Sandbox"
  claims.kind == obj.spec.groupVersionKind.kind
}

package sandboxworkloadidentity

violation[{"msg": msg}] {
  input.review.userInfo.username == input.parameters.sandboxCreator
  not valid_identity
  msg := "identity attestation is missing, invalid, expired or does not match the workload"
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

package sandboxworkloadattestation

violation[{"msg": msg}] {
  input.review.userInfo.username == input.parameters.sandboxCreator
  not grant_satisfied
  msg := "grant attestation is missing, invalid, expired or does not match the workload"
}

# grant_token reads the Claw grant JWT. An empty value means this Workload uses the identity path.
grant_token = t {
  t := object.get(object.get(input.review.object.metadata, "annotations", {}), "agent-sandbox.io/grant-attestation", "")
}

# identity_token reads the primus-identity JWT. Presence is enough here; the identity constraint verifies it.
identity_token = t {
  t := object.get(object.get(input.review.object.metadata, "annotations", {}), "agent-sandbox.io/identity-attestation", "")
}

# grant_satisfied accepts a valid Claw grant, or no grant when an identity attestation is present.
# A grant that is present must verify. An invalid grant is not excused by the other annotation.
grant_satisfied {
  valid_attestation
}

grant_satisfied {
  grant_token == ""
  identity_token != ""
}

# valid_attestation checks the Claw grant JWT against this Workload.
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
  object.get(claims, "exp", 0) > 0
  claims.jti == object.get(object.get(obj.metadata, "labels", {}), "agent-sandbox.io/grant-id", "")
  claims.sub == object.get(object.get(obj.metadata, "labels", {}), "primus-safe.user.id", "")
  claims.workspaces[_] == obj.spec.workspace
  claims.kinds[_] == obj.spec.groupVersionKind.kind
  gpu_count(obj) <= claims.maxGpusPerSandbox
}

# gpu_count sums gpu times replica across workload resources.
gpu_count(obj) = n {
  cards := [resource_gpus(obj.spec.resources[_])]
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

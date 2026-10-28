package sandboxworkloadattestation

violation[{"msg": msg}] {
  input.review.userInfo.username == input.parameters.sandboxCreator
  not valid_attestation
  msg := "grant attestation is missing, invalid, expired or does not match the workload"
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
  cards := [c | resource := obj.spec.resources[_]; c := resource_gpus(resource)]
  n := sum(cards)
}

# resource_gpus returns zero when the resource does not request a GPU.
resource_gpus(resource) = n {
  object.get(resource, "gpu", "") != ""
  n := to_number(resource.gpu) * replica(resource)
}

# replica reads the resource replica, defaulting to one.
replica(resource) = n {
  n := to_number(object.get(resource, "replica", 1))
}

package sandboxworkloadidentity

creator := "system:serviceaccount:agent-sandbox-system:agent-sandbox-controlplane"
issuer := "https://identity.example.test"

cert := `-----BEGIN CERTIFICATE-----
MIIBHjCBxaADAgECAgEBMAoGCCqGSM49BAMCMBgxFjAUBgNVBAMMDXJlZ28taWRl
bnRpdHkwIBcNMjYxMDI2MDAwMDAwWhgPMjA5OTAxMDEwMDAwMDBaMBgxFjAUBgNV
BAMMDXJlZ28taWRlbnRpdHkwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAASiM3UQ
3YBzybd2MTImAXOobBUPZDh0maLlE4dBZ+ftWFnwJFup85N+H9zBhq8EjLXnXOuk
QP0M01Ls8jiHy2e5MAoGCCqGSM49BAMCA0gAMEUCIGc2A9BXQZmNU2V41DLGVuKu
Knuj/urMkgGsmYBYHmS1AiEAg7iICJVCDpHsGQ/xFC5KDgMpH6AaX/z1PIImN353
eZE=
-----END CERTIFICATE-----`

valid_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsInNhbmRib3giOiJzYngtMSIsIm1heEdwdXMiOjEsImV4cCI6NDEwMjQ0NDgwMH0.o9ZYCgUVD5i_eEoZWWZw8-OhEhzrLlRHwkReN1BtrZpf8X4QSSkQU941XXcUW3W4XiFgTZOwJtHytQ6mpFDNtg"
expired_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsInNhbmRib3giOiJzYngtMSIsIm1heEdwdXMiOjEsImV4cCI6MX0.95WeKjML19XS9B8X6q593JK0WFYk69mihthQ4vIqLbUZU2ORnQCFvnSDMDYC4zvViuqtDwLsm-C79ETJ7onSRw"
no_gpu_claim_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsInNhbmRib3giOiJzYngtMSIsImV4cCI6NDEwMjQ0NDgwMH0.D4w0Ud0uYGZidMcljig4ZO5yEAAqHruynJc_vPlYqhT1PY7wyONXhAuXI6Cit_YuYa7Vs4WIXBujgHSejJTHxw"
no_sandbox_claim_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm1heEdwdXMiOjEsImV4cCI6NDEwMjQ0NDgwMH0.t-EdRTFNYsy3qrkiClhXUuQt_LxbjsKNEH8HtYL_PIRzTD-p0YrjFy6Jtwp-cDSaxVBfYpe4bCRXJ58vcyN83Q"

workload(token, workspace) = obj {
  obj := gpu_workload(token, workspace, "")
}

gpu_workload(token, workspace, gpus) = obj {
  obj := named_workload(token, workspace, gpus, "sbx-1")
}

named_workload(token, workspace, gpus, name) = obj {
  obj := {
    "metadata": {
      "name": name,
      "labels": {"primus-safe.user.id": "user-1"},
      "annotations": {"agent-sandbox.io/identity-attestation": token},
    },
    "spec": {
      "workspace": workspace,
      "groupVersionKind": {"kind": "Sandbox"},
      "resources": [{"gpu": gpus, "replica": 1}],
    },
  }
}

review(token, workspace) = inp {
  inp := wrap(workload(token, workspace))
}

wrap(obj) = inp {
  inp := {
    "review": {"userInfo": {"username": creator}, "object": obj},
    "parameters": {"sandboxCreator": creator, "jwks": cert, "issuer": issuer},
  }
}

test_valid_token {
  count(violation) == 0 with input as review(valid_token, "lab")
}

test_wrong_workspace {
  count(violation) == 1 with input as review(valid_token, "other")
}

test_expired_token {
  count(violation) == 1 with input as review(expired_token, "lab")
}

test_missing_token {
  count(violation) == 1 with input as review("", "lab")
}

test_gpu_within_max {
  count(violation) == 0 with input as wrap(gpu_workload(valid_token, "lab", "1"))
}

test_gpu_above_max {
  count(violation) == 1 with input as wrap(gpu_workload(valid_token, "lab", "2"))
}

test_other_workload_name {
  count(violation) == 1 with input as wrap(named_workload(valid_token, "lab", "", "sbx-2"))
}

test_token_without_sandbox_claim {
  count(violation) == 1 with input as review(no_sandbox_claim_token, "lab")
}

test_token_without_gpu_claim {
  count(violation) == 1 with input as review(no_gpu_claim_token, "lab")
}

test_grant_only_skips_identity {
  count(violation) == 0 with input as grant_only_review
}

test_invalid_identity_not_excused_by_grant {
  count(violation) == 1 with input as expired_identity_with_grant
}

grant_only_review = inp {
  obj := {
    "metadata": {
      "labels": {"primus-safe.user.id": "user-1", "agent-sandbox.io/grant-id": "gr-1"},
      "annotations": {"agent-sandbox.io/grant-attestation": "eyJ.grant"},
    },
    "spec": {
      "workspace": "lab",
      "groupVersionKind": {"kind": "Sandbox"},
    },
  }
  inp := wrap(obj)
}

expired_identity_with_grant = inp {
  obj := {
    "metadata": {
      "name": "sbx-1",
      "labels": {"primus-safe.user.id": "user-1"},
      "annotations": {
        "agent-sandbox.io/identity-attestation": expired_token,
        "agent-sandbox.io/grant-attestation": "eyJ.grant",
      },
    },
    "spec": {
      "workspace": "lab",
      "groupVersionKind": {"kind": "Sandbox"},
    },
  }
  inp := wrap(obj)
}

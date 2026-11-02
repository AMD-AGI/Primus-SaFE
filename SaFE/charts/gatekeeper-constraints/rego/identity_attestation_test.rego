package sandboxworkloadidentity

creator := "system:serviceaccount:agent-sandbox-system:agent-sandbox-controlplane"
issuer := "https://identity.example.test"

cert := `-----BEGIN CERTIFICATE-----
MIIBHzCBxaADAgECAgEBMAoGCCqGSM49BAMCMBgxFjAUBgNVBAMMDXJlZ28taWRl
bnRpdHkwIBcNMjYxMDI2MDAwMDAwWhgPMjA5OTAxMDEwMDAwMDBaMBgxFjAUBgNV
BAMMDXJlZ28taWRlbnRpdHkwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAATBJy/j
G1Obre2aROsfXr0qTV59xSvyjXhcof6V0wLjcwS3ohczo2XbB+Ydwta5ccZQdgHT
hHthDBv8TmoFMtUjMAoGCCqGSM49BAMCA0kAMEYCIQDWoT7Rye5xR2qTukcw7IEc
C+xjcD6P1XGIdAfg4I2SjQIhAJzvOoaO4BCqK1LZbmXdrriWPAvWUcqa3L6OjgT6
dDcb
-----END CERTIFICATE-----`

valid_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm1heEdwdXMiOjEsImV4cCI6NDEwMjQ0NDgwMH0.iNwyar8JCqYB9nw85mbSDrtgW-qGzredgKwyl58X-kL5PJjXFcZ7UKjUgo8E_udXG8JFxOiKFCCRlz4Rb27tRw"
expired_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm1heEdwdXMiOjEsImV4cCI6MX0.CRPWoX-1rrtJa9WFKr-nl7DTIbkms3tI6yC7rzC1UiX0CO6GP_3rcB__2-JBrepmQDlJ6_STJca2OWqB1pMZ9w"
no_gpu_claim_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsImV4cCI6NDEwMjQ0NDgwMH0.Ln3QXMEfmiFe9wyD415jGegvCjI_aflgLm1ILokWqOmC1F0kISTbNKVjd3mhQhDvItgvXpl_5Fgr7dlQoCfQkg"

workload(token, workspace) = obj {
  obj := gpu_workload(token, workspace, "")
}

gpu_workload(token, workspace, gpus) = obj {
  obj := {
    "metadata": {
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

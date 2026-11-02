package sandboxworkloadidentity

creator := "system:serviceaccount:agent-sandbox-system:agent-sandbox-controlplane"
issuer := "https://identity.example.test"

cert := `-----BEGIN CERTIFICATE-----
MIIBHjCBxaADAgECAgEBMAoGCCqGSM49BAMCMBgxFjAUBgNVBAMMDXJlZ28taWRl
bnRpdHkwIBcNMjYxMDI2MDAwMDAwWhgPMjA5OTAxMDEwMDAwMDBaMBgxFjAUBgNV
BAMMDXJlZ28taWRlbnRpdHkwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAS7uxK0
K4tqzAawHexPtPleA63SpKJfIqTrDeVX6qwRTX6wWMFXZCv2Sp5C29uz9wDyf94+
GZu1OVpliXXekAJmMAoGCCqGSM49BAMCA0gAMEUCIQCetOpX3vvdldzmYLsN01Tf
mbS5NTbNrZCGxOVZDZ/ZJAIgV3z2t129lQDefQqKn4V1pPcOb8FTZWMKyCum2qFV
6p8=
-----END CERTIFICATE-----`

valid_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsImV4cCI6NDEwMjQ0NDgwMH0.8gTxk7_h4l8qeqqGjec1A4lqaiE16cQkMU1xm3371mgJld_5Jd8qGlqPdvT0_i-wY9-cZZPTlggrcujbt6ooWw"
expired_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6InNhbmRib3gtYXR0ZXN0YXRpb24rand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtaWRlbnRpdHkiLCJzdWIiOiJ1c2VyLTEiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsImV4cCI6MX0.q-P_AzaJoXzHs5leVTu-X4rOqn3ChNb5A_gq_Q2G-jmtRd-T_Oblu9EXSTk2ABKOa2juj7liIaJaBqOl1K2klQ"

workload(token, workspace) = obj {
  obj := {
    "metadata": {
      "labels": {"primus-safe.user.id": "user-1"},
      "annotations": {"agent-sandbox.io/identity-attestation": token},
    },
    "spec": {
      "workspace": workspace,
      "groupVersionKind": {"kind": "Sandbox"},
    },
  }
}

review(token, workspace) = inp {
  inp := {
    "review": {"userInfo": {"username": creator}, "object": workload(token, workspace)},
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

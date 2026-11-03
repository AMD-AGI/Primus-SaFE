package sandboxworkloadattestation

creator := "system:serviceaccount:agent-sandbox-system:agent-sandbox-controlplane"

cert := `-----BEGIN CERTIFICATE-----
MIIBKTCB0KADAgECAhQbzcLME9NfLC+QLMFMCHOHC+9NKzAKBggqhkjOPQQDAjAU
MRIwEAYDVQQDDAlyZWdvLXRlc3QwIBcNMjYxMDI2MTQzNTI2WhgPMjA5OTAxMDEw
MDAwMDBaMBQxEjAQBgNVBAMMCXJlZ28tdGVzdDBZMBMGByqGSM49AgEGCCqGSM49
AwEHA0IABJ81yvahCKjdXi4AvybDTZXhpCY+fva2cg9UELcNaPpvt0AYMSPAYnVd
c0dqFtak6ot16fzY+t8rjP6Dt8Gbih8wCgYIKoZIzj0EAwIDSAAwRQIgK03oqBLd
nqLrPTFu+oKpD5/wng+Uc6uyGScKZRhlzGQCIQDVCOY8IseRgWBa6Ptjc7hd/fiy
r8d+dLsh7Ap4NVHXQg==
-----END CERTIFICATE-----`

grant_valid := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJwcmltdXMtY2xhdyIsImF1ZCI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJqdGkiOiJnci0xIiwic3ViIjoidXNlci0xIiwid29ya3NwYWNlcyI6WyJsYWIiXSwia2luZHMiOlsiU2FuZGJveCJdLCJtYXhHcHVzUGVyU2FuZGJveCI6MSwiZXhwIjo0MTAyNDQ0ODAwfQ._k6NJwq-R1Vch2L95t39PGPPYQm9aJOcUvhh0-Wy-7jtEX5EbycPHBGL96nh53S4LEB-KUYhXR2xcFh5ecr4lw"
grant_expired := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJwcmltdXMtY2xhdyIsImF1ZCI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJqdGkiOiJnci0xIiwic3ViIjoidXNlci0xIiwid29ya3NwYWNlcyI6WyJsYWIiXSwia2luZHMiOlsiU2FuZGJveCJdLCJtYXhHcHVzUGVyU2FuZGJveCI6MSwiZXhwIjoxfQ.BmEw7C-17IGt_QmzuXv5tpbhcvzVxhxqwAlINXaFFVj5EFQHS_M1Hqt1W0DF294-rpHOb5GXMgol6dDS-jYFvw"
grant_missing_exp := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJwcmltdXMtY2xhdyIsImF1ZCI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJqdGkiOiJnci0xIiwic3ViIjoidXNlci0xIiwid29ya3NwYWNlcyI6WyJsYWIiXSwia2luZHMiOlsiU2FuZGJveCJdLCJtYXhHcHVzUGVyU2FuZGJveCI6MX0.HXiQcRCKg3P3oXfPoO5mE_bZ7aVqAbMo9Ktoi2qPCiMoinJPxZowWuvuADWJvmotw-0qj8FHi0WI7IGR0AgYVQ"
issuer := "https://identity.example.test"

identity_jwks := `{"keys":[{"kty":"EC","crv":"P-256","x":"6CGuFJwQpwIJ9X4Vb6jLs9plsGVZqN_N4yhNVuUywaw","y":"lK8cqCW2A6eNavmsZ4mIHnlz9gcm8TH_qr9U-9WvlDo","kid":"k1","alg":"ES256","use":"sig"}]}`

id_valid := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.0BM5n_HOPhV1EBA_WLGLmqFhEIjs8XVaZD-NB_gp4kuv9-kdNTPoK8m1SBy5-jpsAtmggpDgv2SgVOC7P0Nfzw"
id_expired := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjEsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.VAjSRzVbbxvRTB37ZnFbpSx8A9RluqEN3o9820myaaa-DBZB2F9chiWD0sH-YhN1bPn6WXSL5-MeHb-S5FQASA"
id_access_typ := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXQrand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.Doo7k9PtIC69sWWc8NBPPPfSfr7HnZZQlhZw37GhXFihovChabAwLo2pd1X9xecc2hCY4yuo4FvJ9hFb94hWCg"
id_other_actor := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1vdGhlciJ9LCJpYXQiOjE3MDAwMDAwMDAsImV4cCI6NDEwMjQ0NDgwMCwiYXV0aG9yaXphdGlvbl9kZXRhaWxzIjpbeyJ0eXBlIjoiYWdlbnQtc2FuZGJveC13b3JrbG9hZCIsIndvcmtzcGFjZSI6ImxhYiIsImtpbmQiOiJTYW5kYm94IiwibmFtZSI6InNieC0xIiwiZ3B1cyI6MX1dfQ.RGqH3-mgQUGU_5VVO26OWdenE-VUTZQB79zDNEtkj6xKN7v5gVsUBlmVvkKC9Zs7aQ3xD8-C0d4r3f_3IMnXag"
id_other_audience := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6Im90aGVyIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.XuGSA_ZX9RJOX2dO7RCn6mfyD1gkBv7B2z8DEPmlhPm1fOFQzyilVk3WlWl8VEPbNUuGmw-eGmC3Mr2qGt-jsA"
id_other_issuer := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL290aGVyLmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.vpmVbcwQUSE7KITUdr4V4mgQctNBViSEvh_WMISv9iCuGJwuWglLwIjJQ-d7tS8NO8VFjKVxw7_63WUzjobf1w"
id_other_detail_type := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6Im90aGVyIiwid29ya3NwYWNlIjoibGFiIiwia2luZCI6IlNhbmRib3giLCJuYW1lIjoic2J4LTEiLCJncHVzIjoxfV19.mdwPixbGJtcs55XpscwqftKvCc0g68DlzpDuV3g5UFI5wmhylHabVL1A_KLMd6cRc0-uqPDkQzlRSB4rJz1zaQ"
id_no_details := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDB9.tZ1EkYI_oV0t7_VZpS0_DVh66deukk2W7B013vVb8wGZlMh5Se-OGyz0uB1x0NtK7zNoZumkJbS7a94kr4c25w"
id_string_gpus := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOiIxIn1dfQ.VCMLQ6yZlveDCdFiQ4Ci4aeLtdqfpAbMBAEzSOQIxVCcWLBiVRDFsCrZKxBX8dvRvMqWSVDVyR3E5PCp2-cI8A"
id_no_name := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsImdwdXMiOjF9XX0.tRcbhEDWHgphqzAZR7aje5PFcEAJkyVVkHkBK8ri8u-LOSH9tvL4wnPe8JvYmUyS3QXgp_Aekpg3v0GLOR9JOw"
id_foreign_key := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.tl4xChCgX-CIRsWCdhatuAWy4ckPPeCWARuPGHbdcGd8_uJJcYp06eu1G89LjzWjVCAfDYwewgAzq0Jk8lGs0A"

# workload returns a sandbox Workload with the given annotations and GPU count.
workload(annotations, gpus) = obj {
  obj := {
    "metadata": {
      "name": "sbx-1",
      "labels": {"primus-safe.user.id": "user-1", "agent-sandbox.io/grant-id": "gr-1"},
      "annotations": annotations,
    },
    "spec": {
      "workspace": "lab",
      "groupVersionKind": {"kind": "Sandbox"},
      "resources": [{"gpu": gpus, "replica": 1}],
    },
  }
}

params := {
  "sandboxCreator": creator,
  "jwks": cert,
  "identity": {
    "jwks": identity_jwks,
    "issuer": issuer,
    "audience": "agent-sandbox-admission",
    "tokenType": "assertion+jwt",
    "actor": "svc-controlplane",
    "detailType": "agent-sandbox-workload",
    "algorithm": "ES256",
  },
}

# wrap returns a CREATE review of obj by the control plane.
wrap(obj) = inp {
  inp := {"review": {"operation": "CREATE", "userInfo": {"username": creator}, "object": obj}, "parameters": params}
}

# update returns an UPDATE review from old to obj by the control plane.
update(old, obj) = inp {
  inp := {"review": {"operation": "UPDATE", "userInfo": {"username": creator}, "object": obj, "oldObject": old}, "parameters": params}
}

grant(token, gpus) = inp {
  inp := wrap(workload({"agent-sandbox.io/grant-attestation": token}, gpus))
}

identity(token) = inp {
  inp := wrap(identity_workload(token, "1"))
}

identity_workload(token, gpus) = obj {
  obj := workload({"agent-sandbox.io/identity-attestation": token}, gpus)
}

test_grant_valid {
  count(violation) == 0 with input as grant(grant_valid, "1")
}

test_grant_missing_exp {
  count(violation) == 1 with input as grant(grant_missing_exp, "1")
}

test_grant_expired {
  count(violation) == 1 with input as grant(grant_expired, "1")
}

test_grant_gpu_above_max {
  count(violation) == 1 with input as grant(grant_valid, "2")
}

test_grant_cpu_only_resource {
  count(violation) == 0 with input as grant(grant_valid, "")
}

test_missing_both_attestations {
  count(violation) == 1 with input as wrap(workload({}, "1"))
}

test_other_creator_not_checked {
  inp := json.patch(wrap(workload({}, "1")), [{"op": "replace", "path": "/review/userInfo/username", "value": "someone"}])
  count(violation) == 0 with input as inp
}

test_unverified_identity_annotation_rejected {
  count(violation) == 1 with input as identity("eyJ.identity")
}

test_invalid_grant_not_excused_by_identity {
  inp := wrap(workload({"agent-sandbox.io/grant-attestation": grant_expired, "agent-sandbox.io/identity-attestation": id_valid}, "1"))
  count(violation) == 1 with input as inp
}

test_invalid_identity_not_excused_by_grant {
  inp := wrap(workload({"agent-sandbox.io/grant-attestation": grant_valid, "agent-sandbox.io/identity-attestation": id_expired}, "1"))
  count(violation) == 1 with input as inp
}

test_both_valid {
  inp := wrap(workload({"agent-sandbox.io/grant-attestation": grant_valid, "agent-sandbox.io/identity-attestation": id_valid}, "1"))
  count(violation) == 0 with input as inp
}

test_identity_valid {
  count(violation) == 0 with input as identity(id_valid)
}

test_identity_wrong_workspace {
  obj := json.patch(identity_workload(id_valid, "1"), [{"op": "replace", "path": "/spec/workspace", "value": "other"}])
  count(violation) == 1 with input as wrap(obj)
}

test_identity_expired {
  count(violation) == 1 with input as identity(id_expired)
}

test_identity_access_token_type {
  count(violation) == 1 with input as identity(id_access_typ)
}

test_identity_other_actor {
  count(violation) == 1 with input as identity(id_other_actor)
}

test_identity_other_audience {
  count(violation) == 1 with input as identity(id_other_audience)
}

test_identity_other_issuer {
  count(violation) == 1 with input as identity(id_other_issuer)
}

test_identity_foreign_key {
  count(violation) == 1 with input as identity(id_foreign_key)
}

test_identity_other_detail_type {
  count(violation) == 1 with input as identity(id_other_detail_type)
}

test_identity_no_details {
  count(violation) == 1 with input as identity(id_no_details)
}

test_identity_detail_without_name {
  count(violation) == 1 with input as identity(id_no_name)
}

test_identity_string_gpus {
  count(violation) == 1 with input as identity(id_string_gpus)
}

test_identity_other_workload_name {
  obj := json.patch(identity_workload(id_valid, "1"), [{"op": "replace", "path": "/metadata/name", "value": "sbx-2"}])
  count(violation) == 1 with input as wrap(obj)
}

test_identity_gpu_more_than_asserted {
  count(violation) == 1 with input as wrap(identity_workload(id_valid, "2"))
}

test_identity_gpu_fewer_than_asserted {
  count(violation) == 1 with input as wrap(identity_workload(id_valid, ""))
}

test_identity_other_user {
  obj := json.patch(identity_workload(id_valid, "1"), [{"op": "replace", "path": "/metadata/labels/primus-safe.user.id", "value": "user-2"}])
  count(violation) == 1 with input as wrap(obj)
}

test_identity_other_algorithm {
  inp := json.patch(identity(id_valid), [{"op": "replace", "path": "/parameters/identity/algorithm", "value": "RS256"}])
  count(violation) == 1 with input as inp
}

test_identity_unconfigured {
  inp := json.patch(identity(id_valid), [{"op": "replace", "path": "/parameters/identity", "value": {}}])
  count(violation) == 1 with input as inp
}

test_update_keeps_expired_identity {
  obj := identity_workload(id_expired, "1")
  changed := json.patch(obj, [{"op": "add", "path": "/spec/timeout", "value": 3600}])
  count(violation) == 0 with input as update(obj, changed)
}

test_update_keeps_expired_grant {
  obj := workload({"agent-sandbox.io/grant-attestation": grant_expired}, "1")
  changed := json.patch(obj, [{"op": "add", "path": "/spec/timeout", "value": 3600}])
  count(violation) == 0 with input as update(obj, changed)
}

test_update_new_expired_identity {
  count(violation) == 1 with input as update(identity_workload(id_valid, "1"), identity_workload(id_expired, "1"))
}

test_update_kept_identity_rechecks_workload {
  obj := identity_workload(id_expired, "1")
  count(violation) == 1 with input as update(obj, identity_workload(id_expired, "2"))
}

test_gpu_count_sums_resources {
  obj := json.patch(workload({}, "1"), [{"op": "add", "path": "/spec/resources/-", "value": {"gpu": "2", "replica": 3}}])
  gpu_count(obj) == 7
}

package sandboxworkloadidentity

creator := "system:serviceaccount:agent-sandbox-system:agent-sandbox-controlplane"
issuer := "https://identity.example.test"

jwks := `{"keys":[{"kty":"EC","crv":"P-256","x":"6CGuFJwQpwIJ9X4Vb6jLs9plsGVZqN_N4yhNVuUywaw","y":"lK8cqCW2A6eNavmsZ4mIHnlz9gcm8TH_qr9U-9WvlDo","kid":"k1","alg":"ES256","use":"sig"}]}`

valid_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.0BM5n_HOPhV1EBA_WLGLmqFhEIjs8XVaZD-NB_gp4kuv9-kdNTPoK8m1SBy5-jpsAtmggpDgv2SgVOC7P0Nfzw"
expired_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjEsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.VAjSRzVbbxvRTB37ZnFbpSx8A9RluqEN3o9820myaaa-DBZB2F9chiWD0sH-YhN1bPn6WXSL5-MeHb-S5FQASA"
access_token_typ := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXQrand0In0.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.Doo7k9PtIC69sWWc8NBPPPfSfr7HnZZQlhZw37GhXFihovChabAwLo2pd1X9xecc2hCY4yuo4FvJ9hFb94hWCg"
other_actor_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1vdGhlciJ9LCJpYXQiOjE3MDAwMDAwMDAsImV4cCI6NDEwMjQ0NDgwMCwiYXV0aG9yaXphdGlvbl9kZXRhaWxzIjpbeyJ0eXBlIjoiYWdlbnQtc2FuZGJveC13b3JrbG9hZCIsIndvcmtzcGFjZSI6ImxhYiIsImtpbmQiOiJTYW5kYm94IiwibmFtZSI6InNieC0xIiwiZ3B1cyI6MX1dfQ.RGqH3-mgQUGU_5VVO26OWdenE-VUTZQB79zDNEtkj6xKN7v5gVsUBlmVvkKC9Zs7aQ3xD8-C0d4r3f_3IMnXag"
other_audience_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6Im90aGVyIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.XuGSA_ZX9RJOX2dO7RCn6mfyD1gkBv7B2z8DEPmlhPm1fOFQzyilVk3WlWl8VEPbNUuGmw-eGmC3Mr2qGt-jsA"
other_issuer_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL290aGVyLmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.vpmVbcwQUSE7KITUdr4V4mgQctNBViSEvh_WMISv9iCuGJwuWglLwIjJQ-d7tS8NO8VFjKVxw7_63WUzjobf1w"
other_detail_type_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6Im90aGVyIiwid29ya3NwYWNlIjoibGFiIiwia2luZCI6IlNhbmRib3giLCJuYW1lIjoic2J4LTEiLCJncHVzIjoxfV19.mdwPixbGJtcs55XpscwqftKvCc0g68DlzpDuV3g5UFI5wmhylHabVL1A_KLMd6cRc0-uqPDkQzlRSB4rJz1zaQ"
no_details_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDB9.tZ1EkYI_oV0t7_VZpS0_DVh66deukk2W7B013vVb8wGZlMh5Se-OGyz0uB1x0NtK7zNoZumkJbS7a94kr4c25w"
string_gpus_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOiIxIn1dfQ.VCMLQ6yZlveDCdFiQ4Ci4aeLtdqfpAbMBAEzSOQIxVCcWLBiVRDFsCrZKxBX8dvRvMqWSVDVyR3E5PCp2-cI8A"
no_name_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsImdwdXMiOjF9XX0.tRcbhEDWHgphqzAZR7aje5PFcEAJkyVVkHkBK8ri8u-LOSH9tvL4wnPe8JvYmUyS3QXgp_Aekpg3v0GLOR9JOw"
foreign_key_token := "eyJhbGciOiJFUzI1NiIsImtpZCI6ImsxIiwidHlwIjoiYXNzZXJ0aW9uK2p3dCJ9.eyJpc3MiOiJodHRwczovL2lkZW50aXR5LmV4YW1wbGUudGVzdCIsImF1ZCI6ImFnZW50LXNhbmRib3gtYWRtaXNzaW9uIiwic3ViIjoidXNlci0xIiwiYWN0Ijp7InN1YiI6InN2Yy1jb250cm9scGxhbmUifSwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjQxMDI0NDQ4MDAsImF1dGhvcml6YXRpb25fZGV0YWlscyI6W3sidHlwZSI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJ3b3Jrc3BhY2UiOiJsYWIiLCJraW5kIjoiU2FuZGJveCIsIm5hbWUiOiJzYngtMSIsImdwdXMiOjF9XX0.tl4xChCgX-CIRsWCdhatuAWy4ckPPeCWARuPGHbdcGd8_uJJcYp06eu1G89LjzWjVCAfDYwewgAzq0Jk8lGs0A"

workload(token, workspace) = obj {
  obj := gpu_workload(token, workspace, "1")
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
    "parameters": {
      "sandboxCreator": creator,
      "jwks": jwks,
      "issuer": issuer,
      "audience": "agent-sandbox-admission",
      "tokenType": "assertion+jwt",
      "actor": "svc-controlplane",
      "detailType": "agent-sandbox-workload",
    },
  }
}

test_valid_token {
  count(violation) == 0 with input as review(valid_token, "lab")
}

test_wrong_workspace {
  count(violation) == 1 with input as review(valid_token, "other")
}

test_missing_token {
  count(violation) == 1 with input as review("", "lab")
}

test_expired_token {
  count(violation) == 1 with input as review(expired_token, "lab")
}

test_access_token_type {
  count(violation) == 1 with input as review(access_token_typ, "lab")
}

test_other_actor {
  count(violation) == 1 with input as review(other_actor_token, "lab")
}

test_other_audience {
  count(violation) == 1 with input as review(other_audience_token, "lab")
}

test_other_issuer {
  count(violation) == 1 with input as review(other_issuer_token, "lab")
}

test_foreign_key {
  count(violation) == 1 with input as review(foreign_key_token, "lab")
}

test_other_detail_type {
  count(violation) == 1 with input as review(other_detail_type_token, "lab")
}

test_no_details {
  count(violation) == 1 with input as review(no_details_token, "lab")
}

test_detail_without_name {
  count(violation) == 1 with input as review(no_name_token, "lab")
}

test_string_gpus {
  count(violation) == 1 with input as review(string_gpus_token, "lab")
}

test_other_workload_name {
  count(violation) == 1 with input as wrap(named_workload(valid_token, "lab", "1", "sbx-2"))
}

test_gpu_more_than_asserted {
  count(violation) == 1 with input as wrap(gpu_workload(valid_token, "lab", "2"))
}

test_gpu_fewer_than_asserted {
  count(violation) == 1 with input as wrap(gpu_workload(valid_token, "lab", ""))
}

test_other_user {
  obj := json.patch(workload(valid_token, "lab"), [{"op": "replace", "path": "/metadata/labels/primus-safe.user.id", "value": "user-2"}])
  count(violation) == 1 with input as wrap(obj)
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

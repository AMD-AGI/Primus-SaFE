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

valid_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJwcmltdXMtY2xhdyIsImF1ZCI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJqdGkiOiJnci0xIiwic3ViIjoidXNlci0xIiwid29ya3NwYWNlcyI6WyJsYWIiXSwia2luZHMiOlsiU2FuZGJveCJdLCJtYXhHcHVzUGVyU2FuZGJveCI6MSwiZXhwIjo0MTAyNDQ0ODAwfQ._k6NJwq-R1Vch2L95t39PGPPYQm9aJOcUvhh0-Wy-7jtEX5EbycPHBGL96nh53S4LEB-KUYhXR2xcFh5ecr4lw"
expired_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJwcmltdXMtY2xhdyIsImF1ZCI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJqdGkiOiJnci0xIiwic3ViIjoidXNlci0xIiwid29ya3NwYWNlcyI6WyJsYWIiXSwia2luZHMiOlsiU2FuZGJveCJdLCJtYXhHcHVzUGVyU2FuZGJveCI6MSwiZXhwIjoxfQ.BmEw7C-17IGt_QmzuXv5tpbhcvzVxhxqwAlINXaFFVj5EFQHS_M1Hqt1W0DF294-rpHOb5GXMgol6dDS-jYFvw"
missing_exp_token := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJwcmltdXMtY2xhdyIsImF1ZCI6ImFnZW50LXNhbmRib3gtd29ya2xvYWQiLCJqdGkiOiJnci0xIiwic3ViIjoidXNlci0xIiwid29ya3NwYWNlcyI6WyJsYWIiXSwia2luZHMiOlsiU2FuZGJveCJdLCJtYXhHcHVzUGVyU2FuZGJveCI6MX0.HXiQcRCKg3P3oXfPoO5mE_bZ7aVqAbMo9Ktoi2qPCiMoinJPxZowWuvuADWJvmotw-0qj8FHi0WI7IGR0AgYVQ"

workload(token, gpus) = obj {
  obj := {
    "metadata": {
      "labels": {"primus-safe.user.id": "user-1", "agent-sandbox.io/grant-id": "gr-1"},
      "annotations": {"agent-sandbox.io/grant-attestation": token},
    },
    "spec": {
      "workspace": "lab",
      "groupVersionKind": {"kind": "Sandbox"},
      "resources": [{"gpu": gpus, "replica": 1}],
    },
  }
}

review(token, gpus) = inp {
  inp := {
    "review": {"userInfo": {"username": creator}, "object": workload(token, gpus)},
    "parameters": {"sandboxCreator": creator, "jwks": cert},
  }
}

test_valid_token {
  count(violation) == 0 with input as review(valid_token, "1")
}

test_missing_exp {
  count(violation) == 1 with input as review(missing_exp_token, "1")
}

test_expired_exp {
  count(violation) == 1 with input as review(expired_token, "1")
}

test_gpu_above_max {
  count(violation) == 1 with input as review(valid_token, "2")
}

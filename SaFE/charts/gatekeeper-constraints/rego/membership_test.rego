package sandboxworkloadmembership

creator := "system:serviceaccount:agent-sandbox-system:agent-sandbox-controlplane"

review(user_id, workspace) = inp {
  inp := {
    "review": {
      "userInfo": {"username": creator},
      "object": {"metadata": {"labels": {"primus-safe.user.id": user_id}}, "spec": {"workspace": workspace}},
    },
    "parameters": {"sandboxCreator": creator},
  }
}

inventory(resources) = data_doc {
  data_doc := {"cluster": {"amd.com/v1": {"User": {"user-1": {"spec": {"resources": resources}}}}}}
}

test_member_is_allowed {
  count(violation) == 0
    with input as review("user-1", "lab")
    with data.inventory as inventory({"workspaces": ["lab"]})
}

test_non_member_is_denied {
  count(violation) == 1
    with input as review("user-1", "lab")
    with data.inventory as inventory({"workspaces": ["other"]})
}

test_workspace_field_does_not_grant_membership {
  count(violation) == 1
    with input as review("user-1", "lab")
    with data.inventory as inventory({"workspace": ["lab"]})
}

admin_inventory(roles) = data_doc {
  data_doc := {"cluster": {"amd.com/v1": {"User": {"user-1": {"spec": {"roles": roles, "resources": {"workspaces": ["other"]}}}}}}}
}

test_system_admin_is_allowed_in_any_workspace {
  count(violation) == 0
    with input as review("user-1", "lab")
    with data.inventory as admin_inventory(["system-admin"])
}

test_other_role_is_not_an_exception {
  count(violation) == 1
    with input as review("user-1", "lab")
    with data.inventory as admin_inventory(["system-admin-readonly"])
}

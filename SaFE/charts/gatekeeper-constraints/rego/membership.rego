package sandboxworkloadmembership

violation[{"msg": msg}] {
  input.review.userInfo.username == input.parameters.sandboxCreator
  user_id := object.get(object.get(input.review.object.metadata, "labels", {}), "primus-safe.user.id", "")
  workspace := object.get(input.review.object.spec, "workspace", "")
  not member(user_id, workspace)
  msg := sprintf("user %q is not a member of workspace %q", [user_id, workspace])
}

# member reports whether the synced User lists this workspace.
member(user_id, workspace) {
  user_id != ""
  workspace != ""
  listed(user_id, "workspaces")[_] == workspace
}

# member accepts a system-admin User in every workspace.
member(user_id, workspace) {
  user_id != ""
  workspace != ""
  user := data.inventory.cluster["amd.com/v1"]["User"][user_id]
  object.get(object.get(user, "spec", {}), "roles", [])[_] == "system-admin"
}

# listed reads one array from the User's spec.resources.
listed(user_id, key) = items {
  user := data.inventory.cluster["amd.com/v1"]["User"][user_id]
  spec := object.get(user, "spec", {})
  resources := object.get(spec, "resources", {})
  items := object.get(resources, key, [])
}

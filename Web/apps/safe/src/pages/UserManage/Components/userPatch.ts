import type { EditUserResp, UserSelfData } from '@/services'

export interface UserEditForm {
  email?: string
  roles?: string[]
  workspaces: string[]
  restrictedType?: number
  password: string
}

const sameSet = (a: readonly string[] = [], b: readonly string[] = []): boolean =>
  a.length === b.length && [...a].sort().join('\n') === [...b].sort().join('\n')

/**
 * The PATCH /users/{id} body for an edit: the fields the form changed, and nothing else.
 *
 * What the list row shows is not always what is stored. With an identity provider the workspaces
 * and the restriction of a row reflect what the caller's groups grant, so sending an untouched
 * value back would store it, and the record would then diverge from the provider once the group
 * changes. A field the user did not touch is left out and the server keeps what it has.
 *
 * External mode never sends roles (the server refuses a grant) or a password (there are none).
 */
export function buildUserPatch(
  user: UserSelfData,
  form: UserEditForm,
  external: boolean,
): EditUserResp {
  const patch: EditUserResp = {}
  if ((form.email ?? '') !== (user.email ?? '')) patch.email = form.email ?? ''
  if (!external && !sameSet(form.roles, user.roles)) patch.roles = form.roles ?? []
  if (
    !sameSet(
      form.workspaces,
      user.workspaces?.map((ws) => ws.id),
    )
  )
    patch.workspaces = form.workspaces
  if ((form.restrictedType ?? 0) !== (user.restrictedType ?? 0))
    patch.restrictedType = form.restrictedType ?? 0
  if (form.password && !external) patch.password = form.password
  return patch
}

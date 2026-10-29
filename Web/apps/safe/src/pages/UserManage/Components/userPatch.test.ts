import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import type { UserSelfData } from '@/services'
import { buildUserPatch, type UserEditForm } from './userPatch'

const ws = (id: string) => ({ id, name: id }) as UserSelfData['workspaces'][number]

const user = (over: Partial<UserSelfData> = {}): UserSelfData =>
  ({
    id: 'u1',
    name: 'user-one',
    email: 'one@example.com',
    type: 'default',
    roles: ['default'],
    workspaces: [ws('ws-a'), ws('ws-b')],
    managedWorkspaces: [],
    restrictedType: 0,
    creationTime: '2026-10-01T00:00:00Z',
    ...over,
  }) as UserSelfData

// The form as the dialog fills it from a row, before anyone touches it.
const untouched = (u: UserSelfData): UserEditForm => ({
  email: u.email,
  roles: [...(u.roles ?? [])],
  workspaces: u.workspaces.map((w) => w.id),
  restrictedType: u.restrictedType,
  password: '',
})

describe('buildUserPatch', () => {
  it('sends nothing when nothing was edited', () => {
    expect(buildUserPatch(user(), untouched(user()), false)).toEqual({})
    expect(buildUserPatch(user(), untouched(user()), true)).toEqual({})
  })

  // The row of an identity-provider user lists what its groups grant (an administrator's lists
  // none) and the restriction as reported; sending them back stores them.
  it.each([
    ['groups grant two workspaces', user()],
    ['an administrator row lists no workspaces', user({ workspaces: [], roles: ['system-admin'] })],
    ['a frozen account', user({ restrictedType: 1 })],
  ])('editing only the email leaves workspaces and restriction out (%s)', (_name, u) => {
    for (const external of [true, false]) {
      expect(buildUserPatch(u, { ...untouched(u), email: 'new@example.com' }, external)).toEqual({
        email: 'new@example.com',
      })
    }
  })

  it('sends a field that was changed, and only that field', () => {
    const u = user()
    expect(buildUserPatch(u, { ...untouched(u), workspaces: ['ws-a'] }, true)).toEqual({
      workspaces: ['ws-a'],
    })
    expect(buildUserPatch(u, { ...untouched(u), restrictedType: 1 }, true)).toEqual({
      restrictedType: 1,
    })
    expect(buildUserPatch(u, { ...untouched(u), workspaces: [] }, false)).toEqual({
      workspaces: [],
    })
    expect(buildUserPatch(u, { ...untouched(u), email: '' }, false)).toEqual({ email: '' })
  })

  it('does not call a reordered selection a change', () => {
    const u = user()
    expect(buildUserPatch(u, { ...untouched(u), workspaces: ['ws-b', 'ws-a'] }, false)).toEqual({})
  })

  it('treats a missing email or restriction in the row as empty and normal', () => {
    const u = user({ email: undefined, restrictedType: undefined as unknown as number })
    expect(buildUserPatch(u, { ...untouched(u), restrictedType: 0, email: '' }, false)).toEqual({})
    // The dialog copies the row as it is, so a row without them gives a form without them.
    expect(
      buildUserPatch(u, { ...untouched(u), restrictedType: undefined, email: undefined }, false),
    ).toEqual({})
  })

  it('sends roles and a password in builtin mode when they changed', () => {
    const u = user()
    expect(
      buildUserPatch(
        u,
        { ...untouched(u), roles: ['default', 'system-admin'], password: 'pw' },
        false,
      ),
    ).toEqual({ roles: ['default', 'system-admin'], password: 'pw' })
    expect(buildUserPatch(u, untouched(u), false)).toEqual({})
  })

  it('never sends roles or a password in external mode', () => {
    const u = user()
    expect(
      buildUserPatch(
        u,
        { ...untouched(u), roles: ['default', 'system-admin'], password: 'pw' },
        true,
      ),
    ).toEqual({})
  })
})

describe('the user dialog', () => {
  const source = readFileSync(new URL('./AddDialog.vue', import.meta.url), 'utf-8')

  it('builds its edit body with buildUserPatch, not from the whole form', () => {
    expect(source).toContain('buildUserPatch(props.user, form, external.value)')
    expect(source).not.toContain('...baseEditForm')
  })
})

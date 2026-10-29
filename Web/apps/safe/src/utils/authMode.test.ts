import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { isExternalAuth } from './authMode'

const source = (rel: string) => readFileSync(new URL(`../${rel}`, import.meta.url), 'utf-8')

describe('isExternalAuth', () => {
  it.each([
    ['a server that does not report a mode (older apiserver)', undefined, false],
    ['no envs loaded yet', null, false],
    ['builtin', { authMode: 'builtin' as const }, false],
    ['external', { authMode: 'external' as const }, true],
    ['an unknown value is not external', { authMode: 'ldap' as never }, false],
  ])('%s', (_name, envs, want) => {
    expect(isExternalAuth(envs)).toBe(want)
  })
})

// The console must not offer what the server refuses in external mode, local accounts
// included. Each control below is gated on the store's isExternalAuth.
describe('external mode hides the password, registration, key creation and role controls', () => {
  it('the store derives isExternalAuth from /envs', () => {
    expect(source('stores/user.ts')).toContain('isExternalAuth: (s) => isExternalAuth(s.envs)')
  })

  it('the sign-in page has no password form, not even on the admin route', () => {
    const login = source('pages/Login/Login.vue')
    expect(login).toContain('v-else-if="userStore.isExternalAuth"')
    // the external branch sits before the password form's `v-else`
    expect(login.indexOf('v-else-if="userStore.isExternalAuth"')).toBeLessThan(
      login.indexOf('<el-form'),
    )
  })

  it('the registration page leaves when external', () => {
    expect(source('pages/Login/Register.vue')).toContain(
      "if (userStore.isExternalAuth) router.replace('/login')",
    )
  })

  it('settings hide Change Password only', () => {
    const settings = source('pages/Settings/index.vue')
    expect(settings.match(/<template v-if="!userStore\.isExternalAuth">/g)).toHaveLength(1)
  })

  it('API key creation is gated, but the entry points and the delete action stay', () => {
    // A key issued before the switch still authenticates, so it must stay revocable.
    expect(source('components/layout/BaseMenu.vue')).not.toContain('isExternalAuth')
    expect(source('pages/Settings/index.vue')).toContain("router.push('/manageapikeys')")
    const keys = source('pages/APIKeys/index.vue')
    expect(keys).toContain('const manageable = computed(() => !userStore.isExternalAuth)')
    expect(keys).toContain('v-if="manageable"')
    expect(keys).toContain('<el-table-column label="Actions"')
    expect(source('pages/LLMGateway/index.vue')).toContain('if (userStore.isExternalAuth) return')
  })

  it('the user list has no Create User, and the edit dialog sends neither roles nor password', () => {
    const list = source('pages/UserManage/index.vue')
    expect(list).toContain('const creatable = computed(() => !userStore.isExternalAuth)')
    expect(list).toContain('v-if="creatable"')
    const dialog = source('pages/UserManage/Components/AddDialog.vue')
    // What the body leaves out in external mode is buildUserPatch's, tested in userPatch.test.ts.
    expect(dialog).toContain('buildUserPatch(props.user, form, external.value)')
    expect(dialog).toContain(':disabled="external"')
    expect(dialog).toContain('v-if="!external" label="Password"')
  })
})

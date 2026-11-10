import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { apiKeyRouting, apiKeysProviderUrl, isExternalAuth } from './authMode'

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
describe('external mode hides the password, registration and role controls', () => {
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

  it('API key management opens the identity console', () => {
    expect(source('components/layout/BaseMenu.vue')).toContain('index="/manageapikeys"')
    expect(source('pages/Settings/index.vue')).toContain("router.push('/manageapikeys')")
    expect(source('router/index.ts')).toContain("window.location.assign('/identity/#keys')")
    expect(source('pages/LLMGateway/index.vue')).toContain('if (userStore.isExternalAuth) return')
  })

  it('the user list has no create or delete, and the edit dialog sends neither roles nor password', () => {
    const list = source('pages/UserManage/index.vue')
    expect(list).not.toContain('Create User')
    expect(list).not.toContain('deleteUser')
    const dialog = source('pages/UserManage/Components/AddDialog.vue')
    // What the body leaves out in external mode is buildUserPatch's, tested in userPatch.test.ts.
    expect(dialog).toContain('buildUserPatch(props.user, form, external.value)')
    expect(dialog).toContain('v-if="isEdit && !external" label="Roles"')
    expect(dialog).toContain('v-if="!external" label="Password"')
  })
})

describe('apiKeysProviderUrl', () => {
  it.each([
    [
      'external and routed',
      { authMode: 'external' as const, apiKeysUrl: '/api/v1/apikeys' },
      '/api/v1/apikeys',
    ],
    [
      'a trailing slash is dropped',
      { authMode: 'external' as const, apiKeysUrl: '/api/v1/apikeys/' },
      '/api/v1/apikeys',
    ],
    ['external, nothing routed', { authMode: 'external' as const }, ''],
    ['external, an empty path', { authMode: 'external' as const, apiKeysUrl: '' }, ''],
    ['builtin ignores it', { authMode: 'builtin' as const, apiKeysUrl: '/api/v1/apikeys' }, ''],
    ['no mode (an older apiserver) ignores it', { apiKeysUrl: '/api/v1/apikeys' }, ''],
    [
      'an absolute URL is not a path on this origin',
      { authMode: 'external' as const, apiKeysUrl: 'https://idp.example.com/api/v1/apikeys' },
      '',
    ],
    [
      'a protocol relative URL',
      { authMode: 'external' as const, apiKeysUrl: '//idp.example.com/api/v1/apikeys' },
      '',
    ],
    ['no leading slash', { authMode: 'external' as const, apiKeysUrl: 'api/v1/apikeys' }, ''],
    ['a query', { authMode: 'external' as const, apiKeysUrl: '/api/v1/apikeys?x=1' }, ''],
    ['a backslash', { authMode: 'external' as const, apiKeysUrl: '/\\idp.example.com' }, ''],
    ['no envs yet', null, ''],
  ])('%s', (_name, envs, want) => {
    expect(apiKeysProviderUrl(envs)).toBe(want)
  })
})

describe('apiKeyRouting', () => {
  const routed = { authMode: 'external' as const, apiKeysUrl: '/api/v1/apikeys' }
  it.each([
    [
      'builtin: the apiserver lists and creates, nothing legacy',
      { authMode: 'builtin' as const },
      { providerUrl: '', creatable: true, legacy: false },
    ],
    ['no mode: as builtin', {}, { providerUrl: '', creatable: true, legacy: false }],
    [
      'external, no route: the apiserver lists, nothing can be created',
      { authMode: 'external' as const },
      { providerUrl: '', creatable: false, legacy: false },
    ],
    [
      'external, routed: the provider lists and creates, the apiserver keys stay listed and revocable',
      routed,
      { providerUrl: '/api/v1/apikeys', creatable: true, legacy: true },
    ],
    [
      'builtin, a route reported: ignored',
      { authMode: 'builtin' as const, apiKeysUrl: '/api/v1/apikeys' },
      { providerUrl: '', creatable: true, legacy: false },
    ],
  ])('%s', (_name, envs, want) => {
    expect(apiKeyRouting(envs)).toEqual(want)
  })
})

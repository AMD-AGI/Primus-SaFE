import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createSSRApp, h, type SetupContext } from 'vue'
import { renderToString } from 'vue/server-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { EnvsResp, UserSelfData } from '@/services/login/type'

vi.hoisted(() => {
  const storage = new Map<string, string>()
  globalThis.localStorage = {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => storage.set(key, value),
    removeItem: (key: string) => storage.delete(key),
    clear: () => storage.clear(),
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    get length() {
      return storage.size
    },
  } as Storage
})

// This renders the real page template to a string (no DOM is needed), with every service mocked.
// What happens when a button is pressed is tested in useNoAccess.test.ts.
const services = {
  getEnvs: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  getUserData: vi.fn(),
  getSelfData: vi.fn(),
  getSelfWorkspaces: vi.fn(),
  getWorkspace: vi.fn(),
  getClusters: vi.fn(),
  copyText: vi.fn(),
}
vi.mock('element-plus', () => ({ ElMessage: () => {}, ElMessageBox: {} }))
vi.mock('@/services/login', () => ({
  getEnvs: (...a: unknown[]) => services.getEnvs(...a),
  login: (...a: unknown[]) => services.login(...a),
  logout: (...a: unknown[]) => services.logout(...a),
  getUserData: (...a: unknown[]) => services.getUserData(...a),
  getSelfData: (...a: unknown[]) => services.getSelfData(...a),
  getSelfWorkspaces: (...a: unknown[]) => services.getSelfWorkspaces(...a),
}))
vi.mock('@/services/base', () => ({
  getWorkspace: (...a: unknown[]) => services.getWorkspace(...a),
  getClusters: (...a: unknown[]) => services.getClusters(...a),
}))
vi.mock('@/utils', () => ({ copyText: (...a: unknown[]) => services.copyText(...a) }))

import { useUserStore } from '@/stores/user'
import NoAccessPage from './NoAccessPage.vue'

const profile: UserSelfData = {
  id: 'u-123',
  name: 'Ada',
  email: 'ada@example.com',
  type: 'default',
  roles: ['first-login', 'viewer'],
  workspaces: [],
  creationTime: '',
  restrictedType: 0,
}

// Stands in for Element Plus's button, which the console registers globally.
const ElButton = {
  setup:
    (_: unknown, { slots }: SetupContext) =>
    () =>
      h('button', slots.default?.()),
}

async function render(opts: {
  profile?: Record<string, unknown> | null
  envs?: Record<string, unknown> | null
}) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const user = useUserStore()
  user.$patch({ session: 'authenticated', userId: 'u-123' })
  user.profile = (opts.profile === undefined ? profile : opts.profile) as UserSelfData | null
  user.envs = (opts.envs === undefined ? {} : opts.envs) as unknown as EnvsResp | null

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/no-access', component: NoAccessPage }],
  })
  await router.push('/no-access')
  const app = createSSRApp(NoAccessPage).use(pinia).use(router)
  app.component('ElButton', ElButton)
  return renderToString(app)
}

beforeEach(() => {
  for (const fn of Object.values(services)) fn.mockReset()
})

describe('no access page', () => {
  it('shows the title, the account and the two buttons', async () => {
    const html = await render({})
    expect(html).toContain('No access yet')
    expect(html).toContain('Ada')
    expect(html).toContain('ada@example.com')
    expect(html).toContain('u-123')
    expect(html).toContain('first-login, viewer')
    expect(html).toContain('Copy ID')
    expect(html).toContain('Check again')
    expect(html).toContain('Sign out')
  })

  it('shows the contact line from /envs as text', async () => {
    const html = await render({
      envs: { accessContact: 'Ask the platform team at team@example.com' },
    })
    expect(html).toContain('class="contact"')
    expect(html).toContain('Ask the platform team at team@example.com')
  })

  it.each([
    ['markup', '<img src=x onerror=alert(1)>', '&lt;img src=x onerror=alert(1)&gt;'],
    ['a script', '<script>alert(1)</script>', '&lt;script&gt;alert(1)&lt;/script&gt;'],
    ['a quote', 'say "hi" & wait', 'say &quot;hi&quot; &amp; wait'],
  ])('shows %s in the contact line escaped, never as HTML', async (_label, contact, escaped) => {
    const html = await render({ envs: { accessContact: contact } })
    expect(html).toContain(escaped)
    expect(html).not.toContain(contact)
    expect(html).not.toContain('<img')
    expect(html).not.toContain('<script')
  })

  it('shows the account fields escaped too', async () => {
    const html = await render({
      profile: { ...profile, name: '<i>Ada</i>', email: 'a"b@example.com' },
    })
    expect(html).toContain('&lt;i&gt;Ada&lt;/i&gt;')
    expect(html).not.toContain('<i>')
    expect(html).toContain('a&quot;b@example.com')
  })

  it.each([
    ['an empty contact line', { accessContact: '' }],
    ['a blank contact line', { accessContact: '   ' }],
    ['no contact field (older server)', {}],
    ['no /envs answer at all', null],
  ])('shows nothing extra for %s', async (_label, envs) => {
    const html = await render({ envs })
    expect(html).not.toContain('class="contact"')
    expect(html).not.toContain('class="notice"')
  })

  it('has no copy button, and shows dashes, when the record is missing', async () => {
    const html = await render({ profile: null })
    expect(html).not.toContain('Copy ID')
    // Name, email and roles; the id's dash sits in its own span.
    expect(html.match(/<dd[^>]*>-<\/dd>/g)).toHaveLength(3)
    expect(html).toMatch(/<span class="mono"[^>]*>-<\/span>/)
    expect(html).toContain('No access yet')
  })

  it('makes no request while it renders', async () => {
    await render({ envs: { accessContact: 'x' } })
    for (const [name, fn] of Object.entries(services)) {
      expect(fn, name).not.toHaveBeenCalled()
    }
  })
})

import { defineStore } from 'pinia'
import { login, getUserData, getSelfData, logout, getEnvs } from '@/services/login'
import type { LoginReq, UserSelfData, EnvsResp } from '@/services'
import { isExternalAuth } from '@/utils/authMode'
import { useWorkspaceStore } from './workspace'
import { applySelfWorkspaces, loadSelfWorkspaces } from './selfWorkspaces'

const QUICK_START_VERSION = 'v1'

type SessionState = 'unknown' | 'anonymous' | 'authenticated'

type OnboardingSeen = {
  quickStartSeenVersion?: string // last seen version (admin)
  userQuickStartSeenVersion?: string // last seen version (regular user)
  lastSeenAt?: string // ISO timestamp for debugging
}
type SeenByUser = Record<string, OnboardingSeen>

export const useUserStore = defineStore('user', {
  state: () => ({
    session: 'unknown' as SessionState,
    userId: '',
    profile: null as UserSelfData | null,
    _initPromise: null as Promise<void> | null,
    _profileFetched: false,
    envs: null as EnvsResp | null,
    // Onboarding related state
    onboarding: {
      quickStartVersion: QUICK_START_VERSION, // current onboarding version
      seenByUser: {} as SeenByUser, // per-user records
    },
  }),

  // pinia-plugin-persistedstate v4 reads `pick`; the v2 name `paths` is ignored, which saved the
  // whole state, `_initPromise` included. A promise is written as `{}` and read back as a truthy
  // value, so after the first visit ensureSessionOnce returned at once on every load and the
  // session was never asked about again (see user.persist.test.ts).
  persist: {
    key: 'user',
    storage: localStorage,
    pick: ['session', 'userId', 'profile', 'onboarding'],
  } as any,

  getters: {
    isLogin: (s) => s.session === 'authenticated',
    // Check if user role is admin (includes regular admin and read-only admin)
    isManager: (s) => s.profile?.roles?.includes('system-admin') ?? false,
    // Check if user is a read-only admin
    isReadonlyManager: (s) => s.profile?.roles?.includes('system-admin-readonly') ?? false,
    // Check if user has admin access (regular or read-only)
    hasManagerAccess: (s) =>
      (s.profile?.roles?.includes('system-admin') ?? false) ||
      (s.profile?.roles?.includes('system-admin-readonly') ?? false),
    displayRole: (s) => {
      const wsStore = useWorkspaceStore()
      if (s.profile?.roles?.includes('system-admin')) return 'system-admin'
      if (s.profile?.roles?.includes('system-admin-readonly')) return 'system-admin-readonly'
      if (wsStore.isCurrentWorkspaceAdmin()) return 'workspace-admin'
      // Not either type of admin: use the first role from user profile
      return s.profile?.roles?.[0] || 'user'
    },
    shouldAutoShowQuickStart(state): boolean {
      // Whether to show onboarding (only for regular system-admin, not readonly)
      const uid = state.userId || 'anon'
      const seenVer = state.onboarding.seenByUser[uid]?.quickStartSeenVersion
      return (
        state.session === 'authenticated' &&
        seenVer !== state.onboarding.quickStartVersion &&
        (state.profile?.roles?.includes('system-admin') ?? false)
      )
    },
    shouldAutoShowUserQuickStart(state): boolean {
      // Onboarding for regular users (non-admin)
      const uid = state.userId || 'anon'
      const seenVer = state.onboarding.seenByUser[uid]?.userQuickStartSeenVersion
      const isAdmin =
        (state.profile?.roles?.includes('system-admin') ?? false) ||
        (state.profile?.roles?.includes('system-admin-readonly') ?? false)
      return state.session === 'authenticated' && seenVer !== state.onboarding.quickStartVersion && !isAdmin
    },
    // An identity provider owns sign-in, accounts, API keys and platform roles
    isExternalAuth: (s) => isExternalAuth(s.envs),
    // CD approval config: whether approval is required (defaults to true)
    cdRequireApproval: (s) => s.envs?.cdRequireApproval ?? true,
  },
  actions: {
    async fetchEnvs(quiet = false) {
      this.envs = await getEnvs(quiet)
    },

    /**
     * Reset what is scoped to a signed-in user, so that nothing of the previous account shows.
     */
    async resetScopedStores() {
      const { useClusterStore } = await import('./cluster')
      const { useWorkspaceStore } = await import('./workspace')
      useClusterStore().$reset()
      useWorkspaceStore().$reset()
    },

    /**
     * With an identity provider the browser's session is not made by POST /login (the server
     * refuses it) and the provider, not this console, decides who the caller is: ask the server,
     * never a userId saved by an earlier visit. If the answer is a different user from the saved
     * one (another account signed in at the provider in the same browser), nothing of the
     * previous account is kept: the cluster and workspace selections are reset, as login() does.
     * Returns whether there is a session now. A 401 just means nobody is signed in.
     */
    async adoptExternalSession(): Promise<boolean> {
      try {
        const record = await getSelfData()
        if (!record?.id) return false
        const lists = await loadSelfWorkspaces()
        const changed = record.id !== this.userId
        this.userId = record.id
        this.profile = applySelfWorkspaces(record, lists)
        this._profileFetched = true
        this.session = 'authenticated'
        if (changed) await this.resetScopedStores()
        return true
      } catch {
        // Not signed in (401), or the server did not answer: anonymous, as before.
        return false
      }
    },

    async login(payload: LoginReq) {
      const loginRes = await login(payload)
      if (!loginRes?.id) throw new Error('INVALID_RESPONSE')

      this.userId = loginRes?.id || ''

      await this.fetchUser()
      this.session = 'authenticated'

      // Reset cluster & workspace on login to avoid stale data when switching environments
      await this.resetScopedStores()
    },

    async fetchUser(force = false) {
      if (!this.userId) {
        throw new Error('missing userId')
      }
      if (!force && this._profileFetched && this.profile) return

      // The workspace lists come from their own route; the user record's embedded lists
      // are the fallback (see selfWorkspaces.ts).
      const [record, lists] = await Promise.all([getUserData(this.userId), loadSelfWorkspaces()])
      this.profile = applySelfWorkspaces(record, lists)
      this._profileFetched = true
      this.session = 'authenticated'
    },

    async ensureSessionOnce() {
      if (this._initPromise) return this._initPromise
      this._initPromise = (async () => {
        // The auth mode decides how the caller is resolved, so /envs comes first. Awaited: what
        // the console offers depends on it (see isExternalAuth), so a reload must not render the
        // builtin-mode UI first. A failed /envs is not a failed session: the mode stays unknown
        // and a saved userId is resolved as in builtin mode. A visitor's failure is quiet.
        await this.fetchEnvs(!this.userId).catch(() => {})

        if (this.isExternalAuth) {
          // The identity provider owns who the caller is: a userId saved by an earlier visit is
          // not that (another account may have signed in since), so always ask /users/self.
          if (await this.adoptExternalSession()) return
          this.session = 'anonymous'
          this.profile = null
          this.userId = ''
          this._profileFetched = true
          return
        }

        if (!this.userId) {
          // Builtin mode: its only way in is POST /login, which sets userId itself.
          this.session = 'anonymous'
          this._profileFetched = true
          return
        }

        try {
          await this.fetchUser()
        } catch {
          this.session = 'anonymous'
          this.profile = null
          this.userId = ''
          this._profileFetched = true
        }
      })()
      return this._initPromise
    },

    async logout() {
      try {
        await logout()
      } finally {
        this.$patch({
          session: 'anonymous',
          userId: '',
          profile: null,
          _profileFetched: false,
          _initPromise: null,
          envs: null,
        })
        // Reset cluster & workspace after logout to avoid data leakage between accounts
        await this.resetScopedStores()
      }
    },

    markQuickStartSeen() {
      // Mark admin user as having seen the onboarding
      const uid = this.userId || 'anon'
      this.onboarding.seenByUser[uid] = {
        ...this.onboarding.seenByUser[uid],
        quickStartSeenVersion: this.onboarding.quickStartVersion,
        lastSeenAt: new Date().toISOString(),
      }
    },
    markUserQuickStartSeen() {
      // Mark regular user as having seen the onboarding
      const uid = this.userId || 'anon'
      this.onboarding.seenByUser[uid] = {
        ...this.onboarding.seenByUser[uid],
        userQuickStartSeenVersion: this.onboarding.quickStartVersion,
        lastSeenAt: new Date().toISOString(),
      }
    },
  },
})

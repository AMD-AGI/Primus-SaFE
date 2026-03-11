import { defineStore } from 'pinia'
import { login, getUserData, logout, getEnvs } from '@/services/login'
import type { LoginReq, UserSelfData, EnvsResp } from '@/services'
import { useWorkspaceStore } from './workspace'

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

  persist: {
    key: 'user',
    storage: localStorage,
    paths: ['session', 'userId', 'profile', 'onboarding'],
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
    // CD approval config: whether approval is required (defaults to true)
    cdRequireApproval: (s) => s.envs?.cdRequireApproval ?? true,
  },
  actions: {
    async fetchEnvs() {
      this.envs = await getEnvs()
    },

    async login(payload: LoginReq) {
      const loginRes = await login(payload)
      if (!loginRes?.id) throw new Error('INVALID_RESPONSE')

      this.userId = loginRes?.id || ''

      await this.fetchUser()
      this.session = 'authenticated'

      // Reset cluster & workspace on login to avoid stale data when switching environments
      const { useClusterStore } = await import('./cluster')
      const { useWorkspaceStore } = await import('./workspace')
      const clusterStore = useClusterStore()
      const wsStore = useWorkspaceStore()
      clusterStore.$reset()
      wsStore.$reset()
    },

    async fetchUser(force = false) {
      if (!this.userId) {
        throw new Error('missing userId')
      }
      if (!force && this._profileFetched && this.profile) return

      this.profile = await getUserData(this.userId)
      this._profileFetched = true
      this.session = 'authenticated'
    },

    async ensureSessionOnce() {
      if (this._initPromise) return this._initPromise
      this._initPromise = (async () => {
        if (!this.userId) {
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
        const { useClusterStore } = await import('./cluster')
        const { useWorkspaceStore } = await import('./workspace')
        const clusterStore = useClusterStore()
        const wsStore = useWorkspaceStore()
        clusterStore.$reset()
        wsStore.$reset()
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

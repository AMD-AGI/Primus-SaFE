import { useClusterStore } from '@/stores/cluster'
import axios from 'axios'
import { useWorkspaceStore } from '@/stores/workspace'
import type { Router } from 'vue-router'
import { useUserStore } from '@/stores/user'

const statusOf = (e: unknown) => (axios.isAxiosError(e) ? e.response?.status : undefined)

const SKIP_ROUTES = new Set(['/login', '/register', '/error', '/login-admin', '/sso-error'])

export default function setupClusterGuard(router: Router) {
  router.beforeEach(async (to, from, next) => {
    if (SKIP_ROUTES.has(to.path)) return next()

    const clusterStore = useClusterStore()
    const wsStore = useWorkspaceStore()
    const userStore = useUserStore()

    try {
      if (!userStore.isLogin) {
        if (to.path !== '/login') return next('/login')
        return next()
      }

      if (!clusterStore.isFetched) {
        await clusterStore.fetchClusters()
      }

      // Admins are allowed to proceed even when no cluster is available
      if (!clusterStore.currentClusterId && !userStore.hasManagerAccess) {
        if (to.path !== '/error') return next('/error')
        return next()
      }

      // Only non-admin users pass clusterId (same for other list APIs)
      await wsStore.fetchWorkspace()

      const isReady = clusterStore.isReady

      if (!isReady && !userStore.hasManagerAccess && to.path !== '/error') {
        return next('/error')
      }

      if (isReady && to.path === '/error') {
        return next('/')
      }

      // Permission check (workspace-admin)
      if (to.meta?.requiresWorkspaceAdmin) {
        if (!wsStore.isCurrentWorkspaceAdmin() && !userStore.hasManagerAccess) {
          // TODO: redirect to 403 page instead of homepage
          return next('/')
        }
      }

      return next()
    } catch (e) {
      if (statusOf(e) === 401) {
        // Session expired: clear local state and redirect to login (instead of silently cancelling navigation)
        console.warn('[ClusterGuard] 401 detected, session expired. Redirecting to login.')
        userStore.$patch({
          session: 'anonymous',
          userId: '',
          profile: null,
          _profileFetched: false,
          _initPromise: null,
        })
        clusterStore.$reset()
        wsStore.$reset()
        const redirect = encodeURIComponent(to.fullPath || '/')
        return next({ path: '/login', query: { redirect } })
      }

      if (to.path !== '/error') {
        return next('/error')
      }
      return next()
    }
  })
}

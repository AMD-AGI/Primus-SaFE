import { useClusterStore } from '@/stores/cluster'
import { isNoAccess, statusOf } from '@/utils/httpStatus'
import { useWorkspaceStore } from '@/stores/workspace'
import type { Router } from 'vue-router'
import { useUserStore } from '@/stores/user'

// /no-access is skipped so that landing on it does not fetch the clusters again. It is not a
// public route: the auth guard still requires a session for it.
const SKIP_ROUTES = new Set([
  '/login',
  '/register',
  '/error',
  '/403',
  '/login-admin',
  '/sso-error',
  '/no-access',
])

export default function setupClusterGuard(router: Router) {
  router.beforeEach(async (to, from, next) => {
    if (SKIP_ROUTES.has(to.path)) return next()

    const clusterStore = useClusterStore()
    const wsStore = useWorkspaceStore()
    const userStore = useUserStore()
    // The workspace access pages need neither a cluster nor a workspace: a signed-in user who has
    // no access yet opens them to ask for it.
    const accessPage = to.meta?.allowNoAccess === true

    try {
      if (!userStore.isLogin) {
        if (to.path !== '/login') return next('/login')
        return next()
      }

      if (!clusterStore.isFetched) {
        try {
          // Quiet: a refusal is read below, not shown as a toast.
          await clusterStore.fetchClusters(true)
        } catch (e) {
          // An account that holds only the first-login role may read nothing but its own record,
          // so the platform refuses here with a 403 that says "no access yet" (its own error
          // code). That is not a fault: say so, from whatever page was asked for. Any other
          // failure, including a different 403 (a role check that failed for a user who does
          // hold other access), is handled as before.
          if (isNoAccess(e)) return accessPage ? next() : next('/no-access')
          throw e
        }
      }

      if (accessPage) {
        // Loaded for the workspace selector of the layout; a user without a cluster has none.
        if (clusterStore.currentClusterId) await wsStore.fetchWorkspace()
        return next()
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

      // Permission check (system-admin only pages)
      // These System pages are hidden from the nav for non-admins; also block
      // direct URL navigation so access control is not merely client-side.
      if (to.meta?.requiresSystemAdmin && !userStore.hasManagerAccess) {
        return next('/')
      }

      if (to.meta?.requiresSystemAdminWrite && !userStore.isManager) {
        return next('/')
      }

      // Permission check (workspace-admin)
      if (to.meta?.requiresWorkspaceAdmin) {
        if (!wsStore.isCurrentWorkspaceAdmin() && !userStore.hasManagerAccess) {
          return next('/403')
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
        // The router encodes the query value itself.
        return next({ path: '/login', query: { redirect: to.fullPath || '/' } })
      }

      if (to.path !== '/error') {
        return next('/error')
      }
      return next()
    }
  })
}

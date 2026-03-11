import { onMounted, ref } from 'vue'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'

// Auto-refresh interval (milliseconds)
const REFRESH_INTERVAL = 10 * 60 * 1000 // 10 minutes
const LAST_REFRESH_KEY = 'last_user_refresh'

export function useAutoRefreshUserInfo(
  options: {
    immediate?: boolean // Whether to refresh immediately
    interval?: boolean // Whether to enable scheduled refresh
  } = {},
) {
  const userStore = useUserStore()
  const wsStore = useWorkspaceStore()
  const isRefreshing = ref(false)

  // Check if refresh is needed (based on last refresh time)
  const shouldRefresh = () => {
    const lastRefresh = localStorage.getItem(LAST_REFRESH_KEY)
    if (!lastRefresh) return true

    const elapsed = Date.now() - parseInt(lastRefresh)
    return elapsed > REFRESH_INTERVAL
  }

  // Refresh user info
  const refreshUserInfo = async (force = false) => {
    if (!userStore.userId || isRefreshing.value) return

    if (!force && !shouldRefresh()) return

    try {
      isRefreshing.value = true
      await userStore.fetchUser(true)

      // Also refresh workspace info
      if (wsStore.currentWorkspaceId) {
        await wsStore.fetchWorkspace(true)
      }

      localStorage.setItem(LAST_REFRESH_KEY, Date.now().toString())
    } catch (error) {
      console.error('Failed to refresh user info:', error)
    } finally {
      isRefreshing.value = false
    }
  }

  onMounted(() => {
    // Refresh immediately
    if (options.immediate) {
      refreshUserInfo()
    }

    // Enable scheduled refresh
    if (options.interval) {
      const timer = setInterval(() => {
        refreshUserInfo()
      }, REFRESH_INTERVAL)

      // Clear timer on component unmount
      return () => clearInterval(timer)
    }
  })

  return {
    isRefreshing,
    refreshUserInfo,
  }
}

import { watch, onMounted, computed } from 'vue'
import { useRoute, useRouter, LocationQuery } from 'vue-router'
import { useGlobalCluster } from './useGlobalCluster'

/**
 * Composable to sync cluster between URL query params and global state
 * This ensures that cluster is always present in URL for sharing
 */
export function useClusterSync() {
  const route = useRoute()
  const router = useRouter()
  const { selectedCluster, setCluster, initCluster } = useGlobalCluster()

  // Get cluster from URL query
  const urlCluster = computed(() => route.query.cluster as string || '')

  // Sync cluster from URL to global state on mount
  const syncFromUrl = () => {
    if (urlCluster.value && urlCluster.value !== selectedCluster.value) {
      setCluster(urlCluster.value)
    }
  }

  // Update URL with current cluster
  const updateUrlWithCluster = (cluster?: string) => {
    const targetCluster = cluster || selectedCluster.value
    if (!targetCluster) return

    // Only update if cluster is different
    if (route.query.cluster !== targetCluster) {
      router.replace({
        ...route,
        query: {
          ...route.query,
          cluster: targetCluster
        }
      })
    }
  }

  // Ensure cluster is in URL when navigating
  const navigateWithCluster = (to: any) => {
    const cluster = selectedCluster.value
    if (!cluster) {
      return router.push(to)
    }

    // Ensure cluster is in query params
    if (typeof to === 'string') {
      const separator = to.includes('?') ? '&' : '?'
      return router.push(`${to}${separator}cluster=${encodeURIComponent(cluster)}`)
    } else {
      return router.push({
        ...to,
        query: {
          ...to.query,
          cluster
        }
      })
    }
  }

  // Build query params with cluster
  const buildQueryWithCluster = (query?: LocationQuery): LocationQuery => {
    const cluster = selectedCluster.value
    return {
      ...query,
      ...(cluster ? { cluster } : {})
    }
  }

  // Initialize on mount
  onMounted(() => {
    // First init from localStorage
    initCluster()
    
    // Then sync from URL if present (but don't update URL)
    if (urlCluster.value && urlCluster.value !== selectedCluster.value) {
      setCluster(urlCluster.value)
    }
  })

  // Watch for route changes and sync cluster
  watch(() => route.query.cluster, (newCluster) => {
    if (newCluster && newCluster !== selectedCluster.value) {
      setCluster(newCluster as string)
    }
  })

  return {
    selectedCluster,
    urlCluster,
    setCluster,
    syncFromUrl,
    updateUrlWithCluster,
    navigateWithCluster,
    buildQueryWithCluster
  }
}

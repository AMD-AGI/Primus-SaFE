import { ref, watch } from 'vue'

// Initialize cluster from URL or localStorage at module load time
// This ensures cluster is available before any component mounts
const getInitialCluster = (): string => {
  if (typeof window !== 'undefined') {
    // Priority 1: Try to get from URL query params
    const urlParams = new URLSearchParams(window.location.search)
    const urlCluster = urlParams.get('cluster')
    if (urlCluster) {
      // Also save to localStorage for future use
      localStorage.setItem('selectedCluster', urlCluster)
      return urlCluster
    }
    
    // Priority 2: Try to get from localStorage
    if (window.localStorage) {
      return localStorage.getItem('selectedCluster') || ''
    }
  }
  return ''
}

// Global cluster state - initialized from localStorage immediately
const selectedCluster = ref<string>(getInitialCluster())
const clusterOptions = ref<string[]>([])

export function useGlobalCluster() {
  const setCluster = (cluster: string) => {
    selectedCluster.value = cluster
    // Save to localStorage for persistence
    localStorage.setItem('selectedCluster', cluster)
  }

  const setClusters = (clusters: string[]) => {
    clusterOptions.value = clusters
    // If no cluster selected and we have options, select the last one
    if (!selectedCluster.value && clusters.length > 0) {
      setCluster(clusters[clusters.length - 1])
    }
  }

  const initCluster = () => {
    // Try to restore from localStorage (kept for backward compatibility)
    // Note: This is now mostly redundant as we initialize at module load time
    const savedCluster = localStorage.getItem('selectedCluster')
    if (savedCluster && !selectedCluster.value) {
      selectedCluster.value = savedCluster
    }
  }

  return {
    selectedCluster,
    clusterOptions,
    setCluster,
    setClusters,
    initCluster,
  }
}


import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

export const useClusterStore = defineStore('cluster', () => {
  // State
  const currentCluster = ref<string | null>(localStorage.getItem('selectedCluster') || null)
  const clusters = ref<string[]>([])
  const loading = ref(false)

  // Actions
  function setCurrentCluster(cluster: string | null) {
    currentCluster.value = cluster
    if (cluster) {
      localStorage.setItem('selectedCluster', cluster)
    } else {
      localStorage.removeItem('selectedCluster')
    }
  }

  function setClusters(clusterList: string[]) {
    clusters.value = clusterList
    // If current cluster not in list, reset it
    if (currentCluster.value && !clusterList.includes(currentCluster.value)) {
      setCurrentCluster(clusterList[0] || null)
    }
  }

  function addCluster(cluster: string) {
    if (!clusters.value.includes(cluster)) {
      clusters.value.push(cluster)
    }
  }

  // Computed
  const hasSelectedCluster = computed(() => !!currentCluster.value)
  const clusterCount = computed(() => clusters.value.length)

  return {
    // State
    currentCluster,
    clusters,
    loading,
    // Actions
    setCurrentCluster,
    setClusters,
    addCluster,
    // Computed
    hasSelectedCluster,
    clusterCount
  }
})

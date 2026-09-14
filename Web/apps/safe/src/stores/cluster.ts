import { defineStore } from 'pinia'
import { getClusters } from '@/services/base'
import type { ClusterItem } from '@/services'
import { isUsableClusterPhase } from './clusterPhase'

export interface ClusterState {
  totalCount: number
  items: ClusterItem[]
  isReady: boolean
  isFetched: boolean
  currentClusterId?: string
}

export const useClusterStore = defineStore('cluster', {
  state: (): ClusterState => ({
    totalCount: 0,
    items: [],
    isReady: false,
    isFetched: false,
    currentClusterId: '',
  }),

  persist: {
    key: 'cluster',
    storage: localStorage,
    paths: ['currentClusterId'],
  } as any,

  actions: {
    async fetchClusters() {
      try {
        const res = await getClusters()
        this.items = res.items || []
        this.totalCount = res.totalCount

        // If a persisted currentClusterId exists, check if it is still valid
        const persistedCluster = this.items.find((i) => i.clusterId === this.currentClusterId)

        // Keep clusters whose data plane remains available during the upgrade lifecycle.
        if (persistedCluster && isUsableClusterPhase(persistedCluster.phase)) {
          this.isReady = true
        } else {
          const readyItem = this.items.find((i) => isUsableClusterPhase(i?.phase))
          this.currentClusterId = readyItem?.clusterId ?? ''
          this.isReady = !!readyItem
        }

        this.isFetched = true
      } catch (err) {
        this.isReady = false
        this.isFetched = false
        this.currentClusterId = ''
        this.items = []
        this.totalCount = 0
        throw err
      }
    },
    // Update current cluster ID
    setCurrentCluster(clusterId: string) {
      this.currentClusterId = clusterId
    },
  },

  getters: {
    firstCluster(state): ClusterItem | null {
      return state.items[0] ?? null
    },
  },
})

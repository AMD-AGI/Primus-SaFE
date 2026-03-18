import { defineStore } from 'pinia'
import { getWorkspace } from '@/services/base'
import type { WorkspaceItem } from '@/services'
import type { ScopesKeys } from '@/services/base/type'
import { useUserStore } from './user'
import { useClusterStore } from './cluster'

export interface WorkspaceState {
  totalCount: number
  items: WorkspaceItem[]
  isFetched: boolean
  currentWorkspaceId?: string
}

export const useWorkspaceStore = defineStore('workspace', {
  state: (): WorkspaceState => ({
    totalCount: 0,
    items: [],
    isFetched: false,
    currentWorkspaceId: undefined,
  }),

  persist: {
    key: 'workspace',
    storage: localStorage,
    paths: ['currentWorkspaceId'],
  } as any,

  actions: {
    async fetchWorkspace(force = false) {
      try {
        if (this.isFetched && !force) {
          // Even if already fetched, update cluster once (needed on page refresh)
          this.updateClusterFromWorkspace()
          return
        }

        const res = await getWorkspace()

        this.items = res.items || []

        this.totalCount = res.totalCount
        this.isFetched = true

        const exists = this.items.some((i) => i.workspaceId === this.currentWorkspaceId)
        if (!exists) {
          this.currentWorkspaceId = this.firstWorkspace || undefined
        }

        // Automatically update cluster ID
        this.updateClusterFromWorkspace()
      } catch (err) {
        throw err
      }
    },
    async setCurrentWorkspace(id: string) {
      this.currentWorkspaceId = id
      // Automatically update cluster when switching workspace
      this.updateClusterFromWorkspace()
      // Force refresh workspace data when switching to ensure permission info is up-to-date
      await this.fetchWorkspace(true)
    },
    // Update cluster ID based on current workspace
    updateClusterFromWorkspace() {
      const currentWs = this.items.find((item) => item.workspaceId === this.currentWorkspaceId)
      if (currentWs?.clusterId) {
        const clusterStore = useClusterStore()
        clusterStore.setCurrentCluster(currentWs.clusterId)
      }
    },
  },

  getters: {
    firstWorkspace(state): string | null {
      return state.items[0]?.workspaceId ?? null
    },
    currentNodeFlavor(state): string | undefined {
      return state.items?.find((item) => item.workspaceId === state.currentWorkspaceId)?.flavorId
    },
    currentScopes(state): ScopesKeys[] | undefined {
      return state.items?.find((item) => item.workspaceId === state.currentWorkspaceId)?.scopes
    },
    totalNodeNum(state): number | undefined {
      return state.items?.find((item) => item.workspaceId === state.currentWorkspaceId)
        ?.currentNodeCount
    },
    isCurrentWorkspaceAdmin: (s) => {
      return (): boolean => {
        const userStore = useUserStore()
        const currentId = s.currentWorkspaceId
        if (!currentId) return false

        const managed = userStore.profile?.managedWorkspaces ?? []
        if (!Array.isArray(managed) || managed.length === 0) return false

        return managed.some((w) => w.id === currentId)
      }
    },
  },
})

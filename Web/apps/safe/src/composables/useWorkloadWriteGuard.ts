import { computed } from 'vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'

/**
 * When the current workspace has "Sandbox" scope, only system-admin
 * (not system-admin-readonly / workspace-admin) retains write access.
 * All other users become read-only on workload pages.
 */
export function useWorkloadWriteGuard() {
  const wsStore = useWorkspaceStore()
  const userStore = useUserStore()

  const isSandboxWorkspace = computed(() =>
    (wsStore.currentScopes ?? []).includes('Sandbox'),
  )

  const canWrite = computed(() =>
    !isSandboxWorkspace.value || userStore.isManager,
  )

  return { isSandboxWorkspace, canWrite }
}

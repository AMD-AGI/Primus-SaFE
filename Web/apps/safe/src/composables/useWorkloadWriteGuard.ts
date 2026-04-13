import { computed } from 'vue'
import { useWorkspaceStore } from '@/stores/workspace'

/**
 * When the current workspace has "Sandbox" scope, only workspace admins
 * retain write access on workload pages. Regular users become read-only.
 */
export function useWorkloadWriteGuard() {
  const wsStore = useWorkspaceStore()

  const isSandboxWorkspace = computed(() =>
    (wsStore.currentScopes ?? []).includes('Sandbox'),
  )

  const canWrite = computed(() =>
    !isSandboxWorkspace.value || wsStore.isCurrentWorkspaceAdmin(),
  )

  return { isSandboxWorkspace, canWrite }
}

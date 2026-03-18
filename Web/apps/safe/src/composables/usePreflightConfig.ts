import { computed } from 'vue'
import { useWorkspaceStore } from '@/stores/workspace'

export type PreflightMode = 'system' | 'workspace'

export interface PreflightConfig {
  mode: PreflightMode
  /** Whether cluster type selection is allowed */
  allowCluster: boolean
  /** Whether workspace is fixed */
  fixedWorkspace: boolean
  /** Current workspace ID (only valid when fixedWorkspace is true) */
  workspaceId?: string
  /** Available type options */
  availableTypes: Array<'node' | 'cluster' | 'workspace' | 'workload'>
  /** Default type (optional) */
  defaultType?: 'node' | 'cluster' | 'workspace' | 'workload'
}

/**
 * Preflight config composable
 * @param mode - 'system' (system-level) or 'workspace' (workspace-level)
 */
export function usePreflightConfig(mode: PreflightMode = 'system') {
  const wsStore = useWorkspaceStore()

  const config = computed<PreflightConfig>(() => {
    if (mode === 'workspace') {
      return {
        mode: 'workspace',
        allowCluster: false,
        fixedWorkspace: true,
        workspaceId: wsStore.currentWorkspaceId,
        availableTypes: ['node', 'workspace', 'workload'], // Node type also supported in workspace mode
        defaultType: 'workspace',
      }
    }

    // System mode
    return {
      mode: 'system',
      allowCluster: true,
      fixedWorkspace: false,
      availableTypes: ['node', 'cluster', 'workspace', 'workload'],
    }
  })

  return {
    config,
    isSystemMode: computed(() => mode === 'system'),
    isWorkspaceMode: computed(() => mode === 'workspace'),
  }
}

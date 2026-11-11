import { ref } from 'vue'
import { createOptimizationTask } from '@/services/model-optimization'
import type {
  CreateOptimizationTaskPayload,
  OptimizationTask,
} from '@/services/model-optimization/type'
import { loadSelfWorkspaces } from '@/stores/selfWorkspaces'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  initialWorkspace,
  submitFailure,
  taskPayload,
  workspaceChoices,
  type WorkspaceChoice,
} from './taskWorkspace'

/**
 * The create form's workspace: the choices it offers, the one preselected, and what a submit
 * that SaFE refuses shows. `form.workspace` is what the task is created in.
 */
export function useTaskWorkspaces(form: CreateOptimizationTaskPayload) {
  const userStore = useUserStore()
  const workspaceStore = useWorkspaceStore()
  const loading = ref(false)
  const options = ref<WorkspaceChoice[]>([])
  const submitError = ref('')

  // The caller's workspaces from GET /users/self/workspaces, or the lists of the user record
  // when that route is unavailable. A system administrator may also pick any workspace the
  // console lists.
  const load = async () => {
    loading.value = true
    try {
      const lists = (await loadSelfWorkspaces()) ?? userStore.profile
      options.value = workspaceChoices(lists, userStore.isManager ? workspaceStore.items : [])
      if (!options.value.some((w) => w.id === form.workspace)) {
        form.workspace = initialWorkspace(options.value, workspaceStore.currentWorkspaceId)
      }
    } finally {
      loading.value = false
    }
  }

  /**
   * Create the task in the chosen workspace. A refusal (403) is kept in `submitError` for the
   * form to show; any other failure is returned for the caller to report.
   */
  const submit = async (): Promise<
    { task: OptimizationTask; error?: undefined } | { task?: undefined; error: string }
  > => {
    submitError.value = ''
    try {
      return { task: await createOptimizationTask(taskPayload(form), true) }
    } catch (e) {
      const failure = submitFailure(e, form.workspace)
      if (failure.refused) submitError.value = failure.message
      return { error: failure.message }
    }
  }

  return { loading, options, submitError, load, submit }
}

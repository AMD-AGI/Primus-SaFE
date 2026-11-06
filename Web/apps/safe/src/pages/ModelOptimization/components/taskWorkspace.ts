import type { Workspace } from '@/services/login/type'
import type { WorkspaceItem } from '@/services/base/type'
import type { CreateOptimizationTaskPayload } from '@/services/model-optimization/type'

/** A workspace the create form offers: its id, sent as the task's `workspace`, and a label. */
export interface WorkspaceChoice {
  id: string
  label: string
}

const labelOf = (id: string, name?: string) => (name && name !== id ? `${name} (${id})` : id)

/**
 * The workspaces a task may be created in, from GET /users/self/workspaces: the ones the caller
 * belongs to and the ones the caller manages (SaFE lets both create there), each once and in that
 * order. A system administrator may create in any workspace, so `all` (the console's full
 * workspace list) is appended for them; for anyone else pass nothing.
 */
export function workspaceChoices(
  lists: { workspaces?: Workspace[]; managedWorkspaces?: Workspace[] } | null | undefined,
  all: WorkspaceItem[] = [],
): WorkspaceChoice[] {
  const seen = new Set<string>()
  const out: WorkspaceChoice[] = []
  const add = (id: string | undefined, name?: string) => {
    if (!id || seen.has(id)) return
    seen.add(id)
    out.push({ id, label: labelOf(id, name) })
  }
  for (const w of lists?.workspaces ?? []) add(w.id, w.name)
  for (const w of lists?.managedWorkspaces ?? []) add(w.id, w.name)
  for (const w of all) add(w.workspaceId, w.workspaceName)
  return out
}

/** The workspace preselected: the console's current workspace when it is offered, else the first. */
export function initialWorkspace(choices: WorkspaceChoice[], current?: string): string {
  if (current && choices.some((c) => c.id === current)) return current
  return choices[0]?.id ?? ''
}

/** A fresh form, in the given workspace. */
export const newTaskForm = (workspace: string): CreateOptimizationTaskPayload => ({
  modelId: '',
  workspace,
  displayName: '',
  mode: 'local',
  framework: 'sglang',
  precision: 'FP4',
  tp: 1,
  ep: 1,
  gpuType: 'MI355X',
  isl: 1024,
  osl: 1024,
  concurrency: 64,
  kernelBackends: ['Claude Code'],
  geakStepLimit: 100,
  image: '',
  resultsPath: '/workspace/hyperloom/',
  rayReplica: 1,
  rayGpu: 1,
  rayCpu: 32,
  rayMemory: 128,
})

/** What is sent: the form, with the optional fields left empty dropped. */
export function taskPayload(form: CreateOptimizationTaskPayload): CreateOptimizationTaskPayload {
  const payload: CreateOptimizationTaskPayload = { ...form }
  if (!payload.image) delete payload.image
  if (!payload.displayName) delete payload.displayName
  return payload
}

/**
 * Why a submit failed, as the form shows it. A 403 means the caller may not create in that
 * workspace; it names the workspace and carries the server's reason.
 */
export function submitFailure(
  err: unknown,
  workspace: string,
): { refused: boolean; message: string } {
  const e = (err ?? {}) as {
    message?: string
    errorMessage?: string
    response?: { status?: number; data?: { errorMessage?: string; message?: string } }
  }
  const status = e.response?.status
  const reason = e.response?.data?.errorMessage || e.response?.data?.message || e.errorMessage
  if (status === 403) {
    return {
      refused: true,
      message: `You may not create tasks in workspace "${workspace}"${reason ? `: ${reason}` : ''}. Choose a workspace you belong to, or ask its manager to add you.`,
    }
  }
  return { refused: false, message: reason || e.message || 'Failed to create task' }
}

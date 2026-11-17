export interface ResourceCap {
  cpu?: string
  memory?: string
  gpu?: string
}

export interface QosScope {
  maxResources?: ResourceCap
  maxRuntime?: Record<string, number>
}

export interface UserQos extends QosScope {
  clusters?: Record<string, QosScope>
}

export interface QosItem {
  name: string
  maxResources?: ResourceCap
  maxRuntime?: Record<string, number>
}

export interface ListQosResp {
  items: QosItem[]
}

export interface ScopeDraft {
  cpu: string
  memory: string
  gpu: string
  runtimeHours: Record<string, number | undefined>
}

export const WORKLOAD_KINDS = [
  'PyTorchJob',
  'Job',
  'UnifiedJob',
  'TorchFT',
  'MonarchJob',
  'RayJob',
  'Deployment',
  'StatefulSet',
  'DynamoDeployment',
  'InferaDeployment',
  'Atom',
  'SGLangRouter',
  'Authoring',
  'AutoscalingRunnerSet',
  'EphemeralRunner',
  'GithubRunner',
  'Sandbox',
]

/** emptyDraft returns a ceiling with every field unset. */
export function emptyDraft(): ScopeDraft {
  return { cpu: '', memory: '', gpu: '', runtimeHours: {} }
}

/** scopeToDraft converts stored seconds into hours for the form. */
export function scopeToDraft(scope?: QosScope): ScopeDraft {
  const runtimeHours: Record<string, number | undefined> = {}
  for (const [kind, seconds] of Object.entries(scope?.maxRuntime ?? {})) {
    if (seconds > 0) runtimeHours[kind] = seconds / 3600
  }
  return {
    cpu: scope?.maxResources?.cpu ?? '',
    memory: scope?.maxResources?.memory ?? '',
    gpu: scope?.maxResources?.gpu ?? '',
    runtimeHours,
  }
}

/** draftToScope drops empty fields. Hours are stored as seconds. */
export function draftToScope(draft: ScopeDraft): QosScope | undefined {
  const maxResources: ResourceCap = {}
  if (draft.cpu.trim()) maxResources.cpu = draft.cpu.trim()
  if (draft.memory.trim()) maxResources.memory = draft.memory.trim()
  if (draft.gpu.trim()) maxResources.gpu = draft.gpu.trim()
  const maxRuntime: Record<string, number> = {}
  for (const [kind, hours] of Object.entries(draft.runtimeHours)) {
    if (typeof hours === 'number' && Number.isFinite(hours) && hours > 0) {
      maxRuntime[kind] = Math.round(hours * 3600)
    }
  }
  if (!Object.keys(maxResources).length && !Object.keys(maxRuntime).length) return undefined
  const scope: QosScope = {}
  if (Object.keys(maxResources).length) scope.maxResources = maxResources
  if (Object.keys(maxRuntime).length) scope.maxRuntime = maxRuntime
  return scope
}

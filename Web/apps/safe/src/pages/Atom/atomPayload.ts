import { encodeToBase64String } from '@/utils'
import { WorkloadKind } from '@/services/workload/type'

/** Fixed role order expected by the Atom admission webhook. */
export const ATOM_ROLES = ['router', 'prefill', 'decode'] as const
export type AtomRole = (typeof ATOM_ROLES)[number]

/** Default router container port. */
export const ATOM_ROUTER_TARGET_PORT = 8000

export interface AtomRoleForm {
  image: string
  entryPoint: string
  replica: number
  cpu: string
  gpu: string
  memory: string
  ephemeralStorage: string
  sharedMemory: string
  gpuName: string
  rdmaResource: string
}

export interface AtomFormModel {
  displayName: string
  description: string
  priority: number
  servicePort: number
  serviceTargetPort: number
  serviceType: string
  serviceNodePort: number | null
  serviceName: string
  roles: Record<AtomRole, AtomRoleForm>
}

/** Empty role slot. Memory and storage are Gi numbers, converted on submit. */
export function emptyAtomRole(): AtomRoleForm {
  return {
    image: '',
    entryPoint: '',
    replica: 1,
    cpu: '2',
    gpu: '',
    memory: '4',
    ephemeralStorage: '20',
    sharedMemory: '',
    gpuName: '',
    rdmaResource: '',
  }
}

/** Default create form: router, prefill, decode in that order. */
export function initialAtomForm(): AtomFormModel {
  return {
    displayName: '',
    description: '',
    priority: 1,
    servicePort: ATOM_ROUTER_TARGET_PORT,
    serviceTargetPort: ATOM_ROUTER_TARGET_PORT,
    serviceType: 'ClusterIP',
    serviceNodePort: null,
    serviceName: '',
    roles: {
      router: emptyAtomRole(),
      prefill: emptyAtomRole(),
      decode: emptyAtomRole(),
    },
  }
}

function withGi(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return ''
  return /gi$/i.test(trimmed) ? trimmed : `${trimmed}Gi`
}

function roleResource(form: AtomRoleForm) {
  const gpu = form.gpu.trim()
  return {
    replica: Number(form.replica) || 1,
    cpu: String(form.cpu).trim(),
    memory: withGi(form.memory),
    ephemeralStorage: withGi(form.ephemeralStorage),
    ...(gpu && gpu !== '0' ? { gpu } : {}),
    ...(form.sharedMemory.trim() ? { sharedMemory: withGi(form.sharedMemory) } : {}),
    ...(form.gpuName.trim() ? { gpuName: form.gpuName.trim() } : {}),
    ...(form.rdmaResource.trim() ? { rdmaResource: form.rdmaResource.trim() } : {}),
  }
}

/**
 * Build the Atom create/update body.
 * Images, entry points, and resources stay aligned as [router, prefill, decode].
 */
export function buildAtomWorkloadBody(form: AtomFormModel) {
  return {
    displayName: form.displayName.trim(),
    description: form.description,
    priority: form.priority,
    groupVersionKind: {
      kind: WorkloadKind.Atom,
      version: 'v1',
    },
    images: ATOM_ROLES.map((role) => form.roles[role].image.trim()),
    entryPoints: ATOM_ROLES.map((role) => encodeToBase64String(form.roles[role].entryPoint)),
    resources: ATOM_ROLES.map((role) => roleResource(form.roles[role])),
    service: {
      ...(form.serviceName.trim() ? { name: form.serviceName.trim() } : {}),
      protocol: 'TCP',
      port: Number(form.servicePort) || ATOM_ROUTER_TARGET_PORT,
      targetPort: Number(form.serviceTargetPort) || ATOM_ROUTER_TARGET_PORT,
      serviceType: form.serviceType || 'ClusterIP',
      ...(form.serviceType === 'NodePort' && Number(form.serviceNodePort) > 0
        ? { nodePort: Number(form.serviceNodePort) }
        : {}),
    },
  }
}

export interface AtomResumeDetail {
  displayName?: string
  description?: string
  groupVersionKind?: { group?: string; version?: string; kind?: string }
  entryPoints?: string[]
  images?: string[]
  priority?: number
  resources?: Array<Record<string, unknown>>
  service?: {
    name?: string
    protocol?: string
    port?: number
    targetPort?: number
    serviceType?: string
    nodePort?: number
  }
  timeout?: number
  env?: Record<string, string>
  customerLabels?: Record<string, string>
  workloadId?: string
  useWorkspaceStorage?: boolean
  secrets?: { id: string }[]
}

/** Fields the Atom form can clear. Omitting them must not keep the previous value. */
const clearableAtomResourceKeys = ['gpu', 'sharedMemory', 'gpuName', 'rdmaResource'] as const

/**
 * Overlay form resources onto a resumed workload.
 * Empty GPU, shared memory, GPU name, and RDMA fields clear the previous values.
 */
export function mergeAtomResumeResources<T extends object>(
  preserved: readonly object[],
  formResources: readonly T[],
): T[] {
  return formResources.map((item, index) => {
    const previous: Record<string, unknown> = {
      ...((preserved[index] as Record<string, unknown> | undefined) || {}),
    }
    for (const key of clearableAtomResourceKeys) {
      delete previous[key]
    }
    return { ...previous, ...item } as T
  })
}

/** Copy an Atom role resource, including GPU name and RDMA, for resume. */
function atomResumeResource(item: Record<string, unknown>) {
  return {
    replica: item.replica || 1,
    cpu: item.cpu,
    gpu: item.gpu,
    memory: item.memory,
    ephemeralStorage: item.ephemeralStorage,
    sharedMemory: item.sharedMemory,
    ...(item.gpuName ? { gpuName: item.gpuName } : {}),
    ...(item.rdmaResource ? { rdmaResource: item.rdmaResource } : {}),
  }
}

/**
 * Build the Atom resume body from a workload detail.
 * Role resources stay in router, prefill, decode order.
 */
export function buildAtomResumePayload(
  detail: AtomResumeDetail,
  workspace: string,
  secrets: { id: string }[],
) {
  const resources = Array.isArray(detail.resources) ? detail.resources : []
  const servicePayload = detail.service
    ? {
        service: {
          ...(detail.service.name ? { name: detail.service.name } : {}),
          protocol: detail.service.protocol || 'TCP',
          port: detail.service.port,
          targetPort: detail.service.targetPort || ATOM_ROUTER_TARGET_PORT,
          serviceType: detail.service.serviceType || 'ClusterIP',
          ...(Number(detail.service.nodePort) > 0 ? { nodePort: Number(detail.service.nodePort) } : {}),
        },
      }
    : {}
  return {
    workspace,
    displayName: detail.displayName,
    groupVersionKind: detail.groupVersionKind,
    description: detail.description,
    entryPoints: Array.isArray(detail.entryPoints) ? detail.entryPoints : [],
    images: Array.isArray(detail.images) ? detail.images : [],
    priority: detail.priority,
    resources: resources.map(atomResumeResource),
    ...servicePayload,
    ...(detail.env ? { env: detail.env } : {}),
    ...(detail.customerLabels ? { customerLabels: detail.customerLabels } : {}),
    ...(detail.timeout ? { timeout: detail.timeout } : {}),
    ...(secrets.length > 0 ? { secrets } : {}),
    workloadId: detail.workloadId,
    ...(detail.useWorkspaceStorage !== undefined
      ? { useWorkspaceStorage: detail.useWorkspaceStorage }
      : {}),
  }
}

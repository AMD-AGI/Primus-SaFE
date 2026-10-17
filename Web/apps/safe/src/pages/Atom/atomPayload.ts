import { encodeToBase64String } from '@/utils'
import { WorkloadKind } from '@/services/workload/type'

/** Fixed role order expected by the Atom admission webhook. */
export const ATOM_ROLES = ['router', 'prefill', 'decode'] as const
export type AtomRole = (typeof ATOM_ROLES)[number]

/** Router container port. The user Service targetPort must stay on this value. */
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
}

export interface AtomFormModel {
  displayName: string
  description: string
  priority: number
  servicePort: number
  serviceType: string
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
  }
}

/** Default create form: router, prefill, decode in that order. */
export function initialAtomForm(): AtomFormModel {
  return {
    displayName: '',
    description: '',
    priority: 1,
    servicePort: ATOM_ROUTER_TARGET_PORT,
    serviceType: 'ClusterIP',
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

function roleResource(role: AtomRoleForm) {
  const gpu = role.gpu.trim()
  return {
    replica: Number(role.replica) || 1,
    cpu: String(role.cpu).trim(),
    memory: withGi(role.memory),
    ephemeralStorage: withGi(role.ephemeralStorage),
    ...(gpu && gpu !== '0' ? { gpu } : {}),
    ...(role.sharedMemory.trim() ? { sharedMemory: role.sharedMemory.trim() } : {}),
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
      targetPort: ATOM_ROUTER_TARGET_PORT,
      serviceType: form.serviceType || 'ClusterIP',
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
    serviceType?: string
  }
  timeout?: number
  env?: Record<string, string>
  customerLabels?: Record<string, string>
  workloadId?: string
  useWorkspaceStorage?: boolean
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
          targetPort: ATOM_ROUTER_TARGET_PORT,
          serviceType: detail.service.serviceType || 'ClusterIP',
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

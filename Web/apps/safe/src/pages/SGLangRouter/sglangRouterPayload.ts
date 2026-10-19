import { encodeToBase64String } from '@/utils'
import { WorkloadKind } from '@/services/workload/type'

/** Fixed role order expected by the SGLangRouter admission webhook. */
export const SGLANG_ROUTER_ROLES = ['router', 'prefill', 'decode'] as const
export type SGLangRouterRole = (typeof SGLANG_ROUTER_ROLES)[number]

/** Router container port. The user Service targetPort must stay on this value. */
export const SGLANG_ROUTER_TARGET_PORT = 8000

export interface SGLangRouterRoleForm {
  image: string
  entryPoint: string
  replica: number
  cpu: string
  gpu: string
  memory: string
  ephemeralStorage: string
  sharedMemory: string
}

export interface SGLangRouterFormModel {
  displayName: string
  description: string
  priority: number
  servicePort: number
  serviceType: string
  serviceName: string
  roles: Record<SGLangRouterRole, SGLangRouterRoleForm>
}

/** Default commands. Dispatch replaces {prefill} and {decode} on the router. */
export const SGLANG_ROUTER_DEFAULT_ENTRY: Record<SGLangRouterRole, string> = {
  router:
    'python3 -m sglang_router.launch_router --pd-disaggregation --prefill {prefill} --decode {decode} --host 0.0.0.0 --port 8000',
  prefill:
    'python3 -m sglang.launch_server --model-path /models --disaggregation-mode prefill --disaggregation-transfer-backend mooncake --disaggregation-bootstrap-port 8998 --host 0.0.0.0 --port 8010',
  decode:
    'python3 -m sglang.launch_server --model-path /models --disaggregation-mode decode --disaggregation-transfer-backend mooncake --disaggregation-bootstrap-port 9098 --host 0.0.0.0 --port 8020',
}

/** Empty role slot. Memory and storage are Gi numbers, converted on submit. */
export function emptySGLangRouterRole(entryPoint = ''): SGLangRouterRoleForm {
  return {
    image: '',
    entryPoint,
    replica: 1,
    cpu: '2',
    gpu: '',
    memory: '4',
    ephemeralStorage: '20',
    sharedMemory: '',
  }
}

/** Default create form: router, prefill, decode in that order. */
export function initialSGLangRouterForm(): SGLangRouterFormModel {
  return {
    displayName: '',
    description: '',
    priority: 1,
    servicePort: SGLANG_ROUTER_TARGET_PORT,
    serviceType: 'ClusterIP',
    serviceName: '',
    roles: {
      router: emptySGLangRouterRole(SGLANG_ROUTER_DEFAULT_ENTRY.router),
      prefill: emptySGLangRouterRole(SGLANG_ROUTER_DEFAULT_ENTRY.prefill),
      decode: emptySGLangRouterRole(SGLANG_ROUTER_DEFAULT_ENTRY.decode),
    },
  }
}

function withGi(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) return ''
  return /gi$/i.test(trimmed) ? trimmed : `${trimmed}Gi`
}

function roleResource(role: SGLangRouterRoleForm) {
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
 * Build the SGLangRouter create/update body.
 * Images, entry points, and resources stay aligned as [router, prefill, decode].
 */
export function buildSGLangRouterWorkloadBody(form: SGLangRouterFormModel) {
  return {
    displayName: form.displayName.trim(),
    description: form.description,
    priority: form.priority,
    groupVersionKind: {
      kind: WorkloadKind.SGLangRouter,
      version: 'v1',
    },
    images: SGLANG_ROUTER_ROLES.map((role) => form.roles[role].image.trim()),
    entryPoints: SGLANG_ROUTER_ROLES.map((role) => encodeToBase64String(form.roles[role].entryPoint)),
    resources: SGLANG_ROUTER_ROLES.map((role) => roleResource(form.roles[role])),
    service: {
      ...(form.serviceName.trim() ? { name: form.serviceName.trim() } : {}),
      protocol: 'TCP',
      port: Number(form.servicePort) || SGLANG_ROUTER_TARGET_PORT,
      targetPort: SGLANG_ROUTER_TARGET_PORT,
      serviceType: form.serviceType || 'ClusterIP',
    },
  }
}

export interface SGLangRouterResumeDetail {
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

/** Copy an SGLangRouter role resource, including GPU name and RDMA, for resume. */
function sglangRouterResumeResource(item: Record<string, unknown>) {
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
 * Build the SGLangRouter resume body from a workload detail.
 * Role resources stay in router, prefill, decode order.
 */
export function buildSGLangRouterResumePayload(
  detail: SGLangRouterResumeDetail,
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
          targetPort: SGLANG_ROUTER_TARGET_PORT,
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
    resources: resources.map(sglangRouterResumeResource),
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

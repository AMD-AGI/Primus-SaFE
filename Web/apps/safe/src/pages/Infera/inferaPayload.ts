import { encodeToBase64String } from '@/utils'
import type { DynamoPdAggregationRole, DynamoRoleResourceForm } from '../Dynamo/dynamoPayload'

export type InferaKvTransferBackend = 'mori'
export type InferaBackendEngine = 'sglang' | 'vllm'
export type InferaRouterPolicy = 'kv-aware' | 'round-robin'
export type InferaPdAggregationRole = DynamoPdAggregationRole
export type InferaRoleResourceForm = DynamoRoleResourceForm

export interface InferaFormModel {
  displayName: string
  description: string
  priority: number
  image: string
  modelPath: string
  backendEngine: InferaBackendEngine
  workerBackendEngine: InferaBackendEngine
  prefillBackendEngine: InferaBackendEngine
  decodeBackendEngine: InferaBackendEngine
  attentionBackend: string
  memFractionStatic: string
  routerPolicy: InferaRouterPolicy
  frontendEntrypoint: string
  workerEntrypoint: string
  prefillEntrypoint: string
  decodeEntrypoint: string
  enablePd: boolean
  enableAggregation: boolean
  pdAggregationRoles: InferaPdAggregationRole[]
  kvTransferBackend: InferaKvTransferBackend
  env: Record<string, string>
  service: {
    protocol: string
    port: number
    targetPort: number
    serviceType: string
  }
  frontend: InferaRoleResourceForm
  worker: InferaRoleResourceForm
  prefill: InferaRoleResourceForm
  decode: InferaRoleResourceForm
}

export interface InferaCreatePayload {
  workspaceId: string
  displayName: string
  groupVersionKind: {
    kind: 'InferaDeployment'
    version: 'v1'
  }
  description?: string
  priority: number
  images: string[]
  entryPoints: string[]
  resources: InferaResourcePayload[]
  env: Record<string, string>
  service: {
    protocol: string
    port: number
    targetPort: number
    serviceType: string
  }
  inferaOptions: {
    serviceRoles: string[]
    kvTransferBackend?: InferaKvTransferBackend
    multinodeRoles?: string[]
  }
}

export interface InferaResourcePayload {
  replica: number
  cpu: string
  gpu?: string
  memory: string
  sharedMemory?: string
  rdmaResource?: string
}

export const OPTIMUS_DEFAULT_IMAGE =
  'harbor.core42.primus-safe.amd.com/primussafe/infera-engine-sglang:0.1.0-rocm-20260610'

export const OPTIMUS_FRONTEND_ENTRYPOINT =
  'python3 -m infera.server --host 0.0.0.0 --port 8000 --router-tokenizer-path /wekafs/models/DeepSeek-R1-0528'

const FRONTEND_RESOURCE: InferaResourcePayload = {
  replica: 1,
  cpu: '4',
  memory: '16Gi',
}

export const INFERA_SERVICE = {
  protocol: 'TCP',
  port: 8000,
  targetPort: 8000,
  serviceType: 'ClusterIP',
} as const

export function createDefaultInferaForm(): InferaFormModel {
  return {
    displayName: '',
    description: '',
    priority: 1,
    image: OPTIMUS_DEFAULT_IMAGE,
    modelPath: '/wekafs/models/DeepSeek-R1-0528',
    backendEngine: 'sglang',
    workerBackendEngine: 'sglang',
    prefillBackendEngine: 'sglang',
    decodeBackendEngine: 'sglang',
    attentionBackend: 'aiter',
    memFractionStatic: '0.75',
    routerPolicy: 'kv-aware',
    frontendEntrypoint: '',
    workerEntrypoint: '',
    prefillEntrypoint: '',
    decodeEntrypoint: '',
    enablePd: false,
    enableAggregation: false,
    pdAggregationRoles: ['prefill', 'decode'],
    kvTransferBackend: 'mori',
    env: {
      HF_HOME: '/data/hf-cache',
      NCCL_DEBUG: 'INFO',
    },
    service: {
      protocol: 'TCP',
      port: 8000,
      targetPort: 8000,
      serviceType: 'ClusterIP',
    },
    frontend: createDefaultFrontendResource(),
    worker: createDefaultBackendResource(1),
    prefill: createDefaultBackendResource(1),
    decode: createDefaultBackendResource(1),
  }
}

export function getInferaDefaultTpSize(resource: Pick<InferaRoleResourceForm, 'gpu' | 'replica'>) {
  return Number(resource.gpu || 0) * Number(resource.replica || 1)
}

export function buildInferaWorkerEntrypoint(
  form: Pick<
    InferaFormModel,
    | 'modelPath'
    | 'attentionBackend'
    | 'memFractionStatic'
    | 'enablePd'
    | 'enableAggregation'
  >,
  resource: InferaRoleResourceForm,
  backendEngine: InferaBackendEngine = 'sglang',
) {
  const defaultTpSize = getInferaDefaultTpSize(resource)
  const tpSize = form.enableAggregation && !form.enablePd ? defaultTpSize : resource.tpSize || 8
  const epSize = form.enableAggregation && !form.enablePd ? defaultTpSize : resource.epSize || tpSize
  const args = [
    `python3 -m infera.engine.${backendEngine || 'sglang'}`,
    `--model-path ${form.modelPath}`,
    `--tp-size ${tpSize}`,
    `--ep-size ${epSize}`,
    '--enable-dp-attention',
    `--attention-backend ${form.attentionBackend}`,
    '--trust-remote-code',
    `--mem-fraction-static ${form.memFractionStatic || '0.75'}`,
    '--host 0.0.0.0',
  ].filter(Boolean)

  return args.join(' ')
}

export function buildInferaFrontendEntrypoint(
  form: Pick<InferaFormModel, 'modelPath' | 'routerPolicy'>,
) {
  return [
    'python3 -m infera.server',
    '--host 0.0.0.0',
    '--port 8000',
    `--router-policy ${form.routerPolicy || 'kv-aware'}`,
    `--router-tokenizer-path ${form.modelPath}`,
  ].join(' ')
}

export function buildInferaCreatePayload(form: InferaFormModel, workspace: string): InferaCreatePayload {
  validateInferaForm(form)

  const frontendEntryPoint = encodeToBase64String(resolveFrontendEntrypoint(form))
  const workerEntryPoint = encodeToBase64String(
    resolveBackendEntrypoint(form, form.worker, form.workerEntrypoint, form.workerBackendEngine),
  )
  const prefillEntryPoint = encodeToBase64String(
    resolveBackendEntrypoint(form, form.prefill, form.prefillEntrypoint, form.prefillBackendEngine),
  )
  const decodeEntryPoint = encodeToBase64String(
    resolveBackendEntrypoint(form, form.decode, form.decodeEntrypoint, form.decodeBackendEngine),
  )

  if (form.enablePd) {
    return {
      workspaceId: workspace,
      displayName: form.displayName,
      groupVersionKind: { kind: 'InferaDeployment', version: 'v1' },
      ...(form.description ? { description: form.description } : {}),
      priority: form.priority,
      images: [form.image, form.image, form.image],
      entryPoints: [frontendEntryPoint, prefillEntryPoint, decodeEntryPoint],
      resources: [toResourcePayload(form.frontend), toResourcePayload(form.prefill), toResourcePayload(form.decode)],
      env: form.env,
      service: { ...INFERA_SERVICE },
      inferaOptions: {
        serviceRoles: ['frontend', 'prefill', 'decode'],
        kvTransferBackend: form.kvTransferBackend || 'mori',
      },
    }
  }

  return {
    workspaceId: workspace,
    displayName: form.displayName,
    groupVersionKind: { kind: 'InferaDeployment', version: 'v1' },
    ...(form.description ? { description: form.description } : {}),
    priority: form.priority,
    images: [form.image, form.image],
    entryPoints: [frontendEntryPoint, workerEntryPoint],
    resources: [toResourcePayload(form.frontend), toResourcePayload(form.worker)],
    env: form.env,
    service: { ...INFERA_SERVICE },
    inferaOptions: {
      serviceRoles: ['frontend', 'worker'],
    },
  }
}

export function validateInferaForm(form: InferaFormModel) {
  if (!form.enableAggregation) return

  if (!form.enablePd) {
    if (Number(form.worker.replica || 0) <= 1) {
      throw new Error('Aggregation requires worker replica greater than 1')
    }
    return
  }

  if (!form.pdAggregationRoles.length) {
    throw new Error('Please select at least one PD aggregation role')
  }

  for (const role of form.pdAggregationRoles) {
    if (Number(form[role].replica || 0) <= 1) {
      throw new Error(`Aggregation role ${role} requires replica greater than 1`)
    }
  }
}

function createDefaultFrontendResource(): InferaRoleResourceForm {
  return {
    replica: FRONTEND_RESOURCE.replica,
    cpu: FRONTEND_RESOURCE.cpu,
    memory: '16',
  }
}

function createDefaultBackendResource(replica: number): InferaRoleResourceForm {
  return {
    replica,
    cpu: '64',
    gpu: '8',
    memory: '256',
    sharedMemory: '200',
    tpSize: 8,
    epSize: 8,
  }
}

function resolveFrontendEntrypoint(form: InferaFormModel) {
  return form.frontendEntrypoint.trim()
    ? form.frontendEntrypoint
    : buildInferaFrontendEntrypoint(form)
}

function resolveBackendEntrypoint(
  form: Pick<
    InferaFormModel,
    | 'modelPath'
    | 'attentionBackend'
    | 'memFractionStatic'
    | 'enablePd'
    | 'enableAggregation'
  >,
  resource: InferaRoleResourceForm,
  customEntrypoint: string,
  backendEngine: InferaBackendEngine,
) {
  return customEntrypoint.trim()
    ? customEntrypoint
    : buildInferaWorkerEntrypoint(form, resource, backendEngine)
}

function toResourcePayload(resource: InferaRoleResourceForm): InferaResourcePayload {
  return {
    replica: Number(resource.replica || 1),
    cpu: resource.cpu,
    ...(resource.gpu && Number(resource.gpu) !== 0 ? { gpu: resource.gpu } : {}),
    memory: withGi(resource.memory),
    ...(resource.sharedMemory ? { sharedMemory: withGi(resource.sharedMemory) } : {}),
    ...(resource.rdmaResource ? { rdmaResource: resource.rdmaResource } : {}),
  }
}

function withGi(value: string) {
  return /gi$/i.test(value.trim()) ? value.trim() : `${value.trim()}Gi`
}

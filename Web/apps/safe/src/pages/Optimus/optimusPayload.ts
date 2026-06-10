import { encodeToBase64String } from '@/utils'
import type { DynamoPdAggregationRole, DynamoRoleResourceForm } from '../Dynamo/dynamoPayload'

export type OptimusKvTransferBackend = 'mori'
export type OptimusBackendEngine = 'sglang' | 'vllm'
export type OptimusPdAggregationRole = DynamoPdAggregationRole
export type OptimusRoleResourceForm = DynamoRoleResourceForm

export interface OptimusFormModel {
  displayName: string
  description: string
  priority: number
  image: string
  modelPath: string
  backendEngine: OptimusBackendEngine
  attentionBackend: string
  memFractionStatic: string
  workerEntrypoint: string
  enablePd: boolean
  enableAggregation: boolean
  pdAggregationRoles: OptimusPdAggregationRole[]
  kvTransferBackend: OptimusKvTransferBackend
  env: Record<string, string>
  service: {
    protocol: string
    port: number
    targetPort: number
    serviceType: string
  }
  worker: OptimusRoleResourceForm
  prefill: OptimusRoleResourceForm
  decode: OptimusRoleResourceForm
}

export interface OptimusCreatePayload {
  workspaceId: string
  displayName: string
  groupVersionKind: {
    kind: 'OptimusDeployment'
    version: 'v1'
  }
  description?: string
  priority: number
  images: string[]
  entryPoints: string[]
  resources: OptimusResourcePayload[]
  env: Record<string, string>
  service: {
    protocol: string
    port: number
    targetPort: number
    serviceType: string
  }
  optimusOptions: {
    serviceRoles: string[]
    kvTransferBackend?: OptimusKvTransferBackend
    multinodeRoles?: string[]
  }
}

export interface OptimusResourcePayload {
  replica: number
  cpu: string
  gpu?: string
  memory: string
  sharedMemory?: string
  rdmaResource?: string
}

export const OPTIMUS_DEFAULT_IMAGE =
  'harbor.core42.primus-safe.amd.com/primussafe/rocserve-sglang:0.1.0-rocm-20260610'

export const OPTIMUS_FRONTEND_ENTRYPOINT =
  'python3 -m rocserve.server --host 0.0.0.0 --port 8000 --router-policy kv-aware --router-tokenizer-path /wekafs/models/DeepSeek-R1-0528 --discovery-backend kubernetes --request-transport nats --kv-event-transport nats'

const FRONTEND_RESOURCE: OptimusResourcePayload = {
  replica: 1,
  cpu: '4',
  memory: '16Gi',
}

export const OPTIMUS_SERVICE = {
  protocol: 'TCP',
  port: 8000,
  targetPort: 8000,
  serviceType: 'ClusterIP',
} as const

export function createDefaultOptimusForm(): OptimusFormModel {
  return {
    displayName: '',
    description: '',
    priority: 1,
    image: OPTIMUS_DEFAULT_IMAGE,
    modelPath: '/wekafs/models/DeepSeek-R1-0528',
    backendEngine: 'sglang',
    attentionBackend: 'aiter',
    memFractionStatic: '0.75',
    workerEntrypoint: '',
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
    worker: createDefaultBackendResource(1),
    prefill: createDefaultBackendResource(1),
    decode: createDefaultBackendResource(1),
  }
}

export function getOptimusDefaultTpSize(resource: Pick<OptimusRoleResourceForm, 'gpu' | 'replica'>) {
  return Number(resource.gpu || 0) * Number(resource.replica || 1)
}

export function buildOptimusWorkerEntrypoint(
  form: Pick<
    OptimusFormModel,
    | 'modelPath'
    | 'backendEngine'
    | 'attentionBackend'
    | 'memFractionStatic'
    | 'enablePd'
  >,
  resource: OptimusRoleResourceForm,
) {
  const args = [
    `exec python3 -m rocserve.engine.${form.backendEngine || 'sglang'}`,
    `--model-path ${form.modelPath}`,
    `--tp-size ${resource.tpSize || 8}`,
    `--ep-size ${resource.epSize || resource.tpSize || 8}`,
    `--attention-backend ${form.attentionBackend}`,
    '--trust-remote-code',
    `--mem-fraction-static ${form.memFractionStatic || '0.75'}`,
    '--host 0.0.0.0',
    '--discovery-backend kubernetes',
    '--request-transport nats',
    '--kv-event-transport nats',
    form.enablePd ? '--disaggregation-ib-device rocep29s0' : '',
  ].filter(Boolean)

  return args.join(' ')
}

export function buildOptimusFrontendEntrypoint(form: Pick<OptimusFormModel, 'modelPath'>) {
  return [
    'python3 -m rocserve.server',
    '--host 0.0.0.0',
    '--port 8000',
    '--router-policy kv-aware',
    `--router-tokenizer-path ${form.modelPath}`,
    '--discovery-backend kubernetes',
    '--request-transport nats',
    '--kv-event-transport nats',
  ].join(' ')
}

export function buildOptimusCreatePayload(form: OptimusFormModel, workspace: string): OptimusCreatePayload {
  validateOptimusForm(form)

  const rawWorkerEntryPoint = buildOptimusWorkerEntrypoint(form, form.worker)
  const hasCustomWorkerEntrypoint = Boolean(form.workerEntrypoint.trim())
  const resolvedWorkerEntryPoint = hasCustomWorkerEntrypoint
    ? form.workerEntrypoint
    : rawWorkerEntryPoint
  const workerEntryPoint = encodeToBase64String(resolvedWorkerEntryPoint)
  const frontendEntryPoint = encodeToBase64String(buildOptimusFrontendEntrypoint(form))

  if (form.enablePd) {
    return {
      workspaceId: workspace,
      displayName: form.displayName,
      groupVersionKind: { kind: 'OptimusDeployment', version: 'v1' },
      ...(form.description ? { description: form.description } : {}),
      priority: form.priority,
      images: [form.image, form.image, form.image],
      entryPoints: [frontendEntryPoint, workerEntryPoint, workerEntryPoint],
      resources: [FRONTEND_RESOURCE, toResourcePayload(form.prefill), toResourcePayload(form.decode)],
      env: form.env,
      service: { ...OPTIMUS_SERVICE },
      optimusOptions: {
        serviceRoles: ['frontend', 'prefill', 'decode'],
        kvTransferBackend: form.kvTransferBackend || 'mori',
      },
    }
  }

  return {
    workspaceId: workspace,
    displayName: form.displayName,
    groupVersionKind: { kind: 'OptimusDeployment', version: 'v1' },
    ...(form.description ? { description: form.description } : {}),
    priority: form.priority,
    images: [form.image, form.image],
    entryPoints: [frontendEntryPoint, workerEntryPoint],
    resources: [FRONTEND_RESOURCE, toResourcePayload(form.worker)],
    env: form.env,
    service: { ...OPTIMUS_SERVICE },
    optimusOptions: {
      serviceRoles: ['frontend', 'worker'],
    },
  }
}

export function validateOptimusForm(form: OptimusFormModel) {
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

function createDefaultBackendResource(replica: number): OptimusRoleResourceForm {
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

function toResourcePayload(resource: OptimusRoleResourceForm): OptimusResourcePayload {
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

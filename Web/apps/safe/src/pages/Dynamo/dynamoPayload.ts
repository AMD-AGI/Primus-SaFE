import { encodeToBase64String } from '@/utils'

export type DynamoKvTransferBackend = 'nixl' | 'mooncake'
export type DynamoPdAggregationRole = 'prefill' | 'decode'
export type DynamoBackendEngine = 'sglang' | 'vllm'

export interface DynamoEntrypointPreviewToken {
  text: string
  editable: boolean
}

export interface DynamoRoleResourceForm {
  replica: number
  cpu: string
  gpu?: string
  memory: string
  ephemeralStorage?: string
  sharedMemory?: string
  rdmaResource?: string
  tpSize?: number
  epSize?: number
}

export interface DynamoFormModel {
  displayName: string
  description: string
  priority: number
  image: string
  modelPath: string
  backendEngine: DynamoBackendEngine
  attentionBackend: string
  memFractionStatic: string
  enablePd: boolean
  enableAggregation: boolean
  pdAggregationRoles: DynamoPdAggregationRole[]
  kvTransferBackend: DynamoKvTransferBackend
  env: Record<string, string>
  service: {
    protocol: string
    port: number
    targetPort: number
    serviceType: string
  }
  worker: DynamoRoleResourceForm
  prefill: DynamoRoleResourceForm
  decode: DynamoRoleResourceForm
}

export interface DynamoCreatePayload {
  workspaceId: string
  displayName: string
  groupVersionKind: {
    kind: 'DynamoDeployment'
    version: 'v1'
  }
  description?: string
  priority: number
  images: string[]
  entryPoints: string[]
  resources: DynamoResourcePayload[]
  env: Record<string, string>
  service: {
    protocol: string
    port: number
    targetPort: number
    serviceType: string
  }
  dynamoOptions: {
    serviceRoles: string[]
    kvTransferBackend?: DynamoKvTransferBackend
    multinodeRoles?: string[]
  }
}

export interface DynamoResourcePayload {
  replica: number
  cpu: string
  gpu?: string
  memory: string
  ephemeralStorage: string
  sharedMemory?: string
  rdmaResource?: string
}

export const DYNAMO_DEFAULT_IMAGE =
  'harbor.core42.primus-safe.amd.com/custom/sync/sglang-dynamo:1.1.0-rocm-202605271513'

export const DYNAMO_FRONTEND_ENTRYPOINT =
  'python3 -m dynamo.frontend --http-port 8000 --router-mode round-robin'
const DYNAMO_DEFAULT_EPHEMERAL_STORAGE = '100Gi'

const FRONTEND_RESOURCE: DynamoResourcePayload = {
  replica: 1,
  cpu: '4',
  memory: '16Gi',
  ephemeralStorage: DYNAMO_DEFAULT_EPHEMERAL_STORAGE,
}

export const DYNAMO_SERVICE = {
  protocol: 'TCP',
  port: 8000,
  targetPort: 8000,
  serviceType: 'ClusterIP',
} as const

export function createDefaultDynamoForm(): DynamoFormModel {
  return {
    displayName: '',
    description: '',
    priority: 1,
    image: DYNAMO_DEFAULT_IMAGE,
    modelPath: '/wekafs/models/DeepSeek-R1-0528',
    backendEngine: 'sglang',
    attentionBackend: 'aiter',
    memFractionStatic: '0.75',
    enablePd: false,
    enableAggregation: false,
    pdAggregationRoles: ['prefill', 'decode'],
    kvTransferBackend: 'nixl',
    env: {},
    service: {
      protocol: 'TCP',
      port: 8000,
      targetPort: 8000,
      serviceType: 'ClusterIP',
    },
    worker: {
      replica: 1,
      cpu: '64',
      gpu: '8',
      memory: '256',
      ephemeralStorage: '100',
      sharedMemory: '200',
      tpSize: 8,
      epSize: 8,
    },
    prefill: {
      replica: 2,
      cpu: '64',
      gpu: '8',
      memory: '512',
      ephemeralStorage: '100',
      sharedMemory: '300',
      tpSize: 8,
      epSize: 8,
    },
    decode: {
      replica: 2,
      cpu: '64',
      gpu: '8',
      memory: '512',
      ephemeralStorage: '100',
      sharedMemory: '300',
      tpSize: 8,
      epSize: 8,
    },
  }
}

export function getDynamoDefaultTpSize(resource: Pick<DynamoRoleResourceForm, 'gpu' | 'replica'>) {
  return Number(resource.gpu || 0) * Number(resource.replica || 1)
}

export function buildDynamoWorkerEntrypoint(
  form: Pick<
    DynamoFormModel,
    | 'modelPath'
    | 'backendEngine'
    | 'attentionBackend'
    | 'memFractionStatic'
    | 'enableAggregation'
    | 'enablePd'
    | 'kvTransferBackend'
  >,
  resource: DynamoRoleResourceForm,
) {
  const args = [
    `exec python3 -m dynamo.${form.backendEngine || 'sglang'}`,
    `--model-path ${form.modelPath}`,
    `--tp-size ${resource.tpSize || 8}`,
    `--ep-size ${resource.epSize || resource.tpSize || 8}`,
    Number(resource.tpSize || 0) > 8 ? '--enable-dp-attention' : '',
    `--attention-backend ${form.attentionBackend}`,
    '--trust-remote-code',
    `--mem-fraction-static ${form.memFractionStatic || '0.75'}`,
    '--host 0.0.0.0',
    form.enablePd ? `--disaggregation-transfer-backend ${form.kvTransferBackend || 'nixl'}` : '',
  ].filter(Boolean)

  return args.join(' ')
}

export function buildDynamoEntrypointPreviewTokens(command: string): DynamoEntrypointPreviewToken[] {
  const editableValueFlags = new Set([
    '--model-path',
    '--tp-size',
    '--ep-size',
    '--mem-fraction-static',
    '--disaggregation-transfer-backend',
  ])
  const parts = command.match(/\s+|[^\s]+/g) || []
  const tokens: DynamoEntrypointPreviewToken[] = []
  let highlightNextValue = false

  for (const text of parts) {
    if (/^\s+$/.test(text)) {
      tokens.push({ text, editable: false })
      continue
    }

    const backendMatch = text.match(/^dynamo\.(sglang|vllm)$/)
    if (backendMatch) {
      tokens.push({ text: 'dynamo.', editable: false })
      tokens.push({ text: backendMatch[1], editable: true })
      highlightNextValue = false
      continue
    }

    if (editableValueFlags.has(text)) {
      tokens.push({ text, editable: false })
      highlightNextValue = true
      continue
    }

    tokens.push({ text, editable: highlightNextValue })
    highlightNextValue = false
  }

  return tokens
}

export function buildDynamoCreatePayload(form: DynamoFormModel, workspace: string): DynamoCreatePayload {
  validateDynamoForm(form)

  const rawWorkerEntryPoint = buildDynamoWorkerEntrypoint(form, form.worker)
  const workerEntryPoint = encodeToBase64String(form.enablePd ? `${rawWorkerEntryPoint}\n` : rawWorkerEntryPoint)
  const frontendEntryPoint = encodeToBase64String(DYNAMO_FRONTEND_ENTRYPOINT)

  if (form.enablePd) {
    return {
      workspaceId: workspace,
      displayName: form.displayName,
      groupVersionKind: { kind: 'DynamoDeployment', version: 'v1' },
      ...(form.description ? { description: form.description } : {}),
      priority: form.priority,
      images: [form.image, form.image, form.image],
      entryPoints: [frontendEntryPoint, workerEntryPoint, workerEntryPoint],
      resources: [FRONTEND_RESOURCE, toResourcePayload(form.prefill), toResourcePayload(form.decode)],
      env: form.env,
      service: { ...DYNAMO_SERVICE },
      dynamoOptions: {
        serviceRoles: ['frontend', 'prefill', 'decode'],
        kvTransferBackend: form.kvTransferBackend || 'nixl',
        ...(form.enableAggregation && form.pdAggregationRoles.length
          ? { multinodeRoles: form.pdAggregationRoles }
          : {}),
      },
    }
  }

  return {
    workspaceId: workspace,
    displayName: form.displayName,
    groupVersionKind: { kind: 'DynamoDeployment', version: 'v1' },
    ...(form.description ? { description: form.description } : {}),
    priority: form.priority,
    images: [form.image, form.image],
    entryPoints: [frontendEntryPoint, workerEntryPoint],
    resources: [FRONTEND_RESOURCE, toResourcePayload(form.worker)],
    env: form.env,
    service: { ...DYNAMO_SERVICE },
    dynamoOptions: {
      serviceRoles: ['frontend', 'worker'],
      ...(form.enableAggregation ? { multinodeRoles: ['worker'] } : {}),
    },
  }
}

export function validateDynamoForm(form: DynamoFormModel) {
  if (!form.enableAggregation) return

  if (form.enableAggregation && Number(form.worker.tpSize || 0) <= 8) {
    throw new Error('Aggregation TP size must be greater than 8')
  }

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

function toResourcePayload(resource: DynamoRoleResourceForm): DynamoResourcePayload {
  return {
    replica: Number(resource.replica || 1),
    cpu: resource.cpu,
    ...(resource.gpu && Number(resource.gpu) !== 0 ? { gpu: resource.gpu } : {}),
    memory: withGi(resource.memory),
    ephemeralStorage: withGi(resource.ephemeralStorage || DYNAMO_DEFAULT_EPHEMERAL_STORAGE),
    ...(resource.sharedMemory ? { sharedMemory: withGi(resource.sharedMemory) } : {}),
    ...(resource.rdmaResource ? { rdmaResource: resource.rdmaResource } : {}),
  }
}

function withGi(value: string) {
  return /gi$/i.test(value.trim()) ? value.trim() : `${value.trim()}Gi`
}

import { describe, expect, it } from 'vitest'
import { decodeFromBase64String } from '@/utils'
import {
  buildDynamoEntrypointPreviewTokens,
  buildDynamoCreatePayload,
  createDefaultDynamoForm,
  getDynamoDefaultTpSize,
} from './dynamoPayload'

describe('dynamoPayload', () => {
  it('builds a single-node Dynamo payload without multinodeRoles', () => {
    const form = createDefaultDynamoForm()
    form.displayName = 'dynamo-ds-r1-agg'
    form.worker.replica = 1
    form.worker.tpSize = 8
    form.worker.epSize = 8
    form.service.port = 9000
    form.service.targetPort = 9000
    form.service.serviceType = 'NodePort'

    const payload = buildDynamoCreatePayload(form, 'core42-hyperloom')

    expect(payload).toMatchObject({ workspaceId: 'core42-hyperloom' })
    expect(payload).not.toHaveProperty('workspace')
    expect(payload.groupVersionKind).toEqual({ kind: 'DynamoDeployment', version: 'v1' })
    expect(payload.images).toEqual([form.image, form.image])
    expect(payload.resources).toEqual([
      { replica: 1, cpu: '4', memory: '16Gi' },
      {
        replica: 1,
        cpu: '64',
        gpu: '8',
        memory: '256Gi',
        sharedMemory: '200Gi',
      },
    ])
    expect(payload.dynamoOptions).toEqual({ serviceRoles: ['frontend', 'worker'] })
    expect(payload.service).toEqual({
      protocol: 'TCP',
      port: 8000,
      targetPort: 8000,
      serviceType: 'ClusterIP',
    })
    expect(decodeFromBase64String(payload.entryPoints[1])).toContain('--tp-size 8')
  })

  it('does not prefill env vars by default', () => {
    const form = createDefaultDynamoForm()
    form.displayName = 'dynamo-empty-env'

    const payload = buildDynamoCreatePayload(form, 'core42-hyperloom')

    expect(form.env).toEqual({})
    expect(payload.env).toEqual({})
  })

  it('builds an aggregation payload with worker multinodeRoles and default tp size', () => {
    const form = createDefaultDynamoForm()
    form.displayName = 'dynamo-ds-r1-2node'
    form.enableAggregation = true
    form.worker.replica = 2
    form.worker.gpu = '8'
    form.worker.rdmaResource = '1'
    form.worker.tpSize = getDynamoDefaultTpSize(form.worker)
    form.worker.epSize = form.worker.tpSize

    const payload = buildDynamoCreatePayload(form, 'core42-hyperloom')

    expect(payload.resources[1]).toEqual({
      replica: 2,
      cpu: '64',
      gpu: '8',
      memory: '256Gi',
      sharedMemory: '200Gi',
      rdmaResource: '1',
    })
    expect(payload.dynamoOptions).toEqual({
      serviceRoles: ['frontend', 'worker'],
      multinodeRoles: ['worker'],
    })
    expect(decodeFromBase64String(payload.entryPoints[1])).toContain('--tp-size 16')
    expect(decodeFromBase64String(payload.entryPoints[1])).toContain('--enable-dp-attention')
  })

  it('adds dp attention when tp size is greater than 8 even without aggregation', () => {
    const form = createDefaultDynamoForm()
    form.worker.replica = 1
    form.worker.tpSize = 9
    form.worker.epSize = 9

    const payload = buildDynamoCreatePayload(form, 'core42-hyperloom')

    expect(payload.dynamoOptions).toEqual({ serviceRoles: ['frontend', 'worker'] })
    expect(decodeFromBase64String(payload.entryPoints[1])).toContain('--enable-dp-attention')
  })

  it('uses the selected backend engine in the worker entrypoint', () => {
    const form = createDefaultDynamoForm()
    form.backendEngine = 'vllm'

    const payload = buildDynamoCreatePayload(form, 'core42-hyperloom')
    const workerEntrypoint = decodeFromBase64String(payload.entryPoints[1])

    expect(workerEntrypoint).toContain('exec python3 -m dynamo.vllm')
    expect(workerEntrypoint).not.toContain('exec python3 -m dynamo.sglang')
  })

  it('marks only editable entrypoint values as highlighted preview tokens', () => {
    const tokens = buildDynamoEntrypointPreviewTokens(
      'exec python3 -m dynamo.vllm --model-path /models/ds --tp-size 16 --ep-size 16 --mem-fraction-static 0.75',
    )
    const highlighted = tokens.filter((token) => token.editable).map((token) => token.text)

    expect(highlighted).toEqual(['vllm', '/models/ds', '16', '16', '0.75'])
    expect(highlighted).not.toContain('dynamo.')
    expect(highlighted).not.toContain('--model-path')
    expect(highlighted).not.toContain('--tp-size')
  })

  it('does not add dp attention when tp size is 8 or less', () => {
    const form = createDefaultDynamoForm()
    form.worker.tpSize = 8
    form.worker.epSize = 8

    const payload = buildDynamoCreatePayload(form, 'core42-hyperloom')

    expect(decodeFromBase64String(payload.entryPoints[1])).not.toContain('--enable-dp-attention')
  })

  it('builds a PD payload with prefill and decode roles copied from the same entrypoint', () => {
    const form = createDefaultDynamoForm()
    form.displayName = 'dynamo-ds-r1-pd'
    form.enablePd = true
    form.kvTransferBackend = 'mooncake'
    form.prefill.replica = 2
    form.decode.replica = 3

    const payload = buildDynamoCreatePayload(form, 'core42-hyperloom')

    expect(payload.images).toEqual([form.image, form.image, form.image])
    expect(payload.resources).toEqual([
      { replica: 1, cpu: '4', memory: '16Gi' },
      {
        replica: 2,
        cpu: '64',
        gpu: '8',
        memory: '512Gi',
        sharedMemory: '300Gi',
      },
      {
        replica: 3,
        cpu: '64',
        gpu: '8',
        memory: '512Gi',
        sharedMemory: '300Gi',
      },
    ])
    expect(payload.dynamoOptions).toEqual({
      serviceRoles: ['frontend', 'prefill', 'decode'],
      kvTransferBackend: 'mooncake',
    })
    expect(payload.entryPoints).toHaveLength(3)
    expect(payload.entryPoints[1]).toBe(payload.entryPoints[2])
    expect(decodeFromBase64String(payload.entryPoints[1])).toContain(
      '--disaggregation-transfer-backend mooncake',
    )
    expect(decodeFromBase64String(payload.entryPoints[1])).toMatch(/\n$/)
  })

  it('builds a PD aggregation payload with selected prefill multinodeRoles', () => {
    const form = createDefaultDynamoForm()
    form.enablePd = true
    form.enableAggregation = true
    form.pdAggregationRoles = ['prefill']
    form.prefill.replica = 2
    form.decode.replica = 1
    form.worker.tpSize = 9
    form.worker.epSize = 9

    const payload = buildDynamoCreatePayload(form, 'core42-hyperloom')

    expect(payload.dynamoOptions).toEqual({
      serviceRoles: ['frontend', 'prefill', 'decode'],
      kvTransferBackend: 'nixl',
      multinodeRoles: ['prefill'],
    })
    expect(decodeFromBase64String(payload.entryPoints[1])).toContain('--enable-dp-attention')
  })

  it('rejects PD aggregation roles when the selected role replica is not greater than 1', () => {
    const form = createDefaultDynamoForm()
    form.enablePd = true
    form.enableAggregation = true
    form.pdAggregationRoles = ['decode']
    form.decode.replica = 1
    form.worker.tpSize = 9

    expect(() => buildDynamoCreatePayload(form, 'core42-hyperloom')).toThrow(
      'Aggregation role decode requires replica greater than 1',
    )
  })

  it('rejects aggregation when tp size is not greater than 8', () => {
    const form = createDefaultDynamoForm()
    form.enableAggregation = true
    form.worker.replica = 2
    form.worker.tpSize = 8

    expect(() => buildDynamoCreatePayload(form, 'core42-hyperloom')).toThrow(
      'Aggregation TP size must be greater than 8',
    )
  })

  it('rejects aggregation for single-node worker resources', () => {
    const form = createDefaultDynamoForm()
    form.enableAggregation = true
    form.worker.replica = 1
    form.worker.tpSize = 16

    expect(() => buildDynamoCreatePayload(form, 'core42-hyperloom')).toThrow(
      'Aggregation requires worker replica greater than 1',
    )
  })
})

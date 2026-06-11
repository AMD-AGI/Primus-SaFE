import { describe, expect, it } from 'vitest'
import { decodeFromBase64String } from '@/utils'
import {
  buildOptimusCreatePayload,
  createDefaultOptimusForm,
} from './optimusPayload'

describe('optimusPayload', () => {
  it('uses the Optimus defaults image by default', () => {
    const form = createDefaultOptimusForm()

    expect(form.image).toBe(
      'harbor.core42.primus-safe.amd.com/primussafe/rocserve-sglang:0.1.0-rocm-20260610',
    )
  })

  it('uses kv-aware as the default router policy and allows round-robin', () => {
    const form = createDefaultOptimusForm()
    expect(form.routerPolicy).toBe('kv-aware')

    form.routerPolicy = 'round-robin'
    const payload = buildOptimusCreatePayload(form, 'core42-hyperloom')

    const frontendEntrypoint = decodeFromBase64String(payload.entryPoints[0])
    expect(frontendEntrypoint).toBe(
      'python3 -m rocserve.server --host 0.0.0.0 --port 8000 --router-policy round-robin --router-tokenizer-path /wekafs/models/DeepSeek-R1-0528',
    )
    expect(frontendEntrypoint).not.toContain('--discovery-backend')
    expect(frontendEntrypoint).not.toContain('--request-transport')
  })

  it('builds a single-node Optimus payload with the standard worker entrypoint', () => {
    const form = createDefaultOptimusForm()
    form.displayName = 'optimus-ds-r1-single'

    const payload = buildOptimusCreatePayload(form, 'core42-hyperloom')

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
    expect(payload.optimusOptions).toEqual({
      serviceRoles: ['frontend', 'worker'],
    })
    const frontendEntrypoint = decodeFromBase64String(payload.entryPoints[0])
    const workerEntrypoint = decodeFromBase64String(payload.entryPoints[1])
    expect(frontendEntrypoint).toContain('--router-policy kv-aware')
    expect(workerEntrypoint).toContain('--tp-size 8')
    expect(workerEntrypoint).toContain('--ep-size 8')
  })

  it('builds an aggregation Optimus payload with frontend and worker roles', () => {
    const form = createDefaultOptimusForm()
    form.displayName = 'optimus-ds-r1-agg-nats'
    form.enableAggregation = true
    form.worker.replica = 2

    const payload = buildOptimusCreatePayload(form, 'core42-hyperloom')

    expect(payload).toMatchObject({ workspaceId: 'core42-hyperloom' })
    expect(payload.groupVersionKind).toEqual({ kind: 'OptimusDeployment', version: 'v1' })
    expect(payload.images).toEqual([form.image, form.image])
    expect(payload.resources).toEqual([
      { replica: 1, cpu: '4', memory: '16Gi' },
      {
        replica: 2,
        cpu: '64',
        gpu: '8',
        memory: '256Gi',
        sharedMemory: '200Gi',
      },
    ])
    expect(payload.optimusOptions).toEqual({
      serviceRoles: ['frontend', 'worker'],
    })
    expect(payload.env).toEqual({ HF_HOME: '/data/hf-cache', NCCL_DEBUG: 'INFO' })
    expect(payload.service).toEqual({
      protocol: 'TCP',
      port: 8000,
      targetPort: 8000,
      serviceType: 'ClusterIP',
    })
    const workerEntrypoint = decodeFromBase64String(payload.entryPoints[1])
    expect(workerEntrypoint).toContain('python3 -m rocserve.engine.sglang')
    expect(workerEntrypoint).toContain('--tp-size 16')
    expect(workerEntrypoint).toContain('--ep-size 16')
    expect(workerEntrypoint).toContain('--enable-dp-attention')
    expect(workerEntrypoint).not.toContain('--discovery-backend')
    expect(workerEntrypoint).not.toContain('--request-transport')
    expect(workerEntrypoint).not.toContain('--kv-event-transport')
    expect(workerEntrypoint).not.toContain('--disaggregation-ib-device')
  })

  it('builds a PD Optimus payload with role-specific entrypoints and mori KV backend', () => {
    const form = createDefaultOptimusForm()
    form.displayName = 'optimus-ds-r1-pd-nats'
    form.enablePd = true
    form.image = 'harbor.core42.primus-safe.amd.com/primussafe/rocserve-sglang:0.1.0-rocm-v5'
    form.kvTransferBackend = 'mori'
    form.prefillBackendEngine = 'vllm'
    form.decodeBackendEngine = 'sglang'
    form.prefill.tpSize = 8
    form.prefill.epSize = 8
    form.decode.tpSize = 4
    form.decode.epSize = 4

    const payload = buildOptimusCreatePayload(form, 'core42-hyperloom')

    expect(payload.images).toEqual([form.image, form.image, form.image])
    expect(payload.resources).toEqual([
      { replica: 1, cpu: '4', memory: '16Gi' },
      {
        replica: 1,
        cpu: '64',
        gpu: '8',
        memory: '256Gi',
        sharedMemory: '200Gi',
      },
      {
        replica: 1,
        cpu: '64',
        gpu: '8',
        memory: '256Gi',
        sharedMemory: '200Gi',
      },
    ])
    expect(payload.optimusOptions).toEqual({
      serviceRoles: ['frontend', 'prefill', 'decode'],
      kvTransferBackend: 'mori',
    })
    expect(payload.env).toEqual({ HF_HOME: '/data/hf-cache', NCCL_DEBUG: 'INFO' })
    const prefillEntrypoint = decodeFromBase64String(payload.entryPoints[1])
    const decodeEntrypoint = decodeFromBase64String(payload.entryPoints[2])
    expect(prefillEntrypoint).toContain('python3 -m rocserve.engine.vllm')
    expect(decodeEntrypoint).toContain('python3 -m rocserve.engine.sglang')
    expect(prefillEntrypoint).toContain('--tp-size 8')
    expect(prefillEntrypoint).toContain('--ep-size 8')
    expect(decodeEntrypoint).toContain('--tp-size 4')
    expect(decodeEntrypoint).toContain('--ep-size 4')
    expect(prefillEntrypoint).not.toBe(decodeEntrypoint)
    expect(prefillEntrypoint).toContain('--enable-dp-attention')
    expect(decodeEntrypoint).toContain('--enable-dp-attention')
    expect(prefillEntrypoint).not.toContain('--disaggregation-ib-device')
    expect(decodeEntrypoint).not.toContain('--disaggregation-ib-device')
  })

  it('uses custom Optimus role entrypoints independently', () => {
    const form = createDefaultOptimusForm()
    form.enablePd = true
    form.prefillEntrypoint = 'python3 -m rocserve.engine.sglang --model-path /models/prefill\n'
    form.decodeEntrypoint = 'python3 -m rocserve.engine.sglang --model-path /models/decode\n'

    const payload = buildOptimusCreatePayload(form, 'core42-hyperloom')

    expect(decodeFromBase64String(payload.entryPoints[1])).toBe(form.prefillEntrypoint)
    expect(decodeFromBase64String(payload.entryPoints[2])).toBe(form.decodeEntrypoint)
  })
})

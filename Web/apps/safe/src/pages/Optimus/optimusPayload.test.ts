import { describe, expect, it } from 'vitest'
import { decodeFromBase64String } from '@/utils'
import {
  buildOptimusCreatePayload,
  createDefaultOptimusForm,
} from './optimusPayload'

describe('optimusPayload', () => {
  it('builds an aggregation Optimus payload with frontend and worker roles', () => {
    const form = createDefaultOptimusForm()
    form.displayName = 'optimus-ds-r1-agg-nats'
    form.enableAggregation = true
    form.worker.replica = 2
    form.env = { HF_HOME: '/data/hf-cache', NCCL_DEBUG: 'INFO' }

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

    const frontendEntrypoint = decodeFromBase64String(payload.entryPoints[0])
    const workerEntrypoint = decodeFromBase64String(payload.entryPoints[1])
    expect(frontendEntrypoint).toContain('python3 -m rocserve.server')
    expect(frontendEntrypoint).not.toContain('--router-policy')
    expect(frontendEntrypoint).not.toContain('--discovery-backend')
    expect(frontendEntrypoint).not.toContain('--request-transport')
    expect(frontendEntrypoint).not.toContain('--kv-event-transport')
    expect(workerEntrypoint).toContain('exec python3 -m rocserve.engine.sglang')
    expect(workerEntrypoint).toContain('--tp-size 8')
    expect(workerEntrypoint).toContain('--ep-size 8')
    expect(workerEntrypoint).not.toContain('--discovery-backend')
    expect(workerEntrypoint).not.toContain('--request-transport')
    expect(workerEntrypoint).not.toContain('--kv-event-transport')
    expect(workerEntrypoint).toContain('--enable-kv-events')
    expect(workerEntrypoint).not.toContain('--disaggregation-ib-device')
  })

  it('builds a PD Optimus payload with copied worker entrypoints and mori KV backend', () => {
    const form = createDefaultOptimusForm()
    form.displayName = 'optimus-ds-r1-pd-nats'
    form.enablePd = true
    form.kvTransferBackend = 'mori'

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
    expect(payload.entryPoints).toHaveLength(3)
    expect(payload.entryPoints[1]).toBe(payload.entryPoints[2])
    expect(decodeFromBase64String(payload.entryPoints[1])).toContain(
      '--disaggregation-ib-device roceP29s0',
    )
  })

  it('uses a custom worker entrypoint when the full command is edited', () => {
    const form = createDefaultOptimusForm()
    form.workerEntrypoint = 'exec python3 -m rocserve.engine.sglang --model-path /models/custom\n'

    const payload = buildOptimusCreatePayload(form, 'core42-hyperloom')

    expect(decodeFromBase64String(payload.entryPoints[1])).toBe(form.workerEntrypoint)
  })
})

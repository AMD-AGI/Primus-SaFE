import { describe, expect, it } from 'vitest'
import { decodeFromBase64String } from '@/utils'
import {
  buildOptimusCreatePayload,
  createDefaultOptimusForm,
} from './optimusPayload'

const OPTIMUS_FRONTEND_ENTRYPOINT =
  'cHl0aG9uMyAtbSByb2NzZXJ2ZS5zZXJ2ZXIgLS1ob3N0IDAuMC4wLjAgLS1wb3J0IDgwMDAgLS1yb3V0ZXItcG9saWN5IGt2LWF3YXJlIC0tcm91dGVyLXRva2VuaXplci1wYXRoIC93ZWthZnMvbW9kZWxzL0RlZXBTZWVrLVIxLTA1MjggLS1kaXNjb3ZlcnktYmFja2VuZCBrdWJlcm5ldGVzIC0tcmVxdWVzdC10cmFuc3BvcnQgbmF0cyAtLWt2LWV2ZW50LXRyYW5zcG9ydCBuYXRz'
const OPTIMUS_WORKER_ENTRYPOINT =
  'ZXhlYyBweXRob24zIC1tIHJvY3NlcnZlLmVuZ2luZS5zZ2xhbmcgLS1tb2RlbC1wYXRoIC93ZWthZnMvbW9kZWxzL0RlZXBTZWVrLVIxLTA1MjggLS10cC1zaXplIDggLS1lcC1zaXplIDggLS1hdHRlbnRpb24tYmFja2VuZCBhaXRlciAtLXRydXN0LXJlbW90ZS1jb2RlIC0tbWVtLWZyYWN0aW9uLXN0YXRpYyAwLjc1IC0taG9zdCAwLjAuMC4wIC0tZGlzY292ZXJ5LWJhY2tlbmQga3ViZXJuZXRlcyAtLXJlcXVlc3QtdHJhbnNwb3J0IG5hdHMgLS1rdi1ldmVudC10cmFuc3BvcnQgbmF0cw=='
const OPTIMUS_PD_WORKER_ENTRYPOINT =
  'ZXhlYyBweXRob24zIC1tIHJvY3NlcnZlLmVuZ2luZS5zZ2xhbmcgLS1tb2RlbC1wYXRoIC93ZWthZnMvbW9kZWxzL0RlZXBTZWVrLVIxLTA1MjggLS10cC1zaXplIDggLS1lcC1zaXplIDggLS1hdHRlbnRpb24tYmFja2VuZCBhaXRlciAtLXRydXN0LXJlbW90ZS1jb2RlIC0tbWVtLWZyYWN0aW9uLXN0YXRpYyAwLjc1IC0taG9zdCAwLjAuMC4wIC0tZGlzY292ZXJ5LWJhY2tlbmQga3ViZXJuZXRlcyAtLXJlcXVlc3QtdHJhbnNwb3J0IG5hdHMgLS1rdi1ldmVudC10cmFuc3BvcnQgbmF0cyAtLWRpc2FnZ3JlZ2F0aW9uLWliLWRldmljZSByb2NlcDI5czA='

describe('optimusPayload', () => {
  it('uses the Optimus defaults image by default', () => {
    const form = createDefaultOptimusForm()

    expect(form.image).toBe(
      'harbor.core42.primus-safe.amd.com/primussafe/rocserve-sglang:0.1.0-rocm-defaults',
    )
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
    expect(payload.entryPoints).toEqual([OPTIMUS_FRONTEND_ENTRYPOINT, OPTIMUS_WORKER_ENTRYPOINT])

    const workerEntrypoint = decodeFromBase64String(payload.entryPoints[1])
    expect(workerEntrypoint).toContain('exec python3 -m rocserve.engine.sglang')
    expect(workerEntrypoint).toContain('--tp-size 8')
    expect(workerEntrypoint).toContain('--ep-size 8')
    expect(workerEntrypoint).toContain('--discovery-backend kubernetes')
    expect(workerEntrypoint).toContain('--request-transport nats')
    expect(workerEntrypoint).toContain('--kv-event-transport nats')
    expect(workerEntrypoint).not.toContain('--enable-kv-events')
    expect(workerEntrypoint).not.toContain('--disaggregation-ib-device')
  })

  it('builds a PD Optimus payload with copied worker entrypoints and mori KV backend', () => {
    const form = createDefaultOptimusForm()
    form.displayName = 'optimus-ds-r1-pd-nats'
    form.enablePd = true
    form.image = 'harbor.core42.primus-safe.amd.com/primussafe/rocserve-sglang:0.1.0-rocm-v5'
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
    expect(payload.env).toEqual({ HF_HOME: '/data/hf-cache', NCCL_DEBUG: 'INFO' })
    expect(payload.entryPoints).toEqual([
      OPTIMUS_FRONTEND_ENTRYPOINT,
      OPTIMUS_PD_WORKER_ENTRYPOINT,
      OPTIMUS_PD_WORKER_ENTRYPOINT,
    ])
    expect(decodeFromBase64String(payload.entryPoints[1])).toContain(
      '--disaggregation-ib-device rocep29s0',
    )
  })

  it('uses a custom worker entrypoint when the full command is edited', () => {
    const form = createDefaultOptimusForm()
    form.workerEntrypoint = 'exec python3 -m rocserve.engine.sglang --model-path /models/custom\n'

    const payload = buildOptimusCreatePayload(form, 'core42-hyperloom')

    expect(decodeFromBase64String(payload.entryPoints[1])).toBe(form.workerEntrypoint)
  })
})

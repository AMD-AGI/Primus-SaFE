import { describe, expect, it } from 'vitest'
import { buildAtomResumePayload } from './atomPayload'

describe('buildAtomResumePayload', () => {
  it('keeps env, RDMA, GPU name, and customer labels', () => {
    const payload = buildAtomResumePayload(
      {
        displayName: 'atom-qwen3-14b',
        groupVersionKind: { group: 'amd.com', version: 'v1', kind: 'Atom' },
        priority: 2,
        images: ['image', 'image', 'image'],
        entryPoints: ['router', 'prefill', 'decode'],
        env: { MOONCAKE_DISABLE_HIP_DMABUF: '0', MC_DISABLE_HIP: '1' },
        customerLabels: { team: 'atom' },
        resources: [
          { replica: 1, cpu: '4', memory: '16Gi', sharedMemory: '4Gi', ephemeralStorage: '20Gi' },
          {
            replica: 1,
            cpu: '16',
            gpu: '8',
            gpuName: 'amd.com/gpu',
            memory: '128Gi',
            sharedMemory: '64Gi',
            ephemeralStorage: '50Gi',
            rdmaResource: '1k',
          },
          {
            replica: 1,
            cpu: '16',
            gpu: '8',
            gpuName: 'amd.com/gpu',
            memory: '128Gi',
            sharedMemory: '64Gi',
            ephemeralStorage: '50Gi',
            rdmaResource: '1k',
          },
        ],
        workloadId: 'atom-qwen3-14b-abcde',
        useWorkspaceStorage: true,
      },
      'crusoe-spur-vk',
      [],
    )

    expect(payload.env).toEqual({ MOONCAKE_DISABLE_HIP_DMABUF: '0', MC_DISABLE_HIP: '1' })
    expect(payload.customerLabels).toEqual({ team: 'atom' })
    expect(payload.resources[1]).toMatchObject({ gpuName: 'amd.com/gpu', rdmaResource: '1k' })
    expect(payload.resources[2]).toMatchObject({ gpuName: 'amd.com/gpu', rdmaResource: '1k' })
    expect(payload.resources[0]).not.toHaveProperty('rdmaResource')
  })
})

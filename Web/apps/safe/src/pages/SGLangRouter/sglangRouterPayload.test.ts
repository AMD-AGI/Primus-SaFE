import { describe, expect, it } from 'vitest'
import { buildSGLangRouterResumePayload, buildSGLangRouterWorkloadBody, initialSGLangRouterForm } from './sglangRouterPayload'

describe('buildSGLangRouterResumePayload', () => {
  it('keeps env, RDMA, GPU name, and customer labels', () => {
    const payload = buildSGLangRouterResumePayload(
      {
        displayName: 'atom-qwen3-14b',
        groupVersionKind: { group: 'amd.com', version: 'v1', kind: 'SGLangRouter' },
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
      'workspace-a',
      [],
    )

    expect(payload.env).toEqual({ MOONCAKE_DISABLE_HIP_DMABUF: '0', MC_DISABLE_HIP: '1' })
    expect(payload.customerLabels).toEqual({ team: 'atom' })
    expect(payload.resources[1]).toMatchObject({ gpuName: 'amd.com/gpu', rdmaResource: '1k' })
    expect(payload.resources[2]).toMatchObject({ gpuName: 'amd.com/gpu', rdmaResource: '1k' })
    expect(payload.resources[0]).not.toHaveProperty('rdmaResource')
  })
})

describe('buildSGLangRouterWorkloadBody', () => {
  it('sends gpu name, RDMA, shared memory, and a positive node port', () => {
    const form = initialSGLangRouterForm()
    form.displayName = 'sglang'
    form.serviceType = 'NodePort'
    form.serviceNodePort = 32081
    form.roles.decode.gpu = '8'
    form.roles.decode.gpuName = 'amd.com/gpu'
    form.roles.decode.rdmaResource = '1k'
    form.roles.decode.sharedMemory = '32'

    const body = buildSGLangRouterWorkloadBody(form)

    expect(body.service).toMatchObject({ serviceType: 'NodePort', nodePort: 32081 })
    expect(body.resources[2]).toMatchObject({
      sharedMemory: '32Gi',
      gpuName: 'amd.com/gpu',
      rdmaResource: '1k',
    })
  })
})

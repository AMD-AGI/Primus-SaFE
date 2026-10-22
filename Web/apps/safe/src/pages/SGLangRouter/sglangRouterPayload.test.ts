import { describe, expect, it } from 'vitest'
import {
  buildSGLangRouterResumePayload,
  buildSGLangRouterWorkloadBody,
  initialSGLangRouterForm,
  mergeSGLangRouterResumeResources,
} from './sglangRouterPayload'

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

  it('keeps a NodePort and the original target port', () => {
    const payload = buildSGLangRouterResumePayload(
      {
        displayName: 'sglang',
        service: { serviceType: 'NodePort', port: 8000, targetPort: 8000, nodePort: 32081 },
      },
      'workspace-a',
      [{ id: 'secret-a' }],
    )

    expect(payload.service).toMatchObject({
      serviceType: 'NodePort',
      targetPort: 8000,
      nodePort: 32081,
    })
    expect(payload.secrets).toEqual([{ id: 'secret-a' }])
  })

  it('drops RDMA and GPU name cleared in the resume form', () => {
    const merged = mergeSGLangRouterResumeResources(
      [{ gpu: '8', gpuName: 'amd.com/gpu', sharedMemory: '64Gi', rdmaResource: '1k', cpu: '16' }],
      [{ cpu: '16', memory: '128Gi' }],
    )

    expect(merged[0]).not.toHaveProperty('rdmaResource')
    expect(merged[0]).not.toHaveProperty('gpuName')
    expect(merged[0]).not.toHaveProperty('gpu')
    expect(merged[0]).toMatchObject({ cpu: '16', memory: '128Gi' })
  })
})

describe('buildSGLangRouterWorkloadBody', () => {
  it('sends gpu name, RDMA, shared memory, and a positive node port', () => {
    const form = initialSGLangRouterForm()
    form.displayName = 'sglang'
    form.serviceType = 'NodePort'
    form.serviceNodePort = 32081
    form.serviceTargetPort = 9000
    form.roles.decode.gpu = '8'
    form.roles.decode.gpuName = 'amd.com/gpu'
    form.roles.decode.rdmaResource = '1k'
    form.roles.decode.sharedMemory = '32'

    const body = buildSGLangRouterWorkloadBody(form)

    expect(body.service).toMatchObject({ serviceType: 'NodePort', nodePort: 32081, targetPort: 9000 })
    expect(body.resources[2]).toMatchObject({
      sharedMemory: '32Gi',
      gpuName: 'amd.com/gpu',
      rdmaResource: '1k',
    })
  })
})

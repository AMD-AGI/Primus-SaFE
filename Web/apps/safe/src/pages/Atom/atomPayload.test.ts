import { describe, expect, it } from 'vitest'
import { quantityToGi } from '@/utils'
import { buildAtomResumePayload, buildAtomWorkloadBody, initialAtomForm, mergeAtomResumeResources } from './atomPayload'

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
      'workspace-a',
      [],
    )

    expect(payload.env).toEqual({ MOONCAKE_DISABLE_HIP_DMABUF: '0', MC_DISABLE_HIP: '1' })
    expect(payload.customerLabels).toEqual({ team: 'atom' })
    expect(payload.resources[1]).toMatchObject({ gpuName: 'amd.com/gpu', rdmaResource: '1k' })
    expect(payload.resources[2]).toMatchObject({ gpuName: 'amd.com/gpu', rdmaResource: '1k' })
    expect(payload.resources[0]).not.toHaveProperty('rdmaResource')
  })

  it('drops RDMA and GPU name cleared in the resume form', () => {
    const merged = mergeAtomResumeResources(
      [
        { replica: 1, cpu: '4', memory: '16Gi', ephemeralStorage: '20Gi' },
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
      [
        { replica: 1, cpu: '4', memory: '16Gi', ephemeralStorage: '20Gi' },
        { replica: 1, cpu: '16', memory: '128Gi', ephemeralStorage: '50Gi' },
      ],
    )

    expect(merged[1]).not.toHaveProperty('rdmaResource')
    expect(merged[1]).not.toHaveProperty('gpuName')
    expect(merged[1]).not.toHaveProperty('gpu')
    expect(merged[1]).not.toHaveProperty('sharedMemory')
    expect(merged[1]).toMatchObject({ cpu: '16', memory: '128Gi', ephemeralStorage: '50Gi' })
  })

  it('keeps RDMA and GPU name when the resume form still sets them', () => {
    const merged = mergeAtomResumeResources(
      [{ gpu: '4', gpuName: 'old', sharedMemory: '8Gi', rdmaResource: '1k' }],
      [{ gpu: '8', gpuName: 'amd.com/gpu', sharedMemory: '16Gi', rdmaResource: '2k' }],
    )

    expect(merged[0]).toMatchObject({
      gpu: '8',
      gpuName: 'amd.com/gpu',
      sharedMemory: '16Gi',
      rdmaResource: '2k',
    })
  })
})

describe('quantityToGi', () => {
  it('converts Mi and Ti into Gi numbers', () => {
    expect(quantityToGi('16Gi')).toBe('16')
    expect(quantityToGi('4096Mi')).toBe('4')
    expect(quantityToGi('1Ti')).toBe('1024')
  })
})

describe('buildAtomWorkloadBody', () => {
  it('sends gpu name, RDMA, shared memory, and a positive node port', () => {
    const form = initialAtomForm()
    form.displayName = 'atom'
    form.serviceType = 'NodePort'
    form.serviceNodePort = 32080
    form.roles.prefill.gpu = '8'
    form.roles.prefill.gpuName = 'amd.com/gpu'
    form.roles.prefill.rdmaResource = '1k'
    form.roles.prefill.memory = '4'
    form.roles.prefill.sharedMemory = '64'

    const body = buildAtomWorkloadBody(form)

    expect(body.service).toMatchObject({ serviceType: 'NodePort', nodePort: 32080 })
    expect(body.resources[1]).toMatchObject({
      memory: '4Gi',
      sharedMemory: '64Gi',
      gpuName: 'amd.com/gpu',
      rdmaResource: '1k',
    })
    expect(body.resources[0]).not.toHaveProperty('gpuName')
  })
})

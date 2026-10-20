import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const dialogSource = readFileSync(new URL('./AddDialog.vue', import.meta.url), 'utf-8')

describe('Atom AddDialog', () => {
  it('propagates drawer close events to the parent', () => {
    expect(dialogSource).toContain(
      '@update:model-value="(open: boolean) => emit(\'update:visible\', open)"',
    )
  })

  it('requires a node port when NodePort is selected', () => {
    expect(dialogSource).toContain('v-if="form.serviceType === \'NodePort\'"')
    expect(dialogSource).toContain('v-model="form.serviceNodePort"')
    expect(dialogSource).toContain("callback(new Error('Node port is required'))")
  })

  it('exposes GPU name, RDMA, and shared memory fields for each role', () => {
    expect(dialogSource).toContain('v-model="form.roles[role].gpuName"')
    expect(dialogSource).toContain('v-model="form.roles[role].rdmaResource"')
    expect(dialogSource).toContain('v-model="form.roles[role].sharedMemory"')
  })

  it('normalizes Kubernetes memory quantities when loading a workload', () => {
    expect(dialogSource).toContain('memory: quantityToGi(resource.memory)')
    expect(dialogSource).toContain('ephemeralStorage: quantityToGi(resource.ephemeralStorage)')
    expect(dialogSource).toContain('sharedMemory: quantityToGi(resource.sharedMemory)')
  })
})

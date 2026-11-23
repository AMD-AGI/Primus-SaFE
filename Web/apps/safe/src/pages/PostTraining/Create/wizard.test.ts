import { describe, expect, it, vi } from 'vitest'
import { diagnosticsByStep, newRequest, requestFromRun } from './wizard'
import { fieldStep, type RunDetail } from '@/services/postTraining'

vi.mock('@/services/request', () => ({ default: {} }))

describe('fieldStep', () => {
  it('sends each diagnostic to the step that edits its field', () => {
    expect(fieldStep('modelId')).toBe(1)
    expect(fieldStep('choices.advantageEstimator')).toBe(1)
    expect(fieldStep('datasetId')).toBe(2)
    expect(fieldStep('choices.fieldMap.output')).toBe(2)
    expect(fieldStep('choices.valSplit')).toBe(2)
    expect(fieldStep('resources.nodes')).toBe(3)
    expect(fieldStep('choices.rolloutGpus')).toBe(3)
    expect(fieldStep('choices.tp')).toBe(3)
    expect(fieldStep('choices.saveInterval')).toBe(4)
    expect(fieldStep('choices.pd.prefillGpus')).toBe(4)
    expect(fieldStep('resources.image')).toBe(5)
    expect(fieldStep(undefined)).toBe(0)
  })

  it('groups diagnostics by step', () => {
    const groups = diagnosticsByStep([
      { level: 'error', field: 'datasetId', message: 'Choose a training dataset.' },
      { level: 'error', field: 'choices.fieldMap.output', message: 'Map output.' },
      { level: 'error', message: 'Something else.' },
    ])
    expect(groups.get(2)?.length).toBe(2)
    expect(groups.get(0)?.length).toBe(1)
  })
})

describe('newRequest', () => {
  it('starts with one node, one retry, the medium priority it shows and HF export', () => {
    const req = newRequest('ws1')
    expect(req.workspace).toBe('ws1')
    expect(req.resources).toEqual({ nodes: 1, gpusPerNode: 8, maxRetry: 1, priority: 1 })
    expect(req.export.hf).toBe(true)
  })
})

describe('requestFromRun', () => {
  const run = {
    runId: 'pt-1',
    workspace: 'ws',
    displayName: 'qwen-sft',
    trainType: 'sft',
    method: 'full',
    framework: 'slime',
    baseModelId: 'm-old',
    datasetId: 'd-old',
    nodeCount: 2,
    gpuPerNode: 8,
    recipe: {
      model: { id: 'm1', spec: 'qwen3-8B' },
      data: { trainId: 'd1' },
      choices: { lr: 0.00002, tp: 2 },
      layout: { nodes: 2 },
      output: { exportHf: true },
    },
  } as unknown as RunDetail

  it('turns a run snapshot into a request', () => {
    const req = requestFromRun(run)
    expect(req.modelId).toBe('m1')
    expect(req.datasetId).toBe('d1')
    expect(req.choices).toEqual({ lr: 0.00002, tp: 2 })
    expect(req.resources).toEqual({ nodes: 2, gpusPerNode: 8 })
    expect(req.tracking).toEqual({ wandb: {} })
  })
})

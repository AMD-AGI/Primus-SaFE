import { describe, expect, it, vi } from 'vitest'
import {
  applyTemplate,
  diagnosticsByStep,
  newRequest,
  requestFromRun,
  templateContent,
} from './wizard'
import { fieldStep, type RunDetail, type TemplateItem } from '@/services/postTraining'

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
    expect(fieldStep('choices.lora.experts')).toBe(4)
    expect(fieldStep('resources.image')).toBe(5)
    expect(fieldStep('evaluation.benchmarks')).toBe(5)
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
  it('starts with one node, one retry and HF export', () => {
    const req = newRequest('ws1')
    expect(req.workspace).toBe('ws1')
    expect(req.resources).toEqual({ nodes: 1, gpusPerNode: 8, maxRetry: 1 })
    expect(req.export.hf).toBe(true)
  })
})

describe('templates', () => {
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
      evaluation: { enabled: true, benchmarks: ['gsm8k'] },
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

  it('keeps the model and data only when asked, and never the image', () => {
    const req = { ...requestFromRun(run), resources: { nodes: 2, gpusPerNode: 8, image: 'img:9' } }
    const options = templateContent(req, { model: false, data: false })
    expect(options.modelId).toBeUndefined()
    expect(options.datasetId).toBeUndefined()
    expect(options.resources).toEqual({ nodes: 2, gpusPerNode: 8 })
    const all = templateContent(req, { model: true, data: true })
    expect([all.modelId, all.datasetId]).toEqual(['m1', 'd1'])
  })

  it('applies a template and resets parallel settings for another spec', () => {
    const tpl = {
      templateId: 'tpl-1',
      trainType: 'sft',
      method: 'full',
      spec: 'qwen3-8B',
      content: { choices: { lr: 0.00002, tp: 2, pp: 2 }, resources: { nodes: 2 } },
    } as unknown as TemplateItem
    const form = { ...newRequest('ws'), modelId: 'm-4b' }
    expect(applyTemplate(form, tpl, () => 'qwen3-4B')).toBe(true)
    expect(form.choices).toEqual({ lr: 0.00002 })
    expect(form.templateId).toBe('tpl-1')
    // A template without a model keeps the chosen one.
    expect(form.modelId).toBe('m-4b')
    const same = { ...newRequest('ws'), modelId: 'm-8b' }
    expect(applyTemplate(same, tpl, () => 'qwen3-8B')).toBe(false)
    expect(same.choices.tp).toBe(2)
  })
})

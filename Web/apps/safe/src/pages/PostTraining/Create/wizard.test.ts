import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope, nextTick } from 'vue'
import {
  MAX_NAME_LENGTH,
  cloneName,
  createWizard,
  diagnosticsByStep,
  newRequest,
  requestFromRun,
} from './wizard'
import { fieldStep, type RunDetail } from '@/services/postTraining'
import request from '@/services/request'

vi.mock('@/services/request', () => ({ default: { get: vi.fn(), post: vi.fn() } }))

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

describe('cloneName', () => {
  it('adds -clone within the name limit of the API', () => {
    expect(cloneName('qwen-sft')).toBe('qwen-sft-clone')
    const long = cloneName('a'.repeat(MAX_NAME_LENGTH))
    expect(long).toHaveLength(MAX_NAME_LENGTH)
    expect(long.endsWith('-clone')).toBe(true)
  })
})

describe('createWizard', () => {
  const post = vi.mocked(request.post)
  const get = vi.mocked(request.get)

  /** A preview answer the test resolves when it wants. */
  const deferred = () => {
    let resolve!: (v: unknown) => void
    const promise = new Promise((r) => (resolve = r))
    return { promise, resolve }
  }

  /** A wizard in its own scope, as a page's setup gives it; stopping the scope disposes of it. */
  const open = () => {
    const scope = effectScope()
    const w = scope.run(() => createWizard('ws1'))!
    return { w, scope }
  }

  beforeEach(() => {
    post.mockReset()
    get.mockReset()
  })

  it('holds Submit back from an edit until the preview of the latest request lands', async () => {
    const { w, scope } = open()
    const answer = deferred()
    post.mockReturnValueOnce(answer.promise as never)
    w.form.modelId = 'm1'
    await nextTick()
    // The edit waits for the debounce.
    expect(w.previewPending.value).toBe(true)
    const done = w.runPreview()
    // The preview is in flight.
    expect(w.previewPending.value).toBe(true)
    answer.resolve({ diagnostics: [{ level: 'warning', message: 'FP8 rollout' }], verifiedRuns: 0 })
    await done
    expect(w.previewPending.value).toBe(false)
    expect(w.warnings.value.map((d) => d.message)).toEqual(['FP8 rollout'])
    scope.stop()
  })

  it('keeps only the answer to the latest preview', async () => {
    const { w, scope } = open()
    w.form.modelId = 'm1'
    const first = deferred()
    const second = deferred()
    post.mockReturnValueOnce(first.promise as never).mockReturnValueOnce(second.promise as never)
    const a = w.runPreview()
    const b = w.runPreview()
    second.resolve({ diagnostics: [], verifiedRuns: 2 })
    await b
    first.resolve({ diagnostics: [], verifiedRuns: 1 })
    await a
    expect(w.preview.value?.verifiedRuns).toBe(2)
    expect(w.checking.value).toBe(false)
    scope.stop()
  })

  it("shows the API's message when the preview fails", async () => {
    const { w, scope } = open()
    w.form.modelId = 'm1'
    post.mockRejectedValueOnce({
      response: { status: 400, data: { errorMessage: 'invalid request body' } },
    })
    await w.runPreview()
    expect(w.previewError.value).toBe('invalid request body')
    scope.stop()
  })

  it('drops the answer of a preview in flight once disposed', async () => {
    const { w, scope } = open()
    w.form.modelId = 'm1'
    const answer = deferred()
    post.mockReturnValueOnce(answer.promise as never)
    const done = w.runPreview()
    scope.stop()
    answer.resolve({ diagnostics: [], verifiedRuns: 1 })
    await done
    expect(w.preview.value).toBeUndefined()
  })

  it('reads the models for the train type and method, again only when they change', async () => {
    const { w, scope } = open()
    get.mockResolvedValue({ items: [] } as never)
    await w.loadModels()
    expect(get).toHaveBeenLastCalledWith('/post-training/models', {
      params: { workspace: 'ws1', trainType: 'sft', method: 'full' },
      skipErrorHandler: true,
    })
    await w.loadModels()
    expect(get).toHaveBeenCalledTimes(1)
    w.form.trainType = 'rl'
    await w.loadModels()
    expect(get).toHaveBeenCalledTimes(2)
    expect(get).toHaveBeenLastCalledWith('/post-training/models', {
      params: { workspace: 'ws1', trainType: 'rl', method: 'full' },
      skipErrorHandler: true,
    })
    scope.stop()
  })
})

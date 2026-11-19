import { describe, expect, it, vi } from 'vitest'
import {
  assignPanels,
  betterOf,
  cleanupHintBytes,
  matchName,
  panelsOf,
  configDiff,
  fieldsDiffer,
  flattenChanges,
  formatCommand,
  formatDuration,
  freezeState,
  freezingText,
  scatterTooltip,
  groupSamples,
  lastLine,
  nestChanges,
  parseJSON,
  regexErrors,
  rewardOf,
  significance,
  smoothPoints,
  splitTruncated,
  stageDurations,
  stageSegments,
  stallNote,
  timeOf,
  trainableText,
  postTrainingEntryReasons,
  exportPhase,
  curvesDue,
  fillsGapsAfterEnd,
  EXPORT_REGISTER_WAIT,
  backToRuns,
  experimentNamesProblem,
  postTrainingIsOn,
} from './index'
import type { Router } from 'vue-router'
import request from '@/services/request'

vi.mock('@/services/request', () => ({ default: { get: vi.fn() } }))
import type { PostTrainingRun, RunSample } from './types'

type StageRun = Pick<
  PostTrainingRun,
  'status' | 'stage' | 'failedStage' | 'evalEnabled' | 'convertWorkloadId'
>

const states = (run: StageRun) => stageSegments(run).map((s) => `${s.stage}:${s.state}`)

describe('stageSegments', () => {
  it('leaves every stage empty while queued and hatches Evaluate without evaluation', () => {
    expect(states({ status: 'Pending', stage: 'queued', evalEnabled: false })).toEqual([
      'prepare:pending',
      'convert:pending',
      'check:pending',
      'train:pending',
      'export:pending',
      'register:pending',
      'eval:skipped',
    ])
  })

  it('marks earlier stages done and the current one active', () => {
    expect(states({ status: 'Running', stage: 'train', evalEnabled: true }).slice(0, 5)).toEqual([
      'prepare:done',
      'convert:done',
      'check:done',
      'train:active',
      'export:pending',
    ])
  })

  it('marks the failed stage red and converts first with a convert workload', () => {
    expect(
      states({
        status: 'Failed',
        stage: 'train',
        failedStage: 'check',
        evalEnabled: true,
        convertWorkloadId: 'cw',
      }),
    ).toEqual([
      'convert:done',
      'prepare:done',
      'check:failed',
      'train:pending',
      'export:pending',
      'register:pending',
      'eval:pending',
    ])
  })

  it('finishes every stage of a done run and stops at the stage a stopped run reached', () => {
    expect(
      states({ status: 'Succeeded', stage: 'done', evalEnabled: true }).every((s) =>
        s.endsWith(':done'),
      ),
    ).toBe(true)
    expect(states({ status: 'Stopped', stage: 'export', evalEnabled: false })[4]).toBe(
      'export:stopped',
    )
  })
})

describe('helpers', () => {
  it('reads JSON columns and nullable times', () => {
    expect(parseJSON<{ points: number }>('{"points":3}')?.points).toBe(3)
    expect(parseJSON('not json')).toBeUndefined()
    expect(timeOf({ Time: '2026-11-03T10:00:00Z', Valid: true })).toBe('2026-11-03T10:00:00Z')
    expect(timeOf({ Time: '0001-01-01T00:00:00Z', Valid: false })).toBeUndefined()
  })

  it('formats durations', () => {
    expect(formatDuration(5 * 3600 + 10 * 60)).toBe('5h 10m')
    expect(formatDuration(720)).toBe('12m')
    expect(formatDuration(40)).toBe('40s')
  })
})

describe('smoothPoints', () => {
  it('keeps the first point and moves toward later ones', () => {
    const smoothed = smoothPoints(
      [
        [1, 10],
        [2, 0],
        [3, 0],
      ],
      0.5,
    )
    expect(smoothed[0]).toEqual([1, 10])
    expect(smoothed[1][1]).toBeCloseTo(10 / 3)
    expect(smoothed[2][1]).toBeLessThan(smoothed[1][1])
    expect(smoothPoints([[1, 3]], 0)).toEqual([[1, 3]])
  })
})

describe('stageDurations', () => {
  it('times each stage until the next one starts', () => {
    const d = stageDurations(
      [
        { stage: 'queued', time: '2026-11-03T10:00:00Z' },
        { stage: 'prepare', time: '2026-11-03T10:05:00Z' },
        { stage: 'train', time: '2026-11-03T10:06:00Z', event: 'preempted' },
        { stage: 'train', time: '2026-11-03T10:10:00Z' },
        { stage: 'train', time: '2026-11-03T10:30:00Z', attempt: '2', resumeStep: 600 },
      ],
      Date.parse('2026-11-03T11:00:00Z'),
    )
    expect((d.get('queued')!.end - d.get('queued')!.start) / 60000).toBe(5)
    expect((d.get('prepare')!.end - d.get('prepare')!.start) / 60000).toBe(5)
    expect((d.get('train')!.end - d.get('train')!.start) / 60000).toBe(50)
  })
})

describe('cleanupHintBytes', () => {
  const now = Date.parse('2026-11-10T00:00:00Z')
  const run = {
    endTime: { Time: '2026-11-01T00:00:00Z', Valid: true },
    diskUsage: '{"checkpoints":3000,"criticCheckpoints":1000,"finalModel":500}',
    cleanupRequest: '',
    stageHistory: '[]',
  }

  it('counts the checkpoints of a run ended a week ago', () => {
    expect(cleanupHintBytes(run, now)).toBe(4000)
  })

  it('stays quiet for recent, cleaned or cleaning runs', () => {
    expect(
      cleanupHintBytes({ ...run, endTime: { Time: '2026-11-08T00:00:00Z', Valid: true } }, now),
    ).toBe(0)
    expect(cleanupHintBytes({ ...run, cleanupRequest: '{"keep":"none"}' }, now)).toBe(0)
    expect(
      cleanupHintBytes(
        { ...run, stageHistory: [{ stage: 'done', time: '', event: 'checkpoints-cleaned' }] },
        now,
      ),
    ).toBe(0)
  })
})

describe('stallNote', () => {
  const now = Date.parse('2026-11-10T01:00:00Z')
  const since = { Time: '2026-11-10T00:18:00Z', Valid: true }

  it('names the stall of a running run', () => {
    expect(
      stallNote({ status: 'Running', stalledSince: since, message: 'No training progress' }, now),
    ).toBe('No progress for 42 min')
    expect(
      stallNote(
        {
          status: 'Running',
          stalledSince: since,
          message: 'The launcher has not reported for 9 minutes',
        },
        now,
      ),
    ).toBe('Launcher not reporting')
    expect(stallNote({ status: 'Failed', stalledSince: since }, now)).toBe('')
  })
})

describe('configDiff', () => {
  it('lines up the recipe fields, hash first, and marks the ones that differ', () => {
    const rows = configDiff([
      {
        choices: { lr: 1e-5, fieldMap: { prompt: 'q' } },
        output: { base: '/a' },
        provenance: { recipeHash: 'abcdef123' },
      },
      {
        choices: { lr: 2e-5, fieldMap: { prompt: 'q' } },
        output: { base: '/b' },
        provenance: { recipeHash: '0123456789' },
      },
      undefined,
    ])
    expect(rows[0]).toEqual({
      field: 'provenance.recipeHash',
      label: 'Recipe hash',
      values: ['abcdef1', '0123456', '—'],
      differs: true,
    })
    const lr = rows.find((r) => r.field === 'choices.lr')!
    expect(lr.label).toBe('Learning rate')
    expect(lr.values).toEqual(['0.00001', '0.00002', '—'])
    expect(rows.some((r) => r.field === 'output.base')).toBe(false)
    expect(configDiff([{ runId: 'pt-a' }, { runId: 'pt-b' }])).toEqual([])
    expect([fieldsDiffer(1), fieldsDiffer(2)]).toEqual(['1 field differs', '2 fields differ'])
    expect(configDiff([{ a: [1, 2] }, { a: [1, 2] }])).toEqual([
      { field: 'a', label: 'a', values: ['1, 2', '1, 2'], differs: false },
    ])
  })
})

describe('significance', () => {
  it('needs a difference larger than the re-evaluation gap and two standard errors', () => {
    const small = significance([0.3, 0.267], 0.333, 30)
    expect(small.threshold).toBeCloseTo(0.238, 2)
    expect(small.significant).toBe(false)
    const large = significance([0.812, 0.806], 0.866, 500)
    expect(large.diff).toBeCloseTo(0.057, 3)
    expect(large.significant).toBe(true)
    expect(significance([0.5, 0.4], 0.52).threshold).toBeCloseTo(0.1, 5)
  })
})

describe('variant changes', () => {
  it('flattens and nests dotted keys', () => {
    const changes = { choices: { lr: 2e-6, lora: { rank: 32 } }, datasetId: 'd2' }
    const flat = flattenChanges(changes)
    expect(flat).toEqual({ 'choices.lr': 2e-6, 'choices.lora.rank': 32, datasetId: 'd2' })
    expect(nestChanges(flat)).toEqual(changes)
  })
})

describe('panels', () => {
  const panels = [
    { id: 'reward', series: ['rollout/raw_reward', 'eval/*'] },
    { id: 'passrate', series: ['passrate/pass@*', 'eval/*-pass@*'], when: 'present' },
    {
      id: 'length',
      band: ['rollout/response_len/min', 'rollout/response_len/max'],
      series: ['rollout/response_len/median', 'eval/*-truncated_ratio'],
    },
    { id: 'time', series: ['perf/rollout_time'], stack: true, rest: 'perf/step_time' },
    { id: 'staleness', series: ['rollout/staleness/mean'], when: 'async' },
  ]

  it('matches within one segment', () => {
    expect(matchName('eval/*', 'eval/aime')).toBe(true)
    expect(matchName('eval/*', 'eval/val/loss')).toBe(false)
    expect(matchName('eval/*-pass@*', 'eval/aime-pass@4')).toBe(true)
    expect(matchName('train/lr-pg_*', 'train/lr-pg_0')).toBe(true)
  })

  it('puts a metric in the most specific panel only', () => {
    const names = [
      'rollout/raw_reward',
      'eval/aime',
      'eval/aime-pass@4',
      'eval/aime-truncated_ratio',
      'rollout/response_len/min',
      'rollout/response_len/max',
      'rollout/response_len/median',
      'perf/rollout_time',
      'perf/step_time',
      'train/odd',
    ]
    const { byPanel, unassigned } = assignPanels(panels, names)
    expect(byPanel.get('reward')!.series).toEqual(['rollout/raw_reward', 'eval/aime'])
    expect(byPanel.get('passrate')!.series).toEqual(['eval/aime-pass@4'])
    expect(byPanel.get('length')!.band).toEqual([
      'rollout/response_len/min',
      'rollout/response_len/max',
    ])
    expect(byPanel.get('length')!.series).toEqual([
      'rollout/response_len/median',
      'eval/aime-truncated_ratio',
    ])
    expect(byPanel.get('time')!.rest).toBe('perf/step_time')
    expect(unassigned).toEqual(['train/odd'])
  })

  it('shows conditional panels only when they apply', () => {
    const shown = (names: string[], run = {}) =>
      panelsOf(panels, names, run).shown.map((p) => p.panel.id)
    expect(shown(['rollout/raw_reward'])).toEqual(['reward'])
    expect(shown(['passrate/pass@1', 'rollout/staleness/mean'])).toEqual(['passrate'])
    expect(shown(['rollout/staleness/mean'], { schedule: 'async' })).toEqual(['staleness'])
  })

  it('takes the most specific better direction', () => {
    const better = { 'eval/*': 'higher', 'eval/*-truncated_ratio': 'lower' } as const
    expect(betterOf(better, 'eval/aime')).toBe('higher')
    expect(betterOf(better, 'eval/aime-truncated_ratio')).toBe('lower')
    expect(betterOf(better, 'train/loss')).toBeUndefined()
  })
})

describe('samples', () => {
  const sample = (group: number, index: number, reward?: number): RunSample => ({
    id: index,
    runId: 'pt-x',
    kind: 'rollout',
    step: 3,
    dataset: '',
    group,
    index,
    prompt: 'user: 2+2?',
    response: `thinking\n\nthe answer is ${index}\n`,
    label: { String: '4', Valid: true },
    reward: { Float64: reward ?? 0, Valid: reward !== undefined },
    rewardDetail: { String: '', Valid: false },
    status: 'completed',
    tokens: 10,
    promptChars: 10,
    responseChars: 20,
    wallTime: '',
  })

  it('groups responses and finds groups without a learning signal', () => {
    const groups = groupSamples([
      sample(17, 0, 1),
      sample(17, 1, 0),
      sample(42, 2, 1),
      sample(42, 3, 1),
    ])
    expect(groups.map((g) => g.group)).toEqual([17, 42])
    expect(groups[0]).toMatchObject({
      rewardMean: 0.5,
      correct: 1,
      wrong: 1,
      noSignal: false,
      label: '4',
    })
    expect(groups[1].noSignal).toBe(true)
    expect(groupSamples([sample(1, 0)])[0].rewardMean).toBeNull()
  })

  it('splits the launcher note out of a truncated response', () => {
    expect(splitTruncated('Let me … [2180 characters omitted] … check again')).toEqual({
      head: 'Let me',
      omitted: 2180,
      tail: 'check again',
    })
    expect(splitTruncated('short')).toBeNull()
    expect(lastLine('a\n\n  b  \n')).toBe('b')
  })

  it('shows the keyed reward with the dict as detail', () => {
    const s = {
      ...sample(1, 0, 0.12345),
      rewardDetail: { String: '{"score":0.12345}', Valid: true },
    }
    expect(rewardOf(s)).toEqual({ value: '0.123', detail: '{"score":0.12345}' })
    expect(rewardOf(sample(1, 0)).value).toBe('—')
  })
})

describe('postTrainingIsOn', () => {
  it('takes only a 404 for Post Training off', async () => {
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 404 } })
    expect(await postTrainingIsOn('ws1')).toBe(false)
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 503 } })
    expect(await postTrainingIsOn('ws1')).toBe(true)
    vi.mocked(request.get).mockResolvedValueOnce({ items: [] })
    expect(await postTrainingIsOn('ws1')).toBe(true)
  })
})

describe('postTrainingEntryReasons', () => {
  it('offers no entry where Post Training is off', async () => {
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 404 } })
    expect(await postTrainingEntryReasons('ws1')).toBeUndefined()
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 503 } })
    expect((await postTrainingEntryReasons('ws1'))?.('m1')).toBe('Post Training is unavailable.')
  })

  it('names a model missing from the list as not ready here', async () => {
    vi.mocked(request.get).mockResolvedValueOnce({
      items: [{ modelId: 'm1', supported: true }],
    })
    const reasonOf = await postTrainingEntryReasons('ws1')
    expect(reasonOf?.('m1')).toBe('')
    expect(reasonOf?.('m2')).toBe('The model is not ready in this workspace.')
  })
})

describe('experimentNamesProblem', () => {
  it('holds every run name of the experiment to what the backend takes', () => {
    expect(experimentNamesProblem('qwen3-sft-ab', ['lr2e-6'])).toBe('')
    expect(experimentNamesProblem('qwen3-sft-ab', ['LR'])).toContain('Variant name "LR"')
    expect(experimentNamesProblem('e'.repeat(96), ['v'.repeat(32)])).toContain(
      'Shorten the experiment name',
    )
    expect(experimentNamesProblem('e'.repeat(119), ['v'])).toBe('')
    expect(experimentNamesProblem('e'.repeat(120), ['v'])).toContain(`${'e'.repeat(120)}-baseline`)
  })

  it('names a variant that the backend would refuse as a second run of one name', () => {
    expect(experimentNamesProblem('qwen3-sft-ab', ['lr2e-6', 'lr2e-6'])).toContain(
      '"lr2e-6" appears twice',
    )
    expect(experimentNamesProblem('qwen3-sft-ab', ['baseline'])).toContain('named "baseline"')
  })
})

describe('backToRuns', () => {
  it('returns to the list it came from, its filters kept', () => {
    const router = { back: vi.fn(), push: vi.fn() }
    const from = (state: unknown) => vi.stubGlobal('window', { history: { state } })
    from({ back: '/post-training?status=Running&page=2' })
    backToRuns(router as unknown as Router)
    expect(router.back).toHaveBeenCalledOnce()
    from({ back: '/model-square/detail/m1' })
    backToRuns(router as unknown as Router)
    from(null)
    backToRuns(router as unknown as Router)
    expect(router.push).toHaveBeenCalledTimes(2)
    expect(router.push).toHaveBeenCalledWith('/post-training')
    vi.unstubAllGlobals()
  })
})

describe('exportPhase', () => {
  it('ends an export whose model failed or never came', () => {
    expect(exportPhase({ step: 1, modelId: 'm', modelPhase: 'Pending' })).toBe('running')
    expect(exportPhase({ step: 1, modelId: 'm', modelPhase: 'Ready' })).toBe('done')
    expect(exportPhase({ step: 1, modelId: 'm', modelPhase: 'Failed' })).toBe('failed')
    expect(exportPhase({ step: 1, status: 'Stopped' })).toBe('failed')
    expect(exportPhase({ step: 1, status: 'Succeeded' }, 0, EXPORT_REGISTER_WAIT - 1)).toBe(
      'running',
    )
    expect(exportPhase({ step: 1, status: 'Succeeded' }, 0, EXPORT_REGISTER_WAIT)).toBe(
      'unregistered',
    )
  })

  it('tells an export job that failed, which can run again, from one whose model never came', () => {
    expect(exportPhase({ step: 1, status: 'Failed' })).toBe('failed')
    expect(exportPhase({ step: 1, status: 'Succeeded' }, 0, EXPORT_REGISTER_WAIT)).not.toBe(
      'failed',
    )
  })
})

describe('fillsGapsAfterEnd', () => {
  const MINUTE = 60_000
  const end = Date.parse('2026-11-19T00:00:00Z')
  const ended = { Time: '2026-11-19T00:00:00Z', Valid: true }

  it('keeps reading a run with a gap until its end-of-run backfill went over the gap', () => {
    expect(fillsGapsAfterEnd({ metricGap: true, endTime: ended }, end + MINUTE)).toBe(true)
    // The controller gives the backfill an hour.
    expect(fillsGapsAfterEnd({ metricGap: true, endTime: ended }, end + 60 * MINUTE)).toBe(true)
    expect(fillsGapsAfterEnd({ metricGap: true, endTime: ended }, end + 80 * MINUTE)).toBe(false)
    const backfilled = [
      { stage: 'train' as const, time: '2026-11-19T00:02:00Z', event: 'backfilled' },
    ]
    expect(
      fillsGapsAfterEnd(
        { metricGap: true, endTime: ended, stageHistory: backfilled },
        end + 3 * MINUTE,
      ),
    ).toBe(false)
    expect(fillsGapsAfterEnd({ metricGap: false, endTime: ended }, end + MINUTE)).toBe(false)
    expect(fillsGapsAfterEnd({ metricGap: true }, end)).toBe(false)
    expect(fillsGapsAfterEnd(undefined, end)).toBe(false)
  })

  it('counts from the stage done, where the backfill of a run that succeeded starts', () => {
    // The evaluation took two hours.
    const done = Date.parse('2026-11-19T02:00:00Z')
    const stageHistory = JSON.stringify([
      { stage: 'eval', time: '2026-11-19T00:05:00Z' },
      { stage: 'done', time: '2026-11-19T02:00:00Z' },
    ])
    const run = { metricGap: true, endTime: ended, stageHistory }
    expect(fillsGapsAfterEnd(run, done + 60 * MINUTE)).toBe(true)
    expect(fillsGapsAfterEnd(run, done + 80 * MINUTE)).toBe(false)
  })
})

describe('curvesDue', () => {
  it('reads the curves every third tick on the tabs that show them or their markers', () => {
    for (const tab of ['overview', 'metrics', 'checkpoints', 'evaluation']) {
      expect(curvesDue(tab, 0, 20_000)).toBe(false)
      expect(curvesDue(tab, 0, 29_990)).toBe(true)
    }
    expect(curvesDue('recipe', 0, 60_000)).toBe(false)
  })
})

describe('scatterTooltip', () => {
  it('shows the names as text', () => {
    expect(scatterTooltip('<img src=x onerror=alert(1)>', 'lr', 0.1, 'gsm8k<b>', 0.8)).toBe(
      '&lt;img src=x onerror=alert(1)&gt;<br/>lr: 0.1<br/>gsm8k&lt;b&gt;: 0.8',
    )
  })
})

describe('formatCommand', () => {
  it('puts each option with its values on a line', () => {
    expect(
      formatCommand([
        '--tp',
        '2',
        '--sequence-parallel',
        '--eval-prompt-data',
        'val',
        '/d/eval.jsonl',
      ]),
    ).toBe(
      'python3 train.py \\\n  --tp 2 \\\n  --sequence-parallel \\\n  --eval-prompt-data val /d/eval.jsonl',
    )
    expect(formatCommand([])).toBe('python3 train.py')
  })
})

describe('freezing', () => {
  const glm = {
    allowed: true,
    default: ['^(?!.*experts)'],
    label: 'experts in layers 0–69',
    trainableB: 95.91,
    totalB: 744,
  }

  it('tells the default, custom and unfrozen choices apart', () => {
    expect(freezeState(undefined, glm)).toBe('default')
    expect(freezeState({ trainOnly: ['^(?!.*experts)'] }, glm)).toBe('default')
    expect(freezeState({ trainOnly: ['layers\\.7'] }, glm)).toBe('custom')
    expect(freezeState({ trainOnly: [] }, glm)).toBe('off')
    expect(freezeState({}, glm)).toBe('off')
  })

  it('describes what a full run trains', () => {
    expect(trainableText('default', glm)).toBe('≈ 95.91B of 744B')
    expect(trainableText('off', glm)).toBe('All 744B')
    expect(trainableText('custom', glm)).toBe('Custom patterns')
    expect(freezingText('default', glm)).toBe('Experts in layers 0–69 frozen · 95.91B trained')
    expect(freezingText('off', glm)).toBe('Nothing frozen')
  })

  it('flags the patterns JavaScript cannot compile', () => {
    expect(regexErrors(['^(?!.*experts\\.)', 'layers\\.[0-'])).toEqual([
      expect.stringMatching(/^Line 2 is not a valid regex: /),
    ])
  })
})

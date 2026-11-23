import { describe, expect, it, vi } from 'vitest'
import {
  assignPanels,
  betterOf,
  matchName,
  panelsOf,
  formatCommand,
  formatDuration,
  freezeState,
  freezingText,
  parseJSON,
  regexErrors,
  smoothPoints,
  stageDurations,
  stageSegments,
  stallNote,
  timeOf,
  trainableText,
  postTrainingEntryReasons,
  curvesDue,
  backToRuns,
  postTrainingIsOn,
  errorMessageOf,
  noAnswer,
} from './index'
import type { Router } from 'vue-router'
import request from '@/services/request'

vi.mock('@/services/request', () => ({ default: { get: vi.fn() } }))
import type { PostTrainingRun } from './types'

type StageRun = Pick<PostTrainingRun, 'status' | 'stage' | 'failedStage' | 'convertWorkloadId'>

const states = (run: StageRun) => stageSegments(run).map((s) => `${s.stage}:${s.state}`)

describe('stageSegments', () => {
  it('leaves every stage empty while queued', () => {
    expect(states({ status: 'Pending', stage: 'queued' })).toEqual([
      'prepare:pending',
      'convert:pending',
      'check:pending',
      'train:pending',
      'export:pending',
      'register:pending',
    ])
  })

  it('marks earlier stages done and the current one active', () => {
    expect(states({ status: 'Running', stage: 'train' }).slice(0, 5)).toEqual([
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
        convertWorkloadId: 'cw',
      }),
    ).toEqual([
      'convert:done',
      'prepare:done',
      'check:failed',
      'train:pending',
      'export:pending',
      'register:pending',
    ])
  })

  it('finishes every stage of a done run and stops at the stage a stopped run reached', () => {
    expect(states({ status: 'Succeeded', stage: 'done' }).every((s) => s.endsWith(':done'))).toBe(
      true,
    )
    expect(states({ status: 'Stopped', stage: 'export' })[4]).toBe('export:stopped')
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

describe('postTrainingIsOn', () => {
  it('takes only an answer for Post Training on', async () => {
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 404 } })
    expect(await postTrainingIsOn('ws1')).toBe(false)
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 403 } })
    expect(await postTrainingIsOn('ws1')).toBe(false)
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 503 } })
    expect(await postTrainingIsOn('ws1')).toBe(false)
    vi.mocked(request.get).mockResolvedValueOnce({ total: 0, items: [] } as never)
    expect(await postTrainingIsOn('ws1')).toBe(true)
  })

  it('reads one run rather than every model', async () => {
    vi.mocked(request.get).mockClear()
    vi.mocked(request.get).mockResolvedValueOnce({ total: 0, items: [] } as never)
    await postTrainingIsOn('ws1')
    expect(vi.mocked(request.get)).toHaveBeenCalledWith('/post-training/runs', {
      params: { workspace: 'ws1', limit: 1 },
      silentErrors: true,
    })
  })
})

describe('errorMessageOf', () => {
  it("shows the API's message, else the status, else the error", () => {
    const bad = { response: { status: 400, data: { errorMessage: 'choices.tp must be positive' } } }
    expect(errorMessageOf(bad, 'The preflight check')).toBe('choices.tp must be positive')
    expect(errorMessageOf({ response: { status: 502 } }, 'The preflight check')).toBe(
      'The preflight check returned 502.',
    )
    expect(errorMessageOf(new Error('timeout of 10000ms exceeded'), 'x')).toBe(
      'timeout of 10000ms exceeded',
    )
  })
})

describe('noAnswer', () => {
  it('tells a call cut off without an answer from one the server answered', () => {
    expect(noAnswer({ code: 'ECONNABORTED' })).toBe(true)
    expect(noAnswer({ code: 'ERR_NETWORK' })).toBe(true)
    expect(noAnswer({ code: 'ERR_BAD_REQUEST', response: { status: 400 } })).toBe(false)
    expect(noAnswer('Request failed.')).toBe(false)
    expect(noAnswer(undefined)).toBe(false)
  })
})

describe('postTrainingEntryReasons', () => {
  it('offers no entry unless Post Training answers', async () => {
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 404 } })
    expect(await postTrainingEntryReasons('ws1')).toBeUndefined()
    // Only an answer says Post Training is on, as for the menu.
    vi.mocked(request.get).mockRejectedValueOnce({ response: { status: 503 } })
    expect(await postTrainingEntryReasons('ws1')).toBeUndefined()
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

describe('curvesDue', () => {
  it('reads the curves every third tick on the tabs that show them or their markers', () => {
    for (const tab of ['overview', 'metrics']) {
      expect(curvesDue(tab, 0, 20_000)).toBe(false)
      expect(curvesDue(tab, 0, 29_990)).toBe(true)
    }
    expect(curvesDue('recipe', 0, 60_000)).toBe(false)
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

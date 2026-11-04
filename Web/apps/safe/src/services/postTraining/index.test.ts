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
  gateDuration,
  importBanner,
  importTimeline,
  parallelismText,
  resourcePeaks,
  resourceSeries,
  trainableTotal,
  groupSamples,
  importCommandFor,
  importHints,
  importPreviewRows,
  importedCaption,
  importedSegments,
  importedStatusNote,
  importedTip,
  isImportedRun,
  lastLine,
  nestChanges,
  parseJSON,
  rewardOf,
  shortSpurPath,
  significance,
  smoothPoints,
  spurLine,
  splitTruncated,
  stageDurations,
  stageSegments,
  stallNote,
  timeOf,
} from './index'

vi.mock('@/services/request', () => ({ default: {} }))
import type { ImportPreview, ImportSummary, PostTrainingRun, RunSample } from './types'

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

describe('imports', () => {
  const preview: ImportPreview = {
    name: 'j2r3-229263',
    path: '/shared_nfs/zhanglei/glm53-kernel/slime-rocm/runs/j2r3-229263',
    verdict: 'FAIL',
    failedGate: { gate: 'b2_sft', node: 'm2m-022', result: 'fail(1)' },
    status: 'Failed',
    trainType: 'sft',
    slurmJobId: '229263',
    nodes: 2,
    mainLog: { name: 'b2_sft-crsuse2-m2m-022.log', bytes: 131_000, metricLines: 0 },
    series: 0,
    gates: 10,
    markers: 11,
    arguments: 1031,
    keptArguments: 32,
    samples: 27,
    sampleNodes: 2,
    checkpoints: 0,
    saveSet: false,
  }

  it('previews a directory the way the dialog lists it', () => {
    const rows = Object.fromEntries(importPreviewRows(preview).map((r) => [r.label, r.value]))
    expect(rows).toMatchObject({
      Verdict: 'FAIL b2_sft fail(1) on m2m-022',
      Run: 'SFT · Slurm job 229263 · 2 nodes',
      Metrics: 'b2_sft-crsuse2-m2m-022.log · 131 KB · 0 step lines',
      Gates: '10 results · 11 markers',
      Arguments: '1,031 parsed · 32 kept',
      Resources: '27 samples from 2 nodes',
      Checkpoint: '--save not set',
    })
    expect(importPreviewRows({ ...preview, saveSet: true, checkpoints: 1 }).at(-1)?.value).toBe(
      '1 checkpoint in ckpt/',
    )
  })

  it('warns where the import holds less and explains the rest', () => {
    expect(importHints(preview)).toEqual([
      {
        type: 'warning',
        text: 'No step, rollout or eval lines in b2_sft-crsuse2-m2m-022.log, so there are no curves.',
      },
    ])
    const hints = importHints({
      ...preview,
      verdict: '',
      series: 4,
      mainLog: { name: 'b6_grpo-h1.log', bytes: 2.69e9, metricLines: 0 },
      warnings: [
        'Gate c9 is not a known gate; it is shown as its own stage.',
        '3 metric lines could not be parsed.',
      ],
    })
    expect(hints.map((h) => h.type)).toEqual(['warning', 'info', 'info', 'warning'])
    expect(hints[1].text).toBe('b6_grpo-h1.log is 2.7 GB; only its metric lines are kept.')
  })

  it('puts the directory in the command, quoted when a shell would split it', () => {
    const command = 'SAFE_IMPORT_TOKEN=sst_a.b safe-post-training-import <run-dir>'
    expect(importCommandFor(command, '  ')).toBe(command)
    expect(importCommandFor(command, '/shared_nfs/runs/j2r5-229357')).toBe(
      'SAFE_IMPORT_TOKEN=sst_a.b safe-post-training-import /shared_nfs/runs/j2r5-229357',
    )
    expect(importCommandFor(command, "/shared_nfs/it's $HOME")).toBe(
      "SAFE_IMPORT_TOKEN=sst_a.b safe-post-training-import '/shared_nfs/it'\\''s $HOME'",
    )
    expect(shortSpurPath('/shared_nfs/zhanglei/glm53-kernel/slime-rocm/runs/j2r7-242118')).toBe(
      '/shared_nfs/…/j2r7-242118',
    )
  })

  it('shows an imported run with the stages and verdict of its directory', () => {
    const history = JSON.stringify([
      { stage: 'preflight', label: 'Preflight', gates: ['a1_host'], result: 'pass' },
      { stage: 'ray', label: 'Ray', gates: ['a1_ray'], result: 'pass' },
      { stage: 'train', label: 'Train', gates: ['b2_sft'], result: 'running' },
    ])
    expect(importedSegments({ stageHistory: history }).map((s) => s.state)).toEqual([
      'done',
      'done',
      'stopped',
    ])
    const run = { trainType: 'sft' as const, failedStage: 'train' }
    expect(importedCaption({ ...run, status: 'Succeeded' }, 19)).toBe('PASS · 20 steps')
    expect(importedCaption({ ...run, status: 'Succeeded', trainType: 'rl' }, 4)).toBe(
      'PASS · 5 rollouts',
    )
    expect(importedCaption({ ...run, status: 'Stopped' }, 21)).toBe('No verdict · 22 steps logged')
    expect(
      importedCaption({
        ...run,
        status: 'Failed',
        import: {
          sourcePath: '/d',
          via: 'cli',
          failedGate: { gate: 'b2_sft', node: 'h1', result: 'fail(1)' },
        },
      }),
    ).toBe('FAIL at b2_sft')
    expect(
      importedStatusNote({ status: 'Failed', message: 'FAIL: VRAM still rising over steps 10-20' }),
    ).toBe('Check failed: VRAM still rising over steps 10-20.')
    expect(importedStatusNote({ status: 'Stopped', message: 'last marker done_0' })).toBe(
      'No verdict in the run directory',
    )
    const info = {
      sourcePath: '/shared_nfs/zhanglei/runs/j2r7-242118',
      via: 'cli' as const,
      slurmJobId: '242118',
    }
    expect(spurLine({ import: info, nodeCount: 2 })).toBe('Spur · job 242118 · 2 nodes')
    expect(importedTip({ import: { ...info, importedAt: '2026-10-26T16:25:00' } })).toBe(
      'Imported 10-26 16:25 from /shared_nfs/…/j2r7-242118. Read-only, and not counted in Verified.',
    )
    expect(isImportedRun({ source: 'spur' })).toBe(true)
    expect(isImportedRun({})).toBe(false)
  })
})

describe('imported run detail', () => {
  const failed = {
    path: '/shared_nfs/zhanglei/glm53-kernel/slime-rocm/runs/j2r5-229357',
    name: 'j2r5-229357',
    verdict: 'FAIL',
    status: 'Failed',
    message: 'FAIL: VRAM still rising over steps 10-20',
    failLine: 'FAIL: VRAM still rising over steps 10-20',
    trainType: 'sft',
    endTime: '2026-10-21T11:54:03Z',
    nodes: [],
    head: 'crsuse2-m2m-001',
    saveSet: false,
    mainLog: { name: 'b2_sft-crsuse2-m2m-001.log', bytes: 2711821, metricLines: 20 },
    stages: [
      {
        stage: 'preflight',
        label: 'Preflight',
        gates: ['a1_host'],
        start: '2026-10-21T10:13:43Z',
        end: '2026-10-21T10:14:21Z',
        result: 'pass',
      },
      {
        stage: 'ray',
        label: 'Ray',
        gates: ['a1_ray'],
        start: '2026-10-21T10:15:21Z',
        end: '2026-10-21T10:17:55Z',
        result: 'pass',
      },
      {
        stage: 'train',
        label: 'Train',
        gates: ['b2_sft'],
        start: '2026-10-21T10:17:55Z',
        end: '2026-10-21T11:53:02Z',
        result: 'fail',
      },
      {
        stage: 'verdict',
        label: 'Verdict',
        gates: [],
        start: '2026-10-21T11:54:03Z',
        end: '2026-10-21T11:54:03Z',
        result: 'fail',
      },
    ],
    gates: [
      { gate: 'a1_host', node: 'crsuse2-m2m-001', result: 'pass', seconds: 1 },
      { gate: 'b2_sft', node: 'crsuse2-m2m-001', result: 'fail(1)', seconds: 5707 },
      { gate: 'b2_sft', node: 'crsuse2-m2m-153', result: 'pass', seconds: 5698 },
    ],
    markers: [{ name: 'done_1', time: '2026-10-21T11:53:54Z' }],
    check: [],
    arguments: { kept: {}, total: 1031 },
    checkpoints: { iterations: [] },
  } as unknown as ImportSummary

  it('lays out the stages the way the timeline shows them', () => {
    const rows = importTimeline(failed, 'sft', 19)
    expect(rows.map((r) => [r.label, r.state, r.text, r.sub ?? ''])).toEqual([
      ['Preflight', 'done', '38s', ''],
      ['Ray', 'done', '2m 34s', ''],
      ['Train', 'failed', 'b2_sft fail(1) on crsuse2-m2m-001', '20 steps logged'],
      ['Verdict', 'failed', 'FAIL', ''],
    ])
    expect(rows[2].gates).toEqual([
      'b2_sft · crsuse2-m2m-001 · 1h 35m',
      'b2_sft · crsuse2-m2m-153 · 1h 34m',
    ])
    const stopped = {
      ...failed,
      verdict: '',
      stages: [
        {
          stage: 'train',
          label: 'Train',
          gates: ['b2_sft'],
          start: '2026-10-22T23:24:03Z',
          end: null,
          result: 'running' as const,
        },
      ],
    }
    expect(importTimeline(stopped, 'sft', 21).map((r) => [r.state, r.text, r.sub ?? ''])).toEqual([
      ['stopped', 'No END line', '22 steps logged'],
      ['pending', 'No PASS or FAIL file', ''],
    ])
    expect(gateDuration(248)).toBe('4m 08s')
  })

  it('says why an imported run failed, where it stopped, or where it came from', () => {
    expect(importBanner(failed)).toEqual({
      type: 'error',
      text: 'Gate b2_sft failed on crsuse2-m2m-001. Check failed: VRAM still rising over steps 10-20.',
      evidence: 'FAIL: VRAM still rising over steps 10-20',
      logPath:
        '/shared_nfs/zhanglei/glm53-kernel/slime-rocm/runs/j2r5-229357/logs/b2_sft-crsuse2-m2m-001.log',
    })
    expect(importBanner({ ...failed, status: 'Stopped', verdict: '' }).text).toMatch(
      /^No verdict\. The run directory has no PASS or FAIL file\. The last marker is done_1 at \d\d:53:54, and the log was last written at \d\d:54:03\. Curves and resources stop there\.$/,
    )
    expect(importBanner({ ...failed, status: 'Succeeded', verdict: 'PASS' }).type).toBe('primary')
  })

  it('reads the settings and the resource peaks the way the check output states them', () => {
    expect(
      trainableTotal('dense 18.60B over 4 shards, expert 77.31B over 16 shards, total 95.91B'),
    ).toBe('95.91B')
    expect(trainableTotal('')).toBe('')
    expect(
      parallelismText({
        tensor_model_parallel_size: 2,
        pipeline_model_parallel_size: 2,
        context_parallel_size: 2,
        expert_model_parallel_size: 8,
      }),
    ).toBe('TP 2 · PP 2 · CP 2 · EP 8')
    const resources = {
      'crsuse2-m2m-001': {
        samples: [
          [1792577623, 800000000, 200e9],
          [1792577653, 861831680, 284345327616],
        ] as [number, number, number][],
        hostMemPeakKb: 861831680,
        vramPeakBytes: 284345327616,
      },
    }
    const [peak] = resourcePeaks(resources, 'crsuse2-m2m-001')
    expect(peak.head).toBe(true)
    expect((peak.hostMemBytes / 1e12).toFixed(2)).toBe('0.88')
    expect((peak.vramBytes / 1e9).toFixed(1)).toBe('284.3')
    const { memory, vram } = resourceSeries(resources, '2026-10-21T10:13:43Z')
    expect(memory[0].points[1][0]).toBe(0.5)
    expect(vram[0].points[1][1]).toBeCloseTo(284.35, 2)
  })
})

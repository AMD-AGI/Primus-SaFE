import { describe, expect, it, vi } from 'vitest'
import {
  cleanupHintBytes,
  configDiff,
  flattenChanges,
  formatDuration,
  nestChanges,
  parseJSON,
  significance,
  smoothPoints,
  stageDurations,
  stageSegments,
  stallNote,
  timeOf,
} from './index'

vi.mock('@/services/request', () => ({ default: {} }))
import type { PostTrainingRun } from './types'

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

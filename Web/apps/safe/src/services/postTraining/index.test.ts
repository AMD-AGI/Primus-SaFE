import { describe, expect, it, vi } from 'vitest'
import { formatDuration, parseJSON, stageSegments, timeOf } from './index'

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

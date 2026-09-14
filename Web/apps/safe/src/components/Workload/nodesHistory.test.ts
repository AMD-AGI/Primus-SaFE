import { describe, expect, it } from 'vitest'
import { formatDispatchNodes, toNodesHistoryRows } from './nodesHistory'

describe('formatDispatchNodes', () => {
  it('joins each dispatch group', () => {
    expect(formatDispatchNodes([['n1', 'n2'], ['n3']])).toBe('n1, n2 | n3')
  })

  it('returns a dash when empty', () => {
    expect(formatDispatchNodes(undefined)).toBe('-')
    expect(formatDispatchNodes([])).toBe('-')
  })
})

describe('toNodesHistoryRows', () => {
  it('returns newest archived run first', () => {
    const rows = toNodesHistoryRows([
      {
        dispatchCount: 1,
        phase: 'Stopped',
        startTime: '2026-09-14T07:41:55',
        endTime: '2026-09-14T08:06:11',
        nodes: [['tus1-p15-g9']],
      },
      {
        dispatchCount: 2,
        phase: 'Failed',
        nodes: [['n-a'], ['n-b']],
      },
    ])
    expect(rows).toHaveLength(2)
    expect(rows[0].index).toBe(2)
    expect(rows[0].phase).toBe('Failed')
    expect(rows[0].nodes).toBe('n-a | n-b')
    expect(rows[1].index).toBe(1)
    expect(rows[1].phase).toBe('Stopped')
    expect(rows[1].nodes).toBe('tus1-p15-g9')
  })
})

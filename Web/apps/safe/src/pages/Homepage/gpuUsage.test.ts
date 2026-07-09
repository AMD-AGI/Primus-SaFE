import { describe, expect, it } from 'vitest'
import {
  buildGpuStats,
  buildGpuUsageSeries,
  formatDateWithTimezone,
  unwrapGpuAggregationRows,
} from './gpuUsage'

const rows = [
  {
    stat_hour: '2026-07-08T01:00:00Z',
    avg_utilization: 0.5,
    allocation_rate: 75,
    active_workload_count: 4,
  },
  {
    stat_hour: '2026-07-08T02:00:00Z',
    avg_utilization: 80,
    allocation_rate: 0.25,
    active_workload_count: 6,
  },
]

describe('homepage gpu usage helpers', () => {
  it('unwraps Lens namespace hourly stats from supported response shapes', () => {
    expect(unwrapGpuAggregationRows({ data: { data: rows } })).toEqual(rows)
    expect(unwrapGpuAggregationRows({ data: rows })).toEqual(rows)
    expect(unwrapGpuAggregationRows(rows)).toEqual(rows)
    expect(unwrapGpuAggregationRows(null)).toEqual([])
  })

  it('normalizes 0-1 and 0-100 percentage values when computing summary stats', () => {
    expect(buildGpuStats(rows)).toEqual({
      avgUtilization: '65.0',
      avgAllocation: '50.0',
      totalWorkloads: 4,
      lowUtilization: 0,
    })
  })

  it('does not count missing utilization samples as low utilization', () => {
    expect(
      buildGpuStats([
        { stat_hour: '2026-07-08T01:00:00Z', avg_utilization: null, allocation_rate: null },
        { stat_hour: '2026-07-08T02:00:00Z', avg_utilization: 0.2, allocation_rate: 0.4 },
      ]),
    ).toMatchObject({
      avgUtilization: '20.0',
      avgAllocation: '40.0',
      lowUtilization: 1,
    })
  })

  it('builds chart series in chronological order', () => {
    const series = buildGpuUsageSeries(rows)

    expect(series.utilization).toEqual([80, 50])
    expect(series.allocation).toEqual([25, 75])
    expect(series.times).toHaveLength(2)
  })

  it('formats dates with local timezone offset for Lens queries', () => {
    expect(formatDateWithTimezone(new Date('2026-07-08T01:02:03+08:00'))).toMatch(
      /^2026-07-0[78]T\d{2}:02:03[+-]\d{2}:\d{2}$/,
    )
  })
})

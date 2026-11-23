import { describe, expect, it } from 'vitest'
import { buildUsageQuery, fmtPercent, fmtRatio, stepForHours, utilizationTone } from './usageUtils'

describe('buildUsageQuery', () => {
  const now = new Date('2026-11-23T00:00:00Z')

  it('maps a preset to a window ending now with a matching step', () => {
    const q = buildUsageQuery({ key: '7d', custom: null }, now)
    expect(q.end).toBe(now.toISOString())
    expect(q.start).toBe(new Date('2026-11-16T00:00:00Z').toISOString())
    expect(q.step).toBe('1h')
  })

  it('uses the picked dates and a step by their span for Custom', () => {
    const q = buildUsageQuery(
      {
        key: 'custom',
        custom: [new Date('2026-10-01T00:00:00Z'), new Date('2026-11-01T00:00:00Z')],
      },
      now,
    )
    expect(q.start).toBe('2026-10-01T00:00:00.000Z')
    expect(q.step).toBe('4h')
  })

  it('sends nothing for Custom before dates are picked', () => {
    expect(buildUsageQuery({ key: 'custom', custom: null }, now)).toEqual({})
  })
})

describe('stepForHours', () => {
  it('gets coarser as the range grows', () => {
    expect([stepForHours(24), stepForHours(168), stepForHours(720)]).toEqual(['15m', '1h', '4h'])
  })
})

describe('formatting', () => {
  it('shows "-" for unknown values and scales ratios to percent', () => {
    expect(fmtRatio(null)).toBe('-')
    expect(fmtRatio(0.1234)).toBe('12.3%')
    expect(fmtPercent(null)).toBe('-')
    expect(fmtPercent(42.4)).toBe('42%')
  })

  it('keeps unknown utilization neutral and flags low values', () => {
    expect(utilizationTone(null)).toBe('info')
    expect(utilizationTone(0.1)).toBe('danger')
    expect(utilizationTone(0.3)).toBe('warning')
    expect(utilizationTone(0.8)).toBe('success')
  })
})

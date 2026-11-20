import { describe, expect, it } from 'vitest'
import type { LowUtilizationPolicy } from '@/services'
import {
  buildLowUtilization,
  newLowUtilizationEntry,
  parseLowUtilization,
} from './lowUtilizationForm'

const roundTrip = (raw: Record<string, LowUtilizationPolicy>) =>
  buildLowUtilization(parseLowUtilization(raw), Object.keys(raw))

describe('low utilization form', () => {
  it('keeps every field of a policy with rules', () => {
    const raw: Record<string, LowUtilizationPolicy> = {
      Train: {
        mode: 'Min',
        rules: [
          { threshold: 11, window: '0h30m' },
          { threshold: 30, window: '1h0m' },
        ],
        gracePeriod: '1h0m',
        notifyBefore: '0h30m',
        cooldown: '0h45m',
        dryRun: true,
      },
    }
    expect(roundTrip(raw)).toEqual(raw)
  })

  it('saves a legacy policy back as a single threshold and window', () => {
    const raw: Record<string, LowUtilizationPolicy> = {
      Infer: {
        mode: 'Average',
        threshold: 5,
        window: '2h0m',
        notifyBefore: '0h30m',
        dryRun: false,
      },
    }
    expect(roundTrip(raw)).toEqual(raw)
  })

  it('defaults an omitted legacy threshold to 0', () => {
    const parsed = parseLowUtilization({ Train: { mode: 'Average', window: '1h0m' } })
    expect(parsed.Train.rules).toEqual([{ threshold: 0, window: 60 }])
    expect(buildLowUtilization(parsed, ['Train']).Train).toEqual({
      mode: 'Average',
      threshold: 0,
      window: '1h0m',
      dryRun: false,
    })
  })

  it('keeps a zero cooldown apart from the default', () => {
    const parsed = parseLowUtilization({
      Train: { mode: 'Average', rules: [{ threshold: 5, window: '1h0m' }], cooldown: '0s' },
      Infer: { mode: 'Average', rules: [{ threshold: 5, window: '1h0m' }] },
    })
    expect(parsed.Train.cooldown).toBe(0)
    expect(parsed.Infer.cooldown).toBeUndefined()
    const built = buildLowUtilization(parsed, ['Train', 'Infer'])
    expect(built.Train.cooldown).toBe('0h0m')
    expect(built.Infer).not.toHaveProperty('cooldown')
  })

  it('saves a legacy policy given a second rule as rules', () => {
    const parsed = parseLowUtilization({ Train: { mode: 'Average', threshold: 5, window: '1h0m' } })
    parsed.Train.rules.push({ threshold: 20, window: 30 })
    expect(buildLowUtilization(parsed, ['Train']).Train.rules).toEqual([
      { threshold: 5, window: '1h0m' },
      { threshold: 20, window: '0h30m' },
    ])
  })

  it('saves a new entry as rules and skips rules without a window and unselected scopes', () => {
    const entry = newLowUtilizationEntry()
    entry.rules.push({ threshold: 10, window: undefined })
    const built = buildLowUtilization({ Train: entry, Infer: newLowUtilizationEntry() }, ['Train'])
    expect(Object.keys(built)).toEqual(['Train'])
    expect(built.Train).toEqual({
      mode: 'Average',
      rules: [{ threshold: 5, window: '1h0m' }],
      gracePeriod: '0h30m',
      notifyBefore: '0h30m',
      dryRun: true,
    })
  })
})

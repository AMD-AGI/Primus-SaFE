import { describe, expect, it } from 'vitest'
import { clusterPhaseTagType, isUsableClusterPhase } from './cluster'

describe('isUsableClusterPhase', () => {
  it.each(['Ready', 'Upgrading', 'UpgradeFailed'])('accepts %s', (phase) => {
    expect(isUsableClusterPhase(phase)).toBe(true)
  })

  it.each(['Creating', 'Deleting', undefined])('rejects %s', (phase) => {
    expect(isUsableClusterPhase(phase)).toBe(false)
  })

  it('uses a warning tag while upgrading', () => {
    expect(clusterPhaseTagType('Upgrading')).toBe('warning')
    expect(clusterPhaseTagType('UpgradeFailed')).toBe('danger')
  })
})

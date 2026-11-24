import { describe, expect, it } from 'vitest'
import { buildUserLowUtilization, exemptWorkspaceIds } from './lowUtilExemption'

describe('low utilization exemption form', () => {
  it('builds one exempt entry per selected workspace', () => {
    expect(buildUserLowUtilization(['ws-b', 'ws-a'])).toEqual({
      workspaces: { 'ws-a': { exempt: true }, 'ws-b': { exempt: true } },
    })
  })

  it('clears the settings when nothing is selected', () => {
    expect(buildUserLowUtilization([])).toEqual({})
    expect(buildUserLowUtilization([''])).toEqual({})
  })

  it('selects only the exempt workspaces of the stored settings', () => {
    expect(
      exemptWorkspaceIds({
        workspaces: { 'ws-b': { exempt: true }, 'ws-c': {}, 'ws-a': { exempt: true } },
      }),
    ).toEqual(['ws-a', 'ws-b'])
    expect(exemptWorkspaceIds(undefined)).toEqual([])
    expect(exemptWorkspaceIds({})).toEqual([])
  })

  it('round-trips a selection', () => {
    const ids = ['ws-a', 'ws-gone']
    expect(exemptWorkspaceIds(buildUserLowUtilization(ids))).toEqual(ids)
  })
})

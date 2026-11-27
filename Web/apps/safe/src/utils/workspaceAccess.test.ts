import { describe, expect, it } from 'vitest'
import {
  approvalLink,
  approvalPath,
  isPendingRequest,
  requestStatusLabel,
  requestStatusTag,
} from './workspaceAccess'

describe('request status', () => {
  it.each([
    ['pending', 'Pending', 'warning', true],
    ['approved', 'Approved', 'success', false],
    ['rejected', 'Rejected', 'danger', false],
    ['cancelled', 'Cancelled', 'info', false],
    ['auto_closed', 'Auto-closed', 'info', false],
  ])('%s is shown as %s', (status, label, tag, pending) => {
    expect(requestStatusLabel(status)).toBe(label)
    expect(requestStatusTag(status)).toBe(tag)
    expect(isPendingRequest(status)).toBe(pending)
  })

  it('shows an unknown status as it is', () => {
    expect(requestStatusLabel('on_hold')).toBe('on_hold')
    expect(requestStatusTag('on_hold')).toBe('info')
    expect(requestStatusLabel(undefined)).toBe('-')
  })
})

describe('approval link', () => {
  const origin = 'https://console.example.com'

  it('is the path of the approval page', () => {
    expect(approvalPath(42)).toBe('/workspace-access/requests/42')
  })

  it("uses the server's link", () => {
    const url = 'https://safe.example.com/workspace-access/requests/7'
    expect(approvalLink({ id: 7, approvalUrl: url }, origin)).toBe(url)
  })

  it.each(['', '   ', undefined, 'javascript:alert(1)', '/workspace-access/requests/7'])(
    'falls back to the approval page of this console for %j',
    (approvalUrl) => {
      expect(approvalLink({ id: 7, approvalUrl }, origin)).toBe(
        `${origin}/workspace-access/requests/7`,
      )
    },
  )
})

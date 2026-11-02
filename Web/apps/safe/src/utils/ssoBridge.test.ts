import { describe, expect, it } from 'vitest'
import { resolveOwnBridge } from './ssoBridge'

const SELF = 'https://console.example.com'

describe('resolveOwnBridge', () => {
  it('accepts a target on the console origin', () => {
    expect(resolveOwnBridge(`${SELF}/lens/sso-bridge`, SELF)?.origin).toBe(SELF)
    expect(resolveOwnBridge(`${SELF}/x?y=1`, SELF)).not.toBeNull()
  })

  it.each([
    ['other origin', 'https://evil.example.net/sso-bridge'],
    ['other port', 'https://console.example.com:8443/sso-bridge'],
    ['scheme downgrade', 'http://console.example.com/sso-bridge'],
    ['protocol-relative', '//evil.example.net/sso-bridge'],
    ['backslash host', '\\\\evil.example.net/sso-bridge'],
    ['relative path', '/sso-bridge'],
    ['javascript scheme', 'javascript:alert(1)'],
    ['data scheme', 'data:text/html,x'],
    ['userinfo credentials', 'https://user:pw@console.example.com/sso-bridge'],
    ['userinfo that hides the host', 'https://console.example.com@evil.example.net/x'],
    ['sibling domain', 'https://console.example.com.evil.example.net/x'],
    ['empty', ''],
  ])('rejects %s', (_name, candidate) => {
    expect(resolveOwnBridge(candidate, SELF)).toBeNull()
  })
})

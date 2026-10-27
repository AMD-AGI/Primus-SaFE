import { describe, expect, it } from 'vitest'
import { resolveAllowedBridge } from './ssoBridge'

const SELF = 'https://console.example.com'
const ALLOW = ['https://app.example.com/portal', 'https://other.example.com:8443']

describe('resolveAllowedBridge', () => {
  it('accepts the console origin with no allow-list', () => {
    expect(resolveAllowedBridge(`${SELF}/lens/sso-bridge`, [], SELF)?.origin).toBe(SELF)
    expect(resolveAllowedBridge(`${SELF}/x`, undefined, SELF)).not.toBeNull()
    expect(resolveAllowedBridge(`${SELF}/x`, null, SELF)).not.toBeNull()
  })

  it('accepts allow-listed origin with matching path prefix', () => {
    expect(resolveAllowedBridge('https://app.example.com/portal/sso-bridge', ALLOW, SELF)).not.toBeNull()
    expect(resolveAllowedBridge('https://app.example.com/portal', ALLOW, SELF)).not.toBeNull()
    expect(resolveAllowedBridge('https://other.example.com:8443/any/path', ALLOW, SELF)).not.toBeNull()
  })

  it.each([
    ['other origin', 'https://evil.example.net/sso-bridge'],
    ['javascript:', 'javascript:alert(1)'],
    ['data:', 'data:text/html,<script>1</script>'],
    ['protocol-relative', '//evil.example.net/sso-bridge'],
    ['backslash authority', 'https:\\\\evil.example.net'],
    ['relative path', '/sso-bridge'],
    ['userinfo on allowed host', 'https://app.example.com@evil.example.net/portal'],
    ['userinfo credentials', 'https://user:pw@app.example.com/portal/sso-bridge'],
    ['look-alike suffix', 'https://app.example.com.evil.example.net/portal'],
    ['look-alike prefix', 'https://evilapp.example.com/portal'],
    ['scheme downgrade', 'http://app.example.com/portal/sso-bridge'],
    ['wrong port', 'https://other.example.com/x'],
    ['path prefix sibling', 'https://app.example.com/portal-evil/sso-bridge'],
    ['outside path prefix', 'https://app.example.com/other'],
    ['dot-segment escape', 'https://app.example.com/portal/../admin'],
    ['empty', ''],
    ['garbage', 'not a url'],
  ])('rejects %s', (_name, candidate) => {
    expect(resolveAllowedBridge(candidate, ALLOW, SELF)).toBeNull()
  })

  it('ignores malformed allow-list entries', () => {
    expect(resolveAllowedBridge('https://evil.example.net/x', ['*', '', 'evil.example.net', 'javascript:1'], SELF)).toBeNull()
  })
})

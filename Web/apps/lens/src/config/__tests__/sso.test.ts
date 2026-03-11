import { describe, it, expect } from 'vitest'
import { SSO_CONFIG, SSO_API } from '../sso'

// ---------------------------------------------------------------------------
// SSO_CONFIG.parseState
// ---------------------------------------------------------------------------
describe('SSO_CONFIG.parseState', () => {
  it('parses a valid state string', () => {
    const result = SSO_CONFIG.parseState('lens:abc123:https%3A%2F%2Fexample.com%2Fsso-bridge')
    expect(result).toEqual({
      app: 'lens',
      state: 'abc123',
      redirect: 'https://example.com/sso-bridge',
    })
  })

  it('handles redirect URLs containing colons', () => {
    const redirect = encodeURIComponent('https://example.com:8080/callback')
    const result = SSO_CONFIG.parseState(`lens:mystate:${redirect}`)
    expect(result).not.toBeNull()
    expect(result!.redirect).toBe('https://example.com:8080/callback')
  })

  it('returns null for empty string', () => {
    expect(SSO_CONFIG.parseState('')).toBeNull()
  })

  it('returns null for string not starting with "lens:"', () => {
    expect(SSO_CONFIG.parseState('safe:abc:url')).toBeNull()
    expect(SSO_CONFIG.parseState('randomstring')).toBeNull()
  })

  it('returns null for "lens:" with fewer than 3 parts', () => {
    expect(SSO_CONFIG.parseState('lens:onlystate')).toBeNull()
  })

  it('handles state with special characters', () => {
    const result = SSO_CONFIG.parseState('lens:st-abc_123:https%3A%2F%2Ftest.com')
    expect(result).not.toBeNull()
    expect(result!.state).toBe('st-abc_123')
  })
})

// ---------------------------------------------------------------------------
// SSO_CONFIG constants
// ---------------------------------------------------------------------------
describe('SSO_CONFIG constants', () => {
  it('has expected OAuth settings', () => {
    expect(SSO_CONFIG.RESPONSE_TYPE).toBe('code')
    expect(SSO_CONFIG.SCOPE).toBe('openid profile email')
    expect(SSO_CONFIG.LENS_IDENTIFIER).toBe('lens_redirect')
  })
})

// ---------------------------------------------------------------------------
// SSO_API
// ---------------------------------------------------------------------------
describe('SSO_API', () => {
  it('has correct endpoint paths', () => {
    expect(SSO_API.EXCHANGE_CODE).toBe('/api/v1/auth/sso/token')
    expect(SSO_API.GET_USER_INFO).toBe('/api/v1/auth/sso/userinfo')
    expect(SSO_API.LOGOUT).toBe('/api/v1/auth/logout')
  })
})

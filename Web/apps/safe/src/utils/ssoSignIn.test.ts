import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  OAUTH_STATE_KEY,
  PKCE_VERIFIER_KEY,
  buildSignInUrl,
  codeChallengeS256,
  newCodeVerifier,
  pkceRequired,
  sha256,
  signInTarget,
  takeCallbackVerifier,
} from './ssoSignIn'

const AUTH = 'https://idp.example.com/oauth/authorize?client_id=console&code_challenge_method=S256'

// The tests run in Node: a minimal sessionStorage of this tab.
const store = new Map<string, string>()
const sessionStorageStub = {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => void store.set(k, String(v)),
  removeItem: (k: string) => void store.delete(k),
  clear: () => store.clear(),
}
vi.stubGlobal('sessionStorage', sessionStorageStub)

beforeEach(() => sessionStorage.clear())

describe('PKCE', () => {
  it('makes a verifier of the RFC 7636 form', () => {
    const v = newCodeVerifier()
    expect(v).toMatch(/^[A-Za-z0-9_-]{43}$/)
    expect(newCodeVerifier()).not.toBe(v)
  })

  it('computes SHA-256 without crypto.subtle (a console on plain http)', async () => {
    const { createHash } = await import('node:crypto')
    for (const n of [0, 1, 43, 55, 56, 63, 64, 65, 128, 1000]) {
      const msg = new Uint8Array(n).map((_, i) => (i * 31 + 7) & 0xff)
      expect(Buffer.from(sha256(msg)).toString('hex')).toBe(
        createHash('sha256').update(msg).digest('hex'),
      )
    }
    vi.stubGlobal('crypto', { getRandomValues: crypto.getRandomValues.bind(crypto) })
    try {
      expect(await codeChallengeS256('dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk')).toBe(
        'E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM',
      )
    } finally {
      vi.unstubAllGlobals()
      vi.stubGlobal('sessionStorage', sessionStorageStub)
    }
  })

  it('computes the S256 challenge of RFC 7636 appendix B', async () => {
    expect(await codeChallengeS256('dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk')).toBe(
      'E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM',
    )
  })
})

describe('buildSignInUrl', () => {
  it('without PKCE adds only the state, as before', async () => {
    const u = new URL(
      await buildSignInUrl('https://idp.example.com/authorize?client_id=x', 's1', false),
    )
    expect(u.searchParams.get('state')).toBe('s1')
    expect(u.searchParams.get('response_mode')).toBe('query')
    expect(u.searchParams.has('code_challenge')).toBe(false)
    expect(sessionStorage.getItem(OAUTH_STATE_KEY)).toBe('s1')
    expect(sessionStorage.getItem(PKCE_VERIFIER_KEY)).toBeNull()
  })

  it('with PKCE adds the challenge of the verifier it keeps', async () => {
    const u = new URL(await buildSignInUrl(AUTH, 's2', true))
    const verifier = sessionStorage.getItem(PKCE_VERIFIER_KEY)
    expect(verifier).toBeTruthy()
    expect(u.searchParams.get('code_challenge')).toBe(await codeChallengeS256(verifier!))
    expect(u.searchParams.get('code_challenge_method')).toBe('S256')
    expect(u.searchParams.get('client_id')).toBe('console')
    expect(u.searchParams.get('state')).toBe('s2')
  })
})

describe('takeCallbackVerifier', () => {
  it('returns the verifier for the state this tab sent, once', async () => {
    await buildSignInUrl(AUTH, 's3', true)
    const verifier = sessionStorage.getItem(PKCE_VERIFIER_KEY)
    expect(takeCallbackVerifier('s3', true)).toEqual({ ok: true, verifier })
    expect(takeCallbackVerifier('s3', true)).toEqual({ ok: false })
  })

  it('refuses a code this tab did not ask for', async () => {
    expect(takeCallbackVerifier('s4', true)).toEqual({ ok: false })
    await buildSignInUrl(AUTH, 's5', true)
    expect(takeCallbackVerifier('another', true)).toEqual({ ok: false })
    expect(sessionStorage.getItem(PKCE_VERIFIER_KEY)).toBeNull()
    await buildSignInUrl(AUTH, 's6', true)
    expect(takeCallbackVerifier(undefined, true)).toEqual({ ok: false })
  })

  it('uses a kept verifier when /envs could not say whether PKCE is on', async () => {
    await buildSignInUrl(AUTH, 's7', true)
    const verifier = sessionStorage.getItem(PKCE_VERIFIER_KEY)
    expect(takeCallbackVerifier('s7', false)).toEqual({ ok: true, verifier })
  })

  it('without PKCE changes nothing for the callback', () => {
    expect(takeCallbackVerifier(undefined, false)).toEqual({ ok: true })
    expect(takeCallbackVerifier('x', false)).toEqual({ ok: true })
  })
})

describe('signInTarget', () => {
  it('uses ssoLoginUrl with PKCE, and has no target without it', () => {
    expect(signInTarget({ ssoLoginUrl: 'https://b/authorize' })).toEqual({
      url: 'https://b/authorize',
      pkce: true,
    })
    expect(signInTarget({ ssoPkce: true })).toBeNull()
    expect(signInTarget({})).toBeNull()
    expect(signInTarget(undefined)).toBeNull()
    expect(signInTarget(null)).toBeNull()
  })
})

describe('pkceRequired', () => {
  it('follows ssoPkce, and ssoLoginUrl from a server that does not report ssoPkce', () => {
    expect(pkceRequired({ ssoPkce: true })).toBe(true)
    expect(pkceRequired({ ssoLoginUrl: 'https://b/authorize' })).toBe(true)
    expect(pkceRequired({})).toBe(false)
    expect(pkceRequired(null)).toBe(false)
  })
})

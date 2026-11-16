// Building the single sign-on request, and keeping what the callback needs to finish it.
//
// The authorization URL comes from /envs. With ssoLoginUrl the sign-in is at an OpenID Connect provider
// where the console is a public client: the request carries an S256 code challenge (RFC 7636), and the
// verifier stays in this tab's sessionStorage until the callback sends it with the code. A code that
// arrives without one was not asked for by this tab and is not used. External sign-in is loginUrl
// and does not come through here.

export const OAUTH_STATE_KEY = 'oauth_state'
export const PKCE_VERIFIER_KEY = 'oauth_pkce_verifier'

/**
 * Where the console's built-in SSO signs in, from /envs: ssoLoginUrl with PKCE when there is one,
 * and otherwise ssoAuthUrl without PKCE. There is none while sso.login is on (ssoPkce) but its URL
 * is missing. External sign-in uses loginUrl instead.
 */
export function signInTarget(
  envs?: { ssoLoginUrl?: string; ssoAuthUrl?: string; ssoPkce?: boolean } | null,
): { url: string; pkce: boolean } | null {
  if (envs?.ssoLoginUrl) return { url: envs.ssoLoginUrl, pkce: true }
  if (envs?.ssoPkce) return null
  if (envs?.ssoAuthUrl) return { url: envs.ssoAuthUrl, pkce: false }
  return null
}

/** Whether a callback must come with this tab's PKCE verifier, from /envs. */
export function pkceRequired(envs?: { ssoLoginUrl?: string; ssoPkce?: boolean } | null): boolean {
  return !!envs?.ssoPkce || !!envs?.ssoLoginUrl
}

function base64Url(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
}

/** A PKCE code verifier: 32 random bytes, 43 characters of base64url (RFC 7636 section 4.1). */
export function newCodeVerifier(): string {
  const b = new Uint8Array(32)
  crypto.getRandomValues(b)
  return base64Url(b)
}

/** The S256 code challenge of a verifier (RFC 7636 section 4.2). */
export async function codeChallengeS256(verifier: string): Promise<string> {
  const data = new TextEncoder().encode(verifier)
  // crypto.subtle exists only in a secure context (https or localhost); a console served over plain http
  // on another host computes the digest itself.
  if (globalThis.crypto?.subtle) {
    return base64Url(new Uint8Array(await crypto.subtle.digest('SHA-256', data)))
  }
  return base64Url(sha256(data))
}

const K256 = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
])

/** SHA-256 (FIPS 180-4) of a short message, for a browser without crypto.subtle. */
export function sha256(msg: Uint8Array): Uint8Array {
  const len = msg.length
  const blocks = Math.ceil((len + 9) / 64)
  const buf = new Uint8Array(blocks * 64)
  buf.set(msg)
  buf[len] = 0x80
  const view = new DataView(buf.buffer)
  view.setUint32(buf.length - 8, Math.floor(len / 0x20000000))
  view.setUint32(buf.length - 4, (len * 8) >>> 0)
  const h = new Uint32Array([
    0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
  ])
  const w = new Uint32Array(64)
  const rotr = (x: number, n: number) => (x >>> n) | (x << (32 - n))
  for (let off = 0; off < buf.length; off += 64) {
    for (let i = 0; i < 16; i++) w[i] = view.getUint32(off + i * 4)
    for (let i = 16; i < 64; i++) {
      const s0 = rotr(w[i - 15], 7) ^ rotr(w[i - 15], 18) ^ (w[i - 15] >>> 3)
      const s1 = rotr(w[i - 2], 17) ^ rotr(w[i - 2], 19) ^ (w[i - 2] >>> 10)
      w[i] = (w[i - 16] + s0 + w[i - 7] + s1) >>> 0
    }
    let [a, b, c, d, e, f, g, hh] = h
    for (let i = 0; i < 64; i++) {
      const t1 =
        (hh + (rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25)) + ((e & f) ^ (~e & g)) + K256[i] + w[i]) >>>
        0
      const t2 = ((rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22)) + ((a & b) ^ (a & c) ^ (b & c))) >>> 0
      hh = g
      g = f
      f = e
      e = (d + t1) >>> 0
      d = c
      c = b
      b = a
      a = (t1 + t2) >>> 0
    }
    h[0] += a
    h[1] += b
    h[2] += c
    h[3] += d
    h[4] += e
    h[5] += f
    h[6] += g
    h[7] += hh
  }
  const out = new Uint8Array(32)
  const ov = new DataView(out.buffer)
  for (let i = 0; i < 8; i++) ov.setUint32(i * 4, h[i])
  return out
}

/**
 * The URL to send the browser to: the sign-in URL with this tab's state and, with PKCE, the code
 * challenge. The state and the verifier are kept in sessionStorage for the callback.
 */
export async function buildSignInUrl(
  authUrl: string,
  state: string,
  pkce: boolean,
  connection?: string,
): Promise<string> {
  const u = new URL(authUrl)
  sessionStorage.setItem(OAUTH_STATE_KEY, state)
  u.searchParams.set('state', state)
  if (connection) u.searchParams.set('connection', connection)
  if (!u.searchParams.has('response_mode')) u.searchParams.set('response_mode', 'query')
  sessionStorage.removeItem(PKCE_VERIFIER_KEY)
  if (pkce) {
    const verifier = newCodeVerifier()
    sessionStorage.setItem(PKCE_VERIFIER_KEY, verifier)
    u.searchParams.set('code_challenge', await codeChallengeS256(verifier))
    u.searchParams.set('code_challenge_method', 'S256')
  }
  return u.toString()
}

/**
 * What the callback may do with a code. With PKCE (pkce, or a verifier this tab kept), the code is
 * used only together with the verifier, and only when the state it comes back with is the one this
 * tab sent: anything else was not started here. The verifier is taken (single use) whatever the answer.
 */
export function takeCallbackVerifier(
  state: string | undefined,
  pkce: boolean,
): { ok: true; verifier?: string } | { ok: false } {
  const verifier = sessionStorage.getItem(PKCE_VERIFIER_KEY) || undefined
  sessionStorage.removeItem(PKCE_VERIFIER_KEY)
  // A tab that kept a verifier started a PKCE sign-in, even if /envs could not be read just now.
  if (!verifier && !pkce) return { ok: true }
  const saved = sessionStorage.getItem(OAUTH_STATE_KEY)
  if (!verifier || !saved || !state || saved !== state) return { ok: false }
  return { ok: true, verifier }
}

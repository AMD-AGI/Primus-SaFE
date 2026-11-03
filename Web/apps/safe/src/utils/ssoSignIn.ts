// Building the single sign-on request, and keeping what the callback needs to finish it.
//
// The authorization URL comes from /envs. With ssoLoginUrl the sign-in is at an OpenID Connect provider
// where the console is a public client: the request carries an S256 code challenge (RFC 7636), and the
// verifier stays in this tab's sessionStorage until the callback sends it with the code. A code that
// arrives without one was not asked for by this tab and is not used. Without it the sign-in is at
// ssoAuthUrl, as before.

export const OAUTH_STATE_KEY = 'oauth_state'
export const PKCE_VERIFIER_KEY = 'oauth_pkce_verifier'

/**
 * Where the console signs in, from /envs: ssoLoginUrl with PKCE when there is one, else ssoAuthUrl. When
 * the server says the sign-in is PKCE's (ssoPkce) and has no ssoLoginUrl just now, there is none: falling
 * back to ssoAuthUrl would accept codes without PKCE again.
 */
export function signInTarget(
  envs?: { ssoAuthUrl?: string; ssoLoginUrl?: string; ssoPkce?: boolean } | null,
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
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier))
  return base64Url(new Uint8Array(digest))
}

/**
 * The URL to send the browser to: ssoAuthUrl with this tab's state and, with PKCE, the code
 * challenge. The state and the verifier are kept in sessionStorage for the callback.
 */
export async function buildSignInUrl(
  authUrl: string,
  state: string,
  pkce: boolean,
): Promise<string> {
  const u = new URL(authUrl)
  sessionStorage.setItem(OAUTH_STATE_KEY, state)
  u.searchParams.set('state', state)
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

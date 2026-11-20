// Opening the identity console.
//
// sso.enable chooses the sign-in. When it is on and this browser has no identity session, the
// console starts its SSO sign-in and comes back to the identity page (the auth guard reads
// sso.redirect). When it is off, the browser opens the page, which asks for a username and password.

import { buildSignInUrl, signInTarget } from './ssoSignIn'

const SSO_REDIRECT_KEY = 'sso.redirect'

type IdentityEnvs = {
  ssoEnable?: boolean
  ssoLoginUrl?: string
  ssoAuthUrl?: string
  ssoPkce?: boolean
  authMode?: 'builtin' | 'external'
  connection?: string
}

function randomState(len = 16): string {
  const b = new Uint8Array(len)
  crypto.getRandomValues(b)
  return btoa(String.fromCharCode(...b))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
}

/** A same-origin path the console can return to, or null when `url` is another origin. */
export function identityReturnPath(url: string, origin: string): string | null {
  if (!url || url.startsWith('//')) return null
  if (url.startsWith('/')) return url
  try {
    const parsed = new URL(url, origin)
    if (parsed.origin !== origin) return null
    return `${parsed.pathname}${parsed.search}${parsed.hash}`
  } catch {
    return null
  }
}

async function identitySignedIn(): Promise<boolean> {
  try {
    const res = await fetch('/identity/api/v1/users/self', { credentials: 'same-origin' })
    return res.ok
  } catch {
    return false
  }
}

/** Opens the identity console. With sso.enable and no identity session, signs in with SSO first. */
export async function openIdentityConsole(
  envs: IdentityEnvs | null | undefined,
  url: string,
): Promise<void> {
  if (!url) return
  const signIn = envs?.ssoEnable ? signInTarget(envs) : null
  const ret = signIn ? identityReturnPath(url, window.location.origin) : null
  if (signIn && ret && !(await identitySignedIn())) {
    sessionStorage.setItem(SSO_REDIRECT_KEY, ret)
    const connection = envs?.authMode === 'external' ? envs.connection : undefined
    window.location.assign(await buildSignInUrl(signIn.url, randomState(), signIn.pkce, connection))
    return
  }
  window.location.assign(url)
}

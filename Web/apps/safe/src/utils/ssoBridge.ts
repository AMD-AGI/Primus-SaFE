/**
 * SSO bridge target validation.
 *
 * Other web apps start the login through this console and pass a "bridge" URL in the
 * OAuth `state` ({app}:{csrf}:{encodeURIComponent(bridgeUrl)}). The authorization code
 * is only forwarded to a bridge that is this console's own origin or is explicitly
 * allow-listed (backend `sso.bridge_allowlist`, served through /envs).
 */

function parseHttpUrl(value: string): URL | null {
  try {
    const url = new URL(value)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
    // Credentials in the authority ("https://allowed@evil") are never legitimate here.
    if (url.username || url.password) return null
    return url
  } catch {
    return null
  }
}

function pathMatches(pathname: string, prefix: string): boolean {
  const base = prefix.replace(/\/+$/, '')
  if (!base) return true
  return pathname === base || pathname.startsWith(`${base}/`)
}

/**
 * Returns the parsed bridge URL when it targets `selfOrigin` or matches an allow-list
 * entry (an origin, optionally followed by a path prefix); otherwise null.
 * Origins are compared exactly (scheme, host and port), never by substring.
 */
export function resolveAllowedBridge(
  candidate: string,
  allowlist: readonly string[] | null | undefined,
  selfOrigin: string,
): URL | null {
  // Raw check so "\\evil" or "//evil" style inputs can never be normalised into a match.
  if (!/^https?:\/\//i.test(candidate)) return null
  const url = parseHttpUrl(candidate)
  if (!url) return null

  if (url.origin === selfOrigin) return url

  for (const entry of allowlist ?? []) {
    const allowed = typeof entry === 'string' ? parseHttpUrl(entry.trim()) : null
    if (!allowed || allowed.search || allowed.hash) continue
    if (url.origin === allowed.origin && pathMatches(url.pathname, allowed.pathname)) return url
  }
  return null
}

/**
 * SSO bridge target validation.
 *
 * An application served under this console's own origin (the Lens app at /lens/) starts
 * the login through this console and passes a "bridge" URL in the OAuth `state`
 * ({app}:{csrf}:{encodeURIComponent(bridgeUrl)}). The authorization code is only
 * forwarded to a bridge on this console's own origin.
 */

/**
 * Returns the parsed bridge URL when it is an http(s) URL on `selfOrigin`; otherwise null.
 * Origins are compared exactly (scheme, host and port), never by substring.
 */
export function resolveOwnBridge(candidate: string, selfOrigin: string): URL | null {
  // Raw check so "\\evil" or "//evil" style inputs can never be normalised into a match.
  if (!/^https?:\/\//i.test(candidate)) return null
  try {
    const url = new URL(candidate)
    // Credentials in the authority ("https://allowed@evil") are never legitimate here.
    if (url.username || url.password) return null
    return url.origin === selfOrigin ? url : null
  } catch {
    return null
  }
}

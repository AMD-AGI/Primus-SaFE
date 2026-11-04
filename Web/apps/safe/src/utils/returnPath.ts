/**
 * Returning to a web app outside the console after sign-in.
 *
 * Other web apps on this host (Dispatron, autopilot, the identity service's own pages)
 * sign a browser in by sending it to the console's `/login` with the path to come back to:
 *
 *   /login?redirect=<encodeURIComponent('/dispatron/runs/42?tab=log')>
 *
 * The console router only knows its own routes, so such a return is a full-page navigation.
 * Only same-origin paths under the prefixes below qualify: never another origin, never a
 * path the URL parser could turn into one ("//host", "/\host", control characters), and
 * never a path that leaves its prefix ("/dispatron/../x", "/dispatronx").
 */
export const OUTSIDE_CONSOLE_PREFIXES: readonly string[] = ['/dispatron', '/autopilot', '/binding']

// Whitespace and control characters are dropped or folded by the URL parser ("/\t/host").
const UNSAFE_CHARS = /[\u0000- \u007f\\]/

// Encoded slash, backslash or dot in the path: an app behind the prefix may decode them.
const ENCODED_SEPARATOR = /%(2f|5c|2e)/i

function underPrefix(pathname: string, prefix: string): boolean {
  return pathname === prefix || pathname.startsWith(`${prefix}/`)
}

/**
 * Returns the absolute same-origin URL to load with a full-page navigation when `value`
 * (a `redirect` query value, or the target saved for the sign-in) is a return to an
 * allow-listed app outside the console; otherwise null, and the caller stays in the console.
 */
export function outsideConsoleReturn(value: unknown, origin: string): string | null {
  const raw = Array.isArray(value) ? value[0] : value
  if (typeof raw !== 'string' || !raw.startsWith('/') || raw.startsWith('//')) return null
  if (UNSAFE_CHARS.test(raw)) return null

  let url: URL
  try {
    url = new URL(raw, origin)
  } catch {
    return null
  }
  if (url.origin !== origin || url.username || url.password) return null
  if (ENCODED_SEPARATOR.test(url.pathname)) return null
  // The path as written must already be the normalised one: "/dispatron/../login" is refused
  // rather than resolved, so what the app behind the prefix sees is what was checked here.
  if (!raw.startsWith(url.pathname)) return null
  if (!OUTSIDE_CONSOLE_PREFIXES.some((p) => underPrefix(url.pathname, p))) return null
  return url.href
}

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
  // Checked on the path as written: the URL parser already resolves "%2e%2e" segments.
  if (ENCODED_SEPARATOR.test(raw.split(/[?#]/, 1)[0])) return null
  // The path as written must already be the normalised one: "/dispatron/../login" is refused
  // rather than resolved, so what the app behind the prefix sees is what was checked here.
  if (!raw.startsWith(url.pathname)) return null
  if (!OUTSIDE_CONSOLE_PREFIXES.some((p) => underPrefix(url.pathname, p))) return null
  return url.href
}

// The last full-page return this tab made, kept in sessionStorage: { href, at }.
const LAST_RETURN_KEY = 'console.outsideReturn'

/** Within this time a second return to the same page is the app sending the browser back. */
export const REPEAT_RETURN_WINDOW_MS = 15_000

export const RETURN_REFUSED_MESSAGE =
  'The application sent you straight back after signing in, so you stay in the console. ' +
  'If it keeps happening, contact the owner of that application.'

/**
 * Loads `href` (an outsideConsoleReturn result) with a full-page navigation and returns true,
 * unless this tab already did that for the same URL less than REPEAT_RETURN_WINDOW_MS ago.
 * Then the app sent the browser straight back to /login: it does not accept the console's
 * session, and returning again would only loop. It returns false and the caller stays in the
 * console. If sessionStorage cannot be used the return still happens, without this guard.
 */
export function leaveConsoleFor(href: string, now = Date.now()): boolean {
  try {
    const last = JSON.parse(sessionStorage.getItem(LAST_RETURN_KEY) || 'null')
    const age = typeof last?.at === 'number' ? now - last.at : NaN
    if (last?.href === href && age >= 0 && age < REPEAT_RETURN_WINDOW_MS) return false
  } catch {}
  try {
    sessionStorage.setItem(LAST_RETURN_KEY, JSON.stringify({ href, at: now }))
  } catch {}
  window.location.replace(href)
  return true
}

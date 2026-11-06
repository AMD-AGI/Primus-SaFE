// Client-side mirror of the rules the apiserver applies to a workload's spec.hostpath
// (SaFE/common/pkg/workload/hostpath.go): an entry must lie under a hostpath volume the
// workspace mounts into the workload's scope, and for a volume with enableUserDir it may be
// neither the volume itself nor its `users` directory. The UI is stricter in two places: it
// refuses the root of any volume (the volume is already mounted), and it refuses a shared
// directory of the volume or a path under one (shared directories are already mounted, and a
// second mount of the same directory would collide or bypass its read-only flag).

/** Workload scopes as the workspace API names them (v1.WorkspaceScope). */
export type WorkloadScope = 'Train' | 'Infer' | 'Authoring' | 'CICD' | 'Ray' | 'Sandbox'

/** The fields of a workspace volume (GET /workspaces/:id) that decide what it declares. */
export interface HostPathVolumeLike {
  type?: string
  hostPath?: string
  subPath?: string
  mountPath?: string
  enableUserDir?: boolean
  accessMode?: string
  scopes?: string[] | null
  sharedPaths?: Array<{ path: string; readOnly?: boolean }> | null
}

/** A directory a hostpath volume declares for spec.hostpath. */
export interface HostPathRoot {
  /** hostPath joined with subPath: the directory the volume mounts. */
  path: string
  readOnly: boolean
  /** The volume has enableUserDir: its root and its users directory are off limits. */
  userDirs: boolean
  /** Shared directories of the volume, as absolute host paths. */
  sharedPaths: string[]
}

/** Directory under a volume with enableUserDir that holds the user directories. */
export const USER_DIRS_PATH = 'users'

const hasDotDot = (p: string) => p.split('/').includes('..')

/** Lexically cleans an absolute POSIX path ("/a//b/./c/" -> "/a/b/c"). Keeps ".." elements. */
export function cleanPath(p: string): string {
  const abs = p.startsWith('/')
  const parts = p.split('/').filter((s) => s !== '' && s !== '.')
  const joined = parts.join('/')
  return abs ? `/${joined}` : joined || '.'
}

const joinPath = (a: string, b?: string) => cleanPath(b ? `${a}/${b}` : a)

/** The path of `cleaned` below `root`, or null when it does not lie under (or at) `root`. */
function below(root: string, cleaned: string): string | null {
  if (cleaned === root) return ''
  if (root === '/') return cleaned.slice(1)
  if (cleaned.startsWith(`${root}/`)) return cleaned.slice(root.length + 1)
  return null
}

const appliesToScope = (vol: HostPathVolumeLike, scope?: WorkloadScope) =>
  !vol.scopes?.length || !scope || vol.scopes.includes(scope)

/**
 * What the hostpath volumes of the workspace mounted into workloads of `scope` declare
 * (DeclaredHostPathRoots). Volumes with a relative hostPath, an absolute subPath, or a ".."
 * element declare nothing.
 */
export function declaredHostPathRoots(
  volumes: HostPathVolumeLike[] | null | undefined,
  scope?: WorkloadScope,
): HostPathRoot[] {
  const roots: HostPathRoot[] = []
  for (const vol of volumes ?? []) {
    if (vol.type !== 'hostpath' || !appliesToScope(vol, scope)) continue
    const hostPath = vol.hostPath ?? ''
    const subPath = vol.subPath ?? ''
    if (!hostPath.startsWith('/') || hasDotDot(hostPath)) continue
    if (subPath.startsWith('/') || hasDotDot(subPath)) continue
    const path = joinPath(hostPath, subPath)
    roots.push({
      path,
      readOnly: vol.accessMode === 'ReadOnlyMany',
      userDirs: !!vol.enableUserDir,
      sharedPaths: (vol.sharedPaths ?? [])
        .map((s) => s?.path)
        .filter((s): s is string => !!s && !s.startsWith('/') && !hasDotDot(s))
        .map((s) => joinPath(path, s)),
    })
  }
  return roots
}

export interface HostPathCheckOptions {
  /**
   * Accept an entry under no hostpath volume of the workspace. The apiserver admits such an
   * entry only for holders of the workload/privileged grant (workspace and system admins).
   */
  allowUndeclared?: boolean
}

/**
 * Checks one spec.hostpath entry against the declared roots. Returns an error message, or ''
 * when the entry is acceptable.
 */
export function checkHostPathEntry(
  entry: string,
  roots: HostPathRoot[],
  opts: HostPathCheckOptions = {},
): string {
  const raw = entry.trim()
  if (!raw) return 'Path is empty'
  if (!raw.startsWith('/')) return `${raw}: must be an absolute path`
  if (hasDotDot(raw)) return `${raw}: must not contain ".."`
  const cleaned = cleanPath(raw)
  let under = false
  for (const root of roots) {
    const sub = below(root.path, cleaned)
    if (sub === null) continue
    under = true
    if (sub === '') {
      return `${cleaned} is the volume root, which is already mounted; pick a directory under it`
    }
    const first = sub.split('/')[0]
    if (root.userDirs && first === USER_DIRS_PATH) {
      return `${cleaned} is under ${joinPath(root.path, USER_DIRS_PATH)}; your own user directory is mounted automatically and other users' directories cannot be mounted`
    }
    const shared = root.sharedPaths.find((s) => below(s, cleaned) !== null)
    if (shared) {
      return `${cleaned} is in the shared directory ${shared}, which every workload of the workspace already mounts`
    }
  }
  if (under) return ''
  if (opts.allowUndeclared) return ''
  if (!roots.length) {
    return 'This workspace declares no host directory that workloads of this kind may mount'
  }
  return `${cleaned} is not under a workspace volume (${roots.map((r) => r.path).join(', ')})`
}

/**
 * The entries the form checks: those the user added. Entries carried over from the workload
 * being edited, cloned or resumed were accepted by the apiserver, whose rules are looser than
 * these (it admits a volume root without enableUserDir, a shared directory, and undeclared paths
 * for privileged users), so the form leaves them to the apiserver rather than block the action.
 */
export function entriesToCheck(
  entries: string[] | null | undefined,
  inherited: Iterable<string>,
): string[] {
  const keep = new Set(normalizeHostPaths([...inherited]))
  return normalizeHostPaths(entries).filter((e) => !keep.has(e))
}

/** Trims, cleans and de-duplicates entries, dropping empty ones. */
export function normalizeHostPaths(entries: string[] | null | undefined): string[] {
  const out: string[] = []
  for (const e of entries ?? []) {
    const t = typeof e === 'string' ? e.trim() : ''
    if (!t) continue
    const c = t.startsWith('/') ? cleanPath(t) : t
    if (!out.includes(c)) out.push(c)
  }
  return out
}

/** The user's own directory on each volume with enableUserDir, e.g. /shared_nfs/users/u123. */
export function ownUserDirs(roots: HostPathRoot[], userId?: string): string[] {
  if (!userId) return []
  return roots.filter((r) => r.userDirs).map((r) => joinPath(r.path, `${USER_DIRS_PATH}/${userId}`))
}

/** The payload fragment for spec.hostpath: omitted when there is nothing to mount. */
export function hostPathPayload(entries: string[] | null | undefined): { hostpath?: string[] } {
  const list = normalizeHostPaths(entries)
  return list.length ? { hostpath: list } : {}
}

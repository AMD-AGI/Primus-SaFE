import { describe, expect, it } from 'vitest'
import {
  checkHostPathEntry,
  cleanPath,
  declaredHostPathRoots,
  hostPathPayload,
  normalizeHostPaths,
  ownUserDirs,
  type HostPathVolumeLike,
} from './workloadHostPath'

// A workspace shaped like crusoe-spur-vk: /shared_nfs with user directories and shared
// directories, a training-only /apps, a read-only volume with a subPath, and a PFS volume.
const volumes: HostPathVolumeLike[] = [
  {
    type: 'hostpath',
    hostPath: '/shared_nfs',
    mountPath: '/shared_nfs',
    enableUserDir: true,
    sharedPaths: [{ path: 'models', readOnly: true }, { path: 'hf_cache' }],
  },
  { type: 'hostpath', hostPath: '/apps', mountPath: '/apps', scopes: ['Train'] },
  {
    type: 'hostpath',
    hostPath: '/teams',
    subPath: 'a',
    mountPath: '/team',
    accessMode: 'ReadOnlyMany',
  },
  { type: 'pfs', mountPath: '/wekafs' },
  // Declares nothing: relative hostPath, absolute subPath, ".." element.
  { type: 'hostpath', hostPath: 'rel', mountPath: '/rel' },
  { type: 'hostpath', hostPath: '/x', subPath: '/abs', mountPath: '/x' },
  { type: 'hostpath', hostPath: '/y/../z', mountPath: '/y' },
]

describe('declaredHostPathRoots', () => {
  it('takes the hostpath volumes mounted into the scope, joined with subPath', () => {
    expect(declaredHostPathRoots(volumes, 'Train')).toEqual([
      {
        path: '/shared_nfs',
        readOnly: false,
        userDirs: true,
        sharedPaths: ['/shared_nfs/models', '/shared_nfs/hf_cache'],
      },
      { path: '/apps', readOnly: false, userDirs: false, sharedPaths: [] },
      { path: '/teams/a', readOnly: true, userDirs: false, sharedPaths: [] },
    ])
  })

  it('leaves out volumes restricted to other scopes', () => {
    expect(declaredHostPathRoots(volumes, 'Infer').map((r) => r.path)).toEqual([
      '/shared_nfs',
      '/teams/a',
    ])
  })

  it('declares nothing for a missing workspace', () => {
    expect(declaredHostPathRoots(undefined, 'Train')).toEqual([])
    expect(declaredHostPathRoots(null)).toEqual([])
  })
})

describe('checkHostPathEntry', () => {
  const train = declaredHostPathRoots(volumes, 'Train')
  const infer = declaredHostPathRoots(volumes, 'Infer')

  it('accepts a directory under a volume', () => {
    expect(checkHostPathEntry('/shared_nfs/team-a', train)).toBe('')
    expect(checkHostPathEntry('/shared_nfs/team-a/data/', train)).toBe('')
    expect(checkHostPathEntry('/apps/data', train)).toBe('')
    expect(checkHostPathEntry('/teams/a/ckpt', train)).toBe('')
    expect(checkHostPathEntry('  /shared_nfs//usersx  ', train)).toBe('')
  })

  it('refuses the volume root', () => {
    for (const p of ['/shared_nfs', '/shared_nfs/', '/shared_nfs/.', '/apps', '/teams/a']) {
      expect(checkHostPathEntry(p, train)).toMatch(/volume root/)
    }
  })

  it('refuses the users directory and anything under it, the own one included', () => {
    for (const p of [
      '/shared_nfs/users',
      '/shared_nfs/users/',
      '/shared_nfs/users/u1',
      '/shared_nfs/users/u1/x',
    ]) {
      expect(checkHostPathEntry(p, train)).toMatch(/users/)
    }
  })

  it('allows a users directory under a volume without enableUserDir', () => {
    expect(checkHostPathEntry('/apps/users/x', train)).toBe('')
  })

  it('refuses a shared directory or a path in it', () => {
    expect(checkHostPathEntry('/shared_nfs/models', train)).toMatch(
      /shared directory \/shared_nfs\/models/,
    )
    expect(checkHostPathEntry('/shared_nfs/models/llama', train)).toMatch(/shared directory/)
    expect(checkHostPathEntry('/shared_nfs/models2', train)).toBe('')
  })

  it('refuses a path under no volume of the scope', () => {
    expect(checkHostPathEntry('/apps/data', infer)).toMatch(/not under a workspace volume/)
    expect(checkHostPathEntry('/etc', train)).toMatch(/not under a workspace volume/)
    expect(checkHostPathEntry('/shared_nfsx/a', train)).toMatch(/not under a workspace volume/)
    expect(checkHostPathEntry('/teams/b', train)).toMatch(/not under a workspace volume/)
  })

  it('says so when the workspace declares nothing for the scope', () => {
    expect(checkHostPathEntry('/shared_nfs/a', [])).toMatch(/declares no host directory/)
  })

  it('accepts an undeclared path only when allowed, never a forbidden one', () => {
    expect(checkHostPathEntry('/etc', train, { allowUndeclared: true })).toBe('')
    expect(checkHostPathEntry('/shared_nfs', train, { allowUndeclared: true })).toMatch(
      /volume root/,
    )
    expect(checkHostPathEntry('/shared_nfs/users/u2', train, { allowUndeclared: true })).toMatch(
      /users/,
    )
  })

  it('refuses relative paths, ".." elements and empty entries', () => {
    expect(checkHostPathEntry('shared_nfs/a', train)).toMatch(/absolute/)
    expect(checkHostPathEntry('/shared_nfs/a/../users/u2', train)).toMatch(/\.\./)
    expect(checkHostPathEntry('   ', train)).toMatch(/empty/)
  })
})

describe('normalizeHostPaths and hostPathPayload', () => {
  it('trims, cleans and de-duplicates', () => {
    expect(normalizeHostPaths([' /shared_nfs/a/ ', '/shared_nfs//a', '', '/shared_nfs/b'])).toEqual(
      ['/shared_nfs/a', '/shared_nfs/b'],
    )
    expect(normalizeHostPaths(undefined)).toEqual([])
    expect(normalizeHostPaths(null)).toEqual([])
  })

  it('omits hostpath when there is nothing to mount', () => {
    expect(hostPathPayload([])).toEqual({})
    expect(hostPathPayload(undefined)).toEqual({})
    expect(hostPathPayload(['  '])).toEqual({})
    expect(hostPathPayload(['/shared_nfs/a/'])).toEqual({ hostpath: ['/shared_nfs/a'] })
  })

  it('cleans paths lexically', () => {
    expect(cleanPath('/a//b/./c/')).toBe('/a/b/c')
    expect(cleanPath('/')).toBe('/')
  })
})

describe('ownUserDirs', () => {
  it('names the user directory on each volume with enableUserDir', () => {
    expect(ownUserDirs(declaredHostPathRoots(volumes, 'Train'), 'u1')).toEqual([
      '/shared_nfs/users/u1',
    ])
    expect(ownUserDirs(declaredHostPathRoots(volumes, 'Train'), '')).toEqual([])
  })
})

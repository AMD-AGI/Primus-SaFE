import { describe, expect, it } from 'vitest'
import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

// The console's nginx sends /api/v1/apikeys to the identity provider when the deployment says where it
// is (chart auth.api_keys_upstream -> API_KEYS_UPSTREAM). api-keys-route.sh writes that location at
// start-up; nginx.conf includes it ahead of its route to the apiserver.
const web = (rel: string) => new URL(`../../../../${rel}`, import.meta.url).pathname
const run = (upstream: string | undefined) => {
  const dir = mkdtempSync(join(tmpdir(), 'api-keys-route-'))
  const out = join(dir, 'route.conf')
  const env: NodeJS.ProcessEnv = { PATH: process.env.PATH }
  if (upstream !== undefined) env.API_KEYS_UPSTREAM = upstream
  try {
    const r = spawnSync('bash', [web('api-keys-route.sh'), out], { env, encoding: 'utf-8' })
    let conf: string | null = null
    try {
      conf = readFileSync(out, 'utf-8')
    } catch {
      conf = null
    }
    return { status: r.status, stderr: r.stderr, conf }
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
}

describe('api-keys-route.sh', () => {
  it('writes nothing when no upstream is set, so /api/v1/apikeys goes to the apiserver with the rest', () => {
    for (const upstream of [undefined, '']) {
      const r = run(upstream)
      expect(r.status).toBe(0)
      expect(r.conf).toBe('')
    }
  })

  it('routes /api/v1/apikeys and what is below it to the upstream, and nothing else', () => {
    const r = run('identity.idns.svc.cluster.local:8080')
    expect(r.status).toBe(0)
    expect(r.conf).toContain('location ~ ^/api/v1/apikeys(/.*)?$ {')
    expect(r.conf).toContain('set $api_keys_upstream identity.idns.svc.cluster.local:8080;')
    expect(r.conf).toContain('proxy_pass http://$api_keys_upstream;')
    // the apiserver's compatibility alias is not taken: the keys it issued stay reachable there
    expect(r.conf).not.toContain('/api/apikeys')
    expect(r.conf?.match(/location /g)).toHaveLength(1)
  })

  it.each([
    'evil;host',
    'a b',
    'http://idp:80',
    'idp:80/path',
    '-bad.host',
    'idp:123456',
    'idp\nlocation / {}',
  ])('refuses an upstream that is not host[:port]: %j', (upstream) => {
    const r = run(upstream)
    expect(r.status).not.toBe(0)
    expect(r.stderr).toContain('API_KEYS_UPSTREAM must be host[:port]')
  })
})

describe('the console image wires the route', () => {
  it('nginx.conf includes the generated file before the route to the apiserver', () => {
    const conf = readFileSync(web('nginx.conf'), 'utf-8')
    const include = conf.indexOf('include /tmp/api-keys-route.conf;')
    expect(include).toBeGreaterThan(0)
    expect(include).toBeLessThan(conf.indexOf('location /api/v1/ {'))
  })

  it('start.sh generates it before nginx starts, and the Dockerfile ships the script', () => {
    const start = readFileSync(web('start.sh'), 'utf-8')
    expect(start).toContain('/api-keys-route.sh /tmp/api-keys-route.conf')
    expect(start.indexOf('/api-keys-route.sh')).toBeLessThan(start.indexOf('exec nginx'))
    const docker = readFileSync(web('Dockerfile'), 'utf-8')
    expect(docker).toContain('COPY api-keys-route.sh /api-keys-route.sh')
    expect(docker).toMatch(/chmod \+x \/start\.sh \/api-keys-route\.sh/)
  })
})

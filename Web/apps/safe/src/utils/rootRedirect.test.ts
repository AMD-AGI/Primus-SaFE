import { describe, expect, it } from 'vitest'
import { spawnSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

// The console's nginx can send a browser that opens the bare / of its host elsewhere, such as the team portal
// (chart web.root_redirect -> ROOT_REDIRECT). root-redirect.sh writes that rule at start-up; nginx.conf includes
// it in its location for / alone.
const web = (rel: string) => fileURLToPath(new URL(`../../../../${rel}`, import.meta.url))
const run = (target: string | undefined) => {
  const dir = mkdtempSync(join(tmpdir(), 'root-redirect-'))
  const out = join(dir, 'redirect.conf')
  const env: NodeJS.ProcessEnv = { PATH: process.env.PATH }
  if (target !== undefined) env.ROOT_REDIRECT = target
  try {
    const r = spawnSync('bash', [web('root-redirect.sh'), out], { env, encoding: 'utf-8' })
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

describe('root-redirect.sh', () => {
  it('writes nothing when no target is set, so / keeps serving the console', () => {
    for (const target of [undefined, '']) {
      const r = run(target)
      expect(r.status).toBe(0)
      expect(r.conf).toBe('')
    }
  })

  it('redirects a plain page load of / to the path, so a sign-in returning to /?code=... still loads the console', () => {
    for (const target of ['/portal/', '/team/portal/']) {
      const r = run(target)
      expect(r.status).toBe(0)
      expect(r.conf).toBe(`if ($plain_page_load) { return 302 ${target}; }\n`)
    }
  })

  it.each([
    '/',
    '//evil.example.com/',
    'portal/',
    'http://portal.example.com/',
    'https://portal.example.com/',
    '/a b',
    '/portal/; return 200 x',
    '/x"y',
    "/x'y",
    '/x{',
    '/x}',
    '/$host',
    '/x\\y',
    '/portal/\nlocation / {}',
  ])(
    'refuses a target that is not a path on this host nginx can take as it is: %j',
    (target) => {
      const r = run(target)
      expect(r.status).not.toBe(0)
      expect(r.stderr).toContain('ROOT_REDIRECT must be a path on this host such as /portal/')
    },
  )
})

describe('the console image wires the redirect', () => {
  it('nginx.conf includes the generated file in the location for / alone, which otherwise serves the SPA', () => {
    const conf = readFileSync(web('nginx.conf'), 'utf-8')
    const root = conf.match(/location = \/ \{[^}]*\}/)?.[0]
    expect(root).toContain('include /tmp/root-redirect.conf;')
    expect(root).toContain('try_files $uri $uri/ /index.html;')
    // TLS ends in front of the console: an absolute redirect would be an http:// URL
    expect(root).toContain('absolute_redirect off;')
    expect(conf.match(/include \/tmp\/root-redirect\.conf;/g)).toHaveLength(1)
  })

  it('nginx.conf lets the redirect answer only a GET or HEAD without a query string', () => {
    const conf = readFileSync(web('nginx.conf'), 'utf-8')
    const map = conf.match(/map "\$request_method:\$args" \$plain_page_load \{[^}]*\}/)?.[0]
    expect(map?.match(/"[A-Z]+:"(?=\s+1;)/g)).toEqual(['"GET:"', '"HEAD:"'])
    expect(map).toContain('default 0;')
  })

  it('start.sh generates it before nginx starts, and the Dockerfile ships the script', () => {
    const start = readFileSync(web('start.sh'), 'utf-8')
    expect(start).toContain('/root-redirect.sh /tmp/root-redirect.conf')
    expect(start.indexOf('/root-redirect.sh')).toBeLessThan(start.indexOf('exec nginx'))
    const docker = readFileSync(web('Dockerfile'), 'utf-8')
    expect(docker).toContain('COPY root-redirect.sh /root-redirect.sh')
    expect(docker).toMatch(/chmod \+x [^\n]*\/root-redirect\.sh/)
  })
})

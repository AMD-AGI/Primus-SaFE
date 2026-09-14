import { describe, expect, it } from 'vitest'
import {
  buildGithubRunnerCreatePayload,
  buildGithubRunnerEditPayload,
  buildGithubRunnerEnv,
  buildGithubRunnerProxyAuth,
  buildGithubRunnerResources,
  mergeGithubRunnerEnv,
  validateGithubRunnerProxy,
  type GithubRunnerForm,
} from './githubRunnerPayload'

const PROXY = 'http://wstunnel-client.github-proxy.svc.cluster.local:3128'

const baseForm = (overrides: Partial<GithubRunnerForm> = {}): GithubRunnerForm => ({
  displayName: 'spur-autopilot-hosted',
  description: '',
  priority: 1,
  maxRetry: 50,
  image: 'ghcr.io/actions/actions-runner:latest',
  githubConfigUrl: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
  proxyUrl: '',
  proxyUsername: 'github',
  proxyPassword: '',
  isTolerateAll: true,
  forceHostNetwork: false,
  excludedNodes: [],
  secretIds: [],
  resource: {
    replica: 1,
    cpu: '2',
    gpu: '0',
    memory: '4',
    ephemeralStorage: '20',
  },
  ...overrides,
})

describe('GithubRunner env', () => {
  it('derives the runner label from the workload name', () => {
    expect(buildGithubRunnerEnv(baseForm())).toEqual({
      GITHUB_CONFIG_URL: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
      RUNNER_LABELS: 'spur-autopilot-hosted',
    })
  })

  it('carries the proxy URL but never the credential', () => {
    expect(
      buildGithubRunnerEnv(baseForm({ proxyUrl: ` ${PROXY} `, proxyPassword: ' b339b3c ' })),
    ).toEqual({
      GITHUB_CONFIG_URL: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
      RUNNER_LABELS: 'spur-autopilot-hosted',
      PROXY_URL: PROXY,
    })
  })

  it('drops the URL and the backend-owned Secret name when the proxy is cleared on edit', () => {
    expect(
      mergeGithubRunnerEnv(
        {
          GITHUB_CONFIG_URL: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
          RUNNER_LABELS: 'spur-autopilot-hosted',
          PROXY_URL: 'http://proxy:3128',
          PROXY_CREDENTIAL_SECRET: 'spur-autopilot-hosted-abc123',
          UNRELATED: 'keep-me',
        },
        baseForm(),
      ),
    ).toEqual({
      GITHUB_CONFIG_URL: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
      RUNNER_LABELS: 'spur-autopilot-hosted',
      UNRELATED: 'keep-me',
    })
  })

  it('keeps the Secret name while the proxy stays in place', () => {
    expect(
      mergeGithubRunnerEnv(
        { PROXY_URL: 'http://proxy:3128', PROXY_CREDENTIAL_SECRET: 'stored-secret' },
        baseForm({ proxyUrl: PROXY }),
      ),
    ).toMatchObject({ PROXY_URL: PROXY, PROXY_CREDENTIAL_SECRET: 'stored-secret' })
  })
})

describe('GithubRunner proxy credential', () => {
  it('is omitted without a URL or without a password', () => {
    expect(buildGithubRunnerProxyAuth(baseForm({ proxyPassword: 'secret' }))).toBeUndefined()
    expect(buildGithubRunnerProxyAuth(baseForm({ proxyUrl: PROXY }))).toBeUndefined()
  })

  it('falls back to the service account username', () => {
    expect(
      buildGithubRunnerProxyAuth(
        baseForm({ proxyUrl: PROXY, proxyUsername: '  ', proxyPassword: ' b339b3c ' }),
      ),
    ).toEqual({ username: 'github', password: 'b339b3c' })
  })
})

describe('GithubRunner resources', () => {
  it('sends the requested replica and suffixes sizes with Gi', () => {
    expect(
      buildGithubRunnerResources(baseForm({ resource: { ...baseForm().resource, replica: 3 } })),
    ).toEqual([{ replica: 3, cpu: '2', memory: '4Gi', ephemeralStorage: '20Gi' }])
  })

  it('omits gpu when none is requested and keeps it otherwise', () => {
    expect(buildGithubRunnerResources(baseForm())[0]).not.toHaveProperty('gpu')
    expect(
      buildGithubRunnerResources(baseForm({ resource: { ...baseForm().resource, gpu: '2' } }))[0]
        .gpu,
    ).toBe('2')
  })

  it('does not double-suffix a size that already carries Gi', () => {
    expect(
      buildGithubRunnerResources(
        baseForm({ resource: { ...baseForm().resource, memory: '4Gi', ephemeralStorage: '20Gi' } }),
      )[0],
    ).toMatchObject({ memory: '4Gi', ephemeralStorage: '20Gi' })
  })
})

describe('GithubRunner proxy validation', () => {
  it('asks for nothing until a proxy URL is entered', () => {
    expect(validateGithubRunnerProxy(baseForm())).toEqual({})
  })

  it('requires a username and a password alongside the URL', () => {
    expect(validateGithubRunnerProxy(baseForm({ proxyUrl: PROXY, proxyUsername: '' }))).toEqual({
      proxyUsername: 'Please input the proxy username',
      proxyPassword: 'Please input the proxy password',
    })
    expect(validateGithubRunnerProxy(baseForm({ proxyUrl: PROXY, proxyPassword: 's' }))).toEqual({})
  })

  it('lets an edit keep the password stored in the Secret', () => {
    expect(
      validateGithubRunnerProxy(baseForm({ proxyUrl: PROXY }), { hasStoredCredential: true }),
    ).toEqual({})
  })

  it('rejects the URL shapes the API refuses', () => {
    const rejected = [
      'http://proxy.internal', // no explicit port
      'https://proxy.internal:3128', // https is not a forward-proxy scheme here
      'http://user:pw@proxy.internal:3128', // credentials belong in proxyAuth
      'http://proxy.internal:3128/squid', // non-root path
      'proxy.internal:3128', // not absolute
    ]
    for (const proxyUrl of rejected) {
      expect(
        validateGithubRunnerProxy(baseForm({ proxyUrl, proxyPassword: 's' })).proxyUrl,
      ).toBeTruthy()
    }
  })
})

describe('GithubRunner create payload', () => {
  it('sends images, resources and the proxy credential as workload fields', () => {
    const payload = buildGithubRunnerCreatePayload(
      baseForm({ proxyUrl: PROXY, proxyPassword: 'b339b3c61d282c470dc9d1e78d84bcfb' }),
      {
        workspace: 'control-plan-hyperloom',
        githubAuth: { type: 'registration_token', token: 'CLOJQCKV5GDWX2W7MTRQNVLKT7ERY' },
      },
    )

    expect(payload).toEqual({
      displayName: 'spur-autopilot-hosted',
      groupVersionKind: { kind: 'GithubRunner', version: 'v1' },
      workspace: 'control-plan-hyperloom',
      useWorkspaceStorage: true,
      images: ['ghcr.io/actions/actions-runner:latest'],
      resources: [{ replica: 1, cpu: '2', memory: '4Gi', ephemeralStorage: '20Gi' }],
      env: {
        GITHUB_CONFIG_URL: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
        RUNNER_LABELS: 'spur-autopilot-hosted',
        PROXY_URL: PROXY,
      },
      githubAuth: { type: 'registration_token', token: 'CLOJQCKV5GDWX2W7MTRQNVLKT7ERY' },
      proxyAuth: { username: 'github', password: 'b339b3c61d282c470dc9d1e78d84bcfb' },
      priority: 1,
      maxRetry: 50,
      isTolerateAll: true,
      forceHostNetwork: false,
    })
  })

  it('includes optional fields only when they carry a value', () => {
    const payload = buildGithubRunnerCreatePayload(
      baseForm({
        description: 'hosted runner',
        excludedNodes: ['node-a', ''],
        secretIds: ['secret-1'],
      }),
      {
        workspace: 'control-plan-hyperloom',
        githubAuth: { type: 'registration_token', token: 'token' },
        useWorkspaceStorage: false,
      },
    )

    expect(payload).toMatchObject({
      description: 'hosted runner',
      excludedNodes: ['node-a'],
      secrets: [{ id: 'secret-1' }],
      useWorkspaceStorage: false,
    })
    expect(payload).not.toHaveProperty('proxyAuth')
  })
})

describe('GithubRunner edit payload', () => {
  it('patches only the editable fields and preserves unrelated env', () => {
    expect(
      buildGithubRunnerEditPayload(
        baseForm({ description: 'updated', priority: 2, maxRetry: 10 }),
        {
          UNRELATED: 'keep-me',
        },
      ),
    ).toEqual({
      description: 'updated',
      priority: 2,
      maxRetry: 10,
      resources: [{ replica: 1, cpu: '2', memory: '4Gi', ephemeralStorage: '20Gi' }],
      env: {
        UNRELATED: 'keep-me',
        GITHUB_CONFIG_URL: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
        RUNNER_LABELS: 'spur-autopilot-hosted',
      },
    })
  })

  it('omits proxyAuth when the password field was left alone, so the stored one survives', () => {
    const untouched = buildGithubRunnerEditPayload(baseForm({ proxyUrl: PROXY }), {
      PROXY_CREDENTIAL_SECRET: 'stored-secret',
    })
    expect(untouched).not.toHaveProperty('proxyAuth')
    expect(untouched.env).toMatchObject({ PROXY_CREDENTIAL_SECRET: 'stored-secret' })

    expect(
      buildGithubRunnerEditPayload(baseForm({ proxyUrl: PROXY, proxyPassword: 'rotated' }), {}),
    ).toMatchObject({ proxyAuth: { username: 'github', password: 'rotated' } })
  })
})

import { describe, expect, it } from 'vitest'
import {
  buildGithubRunnerCreatePayload,
  buildGithubRunnerEditPayload,
  buildGithubRunnerEnv,
  buildGithubRunnerResources,
  mergeGithubRunnerEnv,
  validateGithubRunnerProxy,
  type GithubRunnerForm,
} from './githubRunnerPayload'

const baseForm = (overrides: Partial<GithubRunnerForm> = {}): GithubRunnerForm => ({
  displayName: 'spur-autopilot-hosted',
  description: '',
  priority: 1,
  maxRetry: 50,
  image: 'ghcr.io/actions/actions-runner:latest',
  githubConfigUrl: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
  githubProxyUrl: '',
  githubProxyPassword: '',
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

  it('writes the proxy password alongside the proxy URL', () => {
    expect(
      buildGithubRunnerEnv(
        baseForm({
          githubProxyUrl: ' http://wstunnel-client.github-proxy.svc.cluster.local:3128 ',
          githubProxyPassword: ' b339b3c ',
        }),
      ),
    ).toEqual({
      GITHUB_CONFIG_URL: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
      RUNNER_LABELS: 'spur-autopilot-hosted',
      GITHUB_PROXY_URL: 'http://wstunnel-client.github-proxy.svc.cluster.local:3128',
      GITHUB_PROXY_PASSWORD: 'b339b3c',
    })
  })

  it('drops stored proxy keys when the URL is cleared on edit', () => {
    expect(
      mergeGithubRunnerEnv(
        {
          GITHUB_CONFIG_URL: 'https://github.com/AMD-BRAIN-Internal/spur-autopilot',
          RUNNER_LABELS: 'spur-autopilot-hosted',
          GITHUB_PROXY_URL: 'http://proxy:3128',
          GITHUB_PROXY_PASSWORD: 'secret',
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
  it('accepts an empty proxy URL', () => {
    expect(validateGithubRunnerProxy(baseForm())).toEqual([])
  })

  it('requires a password once a proxy URL is set', () => {
    expect(validateGithubRunnerProxy(baseForm({ githubProxyUrl: 'http://proxy:3128' }))).toEqual([
      'Please input GitHub proxy password',
    ])
    expect(
      validateGithubRunnerProxy(
        baseForm({ githubProxyUrl: 'http://proxy:3128', githubProxyPassword: 'secret' }),
      ),
    ).toEqual([])
  })
})

describe('GithubRunner create payload', () => {
  it('sends images and resources as workload fields rather than env wrappers', () => {
    const payload = buildGithubRunnerCreatePayload(
      baseForm({
        githubProxyUrl: 'http://wstunnel-client.github-proxy.svc.cluster.local:3128',
        githubProxyPassword: 'b339b3c61d282c470dc9d1e78d84bcfb',
      }),
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
        GITHUB_PROXY_URL: 'http://wstunnel-client.github-proxy.svc.cluster.local:3128',
        GITHUB_PROXY_PASSWORD: 'b339b3c61d282c470dc9d1e78d84bcfb',
      },
      githubAuth: { type: 'registration_token', token: 'CLOJQCKV5GDWX2W7MTRQNVLKT7ERY' },
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
  })
})

describe('GithubRunner edit payload', () => {
  it('patches only the editable fields and preserves unrelated env', () => {
    expect(
      buildGithubRunnerEditPayload(
        baseForm({ description: 'updated', priority: 2, maxRetry: 10 }),
        { UNRELATED: 'keep-me' },
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
})

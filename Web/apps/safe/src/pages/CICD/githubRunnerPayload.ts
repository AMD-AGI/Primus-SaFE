import { WorkloadKind, type GitHubAuthPayload } from '@/services/workload/type'

// A GithubRunner is driven entirely by these env keys; the runner image reads them on
// startup. Unlike AutoscalingRunnerSet, nothing is wrapped into RESOURCES/IMAGE/ENTRYPOINT
// env -- images and resources go on the payload as the backend fields they really are.
export const GITHUB_CONFIG_URL_ENV = 'GITHUB_CONFIG_URL'
export const RUNNER_LABELS_ENV = 'RUNNER_LABELS'
export const GITHUB_PROXY_URL_ENV = 'GITHUB_PROXY_URL'
export const GITHUB_PROXY_PASSWORD_ENV = 'GITHUB_PROXY_PASSWORD'

export interface GithubRunnerResourceForm {
  replica: number
  cpu: string
  gpu?: string
  memory: string
  ephemeralStorage: string
}

export interface GithubRunnerForm {
  displayName: string
  description: string
  priority: number
  maxRetry: number
  image: string
  githubConfigUrl: string
  githubProxyUrl: string
  githubProxyPassword: string
  isTolerateAll: boolean
  forceHostNetwork: boolean
  excludedNodes: string[]
  secretIds: string[]
  resource: GithubRunnerResourceForm
}

export interface GithubRunnerResourcePayload {
  replica: number
  cpu: string
  gpu?: string
  memory: string
  ephemeralStorage: string
}

export interface GithubRunnerCreatePayload {
  displayName: string
  groupVersionKind: { kind: WorkloadKind.GithubRunner; version: string }
  workspace: string
  useWorkspaceStorage: boolean
  images: string[]
  resources: GithubRunnerResourcePayload[]
  env: Record<string, string>
  githubAuth: GitHubAuthPayload
  priority: number
  maxRetry: number
  isTolerateAll: boolean
  forceHostNetwork: boolean
  description?: string
  excludedNodes?: string[]
  secrets?: Array<{ id: string }>
}

export interface GithubRunnerEditPayload {
  description: string
  priority: number
  maxRetry: number
  resources: GithubRunnerResourcePayload[]
  env: Record<string, string>
}

const clean = (value?: string) => (value ?? '').trim()

const withGi = (value: string) => {
  const raw = clean(value)
  return /gi$/i.test(raw) ? raw : `${raw}Gi`
}

// The runner registers itself under RUNNER_LABELS, which is what workflows target in
// `runs-on`. Deriving it from the workload name keeps the label discoverable from the
// list page instead of being a second value users have to remember.
export function buildGithubRunnerEnv(
  form: Pick<
    GithubRunnerForm,
    'displayName' | 'githubConfigUrl' | 'githubProxyUrl' | 'githubProxyPassword'
  >,
): Record<string, string> {
  const proxyUrl = clean(form.githubProxyUrl)
  return {
    [GITHUB_CONFIG_URL_ENV]: clean(form.githubConfigUrl),
    [RUNNER_LABELS_ENV]: clean(form.displayName),
    // The password is only meaningful alongside a proxy URL, so the pair is written and
    // dropped together.
    ...(proxyUrl
      ? {
          [GITHUB_PROXY_URL_ENV]: proxyUrl,
          [GITHUB_PROXY_PASSWORD_ENV]: clean(form.githubProxyPassword),
        }
      : {}),
  }
}

// Edit patches env as a whole, so clearing the proxy URL has to remove the stored keys
// rather than just omit them from the new values.
export function mergeGithubRunnerEnv(
  existingEnv: Record<string, string> | undefined,
  form: Parameters<typeof buildGithubRunnerEnv>[0],
): Record<string, string> {
  const merged = { ...existingEnv, ...buildGithubRunnerEnv(form) }
  if (!clean(form.githubProxyUrl)) {
    delete merged[GITHUB_PROXY_URL_ENV]
    delete merged[GITHUB_PROXY_PASSWORD_ENV]
  }
  return merged
}

export function buildGithubRunnerResources(
  form: Pick<GithubRunnerForm, 'resource'>,
): GithubRunnerResourcePayload[] {
  const gpu = clean(form.resource.gpu)
  return [
    {
      replica: Number(form.resource.replica) || 1,
      cpu: clean(form.resource.cpu),
      ...(Number(gpu) > 0 ? { gpu } : {}),
      memory: withGi(form.resource.memory),
      ephemeralStorage: withGi(form.resource.ephemeralStorage),
    },
  ]
}

export function validateGithubRunnerProxy(
  form: Pick<GithubRunnerForm, 'githubProxyUrl' | 'githubProxyPassword'>,
): string[] {
  if (!clean(form.githubProxyUrl)) return []
  return clean(form.githubProxyPassword) ? [] : ['Please input GitHub proxy password']
}

export function buildGithubRunnerCreatePayload(
  form: GithubRunnerForm,
  options: {
    workspace: string
    githubAuth: GitHubAuthPayload
    useWorkspaceStorage?: boolean
  },
): GithubRunnerCreatePayload {
  const excludedNodes = (form.excludedNodes ?? []).filter(Boolean)
  const secrets = (form.secretIds ?? []).filter(Boolean).map((id) => ({ id }))
  const description = clean(form.description)

  return {
    displayName: clean(form.displayName),
    groupVersionKind: { kind: WorkloadKind.GithubRunner, version: 'v1' },
    workspace: options.workspace,
    useWorkspaceStorage: options.useWorkspaceStorage ?? true,
    images: [clean(form.image)],
    resources: buildGithubRunnerResources(form),
    env: buildGithubRunnerEnv(form),
    githubAuth: options.githubAuth,
    priority: form.priority,
    maxRetry: form.maxRetry,
    isTolerateAll: form.isTolerateAll,
    forceHostNetwork: form.forceHostNetwork,
    ...(description ? { description } : {}),
    ...(excludedNodes.length ? { excludedNodes } : {}),
    ...(secrets.length ? { secrets } : {}),
  }
}

export function buildGithubRunnerEditPayload(
  form: GithubRunnerForm,
  existingEnv: Record<string, string> | undefined,
): GithubRunnerEditPayload {
  return {
    description: form.description,
    priority: form.priority,
    maxRetry: form.maxRetry,
    resources: buildGithubRunnerResources(form),
    env: mergeGithubRunnerEnv(existingEnv, form),
  }
}

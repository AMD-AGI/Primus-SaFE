import { WorkloadKind, type GitHubAuthPayload } from '@/services/workload/type'

// A GithubRunner is driven entirely by these env keys; the runner image reads them on
// startup. Unlike AutoscalingRunnerSet, nothing is wrapped into RESOURCES/IMAGE/ENTRYPOINT
// env -- images and resources go on the payload as the backend fields they really are.
export const GITHUB_CONFIG_URL_ENV = 'GITHUB_CONFIG_URL'
export const RUNNER_LABELS_ENV = 'RUNNER_LABELS'
export const PROXY_URL_ENV = 'PROXY_URL'
// Written by the backend, never by this form: it stores the submitted proxyAuth in a
// Secret and records that Secret's name here. An edit still has to carry the key
// through, or the runner is left naming a credential it no longer has.
export const PROXY_CREDENTIAL_SECRET_ENV = 'PROXY_CREDENTIAL_SECRET'

// The proxy is reached with a service account rather than a per-user login, so the
// username is the same for every runner unless an operator changed it.
export const DEFAULT_PROXY_USERNAME = 'github'

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
  proxyUrl: string
  proxyUsername: string
  proxyPassword: string
  isTolerateAll: boolean
  forceHostNetwork: boolean
  excludedNodes: string[]
  secretIds: string[]
  resource: GithubRunnerResourceForm
}

export interface GithubRunnerProxyAuth {
  username: string
  password: string
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
  proxyAuth?: GithubRunnerProxyAuth
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
  proxyAuth?: GithubRunnerProxyAuth
}

const clean = (value?: string) => (value ?? '').trim()

const withGi = (value: string) => {
  const raw = clean(value)
  return /gi$/i.test(raw) ? raw : `${raw}Gi`
}

// Mirrors the API's admission check. A forward proxy address has no well-known port to
// fall back on, so an omitted one is rejected rather than guessed, and the credential
// has to travel as proxyAuth instead of as userinfo in the URL.
export function isValidProxyUrl(value: string): boolean {
  let parsed: URL
  try {
    parsed = new URL(clean(value))
  } catch {
    return false
  }
  const port = Number(parsed.port)
  return (
    parsed.protocol === 'http:' &&
    !parsed.username &&
    !parsed.password &&
    !parsed.search &&
    !parsed.hash &&
    (parsed.pathname === '' || parsed.pathname === '/') &&
    Number.isInteger(port) &&
    port >= 1 &&
    port <= 65535
  )
}

// The runner registers itself under RUNNER_LABELS, which is what workflows target in
// `runs-on`. Deriving it from the workload name keeps the label discoverable from the
// list page instead of being a second value users have to remember.
export function buildGithubRunnerEnv(
  form: Pick<GithubRunnerForm, 'displayName' | 'githubConfigUrl' | 'proxyUrl'>,
): Record<string, string> {
  const proxyUrl = clean(form.proxyUrl)
  return {
    [GITHUB_CONFIG_URL_ENV]: clean(form.githubConfigUrl),
    [RUNNER_LABELS_ENV]: clean(form.displayName),
    ...(proxyUrl ? { [PROXY_URL_ENV]: proxyUrl } : {}),
  }
}

// The credential is a sibling of env, not a member of it: the backend moves it into a
// Secret and rejects a request that both submits it and names an existing Secret.
export function buildGithubRunnerProxyAuth(
  form: Pick<GithubRunnerForm, 'proxyUrl' | 'proxyUsername' | 'proxyPassword'>,
): GithubRunnerProxyAuth | undefined {
  const password = clean(form.proxyPassword)
  if (!clean(form.proxyUrl) || !password) return undefined
  return { username: clean(form.proxyUsername) || DEFAULT_PROXY_USERNAME, password }
}

// Edit patches env as a whole, so clearing the proxy URL has to remove the stored keys
// rather than just omit them from the new values.
export function mergeGithubRunnerEnv(
  existingEnv: Record<string, string> | undefined,
  form: Parameters<typeof buildGithubRunnerEnv>[0],
): Record<string, string> {
  const merged = { ...existingEnv, ...buildGithubRunnerEnv(form) }
  if (!clean(form.proxyUrl)) {
    delete merged[PROXY_URL_ENV]
    delete merged[PROXY_CREDENTIAL_SECRET_ENV]
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

export interface GithubRunnerProxyErrors {
  proxyUrl?: string
  proxyUsername?: string
  proxyPassword?: string
}

// `hasStoredCredential` is what an edit passes: the password lives in a Secret the API
// carries forward, so an empty field means "keep it" rather than "there is none".
export function validateGithubRunnerProxy(
  form: Pick<GithubRunnerForm, 'proxyUrl' | 'proxyUsername' | 'proxyPassword'>,
  options: { hasStoredCredential?: boolean } = {},
): GithubRunnerProxyErrors {
  if (!clean(form.proxyUrl)) return {}

  const errors: GithubRunnerProxyErrors = {}
  if (!isValidProxyUrl(form.proxyUrl)) {
    errors.proxyUrl = 'Enter an http URL with an explicit port, e.g. http://proxy.internal:3128'
  }
  if (!clean(form.proxyUsername)) {
    errors.proxyUsername = 'Please input the proxy username'
  }
  if (!clean(form.proxyPassword) && !options.hasStoredCredential) {
    errors.proxyPassword = 'Please input the proxy password'
  }
  return errors
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
  const proxyAuth = buildGithubRunnerProxyAuth(form)

  return {
    displayName: clean(form.displayName),
    groupVersionKind: { kind: WorkloadKind.GithubRunner, version: 'v1' },
    workspace: options.workspace,
    useWorkspaceStorage: options.useWorkspaceStorage ?? true,
    images: [clean(form.image)],
    resources: buildGithubRunnerResources(form),
    env: buildGithubRunnerEnv(form),
    githubAuth: options.githubAuth,
    ...(proxyAuth ? { proxyAuth } : {}),
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
  // An omitted proxyAuth leaves the stored credential in place, which is what an edit
  // that did not touch the password field should do.
  const proxyAuth = buildGithubRunnerProxyAuth(form)

  return {
    description: form.description,
    priority: form.priority,
    maxRetry: form.maxRetry,
    resources: buildGithubRunnerResources(form),
    env: mergeGithubRunnerEnv(existingEnv, form),
    ...(proxyAuth ? { proxyAuth } : {}),
  }
}

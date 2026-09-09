import type { GitHubAuthPayload } from '@/services/workload/type'

// 'registration_token' selects the GithubRunner kind rather than AutoscalingRunnerSet:
// the runner registers itself with a short-lived token instead of letting ARC mint
// credentials from an App or PAT. It carries the same single token field as 'pat'.
export type GitHubAuthType = 'github_app' | 'pat' | 'registration_token'

export interface GitHubAuthForm {
  githubAuthType: GitHubAuthType
  githubAppId: string
  githubAppInstallationId: string
  githubAppPrivateKey: string
  githubToken: string
}

const clean = (value: string) => value.trim()

const isTokenAuth = (type: GitHubAuthType): type is 'pat' | 'registration_token' =>
  type === 'pat' || type === 'registration_token'

export const isGithubRunnerAuth = (type: GitHubAuthType) => type === 'registration_token'

export const validateGitHubAuthForm = (form: GitHubAuthForm): string[] => {
  if (isTokenAuth(form.githubAuthType)) {
    return clean(form.githubToken) ? [] : ['Please input GitHub token']
  }

  const missing: string[] = []
  if (!clean(form.githubAppId)) missing.push('Please input GitHub App ID')
  if (!clean(form.githubAppInstallationId)) missing.push('Please input GitHub App installation ID')
  if (!clean(form.githubAppPrivateKey)) missing.push('Please input GitHub App private key')
  return missing
}

export const buildGitHubAuthPayload = (form: GitHubAuthForm): GitHubAuthPayload => {
  if (isTokenAuth(form.githubAuthType)) {
    return {
      type: form.githubAuthType,
      token: clean(form.githubToken),
    }
  }

  return {
    type: 'github_app',
    appId: clean(form.githubAppId),
    installationId: clean(form.githubAppInstallationId),
    privateKey: clean(form.githubAppPrivateKey),
  }
}

export type GitHubAuthType = 'github_app' | 'pat'

export interface GitHubAuthForm {
  githubAuthType: GitHubAuthType
  githubAppId: string
  githubAppInstallationId: string
  githubAppPrivateKey: string
  githubPAT: string
}

export type GitHubAuthPayload =
  | {
      type: 'github_app'
      appId: string
      installationId: string
      privateKey: string
    }
  | {
      type: 'pat'
      token: string
    }

const clean = (value: string) => value.trim()

export const validateGitHubAuthForm = (form: GitHubAuthForm): string[] => {
  if (form.githubAuthType === 'pat') {
    return clean(form.githubPAT) ? [] : ['Please input GitHub PAT']
  }

  const missing: string[] = []
  if (!clean(form.githubAppId)) missing.push('Please input GitHub App ID')
  if (!clean(form.githubAppInstallationId)) missing.push('Please input GitHub App installation ID')
  if (!clean(form.githubAppPrivateKey)) missing.push('Please input GitHub App private key')
  return missing
}

export const buildGitHubAuthPayload = (form: GitHubAuthForm): GitHubAuthPayload => {
  if (form.githubAuthType === 'pat') {
    return {
      type: 'pat',
      token: clean(form.githubPAT),
    }
  }

  return {
    type: 'github_app',
    appId: clean(form.githubAppId),
    installationId: clean(form.githubAppInstallationId),
    privateKey: clean(form.githubAppPrivateKey),
  }
}

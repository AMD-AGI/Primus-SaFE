import { describe, expect, it } from 'vitest'
import {
  buildGitHubAuthPayload,
  isGithubRunnerAuth,
  validateGitHubAuthForm,
  type GitHubAuthForm,
} from '../githubAuth'

const baseForm = (overrides: Partial<GitHubAuthForm>): GitHubAuthForm => ({
  githubAuthType: 'github_app',
  githubAppId: '',
  githubAppInstallationId: '',
  githubAppPrivateKey: '',
  githubToken: '',
  ...overrides,
})

describe('CICD GitHub auth payload', () => {
  it('builds a GitHub App payload', () => {
    expect(
      buildGitHubAuthPayload(
        baseForm({
          githubAppId: ' 12345 ',
          githubAppInstallationId: ' 67890 ',
          githubAppPrivateKey: ' private-key ',
        }),
      ),
    ).toEqual({
      type: 'github_app',
      appId: '12345',
      installationId: '67890',
      privateKey: 'private-key',
    })
  })

  it('builds a legacy PAT payload', () => {
    expect(
      buildGitHubAuthPayload(
        baseForm({
          githubAuthType: 'pat',
          githubToken: ' ghp_test ',
        }),
      ),
    ).toEqual({
      type: 'pat',
      token: 'ghp_test',
    })
  })

  it('builds a registration token payload from the same token field', () => {
    expect(
      buildGitHubAuthPayload(
        baseForm({
          githubAuthType: 'registration_token',
          githubToken: ' CLOJQCKV5GDWX2W7MTRQNVLKT7ERY ',
        }),
      ),
    ).toEqual({
      type: 'registration_token',
      token: 'CLOJQCKV5GDWX2W7MTRQNVLKT7ERY',
    })
  })

  it('validates required GitHub App fields', () => {
    expect(validateGitHubAuthForm(baseForm({ githubAppId: '12345' }))).toEqual([
      'Please input GitHub App installation ID',
      'Please input GitHub App private key',
    ])
  })

  it('validates the token field for both token-based auth types', () => {
    expect(validateGitHubAuthForm(baseForm({ githubAuthType: 'pat' }))).toEqual([
      'Please input GitHub token',
    ])
    expect(validateGitHubAuthForm(baseForm({ githubAuthType: 'registration_token' }))).toEqual([
      'Please input GitHub token',
    ])
  })

  it('only treats registration_token as the GithubRunner kind', () => {
    expect(isGithubRunnerAuth('registration_token')).toBe(true)
    expect(isGithubRunnerAuth('pat')).toBe(false)
    expect(isGithubRunnerAuth('github_app')).toBe(false)
  })
})

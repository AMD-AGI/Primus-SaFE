import { describe, expect, it } from 'vitest'
import { buildGitHubAuthPayload, validateGitHubAuthForm, type GitHubAuthForm } from '../githubAuth'

const baseForm = (overrides: Partial<GitHubAuthForm>): GitHubAuthForm => ({
  githubAuthType: 'github_app',
  githubAppId: '',
  githubAppInstallationId: '',
  githubAppPrivateKey: '',
  githubPAT: '',
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
          githubPAT: ' ghp_test ',
        }),
      ),
    ).toEqual({
      type: 'pat',
      token: 'ghp_test',
    })
  })

  it('validates required GitHub App fields', () => {
    expect(validateGitHubAuthForm(baseForm({ githubAppId: '12345' }))).toEqual([
      'Please input GitHub App installation ID',
      'Please input GitHub App private key',
    ])
  })

  it('validates legacy PAT field', () => {
    expect(validateGitHubAuthForm(baseForm({ githubAuthType: 'pat' }))).toEqual([
      'Please input GitHub PAT',
    ])
  })
})

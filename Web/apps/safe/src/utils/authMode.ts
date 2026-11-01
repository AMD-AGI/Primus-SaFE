import type { EnvsResp } from '@/services/login/type'

/**
 * With `authMode: 'external'` (reported by GET /envs) an identity provider owns sign-in,
 * accounts, API keys and platform roles, and the server refuses the matching writes for
 * everyone, local accounts included. The console uses this to hide what can only fail:
 * the password form, registration, password changes and platform role editing, and API
 * key creation unless the deployment routes it to the provider (apiKeysProviderUrl). A
 * server that reports no mode (an older apiserver) is builtin.
 */
export const isExternalAuth = (envs: Pick<EnvsResp, 'authMode'> | null | undefined): boolean =>
  envs?.authMode === 'external'

/**
 * Where the identity provider serves API keys, as a path on this console's origin, or '' when
 * there is none: the mode is not external, or the deployment routes nothing there (the server
 * reports `apiKeysUrl` only then). Only a path counts, so a value that points elsewhere is
 * taken as not set and the console never sends the session to another origin.
 */
export const apiKeysProviderUrl = (
  envs: Pick<EnvsResp, 'authMode' | 'apiKeysUrl'> | null | undefined,
): string => {
  const url = envs?.authMode === 'external' ? (envs.apiKeysUrl ?? '').trim() : ''
  if (!url.startsWith('/') || url.startsWith('//') || /[\\\s?#]/.test(url)) return ''
  return url.replace(/\/+$/, '')
}

/**
 * What the API Keys page does for a deployment:
 * - `providerUrl`: where the identity provider's keys are created, listed and revoked; '' means the
 *   list is the apiserver's own, as with built-in accounts;
 * - `creatable`: whether there is anything to create a key with. The apiserver refuses to issue keys
 *   in external mode, so without a provider route there is not;
 * - `legacy`: whether the apiserver's own keys are listed beside the provider's. With a provider route
 *   they are not the provider's to list, yet the apiserver still accepts them, so they stay revocable.
 */
export const apiKeyRouting = (
  envs: Pick<EnvsResp, 'authMode' | 'apiKeysUrl'> | null | undefined,
): { providerUrl: string; creatable: boolean; legacy: boolean } => {
  const providerUrl = apiKeysProviderUrl(envs)
  return {
    providerUrl,
    creatable: !isExternalAuth(envs) || providerUrl !== '',
    legacy: providerUrl !== '',
  }
}

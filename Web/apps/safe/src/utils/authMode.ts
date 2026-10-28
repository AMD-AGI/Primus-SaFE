import type { EnvsResp } from '@/services/login/type'

/**
 * With `authMode: 'external'` (reported by GET /envs) an identity provider owns sign-in,
 * accounts, API keys and platform roles, and the server refuses the matching writes for
 * everyone, local accounts included. The console uses this to hide what can only fail:
 * the password form, registration, password changes, API key management and platform
 * role editing. A server that reports no mode (an older apiserver) is builtin.
 */
export const isExternalAuth = (envs: Pick<EnvsResp, 'authMode'> | null | undefined): boolean =>
  envs?.authMode === 'external'

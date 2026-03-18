// SSO Configuration — all secrets come from env vars with safe fallbacks
export const SSO_CONFIG = {
  // Okta OAuth Settings (from environment variables)
  OKTA_DOMAIN: import.meta.env.VITE_SSO_OKTA_DOMAIN || '',
  CLIENT_ID: import.meta.env.VITE_SSO_CLIENT_ID || '',
  AUTHORIZATION_ENDPOINT: import.meta.env.VITE_SSO_AUTH_ENDPOINT || '',
  TOKEN_ENDPOINT: import.meta.env.VITE_SSO_TOKEN_ENDPOINT || '',

  // OAuth Settings
  RESPONSE_TYPE: 'code',
  SCOPE: 'openid profile email',

  // Dynamically use the current origin to avoid redirect_uri mismatch across envs
  get REGISTERED_REDIRECT_URI(): string {
    return window.location.origin
  },

  LENS_IDENTIFIER: 'lens_redirect',

  buildAuthUrl(state: string): string {
    // This callback URL is parsed by SaFE to redirect back to the correct environment
    const lensRedirect = window.location.origin + '/lens/sso-bridge'

    // Format: lens:{state}:{redirect_url} — must be parsable by SaFE
    const lensState = `lens:${state}:${encodeURIComponent(lensRedirect)}`

    const params = new URLSearchParams({
      client_id: this.CLIENT_ID,
      redirect_uri: this.REGISTERED_REDIRECT_URI,
      response_type: this.RESPONSE_TYPE,
      response_mode: 'query',
      scope: this.SCOPE,
      state: lensState
    })

    return `${this.AUTHORIZATION_ENDPOINT}?${params.toString()}`
  },

  parseState(stateString: string): { app: string; state: string; redirect: string } | null {
    try {
      // Expected format: lens:{state}:{redirect_url}
      if (!stateString || !stateString.startsWith('lens:')) {
        return null
      }

      const parts = stateString.split(':')
      if (parts.length >= 3) {
        return {
          app: parts[0],
          state: parts[1],
          redirect: decodeURIComponent(parts.slice(2).join(':')) // URL may contain colons
        }
      }

      return null
    } catch {
      return null
    }
  }
}

// Backend API endpoints for SSO
export const SSO_API = {
  // Exchange authorization code for token
  EXCHANGE_CODE: '/api/v1/auth/sso/token',
  
  // Get user info using token
  GET_USER_INFO: '/api/v1/auth/sso/userinfo',
  
  // Logout endpoint
  LOGOUT: '/api/v1/auth/logout'
}

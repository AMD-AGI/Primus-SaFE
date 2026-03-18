/**
 * TraceLens auth helper functions
 * Ensures iframe can carry correct auth info when accessing TraceLens UI
 */

import request from '@/services/request'

/**
 * Pre-check TraceLens session access permissions
 * Called before loading iframe to ensure auth cookies are set
 */
export async function preAuthCheck(sessionId: string): Promise<boolean> {
  try {
    // Make a simple API request to ensure cookies are set
    // This request goes through the request instance, which automatically includes withCredentials
    const response = await request.get(`/tracelens/sessions/${sessionId}`)
    return response?.status === 'ready' || response?.status === 'initializing'
  } catch (error) {
    console.error('[TraceLens Auth] Pre-auth check failed:', error)
    return false
  }
}

/**
 * Add auth headers to iframe (if needed)
 * Note: iframe src does not support custom headers, this is a fallback
 */
export function buildAuthenticatedUrl(url: string): string {
  // Get current auth info
  const authInfo = localStorage.getItem('lens_auth')
  
  if (authInfo) {
    try {
      const { userId } = JSON.parse(authInfo)
      // Can consider adding query parameters to URL as fallback auth
      // But primarily relies on cookies
      const separator = url.includes('?') ? '&' : '?'
      return `${url}${separator}_uid=${userId}`
    } catch (e) {
      console.error('[TraceLens Auth] Failed to parse auth info:', e)
    }
  }
  
  return url
}

/**
 * Check if iframe can access the URL (via test request)
 */
export async function testIframeAccess(url: string): Promise<boolean> {
  try {
    // Use fetch API to test directly, since iframe is also a direct browser request
    const response = await fetch(url, {
      method: 'HEAD',
      credentials: 'include', // ensure cookies are sent
      mode: 'no-cors' // avoid CORS issues
    })
    
    // In no-cors mode, we cannot read the response status
    // If no exception was thrown, the request succeeded
    return true
  } catch (error) {
    console.error('[TraceLens Auth] Test access failed:', error)
    return false
  }
}

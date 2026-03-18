import { defineStore } from 'pinia'
import { login, getUserData, logout } from '@/services/auth'
import type { LoginReq, UserData } from '@/services/auth'

type SessionState = 'unknown' | 'anonymous' | 'authenticated'

export const useUserStore = defineStore('user', {
  state: () => ({
    session: 'unknown' as SessionState,
    userId: '',
    profile: null as UserData | null,
    _initPromise: null as Promise<void> | null,
    _profileFetched: false,
  }),

  persist: {
    key: 'user',
    storage: localStorage,
    paths: ['session', 'userId', 'profile'],
  },

  getters: {
    isLogin: (state) => state.session === 'authenticated',
    
    displayName: (state) => state.profile?.name || 'User',
    
    isAdmin: (state) => state.profile?.roles?.includes('admin') ?? false,
  },

  actions: {

    async login(payload: LoginReq) {
      try {
        // Mark login in progress
        sessionStorage.setItem('is_logging_in', 'true')
        
        const loginRes = await login(payload)
        
        if (!loginRes?.id) {
          throw new Error('Invalid response from login: missing id')
        }
        
        // Extra validation: check if returned id is empty string
        if (!loginRes.id.trim()) {
          throw new Error('Invalid response from login: empty id')
        }
        
        this.userId = loginRes.id
        this.session = 'authenticated'
        
        // Wait briefly for cookie to be set
        await new Promise(resolve => setTimeout(resolve, 100))
        
        // Try to fetch user profile — also verifies if session was truly established
        try {
          await this.fetchUser()
        } catch (error) {
          console.error('Could not fetch user profile:', error)
          // Set basic profile from login response if available
          if (loginRes.name) {
            this.profile = {
              id: loginRes.id,
              name: loginRes.name,
              email: '',
              roles: []
            }
          }
        }
        
        // Login complete, clear flags and SSO retry counters
        sessionStorage.removeItem('is_logging_in')
        sessionStorage.removeItem('sso_auto_attempts')
        sessionStorage.removeItem('sso_last_attempt_time')
        
        return loginRes
      } catch (error: any) {
        console.error('Login failed:', error)
        this.session = 'anonymous'
        this.userId = ''
        this.profile = null
        
        // Clear flag even on login failure
        sessionStorage.removeItem('is_logging_in')
        
        throw error
      }
    },

    async fetchUser(force = false) {
      if (!this.userId) {
        throw new Error('No user ID available')
      }
      
      if (!force && this._profileFetched && this.profile) return
      
      try {
        this.profile = await getUserData(this.userId)
        this._profileFetched = true
        this.session = 'authenticated'
      } catch (error) {
        console.error('Failed to fetch user profile:', error)
        this.session = 'anonymous'
      }
    },

    async ensureSessionOnce() {
      if (this._initPromise) return this._initPromise
      
      this._initPromise = (async () => {
        if (!this.userId) {
          this.session = 'anonymous'
          this._profileFetched = true
          return
        }
        
        try {
          await this.fetchUser()
        } catch {
          this.session = 'anonymous'
          this.profile = null
          this.userId = ''
          this._profileFetched = true
        }
      })()
      
      return this._initPromise
    },

    async logout() {
      try {
        await logout()
      } catch (error) {
        console.error('Logout API error:', error)
      } finally {
        // Clear all auth info
        this.$patch({
          session: 'anonymous',
          userId: '',
          profile: null,
          _profileFetched: false,
          _initPromise: null,
        })
        
        // Clear auth info in localStorage
        localStorage.removeItem('lens_auth')
        
        // Clear SSO-related info in sessionStorage
        sessionStorage.removeItem('sso.redirect')
        sessionStorage.removeItem('oauth_state')
        sessionStorage.removeItem('is_logging_in')
        
        // Clear all keys starting with sso_processed_ (prevent old flags from affecting new login)
        for (let i = sessionStorage.length - 1; i >= 0; i--) {
          const key = sessionStorage.key(i)
          if (key && key.startsWith('sso_processed_')) {
            sessionStorage.removeItem(key)
          }
        }
        
        // Mark as just logged out, prevent login page from auto-triggering SSO causing loop
        sessionStorage.setItem('just_logged_out', 'true')
      }
    },

  },
})

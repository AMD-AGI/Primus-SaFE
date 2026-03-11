<template>
  <div class="login-page">
    <!-- Background -->
    <div class="login-background">
      <div class="gradient-layer"></div>
      <div class="grid-pattern"></div>
    </div>

    <!-- Login Card -->
    <div class="login-card">
      <!-- Header -->
      <div class="card-header">
        <img class="logo" :src="isDark ? '/logo_w.png' : '/logo_b.png'" alt="Primus Lens" />
        <h1 class="title">{{ loading ? 'Redirecting...' : 'Sign in to Primus Lens' }}</h1>
        <p class="subtitle">
          {{ loading ? 'Redirecting to single sign-on, please wait...' : 'Access GPU monitoring and analysis platform' }}
        </p>
      </div>

      <!-- Body -->
      <div class="card-body">
        <div v-if="loading" class="loading-state">
          <el-icon class="loading-icon" :size="48">
            <Loading />
          </el-icon>
          <p class="loading-text">Redirecting to AMD Single Sign-On...</p>
          <p class="loading-hint">You will be redirected shortly. Please wait.</p>
        </div>

        <div v-else class="sso-section">
          <el-button
            type="primary"
            size="large"
            class="sso-button"
            @click="handleSSO"
          >
            <img
              class="sso-logo"
              :src="isDark ? '/logo_w.png' : '/logo_b.png'"
              alt="AMD"
            />
            <span>Continue with AMD SSO</span>
          </el-button>

          <div class="divider">
            <span>Secure authentication via AMD</span>
          </div>

          <div class="info-section">
            <el-alert
              type="info"
              :closable="false"
              show-icon
            >
              <template #title>
                <span class="alert-title">Single Sign-On Required</span>
              </template>
              <template #default>
                <p class="alert-content">
                  This application requires AMD corporate authentication. 
                  Click the button above to sign in with your AMD credentials.
                </p>
              </template>
            </el-alert>
          </div>
        </div>
      </div>

      <!-- Footer -->
      <div class="card-footer">
        <span class="version">v1.0.0</span>
        <el-tooltip content="Toggle Theme" placement="top">
          <el-button
            circle
            size="small"
            @click="toggleDark()"
          >
            <el-icon>
              <Moon v-if="!isDark" />
              <Sunny v-else />
            </el-icon>
          </el-button>
        </el-tooltip>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Loading, Moon, Sunny } from '@element-plus/icons-vue'
import { useUserStore } from '@/stores/user'
import { useDark, useToggle } from '@vueuse/core'
import { SSO_CONFIG } from '@/config/sso'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

const isDark = useDark()
const toggleDark = useToggle(isDark)

const loading = ref(false)

// Get safe redirect URL
function getSafeRedirect(v: unknown): string {
  const s = Array.isArray(v) ? v[0] : (v as string) || ''
  // Only allow internal paths
  if (!s || s.startsWith('http') || s.startsWith('//')) return '/'
  
  // Do not redirect to URLs containing auth code (avoid loops)
  if (s.includes('code=') || s.includes('/sso-bridge') || s.includes('/sso/')) {
    console.warn('Ignoring unsafe redirect with auth params:', s)
    return '/'
  }
  
  return s.startsWith('/') ? s : '/'
}

// Handle SSO login
async function handleSSO() {
  loading.value = true
  
  try {
    // Get redirect URL from query params
    const redirect = getSafeRedirect(route.query.redirect) || '/'
    
    // Store redirect URL in session storage
    sessionStorage.setItem('sso.redirect', redirect)
    
    // Generate simple alphanumeric state for CSRF protection
    // Ensure state starts with a letter to avoid being parsed as a number
    // Use only lowercase letters to avoid ambiguity
    const chars = 'abcdefghijklmnopqrstuvwxyz'
    const state = 'st' + Array.from({ length: 18 }, () => 
      chars.charAt(Math.floor(Math.random() * chars.length))
    ).join('')
    
    sessionStorage.setItem('oauth_state', state)
    
    // Build and redirect to Okta auth URL
    const authUrl = SSO_CONFIG.buildAuthUrl(state)
    window.location.href = authUrl
  } catch (error) {
    loading.value = false
    ElMessage.error('Failed to initialize SSO. Please try again.')
    console.error('SSO initialization error:', error)
  }
}

// SSO retry limit to prevent infinite loops
const SSO_MAX_ATTEMPTS = 3
const SSO_ATTEMPTS_KEY = 'sso_auto_attempts'
const SSO_LAST_ATTEMPT_KEY = 'sso_last_attempt_time'

function getSSOAttempts(): number {
  const val = sessionStorage.getItem(SSO_ATTEMPTS_KEY)
  return val ? parseInt(val, 10) : 0
}

function incrementSSOAttempts() {
  const current = getSSOAttempts()
  sessionStorage.setItem(SSO_ATTEMPTS_KEY, String(current + 1))
  sessionStorage.setItem(SSO_LAST_ATTEMPT_KEY, String(Date.now()))
}

function resetSSOAttempts() {
  sessionStorage.removeItem(SSO_ATTEMPTS_KEY)
  sessionStorage.removeItem(SSO_LAST_ATTEMPT_KEY)
}

// Auto-redirect to SSO on mount
onMounted(async () => {
  // Check if user is already logged in
  await userStore.ensureSessionOnce()
  
  if (userStore.isLogin) {
    // Login successful, clear retry count
    resetSSOAttempts()
    const redirect = getSafeRedirect(route.query.redirect)
    router.replace(redirect)
    return
  }
  
  // Check if the user just logged out (determined via sessionStorage flag)
  const justLoggedOut = sessionStorage.getItem('just_logged_out')
  if (justLoggedOut) {
    sessionStorage.removeItem('just_logged_out')
    loading.value = false
    resetSSOAttempts()
    return
  }
  
  // Check SSO retry count to prevent infinite loops
  const attempts = getSSOAttempts()
  const lastAttemptTime = parseInt(sessionStorage.getItem(SSO_LAST_ATTEMPT_KEY) || '0', 10)
  const timeSinceLastAttempt = Date.now() - lastAttemptTime
  
  // If max attempts exceeded within 30 seconds, stop auto SSO
  if (attempts >= SSO_MAX_ATTEMPTS && timeSinceLastAttempt < 30000) {
    console.warn(`[Login] Auto-SSO blocked: ${attempts} attempts in last 30 seconds`)
    loading.value = false
    resetSSOAttempts()
    ElMessage.warning('Auto sign-in failed multiple times. Please click the button to sign in manually.')
    return
  }
  
  // Reset counter if more than 30 seconds have passed
  if (timeSinceLastAttempt >= 30000) {
    resetSSOAttempts()
  }
  
  // Auto-start SSO after a short delay
  incrementSSOAttempts()
  loading.value = true
  setTimeout(() => {
    handleSSO()
  }, 1500)
})
</script>

<style scoped lang="scss">
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  position: relative;
}

.login-background {
  position: fixed;
  inset: 0;
  z-index: -1;
  pointer-events: none;
  
  .gradient-layer {
    position: absolute;
    inset: 0;
    opacity: 0.6;
    background:
      radial-gradient(1200px 600px at 60% -10%, rgba(64, 158, 255, 0.1), transparent 70%),
      radial-gradient(800px 400px at 20% 110%, rgba(103, 194, 58, 0.08), transparent 70%);
  }
  
  .grid-pattern {
    position: absolute;
    inset: 0;
    opacity: 0.1;
    background-image:
      linear-gradient(rgba(64, 158, 255, 0.1) 1px, transparent 1px),
      linear-gradient(90deg, rgba(64, 158, 255, 0.1) 1px, transparent 1px);
    background-size: 40px 40px;
  }
}

.login-card {
  width: 100%;
  max-width: 480px;
  background: var(--el-bg-color);
  border-radius: 20px;
  box-shadow: 
    0 20px 60px rgba(0, 0, 0, 0.1),
    0 0 0 1px rgba(0, 0, 0, 0.05);
  overflow: hidden;
  
  .card-header {
    padding: 40px 40px 30px;
    text-align: center;
    background: linear-gradient(135deg, var(--el-bg-color) 0%, rgba(64, 158, 255, 0.02) 100%);
    border-bottom: 1px solid var(--el-border-color-lighter);
    
    .logo {
      height: 48px;
      margin-bottom: 20px;
    }
    
    .title {
      font-size: 24px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin: 0 0 8px;
    }
    
    .subtitle {
      font-size: 14px;
      color: var(--el-text-color-secondary);
      margin: 0;
    }
  }
  
  .card-body {
    padding: 40px;
    
    .loading-state {
      text-align: center;
      padding: 40px 0;
      
      .loading-icon {
        color: var(--el-color-primary);
        animation: rotate 2s linear infinite;
      }
      
      .loading-text {
        font-size: 16px;
        color: var(--el-text-color-primary);
        margin: 20px 0 8px;
      }
      
      .loading-hint {
        font-size: 14px;
        color: var(--el-text-color-secondary);
      }
    }
    
    .sso-section {
      .sso-button {
        width: 100%;
        height: 48px;
        font-size: 16px;
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 12px;
        
        .sso-logo {
          height: 24px;
        }
      }
      
      .divider {
        text-align: center;
        margin: 24px 0;
        position: relative;
        
        span {
          font-size: 12px;
          color: var(--el-text-color-secondary);
          background: var(--el-bg-color);
          padding: 0 16px;
          position: relative;
          z-index: 1;
        }
        
        &::before {
          content: '';
          position: absolute;
          top: 50%;
          left: 0;
          right: 0;
          height: 1px;
          background: var(--el-border-color-lighter);
        }
      }
      
      .info-section {
        .alert-title {
          font-weight: 600;
        }
        
        .alert-content {
          margin: 8px 0 0;
          font-size: 13px;
          line-height: 1.5;
        }
      }
    }
  }
  
  .card-footer {
    padding: 20px 40px;
    border-top: 1px solid var(--el-border-color-lighter);
    display: flex;
    justify-content: space-between;
    align-items: center;
    
    .version {
      font-size: 12px;
      color: var(--el-text-color-secondary);
    }
  }
}

@keyframes rotate {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

</style>

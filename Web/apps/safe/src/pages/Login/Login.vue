<template>
  <div class="min-h-screen flex items-center justify-center p-6">
    <!-- Background -->
    <div class="fixed inset-0 -z-1 pointer-events-none">
      <div
        class="absolute inset-0 opacity-60"
        style="
          background:
            radial-gradient(1200px 600px at 60% -10%, rgba(99, 102, 241, 0.1), transparent 70%),
            radial-gradient(800px 400px at 20% 110%, rgba(34, 197, 94, 0.08), transparent 70%);
        "
      ></div>
      <svg class="absolute inset-0 w-full h-full opacity-30" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
            <path d="M 40 0 L 0 0 0 40" fill="none" stroke="currentColor" stroke-width="0.6" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#grid)" />
      </svg>
    </div>

    <!-- Login card -->
    <div class="login-card w-full max-w-[480px] @1440:max-w-[520px]">
      <!-- Header -->
      <div class="px-6 py-5 header-row">
        <div class="flex items-center gapx-8">
          <div>
            <div class="textx-18 font-600 leading-tight">
              {{ autoSSO ? 'Redirecting...' : 'Sign in' }}
            </div>
            <div class="text-[13px] opacity-80 mt-1">
              <span v-if="autoSSO"> Redirecting to single sign-on, please wait… </span>
              <span v-else> Access your account to continue </span>
            </div>
          </div>
        </div>
      </div>

      <!-- Body -->
      <div class="px-6 pt-4 pb-2">
        <!-- /login And ssoEnable=true: automatic SSO mode -->
        <template v-if="autoSSO">
          <div class="py-12 text-center text-[13px] opacity-80">
            <el-icon class="mb-3">
              <Loading />
            </el-icon>
            <div class="mb-1">Redirecting to SSO…</div>
            <div class="mb-4">You will be redirected to AMD single sign-on shortly.</div>
            <div>
              Having trouble?
              <RouterLink to="/login-admin" class="text-primary"> Use admin login </RouterLink>
            </div>
          </div>
        </template>

        <!-- /login-admin or SSO not enabled: show original form -->
        <template v-else>
          <el-form
            ref="formRef"
            :model="form"
            :rules="rules"
            label-position="left"
            label-width="auto"
            size="large"
            class="login-form"
          >
            <el-form-item prop="name" label="Name">
              <el-input v-model="form.name" placeholder="Name" autocomplete="username" clearable />
            </el-form-item>

            <el-form-item prop="password" label="Password">
              <el-input
                ref="pwdInputRef"
                v-model="form.password"
                type="password"
                placeholder="Your password"
                :prefix-icon="Lock"
                show-password
              />
            </el-form-item>

            <el-button
              class="w-full"
              type="primary"
              size="large"
              :loading="loading"
              @click="onSubmit"
            >
              Sign In
            </el-button>

            <div class="flex items-center justify-between mb-2">
              <el-checkbox v-model="form.remember">Remember me</el-checkbox>
            </div>

            <!-- /login-admin: keep more login options section below -->
            <div v-if="userStore.envs?.ssoEnable" class="mt-16 text-center">
              <el-divider>more login options</el-divider>
              <el-button type="primary" class="w-full mb-2" plain @click="startSSO">
                <img
                  class="app-logo"
                  :src="isDark ? '/logo_w.png' : '/logo_b.png'"
                  alt="Primus SaFE"
                  style="width: 35px; margin-right: 15px"
                />
                Continue with AMD
              </el-button>
            </div>

            <div v-else class="mt-3 text-center text-[13px] opacity-80">
              Don’t have an account?
              <el-link type="primary" :underline="false" @click="onRegister">Create one</el-link>
            </div>
          </el-form>
        </template>
      </div>

      <!-- Footer -->
      <div class="px-6 py-4 footer-row">
        <div class="flex items-center justify-between">
          <div class="text-[12px] opacity-70">v{{ version }}</div>
          <div class="flex items-center gap-2">
            <el-tooltip content="Light / Dark" placement="top">
              <el-button circle size="small" @click="toggleDark()">
                <el-icon><Moon /></el-icon>
              </el-button>
            </el-tooltip>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted, nextTick, toRaw, computed } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { Lock, Moon, Loading } from '@element-plus/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { toggleDark } from '@/composables'
import { randomState } from '@/utils'
import { useDark } from '@vueuse/core'

const isDark = useDark()

const version = '1.0.0'
const ENTRY_KEY = 'sso.redirect'
const OAUTH_STATE_KEY = 'oauth_state'
const SSO_ATTEMPTS_KEY = 'sso.attempts' // Track SSO attempt history (fallback protection)
const MAX_SSO_ATTEMPTS = 5 // Max attempts (adjusted from 3 to 5 for more chances)
const ATTEMPT_WINDOW_MS = 60000 // 60-second attempt window (extended from 30s to 60s)

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const formRef = ref<FormInstance>()
const pwdInputRef = ref()
const loading = ref(false)

const form = reactive({
  name: '',
  type: 'default',
  password: '',
  remember: true,
})

const rules: FormRules = {
  name: [
    { required: true, message: 'Please input account name', trigger: 'blur' },
    { min: 3, message: 'At least 3 characters', trigger: 'blur' },
  ],
  password: [
    { required: true, message: 'Please input password', trigger: 'blur' },
    { max: 64, message: 'Do not more than 64 characters', trigger: 'blur' },
  ],
}

const isAdminLogin = computed(() => route.name === 'LoginAdmin')
const autoSSO = computed(() => !isAdminLogin.value && !!userStore.envs?.ssoEnable)

function getSafeRedirect(v: unknown) {
  const s = Array.isArray(v) ? v[0] : (v as string) || ''
  // Only allow internal paths
  if (!s || s.startsWith('http') || s.startsWith('//')) return '/'
  return s.startsWith('/') ? s : '/'
}

function cleanExpiredAttempts() {
  // Clean up expired SSO attempt records
  const attemptsStr = localStorage.getItem(SSO_ATTEMPTS_KEY) || '[]'
  try {
    const attempts = JSON.parse(attemptsStr) as number[]
    const now = Date.now()
    const recentAttempts = attempts.filter((t) => now - t < ATTEMPT_WINDOW_MS)

    if (recentAttempts.length === 0) {
      // All records expired, clear data
      localStorage.removeItem(SSO_ATTEMPTS_KEY)
    } else if (recentAttempts.length < attempts.length) {
      // Partially expired, update records
      localStorage.setItem(SSO_ATTEMPTS_KEY, JSON.stringify(recentAttempts))
    }
  } catch (_error) {
    // Invalid data format, clear directly
    localStorage.removeItem(SSO_ATTEMPTS_KEY)
    console.warn('[SSO] Invalid attempts data, cleared')
  }
}

function recordSSOAttempt() {
  // Record this SSO attempt (using localStorage to avoid cross-domain loss)
  const now = Date.now()
  const attemptsStr = localStorage.getItem(SSO_ATTEMPTS_KEY) || '[]'
  const attempts = JSON.parse(attemptsStr) as number[]

  // Keep only attempts within the time window
  const recentAttempts = attempts.filter((t) => now - t < ATTEMPT_WINDOW_MS)
  recentAttempts.push(now)

  localStorage.setItem(SSO_ATTEMPTS_KEY, JSON.stringify(recentAttempts))
  return recentAttempts.length
}

function clearSSOAttempts() {
  localStorage.removeItem(SSO_ATTEMPTS_KEY)
}

// ========== Core function: redirect to SSO login page ==========
function startSSO() {
  if (!userStore.envs?.ssoAuthUrl) {
    ElMessage.error('SSO is not configured')
    return
  }

  // 1. Record redirect target (where to go after successful login)
  const target = (route.query.redirect as string) || '/'
  sessionStorage.setItem(ENTRY_KEY, target)

  // 2. Generate random state (security verification)
  const state = randomState()
  sessionStorage.setItem(OAUTH_STATE_KEY, state)

  // 3. Build SSO URL
  const u = new URL(userStore.envs?.ssoAuthUrl)
  u.searchParams.set('state', state)

  if (!u.searchParams.has('response_mode')) u.searchParams.set('response_mode', 'query')

  // 4. Redirect
  window.location.assign(u.toString())
}

async function onSubmit() {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    loading.value = true
    try {
      const { remember, ...payload } = toRaw(form)
      await userStore.login(payload)
      ElMessage.success('Signed in successfully')

      // Login successful, clear SSO attempt records and block flag
      clearSSOAttempts()
      sessionStorage.removeItem('sso.blocked')

      if (form.remember) localStorage.setItem('login.name', form.name)
      else localStorage.removeItem('login.name')

      const safe = getSafeRedirect(route.query.redirect)

      if (userStore.shouldAutoShowQuickStart) {
        // First-time manager: go to QuickStart, put original target in next
        const full = typeof safe === 'string' ? safe : router.resolve(safe).fullPath
        await router.replace({ name: 'QuickStart', query: { next: full } })
      } else if (userStore.shouldAutoShowUserQuickStart) {
        // First-time regular user: go to UserQuickStart
        const full = typeof safe === 'string' ? safe : router.resolve(safe).fullPath
        await router.replace({ name: 'UserQuickStart', query: { next: full } })
      } else {
        // Returning user: redirect per original logic
        await router.replace(safe)
      }
    } finally {
      loading.value = false
    }
  })
}

function onRegister() {
  router.push('/register')
}

onMounted(async () => {
  // ========== Data preparation ==========
  cleanExpiredAttempts() // 🛡️ Protection: clean up expired data

  try {
    await userStore.fetchEnvs() // Fetch environment config (including SSO config)
  } catch (error) {
    console.error('[Login] Failed to fetch envs:', error)
  }

  // Populate form data
  const q = route.query
  if (typeof q.name === 'string') form.name = q.name
  if (typeof q.type === 'string') form.type = q.type
  const savedName = localStorage.getItem('login.name')
  if (savedName && !form.name) form.name = savedName

  // ========== Core: auto SSO redirect logic ==========
  if (autoSSO.value) {
    // 🛡️ Guard 1: check block flag (prevent error page loop)
    const ssoBlocked = sessionStorage.getItem('sso.blocked')
    if (ssoBlocked === 'true') {
      console.warn('[SSO] SSO is blocked')
      ElMessage.warning(
        'SSO authentication is currently disabled. Please contact your administrator.',
      )
      return
    }

    // 🛡️ Guard 2: record attempt count (prevent infinite loop)
    const attemptCount = recordSSOAttempt()

    // 🛡️ Guard 3: stop if max attempts exceeded
    if (attemptCount > MAX_SSO_ATTEMPTS) {
      console.error(`[SSO] BLOCKED: Exceeded ${MAX_SSO_ATTEMPTS} attempts`)
      sessionStorage.setItem('sso.blocked', 'true')
      nextTick(() => {
        router.replace({
          path: '/sso-error',
          query: {
            error: 'sso_repeated_failure',
            error_description:
              'SSO authentication failed multiple times. Your account may not have the required permissions. Please contact your administrator to get access.',
          },
        })
      })
      return
    }

    // ✅ Core: redirect to SSO
    startSSO() // 👈👈👈 This is the core! Redirect to SSO
    return
  }

  // ========== Non-SSO mode: focus input field ==========
  await nextTick()
  try {
    if (!q.name) {
      ;(pwdInputRef.value?.input as HTMLInputElement)?.focus()
    }
  } catch {}
})
</script>

<style scoped>
/* —— 3D feel: soft shadow + translucent border + rounded corners + divider —— */
.login-card {
  --dlg-radius: 16px;
  --dlg-shadow: 0 18px 50px rgba(0, 0, 0, 0.18), 0 3px 10px rgba(0, 0, 0, 0.06);
  --dlg-border: 1px solid color-mix(in oklab, var(--el-border-color) 75%, transparent);
  --dlg-bg: color-mix(in oklab, var(--el-bg-color) 96%, #fff 4%);
  border-radius: var(--dlg-radius);
  background: var(--dlg-bg);
  box-shadow: var(--dlg-shadow);
  border: var(--dlg-border);
  overflow: hidden;
  backdrop-filter: saturate(1.1) blur(2px);
}

/* Header/footer divider and subtle background */
.header-row,
.footer-row {
  background: color-mix(in oklab, var(--el-bg-color) 98%, #fff 2%);
  border-bottom: 1px solid color-mix(in oklab, var(--el-border-color) 85%, transparent);
}
.footer-row {
  border-top: 1px solid color-mix(in oklab, var(--el-border-color) 85%, transparent);
  border-bottom: none;
}

/* Input fields with more depth: border + shadow (slight lift on hover/focus) */
.login-form :deep(.el-input__wrapper) {
  border: 1px solid color-mix(in oklab, var(--el-border-color) 88%, transparent);
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.04) inset;
  transition:
    box-shadow 0.2s ease,
    border-color 0.2s ease,
    background-color 0.2s ease;
}
.login-form :deep(.el-input__wrapper:hover) {
  border-color: color-mix(
    in oklab,
    var(--el-border-color) 60%,
    var(--safe-primary, var(--el-color-primary)) 15%
  );
}
.login-form :deep(.el-input.is-focus .el-input__wrapper),
.login-form :deep(.is-focus .el-input__wrapper) {
  box-shadow: 0 0 0 2px
    color-mix(in oklab, var(--safe-primary, var(--el-color-primary)) 24%, transparent);
  border-color: color-mix(
    in oklab,
    var(--safe-primary, var(--el-color-primary)) 40%,
    var(--el-border-color) 60%
  );
}

/* Large screen strategy: add spacing, don't significantly increase font size */
@media (min-width: 1440px) {
  .header-row {
    padding: 18px 24px 14px;
  }
  .footer-row {
    padding: 14px 24px 16px;
  }
}
</style>

<template>
  <div class="hl-login" :data-theme="theme">
    <div class="hl-login-bg">
      <svg class="hl-login-grid" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
            <path d="M 40 0 L 0 0 0 40" fill="none" stroke="currentColor" stroke-width="0.5" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#grid)" />
      </svg>
    </div>

    <div class="hl-login-card">
      <div class="hl-login-header">
        <span class="brand-amd">AMD</span>
        <span class="brand-name">HyperLoom</span>
      </div>

      <div class="hl-login-body">
        <!-- SSO Redirect -->
        <template v-if="status === 'redirecting'">
          <div class="hl-login-spinner"></div>
          <p>Redirecting to AMD single sign-on...</p>
        </template>

        <!-- Processing callback -->
        <template v-else-if="status === 'processing'">
          <div class="hl-login-spinner"></div>
          <p>Completing sign-in...</p>
        </template>

        <!-- Dev cookie bridge -->
        <template v-else-if="status === 'dev-bridge'">
          <p class="hl-login-hint">
            From SaFE DevTools → Console, run:<br>
            <code class="hl-code-hint">copy(document.cookie)</code><br>
            <small>Then paste below (or fill in manually)</small>
          </p>
          <textarea
            v-model="rawCookieStr"
            class="hl-cookie-textarea"
            placeholder="Token=xxx; userId=xxx; userType=sso"
            rows="3"
          ></textarea>
          <div class="hl-login-actions">
            <button class="hl-login-btn" @click="applyCookie" :disabled="!rawCookieStr.trim()">
              Apply &amp; Enter
            </button>
            <button class="hl-login-btn-secondary" @click="status = 'idle'">Back</button>
          </div>
          <p v-if="bridgeError" class="hl-login-error">{{ bridgeError }}</p>
        </template>

        <!-- Error -->
        <template v-else-if="status === 'error'">
          <p class="hl-login-error">{{ errorMsg }}</p>
          <button class="hl-login-btn" @click="startSSO">Try Again</button>
          <button class="hl-login-btn-secondary" @click="status = 'dev-bridge'">
            Dev: Import Cookie
          </button>
        </template>

        <!-- Idle — initial state -->
        <template v-else>
          <p>Sign in to access GPU Performance Intelligence</p>
          <button class="hl-login-btn" @click="startSSO">Sign in with AMD SSO</button>
          <button class="hl-login-btn-secondary" @click="status = 'dev-bridge'">
            Dev: Import SaFE Cookie
          </button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { getEnvs, getUserSelf } from '@/services/auth.js';

const route = useRoute();
const router = useRouter();
const status = ref('idle');
const errorMsg = ref('');
const bridgeError = ref('');
const theme = ref(localStorage.getItem('hl-theme') || 'light');

const rawCookieStr = ref('');

const ENTRY_KEY = 'hl-sso.redirect';
const STATE_KEY = 'hl-oauth_state';

function randomState() {
  const arr = new Uint8Array(16);
  crypto.getRandomValues(arr);
  return Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('');
}

async function startSSO() {
  status.value = 'redirecting';
  errorMsg.value = '';

  try {
    const envs = await getEnvs();
    if (!envs?.ssoEnable || !envs?.ssoAuthUrl) {
      status.value = 'error';
      errorMsg.value = 'SSO is not enabled on this platform. Please contact your administrator.';
      return;
    }

    const target = route.query.redirect || '/overview';
    sessionStorage.setItem(ENTRY_KEY, target);

    const rand = randomState();
    sessionStorage.setItem(STATE_KEY, rand);

    const callbackUrl = `${window.location.origin}/hyperloom/login`;
    const stateValue = `hyperloom:${rand}:${encodeURIComponent(callbackUrl)}`;

    const u = new URL(envs.ssoAuthUrl);
    u.searchParams.set('state', stateValue);
    if (!u.searchParams.has('response_mode')) {
      u.searchParams.set('response_mode', 'query');
    }

    window.location.assign(u.toString());
  } catch (err) {
    console.error('[HyperLoom SSO] Failed to fetch envs:', err);
    status.value = 'error';
    errorMsg.value = 'Cannot reach the SaFE backend. Check your network or proxy configuration.';
  }
}

async function applyCookie() {
  bridgeError.value = '';
  const raw = rawCookieStr.value.trim();
  if (!raw) return;

  const pairs = raw.split(/;\s*/);
  for (const pair of pairs) {
    const eqIdx = pair.indexOf('=');
    if (eqIdx < 1) continue;
    const name = pair.substring(0, eqIdx).trim();
    const val = pair.substring(eqIdx + 1).trim();
    if (!name || !val) continue;
    document.cookie = `${name}=${val}; path=/; max-age=86400; SameSite=Lax`;
  }

  try {
    const profile = await getUserSelf();
    if (profile?.id) {
      localStorage.setItem(
        'hl-user',
        JSON.stringify({
          id: profile.id,
          name: profile.name,
          email: profile.email,
          session: 'authenticated',
        }),
      );
      const target = route.query.redirect || '/overview';
      router.replace(target);
      return;
    }
    bridgeError.value = 'Cookie set but session invalid. Make sure you copied all cookies (Token, userId, userType).';
  } catch (err) {
    console.error('[HyperLoom Dev Bridge]', err);
    bridgeError.value = 'Session validation failed — paste the full output of document.cookie from SaFE console.';
  }
}

onMounted(async () => {
  if (route.query.error) {
    status.value = 'error';
    const errType = route.query.error;
    if (errType === 'state_mismatch') {
      errorMsg.value = 'OAuth state mismatch. Please try again.';
    } else if (errType === 'sso_failed') {
      errorMsg.value =
        route.query.detail || 'SSO login failed. You may not have access to this platform.';
    } else {
      errorMsg.value = String(route.query.error_description || errType);
    }
    return;
  }

  if (route.query.code) {
    status.value = 'processing';
    return;
  }

  const stored = localStorage.getItem('hl-user');
  if (stored) {
    try {
      const parsed = JSON.parse(stored);
      if (parsed?.session === 'authenticated') {
        await getUserSelf();
        const target = route.query.redirect || '/overview';
        router.replace(target);
        return;
      }
    } catch {
      localStorage.removeItem('hl-user');
    }
  }
});
</script>

<style scoped>
.hl-login {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  background: var(--bg, #f3f5f9);
}

.hl-login-bg {
  position: fixed;
  inset: 0;
  z-index: 0;
  pointer-events: none;
}

.hl-login-grid {
  width: 100%;
  height: 100%;
  opacity: 0.2;
  color: var(--text-muted, #6b7280);
}

.hl-login-card {
  position: relative;
  z-index: 1;
  width: 460px;
  background: var(--white, #fff);
  border: 1px solid var(--border, #e5e7eb);
  border-radius: 16px;
  box-shadow: 0 18px 50px rgba(0,0,0,0.12);
  overflow: hidden;
}

.hl-login-header {
  padding: 24px 28px 16px;
  border-bottom: 1px solid var(--border, #e5e7eb);
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.brand-amd {
  font-weight: 800;
  font-size: 18px;
  color: var(--text-primary, #1a1a2e);
}

.brand-name {
  font-weight: 600;
  font-size: 16px;
  color: var(--text-secondary, #4b5563);
  letter-spacing: 0.5px;
}

.hl-login-body {
  padding: 32px 28px;
  text-align: center;
  min-height: 140px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  color: var(--text-secondary, #4b5563);
  font-size: 14px;
}

.hl-login-hint {
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-secondary, #4b5563);
}

.hl-login-hint small {
  color: var(--text-muted, #6b7280);
  font-size: 11px;
}

.hl-code-hint {
  display: inline-block;
  background: var(--bg, #f3f5f9);
  border: 1px solid var(--border, #e5e7eb);
  border-radius: 4px;
  padding: 2px 8px;
  font-family: monospace;
  font-size: 12px;
  color: var(--amd-red, #e4002b);
  margin: 4px 0;
  user-select: all;
}

.hl-cookie-textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border, #e5e7eb);
  border-radius: 8px;
  font-size: 12px;
  font-family: monospace;
  background: var(--bg, #f3f5f9);
  color: var(--text-primary, #1a1a2e);
  outline: none;
  resize: vertical;
  transition: border-color 0.15s;
  line-height: 1.5;
}

.hl-cookie-textarea:focus {
  border-color: var(--amd-red, #e4002b);
}

.hl-login-actions {
  display: flex;
  gap: 10px;
}

.hl-login-btn {
  padding: 10px 28px;
  background: var(--amd-red, #e4002b);
  color: #fff;
  border: none;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s;
}

.hl-login-btn:hover {
  background: var(--amd-red-light, #ff3355);
}

.hl-login-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.hl-login-btn-secondary {
  padding: 8px 20px;
  background: transparent;
  color: var(--text-muted, #6b7280);
  border: 1px solid var(--border, #e5e7eb);
  border-radius: 8px;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s;
}

.hl-login-btn-secondary:hover {
  color: var(--text-primary, #1a1a2e);
  border-color: var(--text-muted, #6b7280);
}

.hl-login-error {
  color: #dc2626;
  font-size: 13px;
  line-height: 1.5;
}

.hl-login-spinner {
  width: 28px;
  height: 28px;
  border: 3px solid var(--border, #e5e7eb);
  border-top-color: var(--amd-red, #e4002b);
  border-radius: 50%;
  animation: hl-spin 0.7s linear infinite;
}

@keyframes hl-spin {
  to { transform: rotate(360deg); }
}

[data-theme="dark"] .hl-login { background: #0c0e14; }
[data-theme="dark"] .hl-login-card { background: #141620; border-color: #252838; }
[data-theme="dark"] .hl-login-header { border-bottom-color: #252838; }
[data-theme="dark"] .brand-amd { color: #e4e6ef; }
[data-theme="dark"] .brand-name { color: #a0a4b8; }
[data-theme="dark"] .hl-login-body { color: #a0a4b8; }
[data-theme="dark"] .hl-login-hint small { color: #6b7084; }
[data-theme="dark"] .hl-code-hint {
  background: #0c0e14;
  border-color: #252838;
  color: #ff3355;
}

[data-theme="dark"] .hl-cookie-textarea {
  background: #0c0e14;
  border-color: #252838;
  color: #e4e6ef;
}
[data-theme="dark"] .hl-login-btn-secondary {
  border-color: #252838;
  color: #8b8fa8;
}
[data-theme="dark"] .hl-login-btn-secondary:hover {
  color: #e4e6ef;
  border-color: #8b8fa8;
}
[data-theme="dark"] .hl-login-spinner { border-color: #252838; border-top-color: #ff2d55; }
</style>

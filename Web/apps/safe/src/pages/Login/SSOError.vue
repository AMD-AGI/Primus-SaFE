<template>
  <div class="sso-error-container">
    <div class="error-card">
      <!-- ========== Join successful: waiting for sync page ========== -->
      <template v-if="isJoined">
        <div class="error-icon">
          <el-icon :size="80" color="#67C23A">
            <SuccessFilled />
          </el-icon>
        </div>

        <h1 class="error-title">You've Been Added!</h1>

        <div class="error-message">
          <p class="error-detail">
            <strong>{{ joinedUser }}</strong> has been added to the Primus user group.
          </p>
        </div>

        <div class="error-reasons">
          <h3>What happens next?</h3>
          <ul>
            <li>Okta needs up to <strong>2 hours</strong> to sync your group membership</li>
            <li>After the sync completes, you will be able to log in via SSO</li>
            <li>You can try logging in again after some time using the button below</li>
          </ul>
        </div>

        <div class="action-buttons">
          <el-button type="primary" size="large" @click="retrySSO" :loading="retrying">
            <el-icon class="mr-2"><RefreshRight /></el-icon>
            Try SSO Login
          </el-button>
        </div>

        <div class="help-info">
          <p>If you still can't log in after 2 hours, please contact your system administrator.</p>
        </div>
      </template>

      <!-- ========== Original error page ========== -->
      <template v-else>
        <div class="error-icon">
          <el-icon :size="80" color="#F56C6C">
            <CircleCloseFilled />
          </el-icon>
        </div>

        <h1 class="error-title">SSO Authentication Failed</h1>

        <div class="error-message">
          <p v-if="errorMessage" class="error-detail">
            {{ errorMessage }}
          </p>
          <p v-else class="error-detail">
            We were unable to authenticate you through Single Sign-On. This might be due to an
            expired session, network issues, or configuration problems.
          </p>
        </div>

        <div class="error-reasons" v-if="!isUserNotRegistered">
          <h3>Possible reasons:</h3>
          <ul>
            <li>Your SSO session has expired</li>
            <li>Invalid or missing authentication token</li>
            <li>Network connectivity issues</li>
            <li>SSO service temporarily unavailable</li>
          </ul>
        </div>

        <div class="error-reasons" v-else>
          <h3>What should I do?</h3>
          <ul>
            <li>Contact your system administrator to verify your account permissions</li>
            <li>Ensure your account has been registered and granted access</li>
            <li>Verify that your SSO account is properly linked to the system</li>
            <li v-if="errorCode === 'sso_repeated_failure'">
              If you continue experiencing issues, please provide this error code to support
            </li>
          </ul>
        </div>

        <div class="action-buttons">
          <el-button
            v-if="!isUserNotRegistered"
            type="primary"
            size="large"
            @click="retrySSO"
            :loading="retrying"
          >
            <el-icon class="mr-2"><RefreshRight /></el-icon>
            Retry SSO Login
          </el-button>
        </div>

        <div class="help-info">
          <p>If the problem persists, please contact your system administrator.</p>
          <div class="error-code" v-if="errorCode">
            Error Code: <code>{{ errorCode }}</code>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { CircleCloseFilled, SuccessFilled, RefreshRight } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'

const route = useRoute()
const userStore = useUserStore()

const retrying = ref(false)
const errorMessage = ref('')
const errorCode = ref('')
const isUserNotRegistered = ref(false)
const isJoined = ref(false)
const joinedUser = ref('')

onMounted(() => {
  // Get error info from route parameters
  if (route.query.error) {
    errorCode.value = route.query.error as string

    // Just joined user group — show friendly waiting page
    if (errorCode.value === 'joined') {
      isJoined.value = true
      joinedUser.value = decodeURIComponent((route.query.error_description as string) || '')
    }

    // Check if user is not registered or permission error (hide retry button)
    isUserNotRegistered.value =
      errorCode.value === 'user_not_registered' || errorCode.value === 'sso_repeated_failure'
  }
  if (!isJoined.value && route.query.error_description) {
    errorMessage.value = decodeURIComponent(route.query.error_description as string)
  } else if (route.query.message) {
    errorMessage.value = decodeURIComponent(route.query.message as string)
  }

  // Clean error params from URL
  const cleanUrl = window.location.pathname
  window.history.replaceState({}, document.title, cleanUrl)
})

// Retry SSO Login
const retrySSO = async () => {
  retrying.value = true

  try {
    // Get original redirect target
    const redirect = sessionStorage.getItem('sso.redirect') || '/'

    // Clear old session, attempt counter and block flag (allow SSO retry)
    sessionStorage.removeItem('oauth_state')
    sessionStorage.removeItem('sso.redirect')
    sessionStorage.removeItem('sso.blocked')
    localStorage.removeItem('sso.attempts')

    // Get SSO URL and redirect
    await userStore.fetchEnvs()
    const envs = userStore.envs
    if (envs?.ssoAuthUrl) {
      // Generate new state
      const state = Math.random().toString(36).substring(2, 15)
      sessionStorage.setItem('oauth_state', state)
      sessionStorage.setItem('sso.redirect', redirect)

      // Use configured SSO URL
      const u = new URL(envs.ssoAuthUrl)
      u.searchParams.set('state', state)
      if (!u.searchParams.has('response_mode')) {
        u.searchParams.set('response_mode', 'query')
      }

      // Redirect to SSO
      window.location.assign(u.toString())
    } else {
      throw new Error('SSO configuration not found')
    }
  } catch (_error) {
    ElMessage.error('Failed to initiate SSO login. Please try again later.')
    retrying.value = false
  }
}
</script>

<style scoped lang="scss">
.sso-error-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 20px;
}

.error-card {
  background: white;
  border-radius: 16px;
  padding: 48px;
  max-width: 600px;
  width: 100%;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
  text-align: center;
}

.error-icon {
  margin-bottom: 24px;
  animation: shake 0.5s ease-in-out;
}

.error-title {
  font-size: 28px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 24px 0;
}

.error-message {
  margin-bottom: 32px;

  .error-detail {
    font-size: 16px;
    color: #606266;
    line-height: 1.6;
    margin: 0;
  }
}

.error-reasons {
  background: #f5f7fa;
  border-radius: 8px;
  padding: 20px;
  margin-bottom: 32px;
  text-align: left;

  h3 {
    font-size: 14px;
    font-weight: 600;
    color: #606266;
    margin: 0 0 12px 0;
  }

  ul {
    margin: 0;
    padding-left: 20px;

    li {
      font-size: 14px;
      color: #909399;
      line-height: 1.8;
    }
  }
}

.action-buttons {
  display: flex;
  justify-content: center;
  margin-bottom: 32px;

  .el-button {
    min-width: 200px;
  }
}

.help-info {
  font-size: 14px;
  color: #909399;

  p {
    margin: 0 0 8px 0;
  }

  .error-code {
    margin-top: 12px;

    code {
      background: #f5f7fa;
      padding: 4px 8px;
      border-radius: 4px;
      font-family: 'Monaco', 'Courier New', monospace;
      font-size: 13px;
      color: #f56c6c;
    }
  }
}

@keyframes shake {
  0%,
  100% {
    transform: translateX(0);
  }
  10%,
  30%,
  50%,
  70%,
  90% {
    transform: translateX(-5px);
  }
  20%,
  40%,
  60%,
  80% {
    transform: translateX(5px);
  }
}

// Responsive design
@media (max-width: 640px) {
  .error-card {
    padding: 32px 24px;
  }

  .error-title {
    font-size: 24px;
  }

  .action-buttons {
    .el-button {
      width: 100%;
      max-width: 280px;
    }
  }
}
</style>

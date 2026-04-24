<template>
  <div class="settings-wrapper">
    <div class="settings-content">
      <!-- Profile -->
      <el-card class="safe-card" shadow="never">
        <template #header>
          <span class="card-title">Profile</span>
        </template>

        <div class="flex items-start gap-5">
          <div class="avatar-circle">
            <span>{{ initial }}</span>
          </div>

          <div class="profile-grid flex-1 min-w-0">
            <span class="label">Name</span>
            <span class="value">{{ displayName }}</span>

            <span class="label">User ID</span>
            <span class="value uid">{{ userStore.userId || '—' }}</span>

            <span class="label">Role</span>
            <div><el-tag size="small" type="info" effect="plain">{{ userStore.displayRole }}</el-tag></div>

            <span class="label">Created</span>
            <span class="value">{{ formatTimeStr(userStore.profile?.creationTime) }}</span>

            <span class="label">Email</span>
            <div class="value flex items-center gap-2 min-w-0">
              <template v-if="!emailEditing">
                <span class="truncate">{{ emailText || '—' }}</span>
                <el-link :icon="Edit" @click="startEditEmail" />
              </template>
              <template v-else>
                <el-input v-model="emailDraft" size="small" style="width: 240px" />
                <el-link :icon="Check" @click="submitEmail" />
                <el-link :icon="Close" @click="emailEditing = false" />
              </template>
            </div>
          </div>
        </div>
      </el-card>

      <!-- Notifications -->
      <el-card class="safe-card" shadow="never">
        <template #header>
          <span class="card-title">Notifications</span>
        </template>

        <div class="setting-row">
          <div>
            <div class="setting-label">Email Notifications</div>
            <div class="setting-desc">
              Receive email alerts when your workloads change status (Running, Succeeded, Failed,
              Stopped).
            </div>
          </div>
          <el-switch
            v-model="enableNotification"
            :loading="notifLoading"
            @change="onNotifChange"
          />
        </div>
      </el-card>

      <!-- Security -->
      <el-card class="safe-card" shadow="never">
        <template #header>
          <span class="card-title">Security</span>
        </template>

        <div class="setting-row">
          <div>
            <div class="setting-label">Password</div>
            <div class="setting-desc">Update your account password.</div>
          </div>
          <el-button size="small" plain @click="pwdVisible = true">Change Password</el-button>
        </div>

        <el-divider style="margin: 14px 0" />

        <div class="setting-row">
          <div>
            <div class="setting-label">SSH Public Keys</div>
            <div class="setting-desc">Manage SSH keys for secure access.</div>
          </div>
          <el-button size="small" plain @click="router.push('/publickeys')">Manage</el-button>
        </div>

        <el-divider style="margin: 14px 0" />

        <div class="setting-row">
          <div>
            <div class="setting-label">API Keys</div>
            <div class="setting-desc">Manage API keys for programmatic access.</div>
          </div>
          <el-button size="small" plain @click="router.push('/manageapikeys')">Manage</el-button>
        </div>
      </el-card>

      <!-- Environment (admin only) -->
      <el-card v-if="userStore.hasManagerAccess" class="safe-card" shadow="never">
        <template #header>
          <div class="flex items-center justify-between">
            <span class="card-title">Environment</span>
            <el-button size="small" text :icon="Refresh" :loading="envLoading" @click="refreshEnvs">
              Refresh
            </el-button>
          </div>
        </template>

        <div v-if="userStore.envs" class="env-grid">
          <span class="label">Log</span>
          <el-tag :type="userStore.envs.enableLog ? 'success' : 'info'" size="small" effect="plain">
            {{ userStore.envs.enableLog ? 'Enabled' : 'Disabled' }}
          </el-tag>

          <span class="label">Log Download</span>
          <el-tag :type="userStore.envs.enableLogDownload ? 'success' : 'info'" size="small" effect="plain">
            {{ userStore.envs.enableLogDownload ? 'Enabled' : 'Disabled' }}
          </el-tag>

          <span class="label">SSH</span>
          <div class="flex items-center gap-2">
            <el-tag :type="userStore.envs.enableSsh ? 'success' : 'info'" size="small" effect="plain">
              {{ userStore.envs.enableSsh ? 'Enabled' : 'Disabled' }}
            </el-tag>
            <span v-if="userStore.envs.enableSsh && userStore.envs.sshIP" class="value mono">
              {{ userStore.envs.sshIP }}{{ userStore.envs.sshPort ? `:${userStore.envs.sshPort}` : '' }}
            </span>
          </div>

          <span class="label">SSO</span>
          <el-tag :type="userStore.envs.ssoEnable ? 'success' : 'info'" size="small" effect="plain">
            {{ userStore.envs.ssoEnable ? 'Enabled' : 'Disabled' }}
          </el-tag>

          <span class="label">CD Approval</span>
          <el-tag :type="userStore.envs.cdRequireApproval ? 'warning' : 'info'" size="small" effect="plain">
            {{ userStore.envs.cdRequireApproval ? 'Required' : 'Not Required' }}
          </el-tag>

          <template v-if="userStore.envs.authoringImage">
            <span class="label">Authoring Image</span>
            <span class="value mono truncate">{{ userStore.envs.authoringImage }}</span>
          </template>
        </div>
        <el-empty v-else description="Environment data not available" :image-size="48" />
      </el-card>

      <!-- Danger zone -->
      <el-card class="safe-card" shadow="never">
        <template #header>
          <span class="card-title">Account</span>
        </template>
        <div class="setting-row">
          <div>
            <div class="setting-label">Sign Out</div>
            <div class="setting-desc">Sign out of your current session.</div>
          </div>
          <el-button size="small" type="danger" plain @click="onLogout">Logout</el-button>
        </div>
      </el-card>
    </div>

    <!-- Change Password Dialog -->
    <el-dialog
      v-model="pwdVisible"
      title="Change Password"
      width="480px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-form label-width="auto" class="p-4">
        <el-form-item label="New Password">
          <el-input v-model="newPassword" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pwdVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="pwdLoading" @click="handleChangePwd">
          Confirm
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { editUser, getUserSettings, updateUserSettings } from '@/services'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Check, Close, Refresh } from '@element-plus/icons-vue'
import { formatTimeStr } from '@/utils'

defineOptions({ name: 'UserSettings' })

const router = useRouter()
const userStore = useUserStore()

const displayName = computed(
  () =>
    (userStore.profile as any)?.name ||
    (userStore.profile as any)?.username ||
    userStore.userId ||
    'Guest',
)
const emailText = computed(() => (userStore.profile as any)?.email || '')
const initial = computed(() => displayName.value?.charAt(0)?.toUpperCase() || 'U')

// ── Email editing ──
const emailEditing = ref(false)
const emailDraft = ref('')

function startEditEmail() {
  emailDraft.value = emailText.value
  emailEditing.value = true
}

async function submitEmail() {
  await editUser(userStore.userId, { email: emailDraft.value })
  ElMessage.success('Email updated')
  emailEditing.value = false
  await userStore.fetchUser(true)
}

// ── Notification settings ──
const enableNotification = ref(false)
const notifLoading = ref(false)

async function fetchSettings() {
  try {
    const res = await getUserSettings()
    enableNotification.value = res.enableNotification ?? false
  } catch {
    // API may not be deployed yet; silently default to off
  }
}

async function onNotifChange(val: boolean | string | number) {
  notifLoading.value = true
  try {
    await updateUserSettings({ enableNotification: !!val })
    ElMessage.success(val ? 'Notifications enabled' : 'Notifications disabled')
  } catch {
    enableNotification.value = !val
  } finally {
    notifLoading.value = false
  }
}

// ── Password ──
const pwdVisible = ref(false)
const pwdLoading = ref(false)
const newPassword = ref('')

async function handleChangePwd() {
  try {
    pwdLoading.value = true
    const name = (userStore.profile as any)?.name ?? ''
    await editUser(userStore.userId, { password: newPassword.value })
    ElMessage.success('Password updated. Please sign in again.')
    await userStore.logout()
    await nextTick()
    router.push({ path: '/login', query: { name } })
  } finally {
    pwdVisible.value = false
    pwdLoading.value = false
    newPassword.value = ''
  }
}

// ── Logout ──
async function onLogout() {
  ElMessageBox.confirm('Confirm sign out? You will be redirected to the sign-in page.', 'Warning', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await userStore.logout()
    router.replace('/login')
    ElMessage({ type: 'success', message: 'Logout completed' })
  })
}

// ── Environment ──
const envLoading = ref(false)
async function refreshEnvs() {
  envLoading.value = true
  try {
    await userStore.fetchEnvs()
    ElMessage.success('Environment refreshed')
  } catch {
    ElMessage.error('Failed to fetch environment')
  } finally {
    envLoading.value = false
  }
}

onMounted(() => {
  fetchSettings()
  if (!userStore.envs) userStore.fetchEnvs().catch(() => {})
})
</script>

<style scoped>
.settings-wrapper {
  display: flex;
  justify-content: center;
  padding: 16px 24px;
}

.settings-content {
  width: 100%;
  max-width: 780px;
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.card-title {
  font-weight: 600;
  font-size: 15px;
}

/* Avatar */
.avatar-circle {
  flex: 0 0 auto;
  width: 56px;
  height: 56px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background: var(--el-fill-color-dark);
  color: var(--el-text-color-primary);
  font-weight: 700;
  font-size: 20px;
}

/* Profile grid */
.profile-grid {
  display: grid;
  grid-template-columns: 80px 1fr;
  gap: 12px 18px;
  font-size: 14px;
  align-items: center;
}
.profile-grid .label {
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
.profile-grid .value {
  color: var(--el-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.profile-grid .value.uid {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
}

/* Setting row */
.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}
.setting-label {
  font-size: 14px;
  font-weight: 500;
  color: var(--el-text-color-primary);
}
.setting-desc {
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-top: 3px;
}

/* Environment grid */
.env-grid {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: 12px 18px;
  font-size: 14px;
  align-items: center;
}
.env-grid .label {
  color: var(--el-text-color-secondary);
  white-space: nowrap;
}
.env-grid > :deep(.el-tag) {
  justify-self: start;
}
.env-grid .value {
  color: var(--el-text-color-primary);
}
.env-grid .mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
}
</style>

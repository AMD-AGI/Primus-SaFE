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
import { Edit, Check, Close } from '@element-plus/icons-vue'
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

onMounted(fetchSettings)
</script>

<style scoped>
.settings-wrapper {
  display: flex;
  justify-content: center;
  padding-top: 12px;
}

.settings-content {
  width: 100%;
  max-width: 560px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.card-title {
  font-weight: 600;
  font-size: 14px;
}

/* Avatar */
.avatar-circle {
  flex: 0 0 auto;
  width: 48px;
  height: 48px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background: var(--el-fill-color-dark);
  color: var(--el-text-color-primary);
  font-weight: 700;
  font-size: 18px;
}

/* Profile grid */
.profile-grid {
  display: grid;
  grid-template-columns: 72px 1fr;
  gap: 8px 14px;
  font-size: 13px;
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
  font-size: 12px;
}

/* Setting row */
.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}
.setting-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--el-text-color-primary);
}
.setting-desc {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 2px;
}
</style>

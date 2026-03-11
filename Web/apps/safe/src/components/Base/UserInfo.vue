<template>
  <el-popover
    placement="top-start"
    trigger="hover"
    :width="360"
    popper-class="user-popover"
    :offset="8"
  >
    <template #reference>
      <el-button class="user-card-btn w-full !h-auto !p-3 !justify-between" text>
        <div class="flex items-center gap-3 overflow-hidden">
          <!-- Avatar (first letter) -->
          <div class="avatar">
            <span>{{ initial }}</span>
          </div>

          <!-- Text -->
          <div class="flex-1 min-w-0 text-left">
            <div class="name ellipsis" :title="displayName">{{ displayName }}</div>
            <div class="subtle ellipsis" :title="roleDisplayText">
              {{ roleDisplayText }}
            </div>
          </div>
        </div>
      </el-button>
    </template>

    <!-- Popover content: block layout -->
    <div class="space-y-3">
      <!-- Top avatar + name -->
      <div class="flex justify-between items-center">
        <div class="flex items-center gap-3">
          <div class="avatar avatar-lg">
            <span>{{ initial }}</span>
          </div>
          <div>
            <div class="name">{{ displayName }}</div>
            <div class="subtle">{{ emailText || '—' }}</div>
          </div>
        </div>

        <div v-if="store.userId">
          <el-tooltip content="change your password">
            <el-button
              size="small"
              type="warning"
              :icon="Key"
              plain
              @click="editPwdVisible = true"
            />
          </el-tooltip>
          <el-button size="small" type="danger" plain @click="onLogout">Logout</el-button>
        </div>
        <el-button size="small" type="primary" plain @click="onLogin" v-else>Login</el-button>
      </div>

      <el-divider style="margin: 6px 0" v-if="store.userId" />

      <!-- Details -->
      <div class="grid grid-cols-[96px,1fr] gap-y-2 text-sm" v-if="store.userId">
        <div class="key">
          User ID
          <div class="val font-mono break-all">{{ store.userId || '—' }}</div>
        </div>

        <div class="key">
          Email
          <template v-if="!emailEdit">
            <div class="val truncate" :title="emailText">{{ emailText || '—' }}</div>
            <el-link class="ml-2" :icon="Edit" @click="emailEdit = !emailEdit" />
          </template>
          <template v-else>
            <el-input v-model="form.email" size="small" class="ml-2" />
            <el-link class="ml-2" :icon="Check" @click="submitEditEmail" />
          </template>
        </div>

        <div class="key">
          Creation
          <div class="val">{{ formatTimeStr(store.profile?.creationTime) }}</div>
        </div>

        <div class="key">
          Role
          <div class="val">{{ store.displayRole }}</div>
        </div>

        <div class="key">
          My Public Keys
          <el-button size="small" plain type="primary" class="ml-3" @click="onManageKey"
            >Manage</el-button
          >
        </div>
      </div>
    </div>
  </el-popover>

  <el-dialog
    v-model="editPwdVisible"
    title="Change password"
    width="520px"
    :close-on-click-modal="false"
  >
    <el-form :model="form" label-width="auto" style="max-width: 600px" class="p-5">
      <el-form-item label="New Password">
        <el-input v-model="form.password" />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="editPwdVisible = false">Cancel</el-button>
      <el-button type="primary" :loading="editLoading" @click="handleChangePwd">Confirm</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, reactive, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { ElMessageBox, ElMessage } from 'element-plus'
import { editUser } from '@/services'
import { Edit, Check, Key } from '@element-plus/icons-vue'
import { formatTimeStr } from '@/utils'

const store = useUserStore()
const router = useRouter()

const editPwdVisible = ref(false)
const editLoading = ref(false)
const emailEdit = ref(false)

const displayName = computed(
  () => (store.profile as any)?.name || (store.profile as any)?.username || store.userId || 'Guest',
)
const emailText = computed(() => (store.profile as any)?.email || '')

const initial = computed(() => displayName.value?.charAt(0)?.toUpperCase() || 'U')

// Simplify role display
const roleDisplayText = computed(() => {
  const role = store.displayRole
  // Simplify long role names
  const roleMap: Record<string, string> = {
    'system-admin': 'sys-admin',
    'system-admin-readonly': 'sys-admin (ro)',
    'workspace-admin': 'ws-admin',
  }
  return roleMap[role] || role
})

const initialForm = () => ({
  password: '',
  email: store.profile?.email as string,
})
const form = reactive({ ...initialForm() })

async function onLogout() {
  ElMessageBox.confirm('Confirm sign out? You’ll be redirected to the sign-in page.', 'Warning', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await store.logout()
    router.replace('/login')
    ElMessage({
      type: 'success',
      message: 'Logout completed',
    })
  })
}

const submitEditEmail = async () => {
  await editUser(store.userId, { email: form.email })
  ElMessage({ message: 'Edit successful', type: 'success' })
  emailEdit.value = false
  await store.fetchUser(true)
  Object.assign(form, initialForm())
}

const handleChangePwd = async () => {
  try {
    editLoading.value = true

    // Cache current name (prevent being cleared by logout)
    const displayName = store.profile?.name ?? ''

    await editUser(store.userId, { password: form.password })

    ElMessage({
      message: 'Password updated. Please sign in again.',
      type: 'success',
      duration: 2000,
    })

    await store.logout()
    await nextTick()
    router.push({ path: '/login', query: { name: displayName } })
  } finally {
    editPwdVisible.value = false
    editLoading.value = false
  }
}

const onLogin = () => {
  router.push('/login')
}

const onManageKey = () => {
  router.push('/publickeys')
}
</script>

<style scoped>
.user-card-btn {
  --card-bg: var(--el-bg-color);
  --card-border: var(--el-border-color);
  --card-hover: var(--el-fill-color-light);
  width: 100%;
  border-radius: 14px;
  border: 1px solid var(--card-border);
  background: var(--card-bg);
  justify-content: space-between;
  overflow: hidden;
}
.user-card-btn:hover {
  background: var(--card-hover);
}
.ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Avatar */
.avatar {
  flex: 0 0 auto;
  width: 32px;
  height: 32px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background: var(--el-fill-color-dark);
  color: var(--el-text-color-primary);
  font-weight: 700;
  font-size: 12px;
}
.avatar.avatar-lg {
  width: 40px;
  height: 40px;
  font-size: 14px;
}

/* Text styles */
.name {
  font-weight: 700;
  color: var(--el-text-color-primary);
  line-height: 15px;
}
.subtle {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* More compact popover content */
:global(.user-popover) {
  --el-popover-padding: 12px;
}
.key {
  color: var(--el-text-color-secondary);
  display: flex;
}
.val {
  color: var(--el-text-color-primary);
  margin-left: 10px;
}
</style>

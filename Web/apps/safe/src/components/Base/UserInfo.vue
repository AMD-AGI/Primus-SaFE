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
          <div class="avatar">
            <span>{{ initial }}</span>
          </div>
          <div class="flex-1 min-w-0 text-left">
            <div class="name ellipsis" :title="displayName">{{ displayName }}</div>
            <div class="subtle ellipsis" :title="roleDisplayText">
              {{ roleDisplayText }}
            </div>
          </div>
        </div>
      </el-button>
    </template>

    <!-- Popover content -->
    <div class="space-y-3">
      <!-- Header: avatar + name + logout -->
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
          <el-button size="small" type="danger" plain @click="onLogout">Logout</el-button>
        </div>
        <el-button size="small" type="primary" plain @click="router.push('/login')" v-else>
          Login
        </el-button>
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
          <div class="val truncate" :title="emailText">{{ emailText || '—' }}</div>
        </div>

        <div class="key">
          Creation
          <div class="val">{{ formatTimeStr(store.profile?.creationTime) }}</div>
        </div>

        <div class="key">
          Role
          <div class="val">{{ store.displayRole }}</div>
        </div>
      </div>

      <!-- Settings link -->
      <div v-if="store.userId" class="pt-1">
        <el-divider style="margin: 6px 0" />
        <el-link type="primary" :underline="false" class="mt-1" @click="router.push('/settings')">
          <el-icon class="mr-1"><Setting /></el-icon>Settings
        </el-link>
      </div>
    </div>
  </el-popover>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import { ElMessageBox, ElMessage } from 'element-plus'
import { Setting } from '@element-plus/icons-vue'
import { formatTimeStr } from '@/utils'

const store = useUserStore()
const router = useRouter()

const displayName = computed(
  () => (store.profile as any)?.name || (store.profile as any)?.username || store.userId || 'Guest',
)
const emailText = computed(() => (store.profile as any)?.email || '')
const initial = computed(() => displayName.value?.charAt(0)?.toUpperCase() || 'U')

const roleDisplayText = computed(() => {
  const role = store.displayRole
  const roleMap: Record<string, string> = {
    'system-admin': 'sys-admin',
    'system-admin-readonly': 'sys-admin (ro)',
    'workspace-admin': 'ws-admin',
  }
  return roleMap[role] || role
})

async function onLogout() {
  ElMessageBox.confirm('Confirm sign out? You will be redirected to the sign-in page.', 'Warning', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await store.logout()
    router.replace('/login')
    ElMessage({ type: 'success', message: 'Logout completed' })
  })
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

.name {
  font-weight: 700;
  color: var(--el-text-color-primary);
  line-height: 15px;
}
.subtle {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

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

<template>
  <div class="h-screen w-screen overflow-hidden flex flex-col">
    <div class="flex flex-1 overflow-hidden">
      <BaseMenu class="w-55 shrink-0" />

      <main class="flex-1 overflow-y-auto p-6 main-content-with-glow">
        <router-view />
      </main>
    </div>

    <!-- Floating ChatBot -->
    <FloatingChatBot />

    <!-- Migration Notice Dialog -->
    <Transition name="dialog-fade">
      <div v-if="showNotice" class="notice-overlay" @click.self="dismissNotice">
        <div class="notice-dialog">
          <h3 class="dialog-title">Resource Migration Notice</h3>
          <p class="dialog-desc">
            All resources have been migrated to <strong>Core42</strong>.
            OCI machines are no longer available.
          </p>
          <p class="dialog-detail">
            Core42 is equipped with <strong>AMD Instinct MI300</strong> GPUs.
            Please use Core42 for all new workloads.
          </p>
          <div class="dialog-actions">
            <a
              href="https://core42.primus-safe.amd.com/"
              target="_blank"
              rel="noopener"
              class="action-primary"
            >
              Go to Core42 →
            </a>
            <button class="action-dismiss" @click="dismissNotice">
              Dismiss
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import BaseMenu from '@/components/layout/BaseMenu.vue'
import FloatingChatBot from '@/components/Base/FloatingChatBot.vue'

const STORAGE_KEY = 'core42-migration-notice-dismissed'
const route = useRoute()

const isOciDomain = !window.location.hostname.includes('core42')
const dismissed = ref(localStorage.getItem(STORAGE_KEY) === '1')

const isQuickStartPage = computed(() =>
  ['/quickstart', '/userquickstart'].includes(route.path),
)

const showNotice = computed(() => isOciDomain && !dismissed.value && !isQuickStartPage.value)

function dismissNotice() {
  dismissed.value = true
  localStorage.setItem(STORAGE_KEY, '1')
}

function reopenNotice() {
  localStorage.removeItem(STORAGE_KEY)
  dismissed.value = false
}

if (import.meta.env.DEV) {
  ;(window as any).__reopenMigrationNotice = reopenNotice
}
</script>

<style scoped lang="scss">
.main-content-with-glow {
  position: relative;
  background:
    radial-gradient(circle at 20% 30%, rgba(139, 92, 246, 0.08) 0%, transparent 50%),
    radial-gradient(circle at 80% 70%, rgba(59, 130, 246, 0.06) 0%, transparent 50%),
    radial-gradient(circle at 40% 80%, rgba(6, 182, 212, 0.05) 0%, transparent 50%),
    linear-gradient(135deg, #f6f8fb 0%, #eef2f7 50%, #e8ecf1 100%);
  background-attachment: fixed;
}

.notice-overlay {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgb(0 0 0 / 0.45);
  backdrop-filter: blur(4px);
}

.notice-dialog {
  width: 400px;
  padding: 32px;
  border-radius: 16px;
  text-align: center;
  background:
    radial-gradient(ellipse at 30% 0%, rgb(255 255 255 / 6%) 0%, transparent 60%),
    color-mix(in oklab, var(--el-bg-color) 76%, transparent 24%);
  border: 1px solid color-mix(in oklab, var(--el-border-color) 60%, transparent 40%);
  backdrop-filter: blur(20px) saturate(160%);
  -webkit-backdrop-filter: blur(20px) saturate(160%);
  box-shadow:
    0 0 0 1px rgb(255 255 255 / 5%) inset,
    0 24px 48px -12px rgb(0 0 0 / 0.3);
}

.dialog-title {
  margin: 0 0 14px;
  font-size: 17px;
  font-weight: 600;
  letter-spacing: 0.2px;
  color: var(--el-text-color-primary);
}

.dialog-desc {
  margin: 0 0 6px;
  font-size: 14px;
  line-height: 1.6;
  color: var(--el-text-color-regular);
}

.dialog-detail {
  margin: 0 0 28px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}

.dialog-actions {
  display: flex;
  gap: 12px;
}

.action-primary {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 10px 20px;
  border-radius: 10px;
  background: var(--el-text-color-primary);
  color: var(--el-bg-color);
  font-size: 13px;
  font-weight: 500;
  text-decoration: none;
  transition: opacity 0.15s;

  &:hover {
    opacity: 0.85;
  }
}

.action-dismiss {
  flex: 1;
  padding: 10px 20px;
  border: 1px solid color-mix(in oklab, var(--el-border-color) 80%, transparent 20%);
  border-radius: 10px;
  background: color-mix(in oklab, var(--el-fill-color) 50%, transparent 50%);
  color: var(--el-text-color-primary);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: background 0.15s;

  &:hover {
    background: var(--el-fill-color);
  }
}

.dialog-fade-enter-active,
.dialog-fade-leave-active {
  transition: opacity 0.2s ease;

  .notice-dialog {
    transition: transform 0.2s ease;
  }
}

.dialog-fade-enter-from,
.dialog-fade-leave-to {
  opacity: 0;

  .notice-dialog {
    transform: scale(0.95);
  }
}
</style>

<style lang="scss">
// Dark mode (unscoped)
html.dark .main-content-with-glow {
  background:
    radial-gradient(circle at 20% 30%, rgba(139, 92, 246, 0.12) 0%, transparent 50%),
    radial-gradient(circle at 80% 70%, rgba(59, 130, 246, 0.1) 0%, transparent 50%),
    radial-gradient(circle at 40% 80%, rgba(6, 182, 212, 0.08) 0%, transparent 50%),
    linear-gradient(135deg, #0a0e14 0%, #0f1419 50%, #141921 100%);
  background-attachment: fixed;
}
</style>

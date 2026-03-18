<template>
  <el-tag v-if="available" type="success">{{ displayText }}</el-tag>

  <el-tooltip
    v-else
    placement="top"
    :enterable="true"
    :hide-after="0"
    popper-class="status-msg-tooltip"
    effect="light"
  >
    <template #content>
      <div class="status-msg-tooltip__content">
        <div class="status-msg-tooltip__text">{{ messageText }}</div>
        <div v-if="enableSendToChat" class="status-msg-tooltip__actions">
          <el-button
            v-if="messageTrimmed"
            size="small"
            type="primary"
            plain
            @click.stop="sendToChat"
          >
            {{ sendButtonText }}
          </el-button>
        </div>
      </div>
    </template>

    <el-tag type="danger" class="cursor-pointer">{{ displayText }}</el-tag>
  </el-tooltip>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { useChatbotUIStore } from '@/stores/chatbotUI'

type Props = {
  available: boolean
  display?: unknown
  message?: unknown
  enableSendToChat?: boolean
  sendButtonText?: string
  toastText?: string
}

const props = withDefaults(defineProps<Props>(), {
  enableSendToChat: true,
  sendButtonText: 'Send to chat',
  toastText: 'Filled into chat input',
})

const chatbotUIStore = useChatbotUIStore()

const displayText = computed(() => {
  const v = props.display ?? props.available
  if (v === null || v === undefined || v === '') return '-'
  return String(v)
})

const messageText = computed(() => {
  const v = props.message
  if (v === null || v === undefined || v === '') return '-'
  return String(v)
})

const messageTrimmed = computed(() => messageText.value.trim())

const sendToChat = () => {
  if (!messageTrimmed.value) return
  chatbotUIStore.openAndPrefill(messageTrimmed.value)
  ElMessage.success(props.toastText)
}
</script>

<style lang="scss">
.status-msg-tooltip {
  max-width: 520px;
  padding: 10px 10px 8px;
  border-radius: 10px;
  border: 1px solid var(--el-border-color-lighter) !important;
  background: var(--el-bg-color-overlay) !important;
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.12);
  color: var(--el-text-color-primary) !important;
}

.status-msg-tooltip .el-popper__arrow::before {
  background: var(--el-bg-color-overlay) !important;
  border: 1px solid var(--el-border-color-lighter) !important;
}

.status-msg-tooltip__content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.status-msg-tooltip__text {
  padding: 8px 10px;
  border-radius: 8px;
  border: 1px solid var(--el-border-color-lighter);
  background: var(--el-fill-color-light);
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
  font-size: 12px;
  max-height: 180px;
  overflow: auto;
  color: var(--el-text-color-primary);
}

.status-msg-tooltip__actions {
  display: flex;
  justify-content: flex-end;
}

html.dark .status-msg-tooltip {
  border-color: rgba(148, 163, 184, 0.16) !important;
  background: rgba(15, 23, 42, 0.92) !important;
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.55);
}

html.dark .status-msg-tooltip .el-popper__arrow::before {
  background: rgba(15, 23, 42, 0.92) !important;
  border-color: rgba(148, 163, 184, 0.16) !important;
}

html.dark .status-msg-tooltip__text {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(30, 41, 59, 0.55);
  color: rgba(226, 232, 240, 0.92);
}
</style>


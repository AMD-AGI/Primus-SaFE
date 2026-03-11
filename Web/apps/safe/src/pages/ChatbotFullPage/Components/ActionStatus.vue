<template>
  <div class="action-status">
    <div
      v-for="(action, index) in actions"
      :key="index"
      class="action-item"
      :class="action.status"
    >
      <div class="action-indicator">
        <el-icon v-if="action.status === 'success'" class="action-icon success">
          <CircleCheck />
        </el-icon>
        <el-icon v-else-if="action.status === 'failed'" class="action-icon failed">
          <CircleClose />
        </el-icon>
        <el-icon v-else class="action-icon running is-loading">
          <Loading />
        </el-icon>
      </div>

      <div class="action-content">
        <div class="action-header">
          <span class="action-type">{{ formatActionType(action.action_type) }}</span>
          <span class="action-name">{{ action.action_name }}</span>
        </div>

        <div v-if="action.details" class="action-details">
          <div v-for="(value, key) in action.details" :key="key" class="detail-item">
            <span class="detail-key">{{ key }}:</span>
            <span class="detail-value">{{ value }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { CircleCheck, CircleClose, Loading } from '@element-plus/icons-vue'
import type { ActionMessageData } from '@/services/agent'

interface Props {
  actions: ActionMessageData[]
}

const props = defineProps<Props>()

const formatActionType = (type: string) => {
  const typeMap: Record<string, string> = {
    api_call: 'API Call',
    llm_call: 'LLM',
    processing: 'Processing',
  }
  return typeMap[type] || type
}
</script>

<style scoped lang="scss">
.action-status {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 12px 0;
}

.action-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 16px;
  background: linear-gradient(135deg, #fafbfc 0%, #fff 100%);
  border-radius: 12px;
  border: 1.5px solid #e2e8f0;
  border-left: 3px solid #3b82f6;
  padding-left: 13px;
  transition: all 0.25s ease;
  box-shadow:
    0 2px 8px rgba(0, 0, 0, 0.04),
    0 4px 16px rgba(0, 0, 0, 0.02);
  position: relative;

  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 50%;
    border-radius: 12px 12px 0 0;
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.2) 0%, transparent 100%);
    pointer-events: none;
  }

  &.success {
    border-left-color: #10b981;
    background: linear-gradient(135deg, #f0fdf4 0%, #fff 100%);
  }

  &.failed {
    border-left-color: #ef4444;
    background: linear-gradient(135deg, #fef2f2 0%, #fff 100%);
  }

  .action-indicator {
    flex-shrink: 0;
    width: 18px;
    height: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-top: 2px;
    position: relative;
    z-index: 1;
  }

  .action-icon {
    font-size: 18px;

    &.success {
      color: #10b981;
    }

    &.failed {
      color: #ef4444;
    }

    &.running {
      color: #3b82f6;
    }
  }

  .action-content {
    flex: 1;
    min-width: 0;
    position: relative;
    z-index: 1;
  }

  .action-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
    font-size: 14px;

    .action-type {
      color: #3b82f6;
      font-weight: 600;
      white-space: nowrap;
    }

    .action-name {
      color: #1e293b;
      font-weight: 500;
    }
  }

  .action-details {
    font-size: 13px;
    color: #64748b;
    margin-top: 8px;
    padding-left: 4px;

    .detail-item {
      margin-bottom: 4px;
      line-height: 1.6;

      .detail-key {
        color: #94a3b8;
        margin-right: 6px;
      }

      .detail-value {
        color: #475569;
        word-break: break-all;
      }
    }
  }

  &.success .action-header .action-type {
    color: #10b981;
  }

  &.failed .action-header .action-type {
    color: #ef4444;
  }
}

// Dark mode
.dark {
  .action-item {
    background: rgba(30, 41, 59, 0.6);
    border-color: #334155;
    border-left-color: #60a5fa;
    backdrop-filter: blur(10px);
    box-shadow:
      0 2px 8px rgba(0, 0, 0, 0.2),
      0 4px 16px rgba(0, 0, 0, 0.15);

    &::before {
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.03) 0%, transparent 100%);
    }

    &.success {
      border-left-color: #10b981;
      background: rgba(16, 185, 129, 0.08);
    }

    &.failed {
      border-left-color: #ef4444;
      background: rgba(239, 68, 68, 0.08);
    }

    .action-header {
      .action-type {
        color: #60a5fa;
      }

      .action-name {
        color: #e2e8f0;
      }
    }

    .action-details {
      color: #94a3b8;

      .detail-key {
        color: #64748b;
      }

      .detail-value {
        color: #cbd5e1;
      }
    }

    &.success .action-header .action-type {
      color: #34d399;
    }

    &.failed .action-header .action-type {
      color: #f87171;
    }
  }
}
</style>

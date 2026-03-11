<template>
  <div class="workflow-progress">
    <div class="workflow-header">
      <el-icon class="workflow-icon"><Histogram /></el-icon>
      <span class="workflow-name">{{ workflowName }}</span>
    </div>
    <div class="workflow-steps">
      <div v-for="(step, index) in steps" :key="index" class="workflow-step" :class="step.status">
        <div class="step-indicator">
          <el-icon v-if="step.status === 'success'" class="step-icon success">
            <CircleCheck />
          </el-icon>
          <el-icon v-else-if="step.status === 'failed'" class="step-icon failed">
            <CircleClose />
          </el-icon>
          <el-icon v-else-if="step.status === 'running'" class="step-icon running is-loading">
            <Loading />
          </el-icon>
          <div v-else class="step-icon pending"></div>
        </div>
        <div class="step-content">
          <span class="step-name">{{ step.name }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Histogram, CircleCheck, CircleClose, Loading } from '@element-plus/icons-vue'
import type { WorkflowStep } from '@/services/agent'

interface Props {
  workflowName: string
  steps: WorkflowStep[]
  currentStep?: number
}

defineProps<Props>()
</script>

<style scoped lang="scss">
.workflow-progress {
  background: linear-gradient(135deg, #fafbfc 0%, #fff 100%);
  border: 1.5px solid #e2e8f0;
  border-radius: 16px;
  padding: 18px;
  margin: 12px 0;
  box-shadow:
    0 4px 16px rgba(0, 0, 0, 0.08),
    0 8px 32px rgba(0, 0, 0, 0.04);
  position: relative;

  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 50%;
    border-radius: 16px 16px 0 0;
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.2) 0%, transparent 100%);
    pointer-events: none;
  }
}

.workflow-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
  font-weight: 600;
  color: #1e293b;
  position: relative;
  z-index: 1;

  .workflow-icon {
    color: #3b82f6;
    font-size: 18px;
  }

  .workflow-name {
    font-size: 15px;
  }
}

.workflow-steps {
  display: flex;
  flex-direction: column;
  gap: 10px;
  position: relative;
  z-index: 1;
}

.workflow-step {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  transition: all 0.25s ease;

  &.running {
    background: rgba(59, 130, 246, 0.05);
    border-left: 3px solid #3b82f6;
    padding-left: 9px;
  }

  .step-indicator {
    flex-shrink: 0;
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .step-icon {
    font-size: 20px;

    &.success {
      color: #10b981;
    }

    &.failed {
      color: #ef4444;
    }

    &.running {
      color: #3b82f6;
    }

    &.pending {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #d0d0d0;
    }
  }

  .step-content {
    flex: 1;
    font-size: 14px;
    color: #64748b;

    .step-name {
      line-height: 1.6;
    }
  }

  &.success .step-content {
    color: #475569;
  }

  &.running .step-content {
    color: #3b82f6;
    font-weight: 500;
  }

  &.failed .step-content {
    color: #ef4444;
  }
}

// Dark mode
.dark {
  .workflow-progress {
    background: rgba(30, 41, 59, 0.6);
    border-color: #334155;
    backdrop-filter: blur(10px);
    box-shadow:
      0 4px 16px rgba(0, 0, 0, 0.3),
      0 8px 32px rgba(0, 0, 0, 0.2);

    &::before {
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.03) 0%, transparent 100%);
    }
  }

  .workflow-header {
    color: #e2e8f0;

    .workflow-icon {
      color: #60a5fa;
    }
  }

  .workflow-step {
    &.running {
      background: rgba(96, 165, 250, 0.12);
      border-left-color: #60a5fa;
    }

    .step-icon.pending {
      background: #475569;
    }
  }

  .step-content {
    color: #94a3b8;
  }

  .workflow-step {
    &.success .step-content {
      color: #cbd5e1;
    }

    &.running .step-content {
      color: #60a5fa;
    }

    &.failed .step-content {
      color: #f87171;
    }
  }
}
</style>

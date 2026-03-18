<template>
  <div class="analysis-task-item" :class="[`status-${task.status}`, { expanded }]">
    <!-- Header -->
    <div class="task-header" @click="toggleExpand">
      <div class="task-icon">{{ taskIcon }}</div>
      <div class="task-info">
        <div class="task-title">
          <span class="task-type">{{ task.typeDisplay }}</span>
          <el-tag :type="statusTagType" effect="plain" size="small" class="status-tag">
            <el-icon v-if="task.status === 'running'" class="rotating"><Loading /></el-icon>
            {{ task.statusDisplay }}
          </el-tag>
        </div>
        <div class="task-meta">
          <span v-if="task.durationMs" class="duration">{{ formattedDuration }}</span>
          <span v-else-if="task.status === 'running'" class="duration running">
            <el-icon class="rotating"><Loading /></el-icon>
            Running...
          </span>
          <span v-else-if="task.status === 'pending'" class="duration pending">Waiting</span>
        </div>
      </div>
      <div class="task-result-preview" v-if="task.result && task.status === 'completed'">
        <el-tag :type="riskTagType" effect="light" size="small">
          {{ riskIcon }} {{ riskLabel }}
        </el-tag>
      </div>
      <el-icon class="expand-icon" :class="{ rotated: expanded }"><ArrowRight /></el-icon>
    </div>

    <!-- Expanded Content -->
    <el-collapse-transition>
      <div v-show="expanded" class="task-content">
        <!-- Running Progress -->
        <div v-if="task.status === 'running'" class="progress-section">
          <el-progress :percentage="50" :show-text="false" :stroke-width="4" status="warning" />
          <p class="progress-text">Analyzing...</p>
        </div>

        <!-- Result Section -->
        <div v-else-if="task.result" class="result-section">
          <!-- Summary -->
          <div class="result-summary">
            <el-tag :type="riskTagType" effect="light" class="risk-badge">
              {{ riskIcon }} {{ riskLabel }}
              <span v-if="task.result.findingsCount" class="findings-count">
                - {{ task.result.findingsCount }} findings
              </span>
            </el-tag>
            <p class="summary-text">{{ task.result.summary }}</p>
          </div>

          <!-- Findings -->
          <div v-if="task.result.details?.length" class="findings-list">
            <div v-for="(finding, idx) in task.result.details.slice(0, 5)" :key="idx" class="finding-item">
              <el-icon :class="`risk-${finding.risk}`">
                <WarningFilled v-if="finding.risk === 'high'" />
                <Warning v-else-if="finding.risk === 'medium'" />
                <CircleCheck v-else />
              </el-icon>
              <div class="finding-content">
                <span class="finding-file">{{ finding.file }}</span>
                <span class="finding-reason">{{ finding.reason }}</span>
              </div>
            </div>
            <div v-if="task.result.details.length > 5" class="more-findings">
              + {{ task.result.details.length - 5 }} more findings
            </div>
          </div>

          <!-- Actions -->
          <div class="result-actions">
            <el-button 
              v-if="task.result.reportUrl" 
              type="primary" 
              link 
              size="small"
              @click.stop="$emit('view-report', task)"
            >
              <el-icon><Document /></el-icon>
              View Full Report
            </el-button>
          </div>
        </div>

        <!-- Error Section -->
        <div v-else-if="task.error" class="error-section">
          <div class="error-content">
            <el-icon class="error-icon"><CircleCloseFilled /></el-icon>
            <div class="error-info">
              <span class="error-code">{{ task.error.code }}</span>
              <span class="error-message">{{ task.error.message }}</span>
            </div>
          </div>
          <el-button 
            type="primary" 
            size="small" 
            @click.stop="$emit('retry', task)"
          >
            <el-icon><RefreshRight /></el-icon>
            Retry
          </el-button>
        </div>

        <!-- Pending Section -->
        <div v-else-if="task.status === 'pending'" class="pending-section">
          <el-icon class="pending-icon"><Clock /></el-icon>
          <p>Waiting for execution...</p>
        </div>
      </div>
    </el-collapse-transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { 
  ArrowRight, Loading, Document, CircleCloseFilled, RefreshRight, Clock,
  WarningFilled, Warning, CircleCheck
} from '@element-plus/icons-vue'
import { 
  type AnalysisTask, 
  getTaskTypeIcon, 
  getStatusColor, 
  getRiskLevelColor,
  formatDuration 
} from '@/services/analysis-tasks'

const props = defineProps<{
  task: AnalysisTask
}>()

defineEmits<{
  (e: 'retry', task: AnalysisTask): void
  (e: 'view-report', task: AnalysisTask): void
}>()

// State
const expanded = ref(props.task.status === 'completed' || props.task.status === 'failed')

// Computed
const taskIcon = computed(() => getTaskTypeIcon(props.task.type))

const statusTagType = computed(() => getStatusColor(props.task.status))

const riskTagType = computed(() => getRiskLevelColor(props.task.result?.riskLevel))

const riskIcon = computed(() => {
  switch (props.task.result?.riskLevel) {
    case 'high': return '⚠️'
    case 'medium': return '⚡'
    case 'low': return '✅'
    default: return '📋'
  }
})

const riskLabel = computed(() => {
  switch (props.task.result?.riskLevel) {
    case 'high': return 'High Risk'
    case 'medium': return 'Medium Risk'
    case 'low': return 'Low Risk'
    default: return 'Analysis Complete'
  }
})

const formattedDuration = computed(() => formatDuration(props.task.durationMs))

// Methods
const toggleExpand = () => {
  expanded.value = !expanded.value
}
</script>

<style scoped lang="scss">
.analysis-task-item {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  overflow: hidden;
  transition: all 0.2s ease;
  
  &:hover {
    border-color: var(--el-border-color-hover);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
  }
  
  &.status-completed {
    border-left: 3px solid var(--el-color-success);
  }
  
  &.status-running {
    border-left: 3px solid var(--el-color-primary);
    animation: pulse-border 2s ease-in-out infinite;
  }
  
  &.status-failed {
    border-left: 3px solid var(--el-color-danger);
  }
  
  &.status-pending {
    border-left: 3px solid var(--el-text-color-placeholder);
  }
}

@keyframes pulse-border {
  0%, 100% { border-left-color: var(--el-color-primary); }
  50% { border-left-color: var(--el-color-primary-light-5); }
}

.task-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  cursor: pointer;
  background: var(--el-fill-color-lighter);
  
  .task-icon {
    font-size: 20px;
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--el-fill-color);
    border-radius: 6px;
  }
  
  .task-info {
    flex: 1;
    min-width: 0;
    
    .task-title {
      display: flex;
      align-items: center;
      gap: 8px;
      
      .task-type {
        font-weight: 600;
        font-size: 14px;
      }
      
      .status-tag {
        display: flex;
        align-items: center;
        gap: 4px;
        
        .rotating {
          animation: rotate 1s linear infinite;
        }
      }
    }
    
    .task-meta {
      margin-top: 4px;
      font-size: 12px;
      color: var(--el-text-color-secondary);
      
      .duration {
        &.running, &.pending {
          display: flex;
          align-items: center;
          gap: 4px;
        }
      }
    }
  }
  
  .task-result-preview {
    flex-shrink: 0;
  }
  
  .expand-icon {
    transition: transform 0.2s ease;
    color: var(--el-text-color-placeholder);
    
    &.rotated {
      transform: rotate(90deg);
    }
  }
}

.task-content {
  padding: 16px;
  background: var(--el-bg-color);
  border-top: 1px solid var(--el-border-color-lighter);
}

.progress-section {
  text-align: center;
  padding: 16px 0;
  
  .progress-text {
    margin-top: 8px;
    font-size: 13px;
    color: var(--el-text-color-secondary);
  }
}

.result-section {
  .result-summary {
    margin-bottom: 16px;
    
    .risk-badge {
      margin-bottom: 8px;
      
      .findings-count {
        margin-left: 4px;
        font-weight: normal;
      }
    }
    
    .summary-text {
      font-size: 13px;
      color: var(--el-text-color-regular);
      line-height: 1.5;
    }
  }
  
  .findings-list {
    background: var(--el-fill-color-lighter);
    border-radius: 6px;
    padding: 12px;
    margin-bottom: 12px;
    
    .finding-item {
      display: flex;
      align-items: flex-start;
      gap: 8px;
      padding: 8px 0;
      
      &:not(:last-child) {
        border-bottom: 1px solid var(--el-border-color-lighter);
      }
      
      .el-icon {
        flex-shrink: 0;
        margin-top: 2px;
        
        &.risk-high { color: var(--el-color-danger); }
        &.risk-medium { color: var(--el-color-warning); }
        &.risk-low { color: var(--el-color-success); }
      }
      
      .finding-content {
        flex: 1;
        min-width: 0;
        
        .finding-file {
          display: block;
          font-family: monospace;
          font-size: 12px;
          color: var(--el-color-primary);
          margin-bottom: 2px;
        }
        
        .finding-reason {
          display: block;
          font-size: 12px;
          color: var(--el-text-color-secondary);
        }
      }
    }
    
    .more-findings {
      text-align: center;
      padding-top: 8px;
      font-size: 12px;
      color: var(--el-text-color-placeholder);
    }
  }
  
  .result-actions {
    text-align: right;
  }
}

.error-section {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px;
  background: var(--el-color-danger-light-9);
  border-radius: 6px;
  
  .error-content {
    display: flex;
    align-items: center;
    gap: 12px;
    
    .error-icon {
      font-size: 24px;
      color: var(--el-color-danger);
    }
    
    .error-info {
      .error-code {
        display: block;
        font-weight: 600;
        font-size: 13px;
        color: var(--el-color-danger);
      }
      
      .error-message {
        display: block;
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }
  }
}

.pending-section {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 24px;
  color: var(--el-text-color-placeholder);
  
  .pending-icon {
    font-size: 20px;
  }
}

@keyframes rotate {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>

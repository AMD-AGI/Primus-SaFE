<template>
  <el-card class="analysis-tasks-card glass-card">
    <template #header>
      <div class="card-header">
        <div class="header-left">
          <el-icon><Monitor /></el-icon>
          <span>AI Analysis</span>
          <el-tag v-if="hasActiveTasks" type="primary" effect="plain" size="small" class="active-badge">
            <el-icon class="rotating"><Loading /></el-icon>
            Active
          </el-tag>
        </div>
        <div class="header-actions">
          <el-button 
            v-if="!loading" 
            :icon="Refresh" 
            circle 
            size="small" 
            @click="refresh"
            :loading="refreshing"
          />
        </div>
      </div>
    </template>

    <div v-loading="loading" class="tasks-container">
      <!-- Empty State -->
      <div v-if="!loading && tasks.length === 0" class="empty-state">
        <el-empty description="No analysis tasks yet" :image-size="60">
          <template #image>
            <el-icon :size="48" color="var(--el-text-color-placeholder)"><Monitor /></el-icon>
          </template>
          <template #description>
            <p class="empty-hint">Analysis tasks will appear when the workflow triggers them</p>
          </template>
        </el-empty>
      </div>

      <!-- Tasks List -->
      <div v-else class="tasks-list">
        <AnalysisTaskItem 
          v-for="task in tasks" 
          :key="task.id" 
          :task="task"
          @retry="handleRetry"
          @view-report="handleViewReport"
        />
      </div>

      <!-- Summary Footer -->
      <div v-if="summary && tasks.length > 0" class="summary-footer">
        <div class="summary-item" v-if="summary.completed > 0">
          <el-icon class="success"><SuccessFilled /></el-icon>
          <span>{{ summary.completed }} completed</span>
        </div>
        <div class="summary-item" v-if="summary.running > 0">
          <el-icon class="primary"><Loading /></el-icon>
          <span>{{ summary.running }} running</span>
        </div>
        <div class="summary-item" v-if="summary.pending > 0">
          <el-icon class="info"><Clock /></el-icon>
          <span>{{ summary.pending }} pending</span>
        </div>
        <div class="summary-item" v-if="summary.failed > 0">
          <el-icon class="danger"><CircleCloseFilled /></el-icon>
          <span>{{ summary.failed }} failed</span>
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Monitor, Refresh, Loading, SuccessFilled, CircleCloseFilled, Clock } from '@element-plus/icons-vue'
import { 
  getAnalysisTasksByRunId, 
  retryAnalysisTask,
  type AnalysisTask, 
  type AnalysisTaskSummary,
  hasActiveTasks as checkActiveTasks
} from '@/services/analysis-tasks'
import AnalysisTaskItem from './AnalysisTaskItem.vue'

const props = defineProps<{
  runId: number
  autoRefresh?: boolean
  refreshInterval?: number
}>()

const emit = defineEmits<{
  (e: 'view-report', task: AnalysisTask): void
}>()

// State
const loading = ref(true)
const refreshing = ref(false)
const tasks = ref<AnalysisTask[]>([])
const summary = ref<AnalysisTaskSummary | null>(null)

// Polling
let pollTimer: ReturnType<typeof setInterval> | null = null
const POLL_INTERVAL = props.refreshInterval || 5000

// Computed
const hasActiveTasks = computed(() => checkActiveTasks(tasks.value))

// Methods
const fetchTasks = async (isRefresh = false) => {
  if (isRefresh) {
    refreshing.value = true
  } else {
    loading.value = true
  }

  try {
    const response = await getAnalysisTasksByRunId(props.runId)
    tasks.value = response.tasks || []
    summary.value = response.summary || null
  } catch (error) {
    console.error('Failed to fetch analysis tasks:', error)
    if (!isRefresh) {
      // Only show error on initial load
    }
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

const refresh = () => {
  fetchTasks(true)
}

const handleRetry = async (task: AnalysisTask) => {
  try {
    await retryAnalysisTask(task.id)
    ElMessage.success('Task retry initiated')
    refresh()
  } catch (error) {
    console.error('Failed to retry task:', error)
    ElMessage.error('Failed to retry task')
  }
}

const handleViewReport = (task: AnalysisTask) => {
  emit('view-report', task)
}

// Polling management
const startPolling = () => {
  if (pollTimer) return
  pollTimer = setInterval(() => {
    if (hasActiveTasks.value) {
      fetchTasks(true)
    }
  }, POLL_INTERVAL)
}

const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// Watch for active tasks to manage polling
watch(hasActiveTasks, (active) => {
  if (active && props.autoRefresh !== false) {
    startPolling()
  } else {
    stopPolling()
  }
})

// Lifecycle
onMounted(() => {
  fetchTasks()
  if (props.autoRefresh !== false) {
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped lang="scss">
.analysis-tasks-card {
  .card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    
    .header-left {
      display: flex;
      align-items: center;
      gap: 8px;
      font-weight: 600;
      
      .active-badge {
        display: flex;
        align-items: center;
        gap: 4px;
        
        .rotating {
          animation: rotate 1s linear infinite;
        }
      }
    }
  }
  
  .tasks-container {
    min-height: 120px;
  }
  
  .empty-state {
    padding: 20px 0;
    
    .empty-hint {
      color: var(--el-text-color-placeholder);
      font-size: 13px;
      margin-top: 8px;
    }
  }
  
  .tasks-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  
  .summary-footer {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid var(--el-border-color-lighter);
    
    .summary-item {
      display: flex;
      align-items: center;
      gap: 4px;
      font-size: 12px;
      color: var(--el-text-color-secondary);
      
      .el-icon {
        font-size: 14px;
        
        &.success { color: var(--el-color-success); }
        &.primary { color: var(--el-color-primary); }
        &.info { color: var(--el-text-color-placeholder); }
        &.danger { color: var(--el-color-danger); }
      }
    }
  }
}

@keyframes rotate {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>

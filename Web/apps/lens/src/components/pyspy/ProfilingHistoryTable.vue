<template>
  <el-card class="profiling-history-card">
    <template #header>
      <div class="card-header">
        <span class="section-title">Profiling History</span>
        <el-button
          size="small"
          :icon="Refresh"
          @click="refresh"
          :loading="loading"
        >
          Refresh
        </el-button>
      </div>
    </template>

    <el-table
      :data="tasks"
      v-loading="loading"
      stripe
      size="default"
      class="history-table"
      max-height="400"
      empty-text="No profiling history yet. Start a profiling task to see results here."
    >
      <el-table-column prop="taskId" label="Task ID" width="200" fixed show-overflow-tooltip>
        <template #default="{ row }">
          <div class="task-id-cell">
            <el-link
              type="primary"
              :underline="false"
              :disabled="row.status !== 'completed'"
              @click="viewFlamegraph(row)"
              class="task-id-link"
            >
              {{ row.taskId }}
            </el-link>
            <el-icon
              class="copy-icon"
              @click.stop="copyTaskId(row.taskId)"
              title="Copy"
            >
              <DocumentCopy />
            </el-icon>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="podName" label="Pod Name" min-width="250" show-overflow-tooltip />

      <el-table-column prop="pid" label="PID" width="100" align="center" />

      <el-table-column prop="status" label="Status" width="140" align="center">
        <template #default="{ row }">
          <div class="status-cell">
            <el-tag
              :type="getStatusType(row.status)"
              class="status-tag"
            >
              {{ row.status }}
            </el-tag>
            <el-tooltip
              v-if="row.error"
              :content="row.error"
              placement="top"
            >
              <el-icon class="error-info">
                <Warning />
              </el-icon>
            </el-tooltip>
          </div>
        </template>
      </el-table-column>

      <el-table-column prop="format" label="Format" width="130" align="center">
        <template #default="{ row }">
          <el-tag type="info">{{ row.format }}</el-tag>
        </template>
      </el-table-column>

      <el-table-column prop="duration" label="Duration" width="120" align="center">
        <template #default="{ row }">
          {{ row.duration }}s
        </template>
      </el-table-column>

      <el-table-column prop="fileSize" label="File Size" width="120" align="center">
        <template #default="{ row }">
          {{ formatFileSize(row.fileSize) }}
        </template>
      </el-table-column>

      <el-table-column prop="createdAt" label="Created At" width="180">
        <template #default="{ row }">
          {{ formatDateTime(row.createdAt) }}
        </template>
      </el-table-column>

      <el-table-column label="Actions" width="150" fixed="right" align="center">
        <template #default="{ row }">
          <div class="action-buttons">
            <el-tooltip content="View Flamegraph" placement="top">
              <el-button
                link
                type="success"
                size="large"
                :icon="View"
                :disabled="row.status !== 'completed'"
                @click="viewFlamegraph(row)"
              />
            </el-tooltip>
            <el-tooltip content="Download" placement="top">
              <el-button
                link
                type="primary"
                size="large"
                :icon="Download"
                :disabled="row.status !== 'completed'"
                @click="downloadFile(row)"
              />
            </el-tooltip>
            <el-tooltip content="Cancel" placement="top">
              <el-button
                link
                type="danger"
                size="large"
                :icon="Close"
                :disabled="!['pending', 'running'].includes(row.status)"
                @click="cancelTask(row)"
              />
            </el-tooltip>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <!-- Pagination -->
    <div class="pagination-container">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50]"
        layout="total, sizes, prev, pager, next"
        @current-change="fetchTasks"
        @size-change="fetchTasks"
      />
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, reactive } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  DocumentCopy,
  Warning,
  Download,
  Close,
  Refresh,
  View
} from '@element-plus/icons-vue'
import { listPySpyTasks, cancelPySpyTask, type PySpyTask } from '@/services/pyspy'
import dayjs from 'dayjs'

interface Props {
  workloadUid: string
  cluster?: string
}

const props = defineProps<Props>()
const emit = defineEmits(['view-flamegraph'])

// State
const loading = ref(false)
const tasks = ref<PySpyTask[]>([])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0
})

// Polling
let pollingInterval: number | null = null

// Methods
const fetchTasks = async () => {
  loading.value = true
  try {
    const res: any = await listPySpyTasks({
      workloadUid: props.workloadUid,
      cluster: props.cluster,
      limit: pagination.pageSize,
      offset: (pagination.page - 1) * pagination.pageSize
    })
    tasks.value = res.tasks || []
    pagination.total = res.total || 0

    // Start polling if there are pending/running tasks
    const hasPendingTasks = tasks.value.some(t => 
      ['pending', 'running'].includes(t.status)
    )
    if (hasPendingTasks) {
      startPolling()
    } else {
      stopPolling()
    }
  } catch (error) {
    ElMessage.error('Failed to load profiling tasks')
    console.error('Failed to fetch tasks:', error)
  } finally {
    loading.value = false
  }
}

const startPolling = () => {
  if (pollingInterval) return
  
  pollingInterval = window.setInterval(() => {
    fetchTasks()
  }, 3000)
}

const stopPolling = () => {
  if (pollingInterval) {
    clearInterval(pollingInterval)
    pollingInterval = null
  }
}

const copyTaskId = (taskId: string) => {
  navigator.clipboard.writeText(taskId)
  ElMessage.success('Task ID copied to clipboard')
}

const viewFlamegraph = (task: PySpyTask) => {
  // Emit event to parent component to open flamegraph viewer
  emit('view-flamegraph', task.taskId, task.format as 'flamegraph' | 'speedscope')
}

const downloadFile = (task: PySpyTask) => {
  if (!task.outputFile) {
    ElMessage.warning('No output file available')
    return
  }

  const baseUrl = import.meta.env.BASE_URL || ''
  const url = `${baseUrl}v1/pyspy/file/${task.taskId}/${task.outputFile}`
  
  const a = document.createElement('a')
  a.href = url
  a.download = task.outputFile
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

const cancelTask = async (task: PySpyTask) => {
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to cancel task ${task.taskId}?`,
      'Cancel Task',
      {
        confirmButtonText: 'Cancel Task',
        cancelButtonText: 'Keep Running',
        type: 'warning'
      }
    )
    
    await cancelPySpyTask(task.taskId, props.cluster)
    ElMessage.success('Task cancelled successfully')
    await fetchTasks()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to cancel task')
      console.error('Failed to cancel task:', error)
    }
  }
}

const formatDateTime = (dateStr: string) => {
  return dayjs(dateStr).format('YYYY-MM-DD HH:mm:ss')
}

const formatFileSize = (bytes?: number) => {
  if (!bytes) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

const getStatusType = (status: string) => {
  const statusMap: Record<string, any> = {
    pending: 'warning',
    running: 'primary',
    completed: 'success',
    failed: 'danger',
    cancelled: 'info'
  }
  return statusMap[status] || ''
}

// Public methods
const refresh = () => {
  fetchTasks()
}

defineExpose({
  refresh
})

// Lifecycle
onMounted(() => {
  fetchTasks()
})

// Cleanup
onBeforeUnmount(() => {
  stopPolling()
})
</script>

<style scoped lang="scss">
.profiling-history-card {
  margin-bottom: 20px;

  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .section-title {
    font-size: 16px;
    font-weight: 600;
  }

  .history-table {
    width: 100%;
  }

  .task-id-cell {
    display: flex;
    align-items: center;
    gap: 8px;

    .task-id-link {
      font-family: 'Consolas', 'Monaco', monospace;

      &.is-disabled {
        cursor: not-allowed;
        color: var(--el-text-color-regular) !important;
        opacity: 0.8;
      }
    }

    .copy-icon {
      cursor: pointer;
      color: var(--el-text-color-secondary);
      transition: color 0.3s;
      flex-shrink: 0;

      &:hover {
        color: var(--el-color-primary);
      }
    }
  }

  .status-cell {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;

    .error-info {
      color: var(--el-color-danger);
      cursor: help;
    }
  }

  .action-buttons {
    display: flex;
    gap: 4px;
    justify-content: center;
  }

  .pagination-container {
    padding: 20px 0 16px 0;
  }
}
</style>

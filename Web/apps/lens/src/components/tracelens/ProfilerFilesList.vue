<template>
  <div class="profiler-files-list">
    <!-- Header with refresh button -->

    <!-- Files Table -->
    <div class="table-wrapper">
      <el-table
        :data="paginatedFiles"
        v-loading="loading"
        empty-text="No profiler files available"
        style="width: 100%"
        :height="tableHeight"
        stripe
      >
        <!-- File Name Column -->
        <el-table-column prop="fileName" label="File Name" min-width="280">
        <template #default="{ row }">
          <el-tooltip :content="row.fileName" placement="top">
            <span class="file-name">{{ row.fileName }}</span>
          </el-tooltip>
        </template>
      </el-table-column>

      <!-- Type Column -->
      <el-table-column prop="fileType" label="Type" min-width="120">
        <template #default="{ row }">
          <el-tag size="small">{{ row.fileType || 'pytorch_trace' }}</el-tag>
        </template>
      </el-table-column>

      <!-- Size Column -->
      <el-table-column prop="fileSize" label="Size" min-width="120">
        <template #default="{ row }">
          <span class="file-size">{{ formatFileSize(row.fileSize) }}</span>
        </template>
      </el-table-column>

      <!-- Created Time Column -->
      <el-table-column prop="createdAt" label="Created Time" min-width="200">
        <template #default="{ row }">
          <el-tooltip :content="dayjs(row.createdAt).format('YYYY-MM-DD HH:mm:ss')" placement="top">
            <span class="created-time">{{ formatRelativeTime(row.createdAt) }}</span>
          </el-tooltip>
        </template>
      </el-table-column>

      <!-- Active Session Column -->
      <el-table-column prop="activeSession" label="Active Session" min-width="180">
        <template #default="{ row }">
          <div v-if="row.activeSession" class="active-session">
            <el-link type="success" :underline="false" @click="openSession(row.activeSession)">
              <i i="ep-circle-check" class="session-icon" />
              Active
            </el-link>
            <el-tooltip
              v-if="row.sessionRemainingTime"
              :content="`Expires: ${row.sessionRemainingTime.text}`"
              placement="top"
            >
              <el-tag
                size="small"
                :type="row.sessionRemainingTime.isExpiring ? 'warning' : 'info'"
                class="session-time"
              >
                {{ row.sessionRemainingTime.text }}
              </el-tag>
            </el-tooltip>
          </div>
          <span v-else class="no-session">-</span>
        </template>
      </el-table-column>

      <!-- Actions Column -->
      <el-table-column label="Actions" min-width="320" fixed="right">
        <template #default="{ row }">
          <div class="action-buttons">
            <el-tooltip content="Deep analysis with TraceLens" placement="top">
              <el-button
                type="primary"
                size="small"
                @click="handleAnalyze(row)"
                :loading="row.creating"
              >
                {{ row.activeSession ? 'Open Analysis' : 'TraceLens' }}
              </el-button>
            </el-tooltip>
            <el-tooltip content="Quick view with Perfetto" placement="top">
              <el-button
                type="success"
                size="small"
                @click="handlePerfetto(row)"
              >
                Perfetto
              </el-button>
            </el-tooltip>
            <el-button
              size="small"
              @click="handleDownload(row)"
              :icon="Download"
            >
              Download
            </el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>
    </div>

    <!-- Pagination -->
    <div class="pagination-wrapper">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="pageSize"
        :page-sizes="[10, 20, 30, 50]"
        :total="filesWithStatus.length"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
      />
    </div>

    <!-- Create Session Dialog -->
    <TraceLensCreateDialog
      v-model:visible="createDialogVisible"
      :file="selectedFile"
      @created="handleSessionCreated"
    />

    <!-- Loading Page -->
    <TraceLensLoadingPage
      v-if="loadingSession"
      :session-id="creatingSessionId"
      :file-name="selectedFile?.fileName"
      @ready="handleSessionReady"
      @cancel="handleCancelCreate"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Download } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/zh-cn'
import { getProfilerFiles, downloadProfilerFile } from '@/services/dashboard/index'
import { listWorkloadSessions, calculateRemainingTime, formatRelativeTime } from '@/services/tracelens'
import { useGlobalCluster } from '@/composables/useGlobalCluster'
import TraceLensCreateDialog from './TraceLensCreateDialog.vue'
import TraceLensLoadingPage from './TraceLensLoadingPage.vue'

// Initialize dayjs plugins
dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

// Props
const props = defineProps<{
  workloadUid: string
}>()

// Get cluster from global state
const { selectedCluster } = useGlobalCluster()

// Router
const router = useRouter()
const route = useRoute()

// Data
const loading = ref(false)
const files = ref<any[]>([])
const sessions = ref<any[]>([])
const createDialogVisible = ref(false)
const selectedFile = ref<any>(null)
const loadingSession = ref(false)
const creatingSessionId = ref('')

// Pagination
const currentPage = ref(1)
const pageSize = ref(20)

// Table height calculation
const tableHeight = ref('calc(100vh - 260px)') // reserve 300px for page header, pagination, and other elements

// Computed: Files with active session status (all data, used for session lookup)
const filesWithStatus = computed(() => {
  return files.value.map(file => {
    // Only match TraceLens sessions (session_id starts with 'tls-'), not Perfetto sessions ('pft-')
    const activeSession = sessions.value.find(
      session => session.profilerFileId === file.id &&
                 session.status === 'ready' &&
                 session.sessionId?.startsWith('tls-')
    )

    let sessionRemainingTime = null
    if (activeSession?.expiresAt) {
      sessionRemainingTime = calculateRemainingTime(activeSession.expiresAt)
    }

    return {
      ...file,
      activeSession,
      sessionRemainingTime,
      creating: false
    }
  })
})

// Computed: Paginated files for display
const paginatedFiles = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  const end = start + pageSize.value
  return filesWithStatus.value.slice(start, end)
})

// Load profiler files
const loadFiles = async () => {
  loading.value = true
  try {
    // Get profiler files
    const response = await getProfilerFiles(props.workloadUid)

    files.value = response || []

    // Reset pagination when loading new data
    currentPage.value = 1

    // Get active sessions
    if (files.value.length > 0) {
      await loadSessions()
    }
  } catch (error) {
    console.error('Failed to load profiler files:', error)
    ElMessage.error('Failed to load profiler files')
  } finally {
    loading.value = false
  }
}

// Load active sessions for this workload
const loadSessions = async () => {
  try {
    const sessionList = await listWorkloadSessions(props.workloadUid, selectedCluster.value)
    // sessionList is already an array of sessions
    sessions.value = sessionList?.sessions || []
  } catch (error) {
    console.error('Failed to load sessions:', error)
    // Don't show error message as this is not critical
  }
}

// Handle analyze button click
const handleAnalyze = async (file: any) => {
  // Check if there's already an active session
  if (file.activeSession) {
    // Open existing session
    openSession(file.activeSession)
  } else {
    // Show create dialog
    selectedFile.value = file
    createDialogVisible.value = true
  }
}

// Open existing session
const openSession = (session: any) => {
  router.push({
    name: 'TraceLensAnalysis',
    params: {
      workloadUid: props.workloadUid,
      sessionId: session.sessionId
    },
    query: {
      kind: route.query.kind,
      name: route.query.name
    }
  })
}

// Handle session created from dialog
const handleSessionCreated = (sessionId: string) => {
  creatingSessionId.value = sessionId
  loadingSession.value = true
  createDialogVisible.value = false
}

// Handle session ready
const handleSessionReady = (session: any) => {
  loadingSession.value = false
  // Navigate to analysis page
  router.push({
    name: 'TraceLensAnalysis',
    params: {
      workloadUid: props.workloadUid,
      sessionId: session.sessionId
    },
    query: {
      kind: route.query.kind,
      name: route.query.name
    }
  })
}

// Handle cancel create
const handleCancelCreate = () => {
  loadingSession.value = false
  creatingSessionId.value = ''
  // Reload sessions to update status
  loadSessions()
}

// Handle Perfetto viewer
const handlePerfetto = (file: any) => {
  router.push({
    name: 'PerfettoViewer',
    params: {
      workloadUid: props.workloadUid,
      fileId: file.id
    },
    query: {
      cluster: selectedCluster.value,
      workloadName: route.query.name,
      fileName: file.fileName
    }
  })
}

// Handle download file
const handleDownload = (file: any) => {
  try {
    downloadProfilerFile(file.id, file.fileName, selectedCluster.value)
    ElMessage.success(`Downloading ${file.fileName}`)
  } catch (error) {
    console.error('Failed to download file:', error)
    ElMessage.error('Failed to download file')
  }
}

// Format file size
const formatFileSize = (bytes: number) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

// Format relative time is imported from services/tracelens

// Handle page size change
const handleSizeChange = (val: number) => {
  pageSize.value = val
  // Reset to first page when changing page size
  currentPage.value = 1
}

// Handle current page change
const handleCurrentChange = (val: number) => {
  currentPage.value = val
}

// Watch for workload changes
watch(() => props.workloadUid, () => {
  if (props.workloadUid) {
    loadFiles()
  }
})

onMounted(() => {
  if (props.workloadUid) {
    loadFiles()
  }
  // Removed auto-refresh to avoid unnecessary periodic requests
})
</script>

<style scoped lang="scss">
.profiler-files-list {
  padding: 20px;
  background: #fff;
  border-radius: 8px;
  width: 100%;
  box-sizing: border-box;

  .list-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;

    h4 {
      margin: 0;
      font-size: 18px;
      font-weight: 600;
      color: var(--el-text-color-primary);

      @media (min-width: 1920px) {
        font-size: 20px;
      }
    }
  }

  // Table wrapper for proper scrolling
  .table-wrapper {
    overflow-x: auto;
    width: 100%;

    // Table styling similar to WorkloadStats
    :deep(.el-table) {
      font-size: 14px;
      min-width: 100%;

      @media (min-width: 1920px) {
        font-size: 15px;
      }

      // Table row height
      td {
        padding: 14px 0;

        @media (min-width: 1920px) {
          padding: 16px 0;
        }
      }

      // Table header
      th {
        font-size: 14px;
        font-weight: 600;
        padding: 14px 0;

        @media (min-width: 1920px) {
          font-size: 15px;
          padding: 16px 0;
        }
      }

      // Cell padding
      .cell {
        padding-left: 12px;
        padding-right: 12px;
      }

      // Empty state
      .el-table__empty-text {
        font-size: 14px;
        color: var(--el-text-color-secondary);

        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
    }
  }

  .file-name {
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: 'SF Mono', Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace;
    font-size: 14px;
    font-weight: 500;
    color: var(--el-text-color-primary);

    @media (min-width: 1920px) {
      font-size: 15px;
    }
  }

  .file-size {
    font-size: 14px;
    font-weight: 500;
    color: var(--el-text-color-regular);

    @media (min-width: 1920px) {
      font-size: 15px;
    }
  }

  .created-time {
    font-size: 14px;
    color: var(--el-text-color-regular);

    @media (min-width: 1920px) {
      font-size: 15px;
    }
  }

  .active-session {
    display: flex;
    align-items: center;
    gap: 8px;

    :deep(.el-link) {
      font-size: 14px;
      font-weight: 500;

      @media (min-width: 1920px) {
        font-size: 15px;
      }
    }

    .session-icon {
      margin-right: 4px;
      color: var(--el-color-success);
      font-size: 14px;

      @media (min-width: 1920px) {
        font-size: 15px;
      }
    }

    .session-time {
      font-size: 13px;

      @media (min-width: 1920px) {
        font-size: 14px;
      }
    }
  }

  .no-session {
    color: var(--el-text-color-secondary);
    font-size: 14px;

    @media (min-width: 1920px) {
      font-size: 15px;
    }
  }

  // Action buttons layout
  .action-buttons {
    display: flex;
    gap: 10px;
    flex-wrap: nowrap;

    @media (max-width: 768px) {
      gap: 8px;
    }
  }

  // Button styling
  :deep(.el-button) {
    font-size: 14px;

    @media (min-width: 1920px) {
      font-size: 15px;
    }
  }

  :deep(.el-button--small) {
    --el-button-size: 30px;

    @media (min-width: 1920px) {
      --el-button-size: 32px;
    }
  }

  // Tag styling
  :deep(.el-tag) {
    font-size: 13px;

    @media (min-width: 1920px) {
      font-size: 14px;
    }
  }

  :deep(.el-tag--small) {
    height: 22px;
    padding: 0 8px;

    @media (min-width: 1920px) {
      height: 24px;
      padding: 0 10px;
    }
  }

  // Pagination wrapper
  .pagination-wrapper {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    margin-top: 20px;
    padding: 0 12px;

    :deep(.el-pagination) {
      font-size: 14px;

      @media (min-width: 1920px) {
        font-size: 15px;
      }

      .el-input__inner {
        font-size: 14px;

        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
    }
  }
}

// Dark theme support
.dark {
  .profiler-files-list {
    background: var(--el-bg-color);

    h4 {
      color: var(--el-text-color-primary);
    }
  }
}
</style>

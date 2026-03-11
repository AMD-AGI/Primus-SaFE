<template>
  <div class="job-execution-history">
    <!-- Page Header -->
    <div class="page-header">
      <h1 class="page-title">Job Execution History</h1>
      <div class="header-actions">
        <el-input 
          v-model="filterForm.jobName" 
          placeholder="Search Job Name"
          clearable
          size="default"
          style="width: 200px"
          @input="handleFilter"
          @clear="handleFilter"
        >
          <template #prefix>
            <i i="ep-search" />
          </template>
        </el-input>
        <el-input 
          v-model="filterForm.jobType" 
          placeholder="Search Job Type"
          clearable
          size="default"
          style="width: 180px"
          @input="handleFilter"
          @clear="handleFilter"
        >
          <template #prefix>
            <i i="ep-search" />
          </template>
        </el-input>
      </div>
    </div>
    
    <!-- Data Table -->
    <el-card class="table-card">

      <el-table 
        :data="tableData" 
        v-loading="loading"
        style="width: 100%"
        stripe
        @filter-change="handleFilterChange"
      >
        <el-table-column prop="id" label="ID" width="100" fixed>
          <template #default="{ row }">
            <el-link type="primary" @click="handleViewDetail(row)">
              {{ row.id }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column prop="jobName" label="Job Name" width="260">
          <template #default="{ row }">
            <el-tooltip :content="row.jobName" placement="top">
              <span class="text-ellipsis">{{ row.jobName }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="jobType" label="Job Type" width="220" />
        <el-table-column 
          prop="status" 
          label="Status" 
          width="140" 
          :filters="statusFilters" 
          :filter-method="filterStatus"
          :filtered-value="filters.status ? [filters.status] : []"
        >
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="default">
              {{ getStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="clusterName" label="Cluster" width="180" />
        <el-table-column prop="hostname" label="Hostname" width="200" />
        <el-table-column prop="startedAt" label="Started At" width="200">
          <template #default="{ row }">
            {{ formatTime(row.startedAt) }}
          </template>
        </el-table-column>
        <el-table-column prop="duration" label="Duration" width="140">
          <template #default="{ row }">
            {{ formatDuration(row.duration) }}
          </template>
        </el-table-column>
        <el-table-column prop="exitCode" label="Exit Code" width="120">
          <template #default="{ row }">
            {{ row.exitCode ?? '-' }}
          </template>
        </el-table-column>
        
        <template #empty>
          <el-empty description="No Data" />
        </template>
      </el-table>

      <!-- Pagination -->
      <el-pagination
        v-model:current-page="pagination.pageNum"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @current-change="handlePageChange"
        @size-change="handleSizeChange"
        class="mt-5"
      />
    </el-card>

    <!-- Detail Dialog -->
    <el-dialog
      v-model="detailDialogVisible"
      title="Job Execution Details"
      width="60%"
      :close-on-click-modal="false"
    >
      <div v-if="currentDetail" class="detail-content">
        <el-descriptions :column="2" border>
          <el-descriptions-item label="ID">{{ currentDetail.id }}</el-descriptions-item>
          <el-descriptions-item label="Job Name">{{ currentDetail.jobName }}</el-descriptions-item>
          <el-descriptions-item label="Job Type">{{ currentDetail.jobType }}</el-descriptions-item>
          <el-descriptions-item label="Status">
            <el-tag :type="getStatusType(currentDetail.status)">
              {{ getStatusLabel(currentDetail.status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Cluster">{{ currentDetail.clusterName }}</el-descriptions-item>
          <el-descriptions-item label="Hostname">{{ currentDetail.hostname }}</el-descriptions-item>
          <el-descriptions-item label="Started At">{{ formatTime(currentDetail.startedAt) }}</el-descriptions-item>
          <el-descriptions-item label="Finished At">
            {{ currentDetail.finishedAt ? formatTime(currentDetail.finishedAt) : '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="Duration">{{ formatDuration(currentDetail.duration) }}</el-descriptions-item>
          <el-descriptions-item label="Exit Code">{{ currentDetail.exitCode }}</el-descriptions-item>
          <el-descriptions-item label="Error Message" :span="2">
            <span class="text-red-500">{{ currentDetail.errorMessage || '-' }}</span>
          </el-descriptions-item>
        </el-descriptions>
        
        <div v-if="currentDetail.metadata && Object.keys(currentDetail.metadata).length > 0" class="mt-5">
          <div class="text-lg fw-600 mb-3">Metadata</div>
          <el-descriptions :column="2" border>
            <el-descriptions-item 
              v-for="(value, key) in currentDetail.metadata" 
              :key="key"
              :label="key"
            >
              {{ formatMetadataValue(value) }}
            </el-descriptions-item>
          </el-descriptions>
        </div>
      </div>
      <template #footer>
        <el-button @click="detailDialogVisible = false">Close</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { 
  getJobExecutionHistories, 
  getJobExecutionHistory,
  type JobExecutionHistory,
  type JobExecutionHistoryListParams 
} from '@/services/job-execution-history'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'

const route = useRoute()
const router = useRouter()

// Table data
const tableData = ref<JobExecutionHistory[]>([])
const loading = ref(false)

// Pagination info
const pagination = reactive({
  pageNum: 1,
  pageSize: 20,
  total: 0
})

// Filter form
const filterForm = reactive({
  jobName: '',
  jobType: '',
  status: ''
})

// Separate filters state for table column filtering
const filters = ref({
  status: ''
})

// Status filters for table header
const statusFilters = [
  { text: 'Running', value: 'running' },
  { text: 'Success', value: 'success' },
  { text: 'Failed', value: 'failed' },
  { text: 'Cancelled', value: 'cancelled' },
  { text: 'Timeout', value: 'timeout' }
]

// Detail dialog
const detailDialogVisible = ref(false)
const currentDetail = ref<JobExecutionHistory | null>(null)

// Load params from URL
const loadParamsFromUrl = () => {
  const query = route.query
  pagination.pageNum = Number(query.pageNum) || 1
  pagination.pageSize = Number(query.pageSize) || 20
  filterForm.jobName = (query.jobName as string) || ''
  filterForm.jobType = (query.jobType as string) || ''
  filterForm.status = (query.status as string) || ''
}

// Update URL params
const updateUrlParams = () => {
  const query: any = {
    pageNum: pagination.pageNum,
    pageSize: pagination.pageSize
  }
  
  if (filterForm.jobName) query.jobName = filterForm.jobName
  if (filterForm.jobType) query.jobType = filterForm.jobType
  if (filterForm.status) query.status = filterForm.status
  
  router.push({ 
    path: route.path,
    query 
  })
}

// Fetch data
const fetchData = async () => {
  loading.value = true
  try {
    const params: JobExecutionHistoryListParams = {
      pageNum: pagination.pageNum,
      pageSize: pagination.pageSize
    }
    
    if (filterForm.jobName) params.jobName = filterForm.jobName
    if (filterForm.jobType) params.jobType = filterForm.jobType
    if (filterForm.status) params.status = filterForm.status
    
    const res = await getJobExecutionHistories(params)
    tableData.value = res.data || []
    pagination.total = res.total || 0
  } catch (error: any) {
    ElMessage.error(error || 'Failed to fetch data')
    tableData.value = []
    pagination.total = 0
  } finally {
    loading.value = false
  }
}

// Handle filter
const handleFilter = () => {
  pagination.pageNum = 1
  updateUrlParams()
  fetchData()
}

// Handle table column filter change
const handleFilterChange = (filterValues: any) => {
  if (filterValues.status) {
    filters.value.status = filterValues.status[0] || ''
    filterForm.status = filterValues.status[0] || ''
  }
  pagination.pageNum = 1
  updateUrlParams()
  fetchData()
}

// Filter method for status column
const filterStatus = (value: string, row: JobExecutionHistory) => {
  return row.status === value
}

// Reset filter
const handleReset = () => {
  filterForm.jobName = ''
  filterForm.jobType = ''
  filterForm.status = ''
  filters.value.status = ''
  pagination.pageNum = 1
  updateUrlParams()
  fetchData()
}

// Handle page change
const handlePageChange = (page: number) => {
  pagination.pageNum = page
  updateUrlParams()
  fetchData()
}

// Handle size change
const handleSizeChange = (size: number) => {
  pagination.pageSize = size
  pagination.pageNum = 1
  updateUrlParams()
  fetchData()
}

// View details
const handleViewDetail = async (row: JobExecutionHistory) => {
  try {
    currentDetail.value = await getJobExecutionHistory(row.id)
    detailDialogVisible.value = true
  } catch (error: any) {
    ElMessage.error(error || 'Failed to fetch details')
  }
}

// Get status tag type
const getStatusType = (status: string) => {
  const statusMap: Record<string, any> = {
    running: 'info',
    success: 'success',
    failed: 'danger',
    cancelled: 'warning',
    timeout: 'warning'
  }
  return statusMap[status] || ''
}

// Get status label text
const getStatusLabel = (status: string) => {
  const labelMap: Record<string, string> = {
    running: 'Running',
    success: 'Success',
    failed: 'Failed',
    cancelled: 'Cancelled',
    timeout: 'Timeout'
  }
  return labelMap[status] || status
}

// Format time
const formatTime = (time: string) => {
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

// Format duration
const formatDuration = (seconds: number) => {
  if (!seconds && seconds !== 0) return '-'
  
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const secs = Math.floor(seconds % 60)
  
  if (hours > 0) {
    return `${hours}h ${minutes}m ${secs}s`
  } else if (minutes > 0) {
    return `${minutes}m ${secs}s`
  } else {
    return `${secs}s`
  }
}

// Format metadata value
const formatMetadataValue = (value: any) => {
  if (typeof value === 'object') {
    return JSON.stringify(value)
  }
  return String(value)
}

// Watch route changes
watch(() => route.query, () => {
  if (route.path.includes('job-execution-history')) {
    loadParamsFromUrl()
    fetchData()
  }
}, { deep: true })

// Initialize
onMounted(() => {
  loadParamsFromUrl()
  fetchData()
})
</script>

<style scoped lang="scss">
.job-execution-history {
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 20px;
    
    .page-title {
      font-size: 20px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin: 0;
      
      @media (min-width: 1920px) {
        font-size: 24px;
      }
    }
    
    .header-actions {
      display: flex;
      gap: 12px;
      align-items: center;
    }
  }
  
  .table-card {
    border-radius: 15px;
    
    :deep(.el-table) {
      font-size: 14px;
      
      @media (min-width: 1920px) {
        font-size: 15px;
      }
      
      th {
        font-size: 14px;
        font-weight: 600;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
      
      td {
        padding: 14px 0;
        
        @media (min-width: 1920px) {
          padding: 16px 0;
        }
      }
      
      th {
        padding: 14px 0;
        
        @media (min-width: 1920px) {
          padding: 16px 0;
        }
      }
      
      .cell {
        padding-left: 12px;
        padding-right: 12px;
      }
    }
  }
  
  .text-ellipsis {
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .detail-content {
    padding: 10px 0;
    
    :deep(.el-descriptions) {
      font-size: 14px;
      
      @media (min-width: 1920px) {
        font-size: 15px;
      }
      
      .el-descriptions__label {
        font-size: 14px;
        font-weight: 600;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
      
      .el-descriptions__content {
        font-size: 14px;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
    }
  }
  
  .el-pagination {
    justify-content: center;
    
    :deep(.el-pagination__sizes) {
      font-size: 14px;
      
      @media (min-width: 1920px) {
        font-size: 15px;
      }
    }
    
    :deep(.el-pager) {
      font-size: 14px;
      
      @media (min-width: 1920px) {
        font-size: 15px;
      }
    }
  }
}

:deep(.el-dialog) {
  .el-dialog__title {
    font-size: 18px;
    
    @media (min-width: 1920px) {
      font-size: 20px;
    }
  }
  
  .el-dialog__body {
    font-size: 14px;
    
    @media (min-width: 1920px) {
      font-size: 15px;
    }
  }
}
</style>


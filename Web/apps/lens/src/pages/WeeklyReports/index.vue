<template>
  <div class="workload-stats">
    <div class="filter-section">
      <div class="filter-header">
        <h2 class="page-title">Reports</h2>
      </div>
    </div>

    <!-- Data Table -->
    <el-card class="table-card">
      <div class="table-wrapper">
      <el-table 
        v-loading="loading"
        :data="statsData" 
        style="width: 100%"
        @sort-change="handleTableSortChange"
        @filter-change="handleFilterChange"
      >
        <el-table-column 
          prop="id" 
          label="Report ID" 
          min-width="240"
          fixed="left"
        >
          <template #default="{ row }">
            <el-link 
              type="primary" 
              :underline="false" 
              @click="viewReportDetail(row)"
              class="workload-link"
            >
              {{ row.id }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column 
          prop="status" 
          label="Status" 
          width="120"
          fixed="left"
          column-key="status"
          :filters="statusFilters"
          :filtered-value="filters.status ? [filters.status] : []"
        >
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">
              {{ row.status || 'pending' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column 
          prop="periodStart" 
          label="Period Start" 
          min-width="150"
          sortable="custom"
        >
          <template #default="{ row }">
            {{ formatDate(row.periodStart) }}
          </template>
        </el-table-column>
        
        <el-table-column 
          prop="periodEnd" 
          label="Period End" 
          min-width="150"
          sortable="custom"
        >
          <template #default="{ row }">
            {{ formatDate(row.periodEnd) }}
          </template>
        </el-table-column>
        
        <el-table-column 
          prop="metadata.avgUtilization" 
          label="Avg Utilization (%)" 
          min-width="200"
          sortable="custom"
        >
          <template #default="{ row }">
            <span :style="{ color: getUtilizationColor(row.metadata?.avgUtilization) }">
              {{ formatPercent(row.metadata?.avgUtilization) }}
            </span>
          </template>
        </el-table-column>
        
        <el-table-column 
          prop="metadata.avgAllocation" 
          label="Avg Allocation (%)" 
          min-width="200"
          sortable="custom"
        >
          <template #default="{ row }">
            <span :style="{ color: getUtilizationColor(row.metadata?.avgAllocation) }">
              {{ formatPercent(row.metadata?.avgAllocation) }}
            </span>
          </template>
        </el-table-column>
        
        <el-table-column 
          prop="metadata.totalGpus" 
          label="Total GPUs" 
          min-width="170"
          sortable="custom"
        >
          <template #default="{ row }">
            {{ row.metadata?.totalGpus || 0 }}
          </template>
        </el-table-column>
        
        <el-table-column 
          prop="metadata.lowUtilCount" 
          label="Low Util Count" 
          min-width="170"
          sortable="custom"
        >
          <template #default="{ row }">
            {{ row.metadata?.lowUtilCount || 0 }}
          </template>
        </el-table-column>
        
        <el-table-column 
          prop="metadata.wastedGpuDays" 
          label="Wasted GPU Days" 
          min-width="200"
          sortable="custom"
        >
          <template #default="{ row }">
            {{ row.metadata?.wastedGpuDays || 0 }}
          </template>
        </el-table-column>
        
        <el-table-column 
          prop="generatedAt" 
          label="Generated At" 
          min-width="180"
        >
          <template #default="{ row }">
            {{ formatDateTime(row.generatedAt) }}
          </template>
        </el-table-column>
        
        <el-table-column 
          label="Downloads" 
          width="150"
          fixed="right"
          align="center"
        >
          <template #default="{ row }">
            <div class="action-buttons">
              <el-tooltip v-if="row.hasPdf" content="Download PDF Report" placement="top">
                <el-button
                  circle
                  class="download-btn download-btn--pdf"
                  @click="downloadReport(row.id, 'pdf')"
                >
                  <el-icon :size="16"><Download /></el-icon>
                </el-button>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>
        
        <template #empty>
          <el-empty description="No Data" />
        </template>
      </el-table>
      
      <el-pagination
        v-if="pagination.total > 0"
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @current-change="handlePageChange"
        @size-change="handlePageChange"
        class="mt-4"
      />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Document, DocumentCopy, DataAnalysis, Download } from '@element-plus/icons-vue'
import { getWeeklyReports } from '@/services/weekly-reports'
import { useClusterSync } from '@/composables/useClusterSync'
import dayjs from 'dayjs'

// Types
interface WeeklyReport {
  id: string
  clusterName: string
  periodStart: string
  periodEnd: string
  status: string
  generatedAt?: string
  hasHtml?: boolean
  hasJson?: boolean
  hasPdf?: boolean
  metadata?: {
    avgAllocation: number
    avgUtilization: number
    clusterName: string
    lowUtilCount: number
    totalGpus: number
    wastedGpuDays: number
  }
}

// Global cluster state
const { selectedCluster } = useClusterSync()
const router = useRouter()

// State
const loading = ref(false)
const statsData = ref<WeeklyReport[]>([])

// Filters
const filters = ref({
  status: ''
})

// Status filters for table header
const statusFilters = [
  { text: 'Generated', value: 'generated' },
  { text: 'Pending', value: 'pending' },
  { text: 'Failed', value: 'failed' }
]

// Pagination
const pagination = ref({
  page: 1,
  pageSize: 20,
  total: 0
})

// Sorting
const sortConfig = ref({
  prop: '',
  order: ''
})

// Fetch reports
const fetchData = async () => {
  loading.value = true
  try {
    const params = {
      page: pagination.value.page,
      pageSize: pagination.value.pageSize,
      clusterName: selectedCluster.value || undefined,
      status: filters.value.status || undefined,
      sortBy: sortConfig.value.prop || undefined,
      sortOrder: sortConfig.value.order === 'ascending' ? 'asc' : sortConfig.value.order === 'descending' ? 'desc' : undefined
    }
    
    // Remove undefined values
    Object.keys(params).forEach(key => (params as any)[key] === undefined && delete (params as any)[key])
    
    const response: any = await getWeeklyReports(params)
    
    if (response) {
      statsData.value = response.reports || []
      pagination.value.total = response.total || 0
      if (response.size) {
        pagination.value.pageSize = response.size
      }
    }
  } catch (error) {
    console.error('Failed to fetch weekly reports:', error)
    ElMessage.error('Failed to fetch weekly reports')
  } finally {
    loading.value = false
  }
}

// Handle table sort
const handleTableSortChange = ({ prop, order }: { prop: string; order: string }) => {
  sortConfig.value.prop = prop
  sortConfig.value.order = order
  fetchData()
}

// Handle filter change
const handleFilterChange = (filterValues: Record<string, string[]>) => {
  if (filterValues.status && filterValues.status.length > 0) {
    filters.value.status = filterValues.status[0]
  } else {
    filters.value.status = ''
  }
  
  pagination.value.page = 1
  fetchData()
}

// Handle pagination
const handlePageChange = () => {
  fetchData()
}

// Download report
const downloadReport = (reportId: string, format: 'html' | 'json' | 'pdf') => {
  const base = import.meta.env.BASE_URL

  const url = `${base}v1/weekly-reports/gpu_utilization/${reportId}/${format}`

  const a = document.createElement('a')
  a.href = url
  a.style.display = 'none'
  a.setAttribute('download', '')
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

// View report detail - navigate to detail page
const viewReportDetail = (row: WeeklyReport) => {
  router.push({
    path: `/weekly-reports/${row.id}`,
    query: selectedCluster.value ? { cluster: selectedCluster.value } : {}
  })
}

// Formatters
const formatDate = (date: any) => {
  if (!date) return 'N/A'
  return dayjs(date).format('YYYY-MM-DD')
}

const formatDateTime = (date: any) => {
  if (!date) return 'N/A'
  return dayjs(date).format('YYYY-MM-DD HH:mm:ss')
}

const formatPercent = (value: any) => {
  if (value === null || value === undefined) return '0%'
  return `${parseFloat(value).toFixed(2)}%`
}

// Get utilization color
const getUtilizationColor = (value: any) => {
  const percent = parseFloat(value) || 0
  if (percent >= 80) return '#67c23a'
  if (percent >= 60) return '#409eff'
  if (percent >= 40) return '#e6a23c'
  return '#f56c6c'
}

// Get status type
const getStatusType = (status: string) => {
  switch (status) {
    case 'generated':
      return 'success'
    case 'pending':
      return 'warning'
    case 'failed':
      return 'danger'
    default:
      return 'info'
  }
}

// Watch cluster change
watch(selectedCluster, () => {
  pagination.value.page = 1
  fetchData()
})

// Lifecycle
onMounted(() => {
  fetchData()
})
</script>

<style scoped lang="scss">
// Styles copied from WorkloadStats
.workload-stats {
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  box-sizing: border-box;
  padding: 0 20px;
  
  @media (max-width: 768px) {
    padding: 0 12px;
  }
  
  .filter-section {
    margin-bottom: 20px;
    
    .filter-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      padding: 12px 0;
      gap: 20px;
      flex-wrap: wrap;
      
      @media (max-width: 768px) {
        flex-direction: column;
        align-items: stretch;
        gap: 12px;
      }
    }
    
    .page-title {
      font-size: 20px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin: 0;
      flex-shrink: 0;
      
      @media (min-width: 1920px) {
        font-size: 22px;
      }
      
      @media (max-width: 768px) {
        font-size: 18px;
      }
    }
  }
  
  .table-card {
    border-radius: 15px;
    overflow: hidden; // Ensure proper scroll container
    height: calc(100vh - 200px); // Subtract header and filter-section height
    display: flex;
    flex-direction: column;
    
    :deep(.el-card__body) {
      padding: 0;
      height: 100%;
      display: flex;
      flex-direction: column;
    }
    
    // Wrapper for horizontal scroll
    .table-wrapper {
      overflow-x: auto;
      padding: 20px;
      flex: 1;
      display: flex;
      flex-direction: column;
      
      @media (max-width: 768px) {
        padding: 12px;
      }
      
      :deep(.el-table) {
        min-width: 1200px; // Ensure no line wrapping
        height: 100%; // Fill full height
        
        .el-table__body-wrapper {
          flex: 1;
          overflow-y: auto;
        }
        
        @media (max-width: 768px) {
          font-size: 12px;
          
          th, td {
            padding: 8px 0;
          }
        }
      }
      
      .el-pagination {
        margin-top: auto; // Keep pagination fixed at bottom
        padding-top: 20px;
      }
    }
    
    // Table overall font size
    :deep(.el-table) {
      font-size: 14px;
      
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
      
      th {
        font-size: 14px;
        font-weight: 600;
        padding: 14px 0;
        
        @media (min-width: 1920px) {
          font-size: 15px;
          padding: 16px 0;
        }
      }
      
      // Table cell padding
      .cell {
        padding-left: 12px;
        padding-right: 12px;
      }
    }
    
    .action-buttons {
      display: flex;
      align-items: center;
      justify-content: center;
      
      .download-btn {
        width: 30px;
        height: 30px;
        padding: 0;
        border: 1px solid var(--el-border-color-lighter);
        background: var(--el-bg-color);
        transition: all 0.2s ease;
        
        &:hover {
          transform: translateY(-1px);
          box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
          border-color: var(--el-border-color);
        }
        
        &:active {
          transform: translateY(0);
        }
        
        .el-icon {
          font-size: 15px;
          transition: all 0.2s ease;
        }
        
        // PDF button - danger color
        &--pdf {
          .el-icon {
            color: var(--el-color-success);
          }
          
          &:hover {
            background: var(--el-color-success-light-9);
            border-color: var(--el-color-success-light-5);
            
            .el-icon {
              color: var(--el-color-success);
            }
          }
        }

      }
    }
    
    .workload-link {
      font-weight: 500;
      font-size: 14px;
      
      @media (min-width: 1920px) {
        font-size: 15px;
      }
      
      &:hover {
        font-weight: 600;
      }
    }
  }
}
</style>

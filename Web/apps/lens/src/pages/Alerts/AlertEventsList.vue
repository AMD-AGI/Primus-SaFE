<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  alertEventsApi, 
  alertSilencesApi,
  type AlertEvent, 
  type ListAlertsParams,
  type AlertSeverity,
  type AlertStatus,
  type AlertSource
} from '@/services/alerts'
import AlertSeverityBadge from './components/AlertSeverityBadge.vue'
import AlertStatusTag from './components/AlertStatusTag.vue'
import AlertSourceIcon from './components/AlertSourceIcon.vue'
import { useClusterStore } from '@/stores/cluster'

const router = useRouter()
const clusterStore = useClusterStore()

// State
const loading = ref(false)
const alerts = ref<AlertEvent[]>([])
const total = ref(0)
const selectedAlerts = ref<AlertEvent[]>([])

// Filters
const filters = reactive<ListAlertsParams>({
  status: undefined,
  severity: undefined,
  source: undefined,
  alertName: '',
  startsAfter: undefined,
  startsBefore: undefined,
  offset: 0,
  limit: 20
})

const timeRange = ref<[Date, Date] | null>(null)

// Pagination
const currentPage = computed({
  get: () => Math.floor((filters.offset || 0) / (filters.limit || 20)) + 1,
  set: (val: number) => {
    filters.offset = (val - 1) * (filters.limit || 20)
  }
})

// Fetch data
async function fetchAlerts() {
  loading.value = true
  try {
    const params: ListAlertsParams = {
      ...filters,
      cluster: clusterStore.currentCluster || undefined
    }
    
    if (timeRange.value && timeRange.value.length === 2) {
      params.startsAfter = timeRange.value[0].toISOString()
      params.startsBefore = timeRange.value[1].toISOString()
    }
    
    const response = await alertEventsApi.list(params)
    alerts.value = response?.data || []
    total.value = response?.total || 0
  } catch (error) {
    console.error('Failed to fetch alerts:', error)
    ElMessage.error('Failed to fetch alerts')
  } finally {
    loading.value = false
  }
}

// Filter actions
function applyFilters() {
  filters.offset = 0
  fetchAlerts()
}

function resetFilters() {
  filters.status = undefined
  filters.severity = undefined
  filters.source = undefined
  filters.alertName = ''
  filters.offset = 0
  timeRange.value = null
  fetchAlerts()
}

// Selection
function handleSelectionChange(selection: AlertEvent[]) {
  selectedAlerts.value = selection
}

// Actions
function goToDetail(alert: AlertEvent) {
  router.push(`/alerts/events/${alert.id}`)
}

async function silenceSelected() {
  if (selectedAlerts.value.length === 0) {
    ElMessage.warning('Please select alerts to silence')
    return
  }
  
  try {
    await ElMessageBox.confirm(
      `Create silence for ${selectedAlerts.value.length} selected alert(s)?`,
      'Confirm Silence',
      {
        confirmButtonText: 'Create Silence',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    
    // Get unique alert names
    const alertNames = [...new Set(selectedAlerts.value.map(a => a.alertName))]
    
    await alertSilencesApi.create({
      name: `Quick Silence - ${alertNames.join(', ').substring(0, 50)}`,
      silenceType: 'alert_name',
      alertNames,
      startsAt: new Date().toISOString(),
      endsAt: new Date(Date.now() + 2 * 60 * 60 * 1000).toISOString(), // 2 hours
      reason: 'Quick silence from alert list',
      enabled: true
    })
    
    ElMessage.success('Silence created successfully')
    fetchAlerts()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Failed to create silence:', error)
      ElMessage.error('Failed to create silence')
    }
  }
}

// Utility functions
function formatTime(timestamp: string) {
  return new Date(timestamp).toLocaleString()
}

function formatRelativeTime(timestamp: string) {
  const now = new Date()
  const time = new Date(timestamp)
  const diff = Math.floor((now.getTime() - time.getTime()) / 1000)
  
  if (diff < 60) return `${diff}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

// Shortcuts for time range
function setTimeRange(range: string) {
  const now = new Date()
  let start: Date
  
  switch (range) {
    case '1h':
      start = new Date(now.getTime() - 60 * 60 * 1000)
      break
    case '6h':
      start = new Date(now.getTime() - 6 * 60 * 60 * 1000)
      break
    case '24h':
      start = new Date(now.getTime() - 24 * 60 * 60 * 1000)
      break
    case '7d':
      start = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000)
      break
    default:
      return
  }
  
  timeRange.value = [start, now]
  applyFilters()
}

// Watch cluster change
watch(() => clusterStore.currentCluster, () => {
  filters.offset = 0
  fetchAlerts()
})

// Handle pagination
function handlePageChange(page: number) {
  currentPage.value = page
  fetchAlerts()
}

function handleSizeChange(size: number) {
  filters.limit = size
  filters.offset = 0
  fetchAlerts()
}

onMounted(() => {
  fetchAlerts()
})
</script>

<template>
  <div class="alert-events-list">
    <!-- Header -->
    <div class="page-header">
      <h1 class="page-title">
        <el-icon class="title-icon"><Bell /></el-icon>
        Alert Events
      </h1>
    </div>

    <!-- Filters -->
    <el-card class="filter-card" shadow="hover">
      <el-form :inline="true" :model="filters" class="filter-form">
        <el-form-item label="Status">
          <el-select v-model="filters.status" placeholder="All" clearable style="width: 130px">
            <el-option label="All" :value="undefined" />
            <el-option label="Firing" value="firing">
              <el-icon class="option-icon" style="color: #f56c6c"><Alarm /></el-icon>
              Firing
            </el-option>
            <el-option label="Resolved" value="resolved">
              <el-icon class="option-icon" style="color: #67c23a"><CircleCheck /></el-icon>
              Resolved
            </el-option>
            <el-option label="Silenced" value="silenced">
              <el-icon class="option-icon" style="color: #909399"><MuteNotification /></el-icon>
              Silenced
            </el-option>
          </el-select>
        </el-form-item>
        
        <el-form-item label="Severity">
          <el-select v-model="filters.severity" placeholder="All" clearable style="width: 130px">
            <el-option label="All" :value="undefined" />
            <el-option label="Critical" value="critical" />
            <el-option label="High" value="high" />
            <el-option label="Warning" value="warning" />
            <el-option label="Info" value="info" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Source">
          <el-select v-model="filters.source" placeholder="All" clearable style="width: 120px">
            <el-option label="All" :value="undefined" />
            <el-option label="Metric" value="metric" />
            <el-option label="Log" value="log" />
            <el-option label="Trace" value="trace" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Time Range">
          <el-date-picker
            v-model="timeRange"
            type="datetimerange"
            range-separator="to"
            start-placeholder="Start"
            end-placeholder="End"
            format="MM/DD HH:mm"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            style="width: 300px"
          />
        </el-form-item>
        
        <el-form-item label="Search">
          <el-input
            v-model="filters.alertName"
            placeholder="Alert name, workload, pod..."
            clearable
            style="width: 240px"
            @keyup.enter="applyFilters"
          >
            <template #prefix>
              <el-icon><Search /></el-icon>
            </template>
          </el-input>
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="applyFilters">Apply</el-button>
          <el-button @click="resetFilters">Reset</el-button>
        </el-form-item>
      </el-form>
      
      <!-- Quick time filters -->
      <div class="quick-filters">
        <span class="quick-label">Quick:</span>
        <el-button size="small" text @click="setTimeRange('1h')">Last 1h</el-button>
        <el-button size="small" text @click="setTimeRange('6h')">Last 6h</el-button>
        <el-button size="small" text @click="setTimeRange('24h')">Last 24h</el-button>
        <el-button size="small" text @click="setTimeRange('7d')">Last 7d</el-button>
      </div>
    </el-card>

    <!-- Table Actions -->
    <div class="table-actions" v-if="selectedAlerts.length > 0">
      <span class="selection-count">{{ selectedAlerts.length }} selected</span>
      <el-button type="warning" size="small" @click="silenceSelected">
        <el-icon><MuteNotification /></el-icon>
        Silence Selected
      </el-button>
    </div>

    <!-- Alerts Table -->
    <el-card class="table-card" shadow="hover">
      <el-table
        v-loading="loading"
        :data="alerts"
        stripe
        @selection-change="handleSelectionChange"
        @row-click="goToDetail"
        row-class-name="clickable-row"
      >
        <el-table-column type="selection" width="50" />
        
        <el-table-column label="Severity" width="120">
          <template #default="{ row }">
            <AlertSeverityBadge :severity="row.severity" size="small" />
          </template>
        </el-table-column>
        
        <el-table-column label="Alert Name" min-width="250">
          <template #default="{ row }">
            <div class="alert-name-cell">
              <AlertSourceIcon :source="row.source" size="small" />
              <div class="alert-info">
                <span class="alert-name">{{ row.alertName }}</span>
                <span class="alert-summary">{{ row.annotations?.summary || row.annotations?.description }}</span>
              </div>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column label="Status" width="120">
          <template #default="{ row }">
            <AlertStatusTag :status="row.status" size="small" />
          </template>
        </el-table-column>
        
        <el-table-column label="Resource" min-width="200">
          <template #default="{ row }">
            <div class="resource-cell">
              <span class="resource-name">{{ row.podName || row.nodeName || row.workloadName || '-' }}</span>
              <span class="resource-cluster">{{ row.clusterName }}</span>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column label="Started" width="160">
          <template #default="{ row }">
            <el-tooltip :content="formatTime(row.startsAt)" placement="top">
              <span class="time-text">{{ formatRelativeTime(row.startsAt) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        
        <el-table-column label="Duration" width="120">
          <template #default="{ row }">
            <span v-if="row.status === 'firing'" class="duration-firing">
              {{ formatRelativeTime(row.startsAt).replace(' ago', '') }}
            </span>
            <span v-else class="duration-resolved">
              {{ row.endsAt ? formatRelativeTime(row.endsAt).replace(' ago', '') : '-' }}
            </span>
          </template>
        </el-table-column>
        
        <el-table-column label="Actions" width="100" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" text size="small" @click.stop="goToDetail(row)">
              Details
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      
      <!-- Pagination -->
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="filters.limit"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="handlePageChange"
          @size-change="handleSizeChange"
        />
      </div>
    </el-card>
  </div>
</template>

<style lang="scss" scoped>
.alert-events-list {
  padding: 0;
}

.page-header {
  margin-bottom: 24px;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  
  .title-icon {
    color: var(--el-color-warning);
  }
}

.filter-card {
  margin-bottom: 16px;
  border-radius: 12px;
}

.filter-form {
  :deep(.el-form-item) {
    margin-bottom: 12px;
    margin-right: 16px;
  }
}

.option-icon {
  margin-right: 6px;
}

.quick-filters {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.quick-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.table-actions {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
  padding: 12px 16px;
  background: var(--el-color-primary-light-9);
  border-radius: 8px;
}

.selection-count {
  color: var(--el-color-primary);
  font-weight: 500;
}

.table-card {
  border-radius: 12px;
  
  :deep(.el-table) {
    .clickable-row {
      cursor: pointer;
      
      &:hover {
        background-color: var(--el-fill-color-light);
      }
    }
  }
}

.alert-name-cell {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}

.alert-info {
  display: flex;
  flex-direction: column;
}

.alert-name {
  font-weight: 500;
  color: var(--el-text-color-primary);
}

.alert-summary {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 2px;
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.resource-cell {
  display: flex;
  flex-direction: column;
}

.resource-name {
  font-weight: 500;
}

.resource-cluster {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.time-text {
  color: var(--el-text-color-regular);
}

.duration-firing {
  color: var(--el-color-danger);
}

.duration-resolved {
  color: var(--el-text-color-secondary);
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>

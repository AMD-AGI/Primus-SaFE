<template>
  <div class="detection-status">
    <!-- Page Header -->
    <div class="page-header">
      <h1 class="page-title">Detection Status</h1>
      <div class="header-actions">
        <el-select
          v-model="filterForm.status"
          placeholder="Status"
          clearable
          size="default"
          style="width: 140px"
          @change="handleFilter"
        >
          <el-option
            v-for="item in statusOptions"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
        <el-select
          v-model="filterForm.state"
          placeholder="State"
          clearable
          size="default"
          style="width: 140px"
          @change="handleFilter"
        >
          <el-option
            v-for="item in stateOptions"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
        <el-button @click="handleRefresh" :loading="loading">
          <i i="ep-refresh" class="mr-1" />
          Refresh
        </el-button>
      </div>
    </div>

    <!-- Summary Cards -->
    <div class="summary-cards" v-loading="summaryLoading">
      <div class="summary-card">
        <div class="summary-value">{{ summary?.totalWorkloads ?? 0 }}</div>
        <div class="summary-label">Total Workloads</div>
      </div>
      <div class="summary-card confirmed">
        <div class="summary-value">{{ summary?.statusCounts?.confirmed ?? 0 }}</div>
        <div class="summary-label">Confirmed</div>
      </div>
      <div class="summary-card suspected">
        <div class="summary-value">{{ summary?.statusCounts?.suspected ?? 0 }}</div>
        <div class="summary-label">Suspected</div>
      </div>
      <div class="summary-card verified">
        <div class="summary-value">{{ summary?.statusCounts?.verified ?? 0 }}</div>
        <div class="summary-label">Verified</div>
      </div>
      <div class="summary-card conflict">
        <div class="summary-value">{{ summary?.statusCounts?.conflict ?? 0 }}</div>
        <div class="summary-label">Conflict</div>
      </div>
      <div class="summary-card unknown">
        <div class="summary-value">{{ summary?.statusCounts?.unknown ?? 0 }}</div>
        <div class="summary-label">Unknown</div>
      </div>
    </div>

    <!-- Data Table -->
    <el-card class="table-card">
      <el-table
        :data="tableData"
        v-loading="loading"
        style="width: 100%"
        stripe
        @row-click="handleRowClick"
        row-class-name="clickable-row"
      >
        <el-table-column prop="workloadUid" label="Workload UID" min-width="200" fixed>
          <template #default="{ row }">
            <el-tooltip :content="row.workloadUid" placement="top">
              <el-link type="primary" @click.stop="handleViewDetail(row)">
                <span class="text-ellipsis">{{ row.workloadUid }}</span>
              </el-link>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="Status" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusTagType(row.status)" size="default">
              {{ getStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="detectionState" label="State" width="130">
          <template #default="{ row }">
            <el-tag :type="getStateTagType(row.detectionState)" size="default" effect="plain">
              {{ getStateLabel(row.detectionState) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="framework" label="Framework" width="140">
          <template #default="{ row }">
            <div class="framework-cell">
              <span class="framework-name">{{ row.framework || '-' }}</span>
              <el-tooltip v-if="row.frameworks?.length > 1" :content="row.frameworks.join(', ')">
                <el-tag size="small" effect="plain" class="ml-1">+{{ row.frameworks.length - 1 }}</el-tag>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="workloadType" label="Type" width="100">
          <template #default="{ row }">
            <el-tag :type="row.workloadType === 'training' ? 'warning' : 'info'" size="small">
              {{ row.workloadType || '-' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="confidence" label="Confidence" width="120">
          <template #default="{ row }">
            <el-progress
              :percentage="Math.round((row.confidence || 0) * 100)"
              :color="getConfidenceColor(row.confidence)"
              :stroke-width="8"
            />
          </template>
        </el-table-column>
        <el-table-column prop="evidenceCount" label="Evidence" width="100" align="center">
          <template #default="{ row }">
            <el-badge :value="row.evidenceCount" :max="99" type="primary" />
          </template>
        </el-table-column>
        <el-table-column prop="attemptCount" label="Attempts" width="100" align="center">
          <template #default="{ row }">
            {{ row.attemptCount }}/{{ row.maxAttempts }}
          </template>
        </el-table-column>
        <el-table-column prop="updatedAt" label="Updated At" width="180">
          <template #default="{ row }">
            {{ formatTime(row.updatedAt) }}
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="100" fixed="right">
          <template #default="{ row }">
            <el-button
              type="primary"
              size="small"
              link
              @click.stop="handleTriggerDetection(row)"
              :loading="triggeringWorkloads.has(row.workloadUid)"
            >
              Trigger
            </el-button>
          </template>
        </el-table-column>

        <template #empty>
          <el-empty description="No Data" />
        </template>
      </el-table>

      <!-- Pagination -->
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @current-change="handlePageChange"
        @size-change="handleSizeChange"
        class="mt-5"
      />
    </el-card>

    <!-- Detail Drawer -->
    <el-drawer
      v-model="detailDrawerVisible"
      title="Detection Status Details"
      size="60%"
      :close-on-click-modal="true"
    >
      <div v-if="currentDetail" class="detail-content">
        <!-- Basic Info -->
        <div class="detail-section">
          <div class="section-title">
            <i i="ep-info-filled" class="mr-2" />
            Basic Information
          </div>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="Workload UID" :span="2">
              <code>{{ currentDetail.workloadUid }}</code>
            </el-descriptions-item>
            <el-descriptions-item label="Status">
              <el-tag :type="getStatusTagType(currentDetail.status)">
                {{ getStatusLabel(currentDetail.status) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Detection State">
              <el-tag :type="getStateTagType(currentDetail.detectionState)" effect="plain">
                {{ getStateLabel(currentDetail.detectionState) }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Framework">
              {{ currentDetail.framework || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="All Frameworks">
              <el-tag
                v-for="fw in currentDetail.frameworks"
                :key="fw"
                size="small"
                class="mr-1"
              >
                {{ fw }}
              </el-tag>
              <span v-if="!currentDetail.frameworks?.length">-</span>
            </el-descriptions-item>
            <el-descriptions-item label="Workload Type">
              <el-tag :type="currentDetail.workloadType === 'training' ? 'warning' : 'info'" size="small">
                {{ currentDetail.workloadType || '-' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Confidence">
              <el-progress
                :percentage="Math.round((currentDetail.confidence || 0) * 100)"
                :color="getConfidenceColor(currentDetail.confidence)"
                :stroke-width="10"
                style="width: 120px"
              />
            </el-descriptions-item>
            <el-descriptions-item label="Framework Layer">
              {{ currentDetail.frameworkLayer || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Wrapper Framework">
              {{ currentDetail.wrapperFramework || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Base Framework">
              {{ currentDetail.baseFramework || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Has Conflicts">
              <el-tag :type="currentDetail.hasConflicts ? 'danger' : 'success'" size="small">
                {{ currentDetail.hasConflicts ? 'Yes' : 'No' }}
              </el-tag>
            </el-descriptions-item>
          </el-descriptions>
        </div>

        <!-- Detection Progress -->
        <div class="detail-section">
          <div class="section-title">
            <i i="ep-data-analysis" class="mr-2" />
            Detection Progress
          </div>
          <el-descriptions :column="2" border>
            <el-descriptions-item label="Attempts">
              {{ currentDetail.attemptCount }}/{{ currentDetail.maxAttempts }}
            </el-descriptions-item>
            <el-descriptions-item label="Evidence Count">
              {{ currentDetail.evidenceCount }}
            </el-descriptions-item>
            <el-descriptions-item label="Evidence Sources">
              <el-tag
                v-for="src in currentDetail.evidenceSources"
                :key="src"
                size="small"
                effect="plain"
                class="mr-1"
              >
                {{ src }}
              </el-tag>
              <span v-if="!currentDetail.evidenceSources?.length">-</span>
            </el-descriptions-item>
            <el-descriptions-item label="Confirmed At">
              {{ currentDetail.confirmedAt ? formatTime(currentDetail.confirmedAt) : '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Last Attempt">
              {{ currentDetail.lastAttemptAt ? formatTime(currentDetail.lastAttemptAt) : '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Next Attempt">
              {{ currentDetail.nextAttemptAt ? formatTime(currentDetail.nextAttemptAt) : '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Created At">
              {{ formatTime(currentDetail.createdAt) }}
            </el-descriptions-item>
            <el-descriptions-item label="Updated At">
              {{ formatTime(currentDetail.updatedAt) }}
            </el-descriptions-item>
          </el-descriptions>
        </div>

        <!-- Coverage -->
        <div class="detail-section" v-if="currentDetail.coverage?.length">
          <div class="section-title">
            <i i="ep-pie-chart" class="mr-2" />
            Coverage Status
          </div>
          <el-table :data="currentDetail.coverage" stripe border>
            <el-table-column prop="source" label="Source" width="120">
              <template #default="{ row }">
                <el-tag effect="plain" size="small">{{ row.source }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="status" label="Status" width="120">
              <template #default="{ row }">
                <el-tag :type="getCoverageStatusType(row.status)" size="small">
                  {{ row.status }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="attemptCount" label="Attempts" width="100" align="center" />
            <el-table-column prop="evidenceCount" label="Evidence" width="100" align="center" />
            <el-table-column prop="lastSuccessAt" label="Last Success" min-width="160">
              <template #default="{ row }">
                {{ row.lastSuccessAt ? formatTime(row.lastSuccessAt) : '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="hasGap" label="Has Gap" width="100" align="center">
              <template #default="{ row }">
                <el-tag v-if="row.hasGap !== undefined" :type="row.hasGap ? 'warning' : 'success'" size="small">
                  {{ row.hasGap ? 'Yes' : 'No' }}
                </el-tag>
                <span v-else>-</span>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <!-- Tasks -->
        <div class="detail-section" v-if="currentDetail.tasks?.length">
          <div class="section-title">
            <i i="ep-list" class="mr-2" />
            Related Tasks
          </div>
          <el-table :data="currentDetail.tasks" stripe border>
            <el-table-column prop="taskType" label="Task Type" min-width="180">
              <template #default="{ row }">
                <code>{{ row.taskType }}</code>
              </template>
            </el-table-column>
            <el-table-column prop="status" label="Status" width="120">
              <template #default="{ row }">
                <el-tag :type="getTaskStatusType(row.status)" size="small">
                  {{ row.status }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="attemptCount" label="Attempts" width="100" align="center">
              <template #default="{ row }">
                {{ row.attemptCount ?? '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="coordinatorState" label="Coordinator State" width="150">
              <template #default="{ row }">
                {{ row.coordinatorState || '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="updatedAt" label="Updated At" width="160">
              <template #default="{ row }">
                {{ formatTime(row.updatedAt) }}
              </template>
            </el-table-column>
          </el-table>
        </div>

        <!-- Evidence Section -->
        <div class="detail-section">
          <div class="section-title">
            <i i="ep-document" class="mr-2" />
            Evidence Records
            <el-button size="small" class="ml-4" @click="loadEvidence" :loading="evidenceLoading">
              Load Evidence
            </el-button>
          </div>
          <div v-if="evidenceData.length" class="evidence-list">
            <el-collapse accordion>
              <el-collapse-item
                v-for="ev in evidenceData"
                :key="ev.id"
                :name="ev.id"
              >
                <template #title>
                  <div class="evidence-header">
                    <el-tag :type="ev.sourceType === 'active' ? 'primary' : 'info'" size="small">
                      {{ ev.source }}
                    </el-tag>
                    <span class="evidence-framework">{{ ev.framework }}</span>
                    <el-progress
                      :percentage="Math.round(ev.confidence * 100)"
                      :color="getConfidenceColor(ev.confidence)"
                      :stroke-width="6"
                      style="width: 80px"
                      class="ml-3"
                    />
                    <span class="evidence-time">{{ formatTime(ev.detectedAt) }}</span>
                  </div>
                </template>
                <div class="evidence-body">
                  <el-descriptions :column="2" border size="small">
                    <el-descriptions-item label="Source Type">{{ ev.sourceType }}</el-descriptions-item>
                    <el-descriptions-item label="Framework Layer">{{ ev.frameworkLayer }}</el-descriptions-item>
                    <el-descriptions-item label="Wrapper Framework">{{ ev.wrapperFramework || '-' }}</el-descriptions-item>
                    <el-descriptions-item label="Base Framework">{{ ev.baseFramework || '-' }}</el-descriptions-item>
                    <el-descriptions-item label="Workload Type">{{ ev.workloadType }}</el-descriptions-item>
                    <el-descriptions-item label="Detected At">{{ formatTime(ev.detectedAt) }}</el-descriptions-item>
                  </el-descriptions>
                  <div class="evidence-data mt-3">
                    <div class="text-sm text-gray-500 mb-1">Evidence Data:</div>
                    <el-input
                      type="textarea"
                      :model-value="JSON.stringify(ev.evidence, null, 2)"
                      :rows="6"
                      readonly
                    />
                  </div>
                </div>
              </el-collapse-item>
            </el-collapse>
          </div>
          <el-empty v-else-if="evidenceLoaded" description="No evidence records" :image-size="60" />
        </div>
      </div>

      <template #footer>
        <el-button @click="detailDrawerVisible = false">Close</el-button>
        <el-button
          type="primary"
          @click="handleTriggerDetection(currentDetail!)"
          :loading="triggeringWorkloads.has(currentDetail?.workloadUid || '')"
          :disabled="!currentDetail"
        >
          Trigger Detection
        </el-button>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  getDetectionStatusList,
  getDetectionStatus,
  getDetectionSummary,
  getDetectionEvidence,
  triggerDetection,
  type DetectionStatus,
  type DetectionSummary,
  type DetectionEvidence,
  type DetectionStatusValue,
  type DetectionState
} from '@/services/detection-status'
import { ElMessage } from 'element-plus'
import dayjs from 'dayjs'

const route = useRoute()
const router = useRouter()

// Summary data
const summary = ref<DetectionSummary | null>(null)
const summaryLoading = ref(false)

// Table data
const tableData = ref<DetectionStatus[]>([])
const loading = ref(false)

// Pagination
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0
})

// Filter form
const filterForm = reactive({
  status: '' as DetectionStatusValue | '',
  state: '' as DetectionState | ''
})

// Status options
const statusOptions = [
  { value: 'unknown', label: 'Unknown' },
  { value: 'suspected', label: 'Suspected' },
  { value: 'confirmed', label: 'Confirmed' },
  { value: 'verified', label: 'Verified' },
  { value: 'conflict', label: 'Conflict' }
]

// State options
const stateOptions = [
  { value: 'pending', label: 'Pending' },
  { value: 'in_progress', label: 'In Progress' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' }
]

// Detail drawer
const detailDrawerVisible = ref(false)
const currentDetail = ref<DetectionStatus | null>(null)

// Evidence data
const evidenceData = ref<DetectionEvidence[]>([])
const evidenceLoading = ref(false)
const evidenceLoaded = ref(false)

// Triggering state
const triggeringWorkloads = ref(new Set<string>())

// Fetch summary
const fetchSummary = async () => {
  summaryLoading.value = true
  try {
    summary.value = await getDetectionSummary()
  } catch (error: any) {
    console.error('Failed to fetch summary:', error)
  } finally {
    summaryLoading.value = false
  }
}

// Fetch table data
const fetchData = async () => {
  loading.value = true
  try {
    const params: any = {
      page: pagination.page,
      pageSize: pagination.pageSize
    }
    if (filterForm.status) params.status = filterForm.status
    if (filterForm.state) params.state = filterForm.state

    const res = await getDetectionStatusList(params)
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
  pagination.page = 1
  fetchData()
}

// Handle refresh
const handleRefresh = () => {
  fetchSummary()
  fetchData()
}

// Handle page change
const handlePageChange = (page: number) => {
  pagination.page = page
  fetchData()
}

// Handle size change
const handleSizeChange = (size: number) => {
  pagination.pageSize = size
  pagination.page = 1
  fetchData()
}

// Handle row click
const handleRowClick = (row: DetectionStatus) => {
  handleViewDetail(row)
}

// View detail
const handleViewDetail = async (row: DetectionStatus) => {
  try {
    currentDetail.value = await getDetectionStatus(row.workloadUid)
    evidenceData.value = []
    evidenceLoaded.value = false
    detailDrawerVisible.value = true
  } catch (error: any) {
    ElMessage.error(error || 'Failed to fetch details')
  }
}

// Load evidence
const loadEvidence = async () => {
  if (!currentDetail.value) return
  evidenceLoading.value = true
  try {
    const res = await getDetectionEvidence(currentDetail.value.workloadUid)
    evidenceData.value = res.evidence || []
    evidenceLoaded.value = true
  } catch (error: any) {
    ElMessage.error(error || 'Failed to load evidence')
  } finally {
    evidenceLoading.value = false
  }
}

// Trigger detection
const handleTriggerDetection = async (row: DetectionStatus) => {
  if (!row?.workloadUid) return
  triggeringWorkloads.value.add(row.workloadUid)
  try {
    await triggerDetection(row.workloadUid)
    ElMessage.success('Detection triggered successfully')
    // Refresh data after trigger
    await fetchData()
    if (currentDetail.value?.workloadUid === row.workloadUid) {
      currentDetail.value = await getDetectionStatus(row.workloadUid)
    }
  } catch (error: any) {
    ElMessage.error(error || 'Failed to trigger detection')
  } finally {
    triggeringWorkloads.value.delete(row.workloadUid)
  }
}

// Get status tag type
const getStatusTagType = (status: DetectionStatusValue) => {
  const map: Record<DetectionStatusValue, any> = {
    unknown: 'info',
    suspected: 'warning',
    confirmed: 'success',
    verified: '',
    conflict: 'danger'
  }
  return map[status] || 'info'
}

// Get status label
const getStatusLabel = (status: DetectionStatusValue) => {
  const map: Record<DetectionStatusValue, string> = {
    unknown: 'Unknown',
    suspected: 'Suspected',
    confirmed: 'Confirmed',
    verified: 'Verified',
    conflict: 'Conflict'
  }
  return map[status] || status
}

// Get state tag type
const getStateTagType = (state: DetectionState) => {
  const map: Record<DetectionState, any> = {
    pending: 'info',
    in_progress: 'warning',
    completed: 'success',
    failed: 'danger'
  }
  return map[state] || 'info'
}

// Get state label
const getStateLabel = (state: DetectionState) => {
  const map: Record<DetectionState, string> = {
    pending: 'Pending',
    in_progress: 'In Progress',
    completed: 'Completed',
    failed: 'Failed'
  }
  return map[state] || state
}

// Get coverage status type
const getCoverageStatusType = (status: string) => {
  const map: Record<string, any> = {
    pending: 'info',
    collecting: 'warning',
    collected: 'success',
    failed: 'danger',
    not_applicable: ''
  }
  return map[status] || 'info'
}

// Get task status type
const getTaskStatusType = (status: string) => {
  const map: Record<string, any> = {
    pending: 'info',
    running: 'warning',
    completed: 'success',
    failed: 'danger'
  }
  return map[status] || 'info'
}

// Get confidence color
const getConfidenceColor = (confidence: number) => {
  if (confidence >= 0.8) return '#67c23a'
  if (confidence >= 0.5) return '#e6a23c'
  return '#f56c6c'
}

// Format time
const formatTime = (time: string) => {
  if (!time) return '-'
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

// Initialize
onMounted(() => {
  fetchSummary()
  fetchData()
})
</script>

<style scoped lang="scss">
.detection-status {
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

  .summary-cards {
    display: grid;
    grid-template-columns: repeat(6, 1fr);
    gap: 16px;
    margin-bottom: 20px;

    @media (max-width: 1400px) {
      grid-template-columns: repeat(3, 1fr);
    }

    @media (max-width: 768px) {
      grid-template-columns: repeat(2, 1fr);
    }

    .summary-card {
      background: var(--el-bg-color-overlay);
      border-radius: 12px;
      padding: 20px;
      text-align: center;
      border: 1px solid var(--el-border-color-light);
      transition: all 0.3s;

      &:hover {
        transform: translateY(-2px);
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
      }

      .summary-value {
        font-size: 28px;
        font-weight: 700;
        color: var(--el-text-color-primary);

        @media (min-width: 1920px) {
          font-size: 32px;
        }
      }

      .summary-label {
        font-size: 13px;
        color: var(--el-text-color-secondary);
        margin-top: 4px;
      }

      &.confirmed {
        border-color: var(--el-color-success-light-5);
        .summary-value { color: var(--el-color-success); }
      }

      &.suspected {
        border-color: var(--el-color-warning-light-5);
        .summary-value { color: var(--el-color-warning); }
      }

      &.verified {
        border-color: var(--el-color-primary-light-5);
        .summary-value { color: var(--el-color-primary); }
      }

      &.conflict {
        border-color: var(--el-color-danger-light-5);
        .summary-value { color: var(--el-color-danger); }
      }

      &.unknown {
        border-color: var(--el-border-color);
        .summary-value { color: var(--el-text-color-secondary); }
      }
    }
  }

  .table-card {
    border-radius: 15px;

    :deep(.el-table) {
      font-size: 14px;

      @media (min-width: 1920px) {
        font-size: 15px;
      }

      .clickable-row {
        cursor: pointer;

        &:hover {
          background-color: var(--el-fill-color-light) !important;
        }
      }

      th {
        font-size: 14px;
        font-weight: 600;

        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }

      td, th {
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

    .framework-cell {
      display: flex;
      align-items: center;

      .framework-name {
        font-weight: 500;
      }
    }
  }

  .text-ellipsis {
    display: inline-block;
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .el-pagination {
    justify-content: center;
  }
}

.detail-content {
  padding: 0 10px;

  .detail-section {
    margin-bottom: 28px;

    .section-title {
      font-size: 16px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin-bottom: 16px;
      display: flex;
      align-items: center;

      i {
        color: var(--el-color-primary);
      }
    }

    :deep(.el-descriptions) {
      .el-descriptions__label {
        font-weight: 600;
        width: 140px;
      }
    }

    code {
      background: var(--el-fill-color-light);
      padding: 2px 8px;
      border-radius: 4px;
      font-family: 'JetBrains Mono', monospace;
      font-size: 13px;
    }
  }

  .evidence-list {
    .evidence-header {
      display: flex;
      align-items: center;
      gap: 12px;
      width: 100%;
      padding-right: 20px;

      .evidence-framework {
        font-weight: 500;
        min-width: 80px;
      }

      .evidence-time {
        margin-left: auto;
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }

    .evidence-body {
      padding: 16px 0;

      .evidence-data {
        :deep(.el-textarea__inner) {
          font-family: 'JetBrains Mono', monospace;
          font-size: 12px;
        }
      }
    }
  }
}
</style>


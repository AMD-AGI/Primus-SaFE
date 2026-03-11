<template>
  <div class="config-detail-page" v-loading="loading">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <el-button link :icon="ArrowLeft" @click="goBack">Back to Configs</el-button>
      </div>
    </div>

    <template v-if="config">
      <!-- Config Info -->
      <div class="config-info">
        <div class="info-header">
          <div class="info-title">
            <h2>{{ config.name }}</h2>
            <el-tag :type="config.enabled ? 'success' : 'info'" size="small">
              {{ config.enabled ? 'Enabled' : 'Disabled' }}
            </el-tag>
          </div>
          <div class="info-actions">
            <el-button :icon="Edit" @click="editConfig">Edit</el-button>
            <el-button :icon="Refresh" @click="triggerBackfillDialog">Backfill</el-button>
          </div>
        </div>
        <p class="info-description" v-if="config.description">{{ config.description }}</p>
      </div>

      <!-- Stats Cards -->
      <div class="stats-grid">
        <el-card class="stat-card" shadow="hover">
          <div class="stat-content">
            <div class="stat-icon" style="background: rgba(64, 158, 255, 0.1); color: #409eff;">
              <el-icon :size="24"><Link /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-label">Repository</div>
              <div class="stat-value">{{ config.githubOwner }}/{{ config.githubRepo }}</div>
            </div>
          </div>
        </el-card>

        <el-card class="stat-card" shadow="hover">
          <div class="stat-content">
            <div class="stat-icon" style="background: rgba(103, 194, 58, 0.1); color: #67c23a;">
              <el-icon :size="24"><Monitor /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-label">Runner Set</div>
              <div class="stat-value">{{ config.runnerSetName }}</div>
            </div>
          </div>
        </el-card>

        <el-card class="stat-card" shadow="hover">
          <div class="stat-content">
            <div class="stat-icon" style="background: rgba(230, 162, 60, 0.1); color: #e6a23c;">
              <el-icon :size="24"><List /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-label">Total Runs</div>
              <div class="stat-value">{{ stats?.completedRuns || 0 }}</div>
            </div>
          </div>
        </el-card>

        <el-card class="stat-card" shadow="hover">
          <div class="stat-content">
            <div class="stat-icon" style="background: rgba(144, 147, 153, 0.1); color: #909399;">
              <el-icon :size="24"><Histogram /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-label">Total Metrics</div>
              <div class="stat-value">{{ formatNumber(stats?.totalMetrics || 0) }}</div>
            </div>
          </div>
        </el-card>
      </div>

      <!-- Schema Section -->
      <el-card class="section-card">
        <template #header>
          <div class="section-header">
            <span class="section-title">Schema Management</span>
            <el-button :icon="Refresh" :loading="regenerating" @click="regenerateSchema">
              Regenerate Schema
            </el-button>
          </div>
        </template>

        <div v-if="activeSchema" class="schema-content">
          <div class="schema-meta">
            <span class="schema-name">{{ activeSchema.name }}</span>
            <el-tag size="small" type="success">v{{ activeSchema.version }}</el-tag>
            <el-tag size="small" :type="activeSchema.generatedBy === 'ai' ? 'warning' : 'info'">
              {{ activeSchema.generatedBy === 'ai' ? 'AI Generated' : 'User Defined' }}
            </el-tag>
          </div>

          <el-divider content-position="left">Dimension Fields</el-divider>
          <div class="fields-list">
            <el-tag v-for="field in activeSchema.dimensionFields" :key="field" class="field-tag">
              {{ field }}
            </el-tag>
            <span v-if="!activeSchema.dimensionFields?.length" class="no-data">No dimension fields</span>
          </div>

          <el-divider content-position="left">Metric Fields</el-divider>
          <div class="fields-list">
            <el-tag v-for="field in activeSchema.metricFields" :key="field" type="success" class="field-tag">
              {{ field }}
            </el-tag>
            <span v-if="!activeSchema.metricFields?.length" class="no-data">No metric fields</span>
          </div>

          <el-divider content-position="left">All Fields</el-divider>
          <el-table :data="activeSchema.fields" size="small" border>
            <el-table-column prop="name" label="Name" width="200" />
            <el-table-column prop="type" label="Type" width="100" />
            <el-table-column prop="unit" label="Unit" width="100">
              <template #default="{ row }">{{ row.unit || '-' }}</template>
            </el-table-column>
            <el-table-column prop="description" label="Description" />
          </el-table>
        </div>

        <el-empty v-else description="No active schema. Generate one using AI." />
      </el-card>

      <!-- Recent Runs Section -->
      <el-card class="section-card">
        <template #header>
          <div class="section-header">
            <span class="section-title">Recent Runs</span>
            <el-button link type="primary" @click="goToRuns">View All Runs</el-button>
          </div>
        </template>

        <el-table :data="recentRuns" size="small">
          <el-table-column prop="id" label="ID" width="80" />
          <el-table-column prop="workloadName" label="Workload" min-width="200">
            <template #default="{ row }">
              <el-tooltip :content="row.workloadName" placement="top">
                <span class="workload-name">{{ row.workloadName }}</span>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column prop="filesProcessed" label="Files" width="80" align="center" />
          <el-table-column prop="metricsCount" label="Metrics" width="100" align="center" />
          <el-table-column prop="status" label="Status" width="120" align="center">
            <template #default="{ row }">
              <el-tag :type="getStatusType(row.status)" size="small">
                {{ row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="createdAt" label="Created" width="180">
            <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <!-- Backfill Dialog -->
    <el-dialog v-model="showBackfillDialog" title="Trigger Backfill" width="500px">
      <el-form :model="backfillForm" label-width="100px">
        <el-form-item label="Time Range">
          <el-date-picker
            v-model="backfillForm.timeRange"
            type="datetimerange"
            range-separator="to"
            start-placeholder="Start"
            end-placeholder="End"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            style="width: 100%"
          />
        </el-form-item>
        <el-form-item label="Dry Run">
          <el-switch v-model="backfillForm.dryRun" />
          <span class="form-tip">Preview without actually processing</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showBackfillDialog = false">Cancel</el-button>
        <el-button type="primary" :loading="backfilling" @click="submitBackfill">
          Start Backfill
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Edit, Refresh, Link, Monitor, List, Histogram } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  getConfig,
  getConfigStats,
  getSchemasByConfig,
  getRunsByConfig,
  regenerateSchema as apiRegenerateSchema,
  triggerBackfill,
  type WorkflowConfig,
  type ConfigStats,
  type WorkflowSchema,
  type WorkflowRun
} from '@/services/workflow-metrics'

const route = useRoute()
const router = useRouter()

const configId = computed(() => Number(route.params.id))

// State
const loading = ref(false)
const config = ref<WorkflowConfig | null>(null)
const stats = ref<ConfigStats | null>(null)
const activeSchema = ref<WorkflowSchema | null>(null)
const recentRuns = ref<WorkflowRun[]>([])
const regenerating = ref(false)

// Backfill
const showBackfillDialog = ref(false)
const backfilling = ref(false)
const backfillForm = reactive({
  timeRange: [] as string[],
  dryRun: false
})

// Methods
const fetchData = async () => {
  loading.value = true
  try {
    const [configRes, statsRes, schemasRes, runsRes] = await Promise.all([
      getConfig(configId.value),
      getConfigStats(configId.value),
      getSchemasByConfig(configId.value),
      getRunsByConfig(configId.value, { limit: 5 })
    ])
    
    config.value = configRes
    stats.value = statsRes
    activeSchema.value = schemasRes.schemas?.find(s => s.isActive) || null
    recentRuns.value = runsRes.runs || []
  } catch (error) {
    console.error('Failed to fetch config:', error)
    ElMessage.error('Failed to load configuration')
  } finally {
    loading.value = false
  }
}

const goBack = () => {
  router.push('/workflow-metrics/configs')
}

const editConfig = () => {
  // TODO: Open edit dialog
  ElMessage.info('Edit functionality coming soon')
}

const goToRuns = () => {
  router.push(`/workflow-metrics/runs?configId=${configId.value}`)
}

const regenerateSchema = async () => {
  regenerating.value = true
  try {
    const res = await apiRegenerateSchema(configId.value)
    ElMessage.success(`Schema generated: ${res.name} v${res.version}`)
    fetchData()
  } catch (error) {
    console.error('Failed to regenerate schema:', error)
    ElMessage.error('Failed to regenerate schema')
  } finally {
    regenerating.value = false
  }
}

const triggerBackfillDialog = () => {
  const now = dayjs()
  backfillForm.timeRange = [
    now.subtract(7, 'day').format('YYYY-MM-DDTHH:mm:ssZ'),
    now.format('YYYY-MM-DDTHH:mm:ssZ')
  ]
  backfillForm.dryRun = false
  showBackfillDialog.value = true
}

const submitBackfill = async () => {
  if (!backfillForm.timeRange?.length) {
    ElMessage.warning('Please select time range')
    return
  }
  
  backfilling.value = true
  try {
    const res = await triggerBackfill(configId.value, {
      startTime: backfillForm.timeRange[0],
      endTime: backfillForm.timeRange[1],
      dryRun: backfillForm.dryRun
    })
    ElMessage.success(`Backfill task created: ${res.taskId}`)
    showBackfillDialog.value = false
  } catch (error) {
    console.error('Failed to trigger backfill:', error)
    ElMessage.error('Failed to trigger backfill')
  } finally {
    backfilling.value = false
  }
}

const formatDate = (date: string) => {
  return dayjs(date).format('YYYY-MM-DD HH:mm:ss')
}

const formatNumber = (num: number) => {
  return num.toLocaleString()
}

const getStatusType = (status: string) => {
  const types: Record<string, string> = {
    pending: 'info',
    collecting: 'primary',
    extracting: 'warning',
    completed: 'success',
    failed: 'danger'
  }
  return types[status] || 'info'
}

onMounted(() => {
  fetchData()
})
</script>

<style scoped lang="scss">
.config-detail-page {
  .page-header {
    margin-bottom: 20px;
  }

  .config-info {
    margin-bottom: 24px;

    .info-header {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      margin-bottom: 8px;

      .info-title {
        display: flex;
        align-items: center;
        gap: 12px;

        h2 {
          margin: 0;
          font-size: 24px;
          font-weight: 600;
        }
      }

      .info-actions {
        display: flex;
        gap: 8px;
      }
    }

    .info-description {
      color: var(--el-text-color-secondary);
      margin: 0;
    }
  }

  .stats-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 16px;
    margin-bottom: 24px;

    @media (max-width: 1200px) {
      grid-template-columns: repeat(2, 1fr);
    }

    .stat-card {
      border-radius: 12px;

      .stat-content {
        display: flex;
        align-items: center;
        gap: 16px;

        .stat-icon {
          width: 48px;
          height: 48px;
          border-radius: 12px;
          display: flex;
          align-items: center;
          justify-content: center;
        }

        .stat-info {
          .stat-label {
            font-size: 13px;
            color: var(--el-text-color-secondary);
            margin-bottom: 4px;
          }

          .stat-value {
            font-size: 16px;
            font-weight: 600;
            color: var(--el-text-color-primary);
          }
        }
      }
    }
  }

  .section-card {
    margin-bottom: 24px;
    border-radius: 12px;

    .section-header {
      display: flex;
      justify-content: space-between;
      align-items: center;

      .section-title {
        font-size: 16px;
        font-weight: 600;
      }
    }

    .schema-content {
      .schema-meta {
        display: flex;
        align-items: center;
        gap: 12px;
        margin-bottom: 16px;

        .schema-name {
          font-size: 15px;
          font-weight: 500;
        }
      }

      .fields-list {
        display: flex;
        flex-wrap: wrap;
        gap: 8px;

        .field-tag {
          font-family: monospace;
        }

        .no-data {
          color: var(--el-text-color-secondary);
          font-size: 13px;
        }
      }
    }

    .workload-name {
      display: inline-block;
      max-width: 180px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .form-tip {
    margin-left: 12px;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }
}
</style>


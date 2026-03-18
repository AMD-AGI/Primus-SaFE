<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  alertAdvicesApi,
  type AlertRuleAdvice,
  type AdviceSummary,
  type ListAdvicesParams
} from '@/services/alerts'
import AlertSeverityBadge from './components/AlertSeverityBadge.vue'
import { useClusterStore } from '@/stores/cluster'

const router = useRouter()
const clusterStore = useClusterStore()

// State
const loading = ref(false)
const advices = ref<AlertRuleAdvice[]>([])
const total = ref(0)
const summary = ref<AdviceSummary>({
  pending: 0,
  accepted: 0,
  rejected: 0,
  avgConfidence: 0
})
const refreshing = ref(false)

// Filters
const filters = reactive<ListAdvicesParams>({
  status: 'pending',
  category: undefined,
  minPriority: undefined,
  offset: 0,
  limit: 20
})

// Pagination
const currentPage = computed({
  get: () => Math.floor((filters.offset || 0) / (filters.limit || 20)) + 1,
  set: (val: number) => {
    filters.offset = (val - 1) * (filters.limit || 20)
  }
})

// Fetch data
async function fetchAdvices() {
  loading.value = true
  try {
    const params: ListAdvicesParams = {
      ...filters,
      cluster: clusterStore.currentCluster || undefined
    }
    
    const [advicesRes, summaryRes] = await Promise.all([
      alertAdvicesApi.list(params),
      alertAdvicesApi.getSummary({ cluster: clusterStore.currentCluster || undefined })
    ])
    
    advices.value = advicesRes?.data || []
    total.value = advicesRes?.total || 0
    summary.value = summaryRes || summary.value
  } catch (error) {
    console.error('Failed to fetch advices:', error)
    ElMessage.error('Failed to fetch advices')
  } finally {
    loading.value = false
  }
}

// Actions
async function acceptAdvice(advice: AlertRuleAdvice) {
  try {
    await ElMessageBox.confirm(
      `Accept and create rule "${advice.suggestedRule.name}"?`,
      'Accept Advice',
      {
        confirmButtonText: 'Accept & Create',
        cancelButtonText: 'Cancel',
        type: 'info'
      }
    )
    
    await alertAdvicesApi.apply(advice.id)
    ElMessage.success('Rule created from advice')
    fetchAdvices()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to apply advice')
    }
  }
}

async function rejectAdvice(advice: AlertRuleAdvice) {
  try {
    const { value: reason } = await ElMessageBox.prompt(
      'Enter reason for rejection (optional):',
      'Reject Advice',
      {
        confirmButtonText: 'Reject',
        cancelButtonText: 'Cancel',
        inputPlaceholder: 'Reason...'
      }
    )
    
    await alertAdvicesApi.updateStatus(advice.id, 'rejected', reason)
    ElMessage.success('Advice rejected')
    fetchAdvices()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to reject advice')
    }
  }
}

function customizeAdvice(advice: AlertRuleAdvice) {
  // Navigate to log rules page with pre-filled form
  router.push({
    path: '/alerts/rules/log',
    query: { 
      fromAdvice: advice.id.toString(),
      name: advice.suggestedRule.name,
      pattern: (advice.suggestedRule.matchConfig as any)?.pattern
    }
  })
}

async function refreshAdvices() {
  refreshing.value = true
  try {
    await alertAdvicesApi.refresh({
      cluster: clusterStore.currentCluster || undefined
    })
    ElMessage.success('AI analysis triggered')
    // Wait a bit then refresh
    setTimeout(() => fetchAdvices(), 3000)
  } catch (error) {
    ElMessage.error('Failed to trigger AI analysis')
  } finally {
    refreshing.value = false
  }
}

// Utility
function getConfidenceColor(confidence: number) {
  if (confidence >= 0.9) return '#67c23a'
  if (confidence >= 0.7) return '#e6a23c'
  return '#909399'
}

function getCategoryIcon(category: string) {
  const icons: Record<string, string> = {
    'Error Detection': 'WarningFilled',
    'Performance': 'Odometer',
    'Resource': 'Cpu',
    'Network': 'Connection'
  }
  return icons[category] || 'QuestionFilled'
}

// Watch
watch(() => clusterStore.currentCluster, () => {
  filters.offset = 0
  fetchAdvices()
})

onMounted(() => {
  fetchAdvices()
})
</script>

<template>
  <div class="alert-advices">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">
          <el-icon class="title-icon"><MagicStick /></el-icon>
          Alert Rule Advices
        </h1>
      </div>
      <div class="header-right">
        <el-button 
          type="primary" 
          :loading="refreshing"
          @click="refreshAdvices"
        >
          <el-icon><Refresh /></el-icon>
          Refresh AI Analysis
        </el-button>
      </div>
    </div>

    <!-- Summary Cards -->
    <div class="summary-cards">
      <el-card class="summary-card" shadow="hover">
        <div class="summary-content">
          <el-icon class="summary-icon" style="color: #e6a23c"><Clock /></el-icon>
          <div class="summary-info">
            <div class="summary-value">{{ summary.pending }}</div>
            <div class="summary-label">Pending</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="summary-card" shadow="hover">
        <div class="summary-content">
          <el-icon class="summary-icon" style="color: #67c23a"><CircleCheck /></el-icon>
          <div class="summary-info">
            <div class="summary-value">{{ summary.accepted }}</div>
            <div class="summary-label">Accepted</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="summary-card" shadow="hover">
        <div class="summary-content">
          <el-icon class="summary-icon" style="color: #f56c6c"><CircleClose /></el-icon>
          <div class="summary-info">
            <div class="summary-value">{{ summary.rejected }}</div>
            <div class="summary-label">Rejected</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="summary-card" shadow="hover">
        <div class="summary-content">
          <el-icon class="summary-icon" style="color: #409eff"><TrendCharts /></el-icon>
          <div class="summary-info">
            <div class="summary-value">{{ (summary.avgConfidence * 100).toFixed(0) }}%</div>
            <div class="summary-label">Avg Confidence</div>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Filters -->
    <el-card class="filter-card" shadow="hover">
      <el-form :inline="true" :model="filters">
        <el-form-item label="Status">
          <el-select v-model="filters.status" placeholder="All" clearable style="width: 130px">
            <el-option label="All" :value="undefined" />
            <el-option label="Pending" value="pending" />
            <el-option label="Accepted" value="accepted" />
            <el-option label="Rejected" value="rejected" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Category">
          <el-select v-model="filters.category" placeholder="All" clearable style="width: 150px">
            <el-option label="All" :value="undefined" />
            <el-option label="Error Detection" value="Error Detection" />
            <el-option label="Performance" value="Performance" />
            <el-option label="Resource" value="Resource" />
            <el-option label="Network" value="Network" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Min Priority">
          <el-slider 
            v-model="filters.minPriority" 
            :min="1" 
            :max="10" 
            :show-tooltip="true"
            style="width: 150px"
          />
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="fetchAdvices">Apply</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Advices List -->
    <div v-loading="loading" class="advices-list">
      <el-card 
        v-for="advice in advices" 
        :key="advice.id"
        class="advice-card"
        shadow="hover"
      >
        <div class="advice-header">
          <div class="advice-title">
            <el-icon class="advice-icon"><MagicStick /></el-icon>
            <span>Suggested: {{ advice.suggestedRule.name }}</span>
          </div>
          <el-tag 
            :type="advice.status === 'pending' ? 'warning' : advice.status === 'accepted' ? 'success' : 'info'"
            size="small"
          >
            {{ advice.status }}
          </el-tag>
        </div>
        
        <div class="advice-meta">
          <span class="meta-item">
            <el-icon><component :is="getCategoryIcon(advice.category)" /></el-icon>
            {{ advice.category }}
          </span>
          <span class="meta-item">
            Priority: {{ advice.priority }}/10
          </span>
          <span class="meta-item">
            Confidence: 
            <span :style="{ color: getConfidenceColor(advice.confidence) }">
              {{ (advice.confidence * 100).toFixed(0) }}%
            </span>
          </span>
        </div>
        
        <div class="advice-reason">
          <el-icon><InfoFilled /></el-icon>
          {{ advice.reason }}
        </div>
        
        <div class="suggested-rule">
          <div class="rule-label">Suggested Rule:</div>
          <div class="rule-content">
            <code class="rule-pattern">{{ (advice.suggestedRule.matchConfig as any)?.pattern || '-' }}</code>
            <div class="rule-meta">
              <AlertSeverityBadge 
                v-if="advice.suggestedRule.severity" 
                :severity="advice.suggestedRule.severity" 
                size="small" 
              />
              <span v-if="advice.workloadName" class="workload-label">
                Workload: {{ advice.workloadName }}
              </span>
            </div>
          </div>
        </div>
        
        <div v-if="advice.status === 'pending'" class="advice-actions">
          <el-button type="danger" text @click="rejectAdvice(advice)">
            Reject
          </el-button>
          <el-button type="primary" text @click="customizeAdvice(advice)">
            Customize...
          </el-button>
          <el-button type="success" @click="acceptAdvice(advice)">
            <el-icon><Check /></el-icon>
            Accept & Create
          </el-button>
        </div>
        
        <div v-else class="advice-status-info">
          <span v-if="advice.statusReason">Reason: {{ advice.statusReason }}</span>
          <span v-if="advice.appliedRuleId">
            <el-button type="primary" text size="small" @click="$router.push('/alerts/rules/log')">
              View Created Rule →
            </el-button>
          </span>
        </div>
      </el-card>
      
      <el-empty v-if="advices.length === 0" description="No advices available" />
    </div>

    <!-- Pagination -->
    <div class="pagination-wrapper" v-if="total > filters.limit!">
      <el-pagination
        v-model:current-page="currentPage"
        v-model:page-size="filters.limit"
        :total="total"
        :page-sizes="[10, 20, 50]"
        layout="total, sizes, prev, pager, next"
        @current-change="fetchAdvices"
        @size-change="fetchAdvices"
      />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.alert-advices {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
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

// Summary Cards
.summary-cards {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 24px;
  
  @media (max-width: 1024px) {
    grid-template-columns: repeat(2, 1fr);
  }
  
  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
}

.summary-card {
  border-radius: 12px;
}

.summary-content {
  display: flex;
  align-items: center;
  gap: 16px;
}

.summary-icon {
  font-size: 32px;
}

.summary-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--el-text-color-primary);
}

.summary-label {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.filter-card {
  margin-bottom: 16px;
  border-radius: 12px;
}

// Advices List
.advices-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.advice-card {
  border-radius: 12px;
}

.advice-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.advice-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
}

.advice-icon {
  color: var(--el-color-warning);
}

.advice-meta {
  display: flex;
  gap: 20px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
  margin-bottom: 12px;
}

.meta-item {
  display: flex;
  align-items: center;
  gap: 4px;
}

.advice-reason {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 12px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
  margin-bottom: 16px;
  font-size: 14px;
  line-height: 1.5;
  
  .el-icon {
    color: var(--el-color-info);
    margin-top: 2px;
  }
}

.suggested-rule {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 12px;
  margin-bottom: 16px;
}

.rule-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 8px;
}

.rule-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.rule-pattern {
  font-size: 13px;
  background: var(--el-fill-color-dark);
  padding: 8px 12px;
  border-radius: 4px;
  display: block;
  word-break: break-all;
}

.rule-meta {
  display: flex;
  align-items: center;
  gap: 12px;
}

.workload-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.advice-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.advice-status-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>

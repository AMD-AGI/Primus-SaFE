<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  alertSilencesApi,
  type AlertSilence,
  type ListSilencesParams,
  type SilenceType,
  type ResourceFilter,
  type LabelMatcher
} from '@/services/alerts'
import { useClusterStore } from '@/stores/cluster'

const clusterStore = useClusterStore()

// State
const loading = ref(false)
const silences = ref<AlertSilence[]>([])
const total = ref(0)
const showDialog = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const showExpired = ref(false)

// Filters
const filters = reactive<ListSilencesParams>({
  enabled: true,
  silenceType: undefined,
  search: '',
  offset: 0,
  limit: 20
})

// Form
const form = reactive<Partial<AlertSilence>>({
  name: '',
  description: '',
  clusterName: '',
  enabled: true,
  silenceType: 'resource',
  resourceFilters: [],
  labelMatchers: [],
  alertNames: [],
  startsAt: new Date().toISOString(),
  endsAt: undefined,
  reason: ''
})

const durationHours = ref(2)
const isPermanent = ref(false)
const alertNamesInput = ref('')

// Computed
const activeSilences = computed(() => 
  silences.value.filter(s => {
    if (!s.endsAt) return true
    return new Date(s.endsAt) > new Date()
  })
)

const expiredSilences = computed(() => 
  silences.value.filter(s => {
    if (!s.endsAt) return false
    return new Date(s.endsAt) <= new Date()
  })
)

// Pagination
const currentPage = computed({
  get: () => Math.floor((filters.offset || 0) / (filters.limit || 20)) + 1,
  set: (val: number) => {
    filters.offset = (val - 1) * (filters.limit || 20)
  }
})

// Fetch silences
async function fetchSilences() {
  loading.value = true
  try {
    const params: ListSilencesParams = {
      ...filters,
      cluster: clusterStore.currentCluster || undefined
    }
    
    const response = await alertSilencesApi.list(params)
    silences.value = response?.data || []
    total.value = response?.total || 0
  } catch (error) {
    console.error('Failed to fetch silences:', error)
    ElMessage.error('Failed to fetch silences')
  } finally {
    loading.value = false
  }
}

// Dialog actions
function openCreateDialog() {
  dialogMode.value = 'create'
  resetForm()
  form.clusterName = clusterStore.currentCluster || ''
  showDialog.value = true
}

function openEditDialog(silence: AlertSilence) {
  dialogMode.value = 'edit'
  Object.assign(form, {
    id: silence.id,
    name: silence.name,
    description: silence.description,
    clusterName: silence.clusterName,
    enabled: silence.enabled,
    silenceType: silence.silenceType,
    resourceFilters: silence.resourceFilters ? [...silence.resourceFilters] : [],
    labelMatchers: silence.labelMatchers ? [...silence.labelMatchers] : [],
    alertNames: silence.alertNames ? [...silence.alertNames] : [],
    startsAt: silence.startsAt,
    endsAt: silence.endsAt,
    reason: silence.reason
  })
  
  alertNamesInput.value = (form.alertNames || []).join(', ')
  isPermanent.value = !silence.endsAt
  
  showDialog.value = true
}

function resetForm() {
  form.id = undefined
  form.name = ''
  form.description = ''
  form.clusterName = ''
  form.enabled = true
  form.silenceType = 'resource'
  form.resourceFilters = []
  form.labelMatchers = []
  form.alertNames = []
  form.startsAt = new Date().toISOString()
  form.endsAt = undefined
  form.reason = ''
  durationHours.value = 2
  isPermanent.value = false
  alertNamesInput.value = ''
}

async function submitForm() {
  if (!form.name || !form.reason) {
    ElMessage.warning('Name and reason are required')
    return
  }
  
  try {
    // Calculate end time
    if (!isPermanent.value) {
      const startTime = new Date(form.startsAt!)
      form.endsAt = new Date(startTime.getTime() + durationHours.value * 60 * 60 * 1000).toISOString()
    } else {
      form.endsAt = undefined
    }
    
    // Parse alert names
    if (form.silenceType === 'alert_name') {
      form.alertNames = alertNamesInput.value.split(',').map(s => s.trim()).filter(Boolean)
    }
    
    if (dialogMode.value === 'create') {
      await alertSilencesApi.create(form)
      ElMessage.success('Silence created successfully')
    } else {
      await alertSilencesApi.update(form.id!, form)
      ElMessage.success('Silence updated successfully')
    }
    
    showDialog.value = false
    fetchSilences()
  } catch (error) {
    console.error('Failed to save silence:', error)
    ElMessage.error('Failed to save silence')
  }
}

// Resource Filter Management
function addResourceFilter() {
  if (!form.resourceFilters) form.resourceFilters = []
  form.resourceFilters.push({
    resourceType: 'node',
    operator: '=',
    value: ''
  })
}

function removeResourceFilter(index: number) {
  form.resourceFilters?.splice(index, 1)
}

// Label Matcher Management
function addLabelMatcher() {
  if (!form.labelMatchers) form.labelMatchers = []
  form.labelMatchers.push({
    name: '',
    operator: '=',
    value: ''
  })
}

function removeLabelMatcher(index: number) {
  form.labelMatchers?.splice(index, 1)
}

// Actions
async function deleteSilence(silence: AlertSilence) {
  try {
    await ElMessageBox.confirm(
      `Are you sure to delete silence "${silence.name}"?`,
      'Confirm Delete',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    
    await alertSilencesApi.delete(silence.id)
    ElMessage.success('Silence deleted successfully')
    fetchSilences()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to delete silence')
    }
  }
}

async function endSilence(silence: AlertSilence) {
  try {
    await ElMessageBox.confirm(
      `End silence "${silence.name}" now?`,
      'Confirm End',
      {
        confirmButtonText: 'End Now',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    
    await alertSilencesApi.end(silence.id)
    ElMessage.success('Silence ended')
    fetchSilences()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to end silence')
    }
  }
}

// Utility
function getSilenceTypeLabel(type: string) {
  const labels: Record<string, string> = {
    resource: 'Resource',
    label: 'Label',
    alert_name: 'Alert Name',
    expression: 'Expression'
  }
  return labels[type] || type
}

function formatTime(timestamp?: string) {
  if (!timestamp) return 'Permanent'
  return new Date(timestamp).toLocaleString()
}

function getRemaining(endsAt?: string) {
  if (!endsAt) return 'Permanent'
  const now = new Date()
  const end = new Date(endsAt)
  const diff = Math.floor((end.getTime() - now.getTime()) / 1000)
  
  if (diff <= 0) return 'Expired'
  if (diff < 3600) return `${Math.floor(diff / 60)}m remaining`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ${Math.floor((diff % 3600) / 60)}m remaining`
  return `${Math.floor(diff / 86400)}d remaining`
}

function getProgress(startsAt: string, endsAt?: string) {
  if (!endsAt) return 0
  const start = new Date(startsAt).getTime()
  const end = new Date(endsAt).getTime()
  const now = new Date().getTime()
  
  if (now >= end) return 100
  return Math.floor(((now - start) / (end - start)) * 100)
}

function getMatchersPreview(silence: AlertSilence) {
  if (silence.silenceType === 'resource' && silence.resourceFilters) {
    return silence.resourceFilters.map(f => `${f.resourceType}${f.operator}"${f.value}"`).join(', ')
  }
  if (silence.silenceType === 'label' && silence.labelMatchers) {
    return silence.labelMatchers.map(m => `${m.name}${m.operator}"${m.value}"`).join(', ')
  }
  if (silence.silenceType === 'alert_name' && silence.alertNames) {
    return silence.alertNames.join(', ')
  }
  return silence.matchExpression || '-'
}

// Watch
watch(() => clusterStore.currentCluster, () => {
  filters.offset = 0
  fetchSilences()
})

onMounted(() => {
  fetchSilences()
})
</script>

<template>
  <div class="alert-silences">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">
          <el-icon class="title-icon"><MuteNotification /></el-icon>
          Alert Silences
        </h1>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="openCreateDialog">
          <el-icon><Plus /></el-icon>
          Create Silence
        </el-button>
      </div>
    </div>

    <!-- Filters -->
    <el-card class="filter-card" shadow="hover">
      <el-form :inline="true" :model="filters">
        <el-form-item label="Status">
          <el-select v-model="filters.enabled" placeholder="All" style="width: 120px">
            <el-option label="Active" :value="true" />
            <el-option label="All" :value="undefined" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Type">
          <el-select v-model="filters.silenceType" placeholder="All" clearable style="width: 130px">
            <el-option label="All" :value="undefined" />
            <el-option label="Resource" value="resource" />
            <el-option label="Label" value="label" />
            <el-option label="Alert Name" value="alert_name" />
            <el-option label="Expression" value="expression" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Search">
          <el-input
            v-model="filters.search"
            placeholder="Search by name..."
            clearable
            style="width: 200px"
            @keyup.enter="fetchSilences"
          >
            <template #prefix>
              <el-icon><Search /></el-icon>
            </template>
          </el-input>
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="fetchSilences">Search</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Active Silences -->
    <el-card class="silences-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <span class="card-title">Active Silences ({{ activeSilences.length }})</span>
        </div>
      </template>
      
      <div v-loading="loading" class="silences-list">
        <div 
          v-for="silence in activeSilences" 
          :key="silence.id"
          class="silence-item"
        >
          <div class="silence-header">
            <el-icon class="silence-icon"><MuteNotification /></el-icon>
            <span class="silence-name">{{ silence.name }}</span>
            <el-tag type="info" size="small">{{ getSilenceTypeLabel(silence.silenceType) }}</el-tag>
            <div class="silence-actions">
              <el-button type="primary" text size="small" @click="openEditDialog(silence)">
                Edit
              </el-button>
              <el-button type="warning" text size="small" @click="endSilence(silence)">
                End Now
              </el-button>
            </div>
          </div>
          
          <div class="silence-details">
            <div class="detail-item">
              <span class="detail-label">Cluster:</span>
              <span class="detail-value">{{ silence.clusterName || 'All Clusters' }}</span>
            </div>
            <div class="detail-item">
              <span class="detail-label">Created by:</span>
              <span class="detail-value">{{ silence.createdBy || 'System' }}</span>
            </div>
          </div>
          
          <div class="silence-matchers">
            <span class="matchers-label">Matches:</span>
            <code class="matchers-value">{{ getMatchersPreview(silence) }}</code>
          </div>
          
          <div class="silence-reason">
            <span class="reason-label">Reason:</span>
            <span class="reason-value">{{ silence.reason }}</span>
          </div>
          
          <div class="silence-progress">
            <el-progress 
              :percentage="getProgress(silence.startsAt, silence.endsAt)"
              :status="!silence.endsAt ? 'warning' : undefined"
              :stroke-width="8"
            />
            <span class="progress-text">{{ getRemaining(silence.endsAt) }}</span>
          </div>
          
          <div class="silence-silenced" v-if="silence.silencedCount">
            <span class="silenced-count">{{ silence.silencedCount }} alerts silenced</span>
          </div>
        </div>
        
        <el-empty v-if="activeSilences.length === 0" description="No active silences" />
      </div>
    </el-card>

    <!-- Expired Silences -->
    <el-card v-if="showExpired || expiredSilences.length > 0" class="silences-card expired-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <span class="card-title">Expired Silences ({{ expiredSilences.length }})</span>
          <el-switch v-model="showExpired" active-text="Show" />
        </div>
      </template>
      
      <div v-if="showExpired" class="silences-list">
        <el-table :data="expiredSilences" stripe>
          <el-table-column prop="name" label="Name" min-width="180" />
          <el-table-column label="Type" width="120">
            <template #default="{ row }">
              <el-tag type="info" size="small">{{ getSilenceTypeLabel(row.silenceType) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Ended" width="160">
            <template #default="{ row }">
              {{ formatTime(row.endsAt) }}
            </template>
          </el-table-column>
          <el-table-column prop="reason" label="Reason" min-width="200" />
          <el-table-column label="Actions" width="100">
            <template #default="{ row }">
              <el-button type="danger" text size="small" @click="deleteSilence(row)">
                Delete
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>

    <!-- Create/Edit Dialog -->
    <el-dialog
      v-model="showDialog"
      :title="dialogMode === 'create' ? 'Create Alert Silence' : 'Edit Alert Silence'"
      width="700px"
      destroy-on-close
    >
      <el-form :model="form" label-width="120px">
        <!-- Basic Info -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <span class="section-title">Basic Info</span>
          </template>
          
          <el-form-item label="Name" required>
            <el-input v-model="form.name" placeholder="Enter silence name" />
          </el-form-item>
          
          <el-form-item label="Cluster">
            <el-select v-model="form.clusterName" placeholder="All clusters" clearable style="width: 100%">
              <el-option label="All Clusters" value="" />
              <el-option 
                v-for="cluster in clusterStore.clusters" 
                :key="cluster" 
                :label="cluster" 
                :value="cluster" 
              />
            </el-select>
          </el-form-item>
          
          <el-form-item label="Description">
            <el-input 
              v-model="form.description" 
              type="textarea" 
              :rows="2"
              placeholder="Optional description" 
            />
          </el-form-item>
        </el-card>

        <!-- Silence Type -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <span class="section-title">Silence Type</span>
          </template>
          
          <el-form-item label="Type">
            <el-radio-group v-model="form.silenceType">
              <el-radio value="resource">Resource-based</el-radio>
              <el-radio value="label">Label-based</el-radio>
              <el-radio value="alert_name">Alert Name</el-radio>
            </el-radio-group>
          </el-form-item>

          <!-- Resource Filters -->
          <div v-if="form.silenceType === 'resource'" class="filters-section">
            <div 
              v-for="(filter, index) in form.resourceFilters" 
              :key="index"
              class="filter-row"
            >
              <el-select v-model="filter.resourceType" style="width: 120px">
                <el-option label="Cluster" value="cluster" />
                <el-option label="Node" value="node" />
                <el-option label="Namespace" value="namespace" />
                <el-option label="Workload" value="workload" />
                <el-option label="Pod" value="pod" />
              </el-select>
              <el-select v-model="filter.operator" style="width: 80px">
                <el-option label="=" value="=" />
                <el-option label="!=" value="!=" />
                <el-option label="=~" value="=~" />
              </el-select>
              <el-input v-model="filter.value as string" placeholder="Value" style="flex: 1" />
              <el-button type="danger" text @click="removeResourceFilter(index)">
                <el-icon><Delete /></el-icon>
              </el-button>
            </div>
            <el-button type="primary" text @click="addResourceFilter">
              <el-icon><Plus /></el-icon>
              Add Filter
            </el-button>
          </div>

          <!-- Label Matchers -->
          <div v-if="form.silenceType === 'label'" class="filters-section">
            <div 
              v-for="(matcher, index) in form.labelMatchers" 
              :key="index"
              class="filter-row"
            >
              <el-input v-model="matcher.name" placeholder="Label name" style="width: 150px" />
              <el-select v-model="matcher.operator" style="width: 80px">
                <el-option label="=" value="=" />
                <el-option label="!=" value="!=" />
                <el-option label="=~" value="=~" />
                <el-option label="!~" value="!~" />
              </el-select>
              <el-input v-model="matcher.value" placeholder="Value" style="flex: 1" />
              <el-button type="danger" text @click="removeLabelMatcher(index)">
                <el-icon><Delete /></el-icon>
              </el-button>
            </div>
            <el-button type="primary" text @click="addLabelMatcher">
              <el-icon><Plus /></el-icon>
              Add Matcher
            </el-button>
          </div>

          <!-- Alert Names -->
          <div v-if="form.silenceType === 'alert_name'">
            <el-form-item label="Alert Names">
              <el-input 
                v-model="alertNamesInput"
                type="textarea"
                :rows="2"
                placeholder="Enter alert names, separated by commas"
              />
            </el-form-item>
          </div>
        </el-card>

        <!-- Time Window -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <span class="section-title">Time Window</span>
          </template>
          
          <el-form-item label="Duration">
            <el-checkbox v-model="isPermanent">Permanent (no end time)</el-checkbox>
          </el-form-item>
          
          <el-form-item v-if="!isPermanent" label="Hours">
            <el-input-number v-model="durationHours" :min="1" :max="720" />
            <span class="unit-label">hours</span>
          </el-form-item>
        </el-card>

        <!-- Reason -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <span class="section-title">Reason & Tracking</span>
          </template>
          
          <el-form-item label="Reason" required>
            <el-input 
              v-model="form.reason"
              type="textarea"
              :rows="2"
              placeholder="Why is this silence being created?"
            />
          </el-form-item>
        </el-card>
      </el-form>
      
      <template #footer>
        <el-button @click="showDialog = false">Cancel</el-button>
        <el-button type="primary" @click="submitForm">
          {{ dialogMode === 'create' ? 'Create Silence' : 'Save' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style lang="scss" scoped>
.alert-silences {
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
    color: var(--el-color-info);
  }
}

.filter-card {
  margin-bottom: 16px;
  border-radius: 12px;
}

.silences-card {
  margin-bottom: 16px;
  border-radius: 12px;
  
  &.expired-card {
    background: var(--el-fill-color-lighter);
  }
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-title {
  font-weight: 600;
}

.silences-list {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.silence-item {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 16px;
  background: var(--el-bg-color);
}

.silence-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.silence-icon {
  color: var(--el-color-info);
  font-size: 20px;
}

.silence-name {
  font-weight: 600;
  font-size: 16px;
}

.silence-actions {
  margin-left: auto;
  display: flex;
  gap: 8px;
}

.silence-details {
  display: flex;
  gap: 24px;
  margin-bottom: 12px;
}

.detail-item {
  display: flex;
  gap: 8px;
  font-size: 13px;
}

.detail-label {
  color: var(--el-text-color-secondary);
}

.silence-matchers {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.matchers-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.matchers-value {
  font-size: 12px;
  background: var(--el-fill-color-light);
  padding: 4px 8px;
  border-radius: 4px;
}

.silence-reason {
  display: flex;
  gap: 8px;
  margin-bottom: 12px;
}

.reason-label {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.reason-value {
  font-size: 13px;
}

.silence-progress {
  display: flex;
  align-items: center;
  gap: 12px;
  
  .el-progress {
    flex: 1;
  }
}

.progress-text {
  font-size: 13px;
  color: var(--el-color-warning);
  white-space: nowrap;
}

.silence-silenced {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.silenced-count {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

// Dialog styles
.form-section {
  margin-bottom: 16px;
  
  &:last-child {
    margin-bottom: 0;
  }
}

.section-title {
  font-weight: 600;
}

.filters-section {
  padding: 8px 0;
}

.filter-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
  align-items: center;
}

.unit-label {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
}
</style>

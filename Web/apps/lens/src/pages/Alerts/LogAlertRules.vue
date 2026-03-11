<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  logAlertRulesApi,
  type LogAlertRule,
  type ListLogRulesParams,
  type MatchType,
  type AlertSeverity,
  type PatternConfig,
  type ThresholdConfig
} from '@/services/alerts'
import AlertSeverityBadge from './components/AlertSeverityBadge.vue'
import { useClusterStore } from '@/stores/cluster'

const clusterStore = useClusterStore()

// State
const loading = ref(false)
const rules = ref<LogAlertRule[]>([])
const total = ref(0)
const showDialog = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const currentStep = ref(0)
const testResult = ref<{ matched: boolean; captures?: string[] } | null>(null)

// Filters
const filters = reactive<ListLogRulesParams>({
  enabled: undefined,
  matchType: undefined,
  severity: undefined,
  search: '',
  offset: 0,
  limit: 20
})

// Form
const formRef = ref()
const form = reactive<Partial<LogAlertRule>>({
  name: '',
  description: '',
  clusterName: '',
  enabled: true,
  priority: 50,
  matchType: 'pattern',
  matchConfig: {
    pattern: '',
    flags: { caseInsensitive: false, multiLine: false }
  } as PatternConfig,
  labelSelectors: [],
  severity: 'warning',
  groupWait: 60,
  repeatInterval: 3600,
  alertTemplate: {
    title: '',
    summary: '',
    description: ''
  }
})

const testSampleLog = ref('')

// Pagination
const currentPage = computed({
  get: () => Math.floor((filters.offset || 0) / (filters.limit || 20)) + 1,
  set: (val: number) => {
    filters.offset = (val - 1) * (filters.limit || 20)
  }
})

// Fetch rules
async function fetchRules() {
  loading.value = true
  try {
    const params: ListLogRulesParams = {
      ...filters,
      cluster: clusterStore.currentCluster || undefined
    }
    
    const response = await logAlertRulesApi.list(params)
    rules.value = response?.data || []
    total.value = response?.total || 0
  } catch (error) {
    console.error('Failed to fetch log alert rules:', error)
    ElMessage.error('Failed to fetch rules')
  } finally {
    loading.value = false
  }
}

// Dialog actions
function openCreateDialog() {
  dialogMode.value = 'create'
  resetForm()
  form.clusterName = clusterStore.currentCluster || ''
  currentStep.value = 0
  showDialog.value = true
}

function openEditDialog(rule: LogAlertRule) {
  dialogMode.value = 'edit'
  Object.assign(form, {
    id: rule.id,
    name: rule.name,
    description: rule.description,
    clusterName: rule.clusterName,
    enabled: rule.enabled,
    priority: rule.priority,
    matchType: rule.matchType,
    matchConfig: JSON.parse(JSON.stringify(rule.matchConfig)),
    labelSelectors: rule.labelSelectors ? [...rule.labelSelectors] : [],
    severity: rule.severity,
    groupWait: rule.groupWait,
    repeatInterval: rule.repeatInterval,
    alertTemplate: rule.alertTemplate ? { ...rule.alertTemplate } : { title: '', summary: '', description: '' }
  })
  currentStep.value = 0
  showDialog.value = true
}

function resetForm() {
  form.id = undefined
  form.name = ''
  form.description = ''
  form.clusterName = ''
  form.enabled = true
  form.priority = 50
  form.matchType = 'pattern'
  form.matchConfig = {
    pattern: '',
    flags: { caseInsensitive: false, multiLine: false }
  }
  form.labelSelectors = []
  form.severity = 'warning'
  form.groupWait = 60
  form.repeatInterval = 3600
  form.alertTemplate = { title: '', summary: '', description: '' }
  testResult.value = null
  testSampleLog.value = ''
}

async function submitForm() {
  try {
    if (dialogMode.value === 'create') {
      await logAlertRulesApi.create(form)
      ElMessage.success('Rule created successfully')
    } else {
      await logAlertRulesApi.update(form.id!, form)
      ElMessage.success('Rule updated successfully')
    }
    
    showDialog.value = false
    fetchRules()
  } catch (error) {
    console.error('Failed to save rule:', error)
    ElMessage.error('Failed to save rule')
  }
}

// Match type change
function handleMatchTypeChange(type: MatchType) {
  form.matchType = type
  if (type === 'pattern') {
    form.matchConfig = {
      pattern: '',
      flags: { caseInsensitive: false, multiLine: false }
    }
  } else if (type === 'threshold') {
    form.matchConfig = {
      pattern: '',
      threshold: { count: 5, window: '10m' }
    }
  }
}

// Label selector management
function addLabelSelector() {
  if (!form.labelSelectors) form.labelSelectors = []
  form.labelSelectors.push({
    key: '',
    operator: '=',
    value: ''
  })
}

function removeLabelSelector(index: number) {
  form.labelSelectors?.splice(index, 1)
}

// Test pattern
async function testPattern() {
  if (!form.matchConfig || !testSampleLog.value) return
  
  try {
    const config = form.matchConfig as PatternConfig
    testResult.value = await logAlertRulesApi.test({
      pattern: config.pattern,
      sampleLog: testSampleLog.value,
      flags: config.flags
    })
  } catch (error) {
    ElMessage.error('Test failed')
  }
}

// Actions
async function deleteRule(rule: LogAlertRule) {
  try {
    await ElMessageBox.confirm(
      `Are you sure to delete rule "${rule.name}"?`,
      'Confirm Delete',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    
    await logAlertRulesApi.delete(rule.id)
    ElMessage.success('Rule deleted successfully')
    fetchRules()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to delete rule')
    }
  }
}

async function cloneRule(rule: LogAlertRule) {
  try {
    await logAlertRulesApi.clone(rule.id, { name: `${rule.name}_copy` })
    ElMessage.success('Rule cloned successfully')
    fetchRules()
  } catch (error) {
    ElMessage.error('Failed to clone rule')
  }
}

async function toggleEnabled(rule: LogAlertRule) {
  try {
    await logAlertRulesApi.update(rule.id, { enabled: !rule.enabled })
    rule.enabled = !rule.enabled
    ElMessage.success(`Rule ${rule.enabled ? 'enabled' : 'disabled'}`)
  } catch (error) {
    ElMessage.error('Failed to update rule')
  }
}

// Utility
function getMatchTypeLabel(type: string) {
  const labels: Record<string, string> = {
    pattern: 'Pattern',
    threshold: 'Threshold',
    composite: 'Composite'
  }
  return labels[type] || type
}

function formatTime(timestamp?: string) {
  if (!timestamp) return '-'
  return new Date(timestamp).toLocaleString()
}

function getPatternPreview(config: any) {
  if (config?.pattern) {
    return config.pattern.length > 40 ? config.pattern.substring(0, 40) + '...' : config.pattern
  }
  return '-'
}

// Watch
watch(() => clusterStore.currentCluster, () => {
  filters.offset = 0
  fetchRules()
})

// Steps
const steps = [
  { title: 'Basic Info', description: 'Name and cluster' },
  { title: 'Match Config', description: 'Pattern or threshold' },
  { title: 'Alert Template', description: 'Alert content' },
  { title: 'Review', description: 'Confirm settings' }
]

function nextStep() {
  if (currentStep.value < steps.length - 1) {
    currentStep.value++
  }
}

function prevStep() {
  if (currentStep.value > 0) {
    currentStep.value--
  }
}

onMounted(() => {
  fetchRules()
})
</script>

<template>
  <div class="log-alert-rules">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">
          <el-icon class="title-icon"><Document /></el-icon>
          Log Alert Rules
        </h1>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="openCreateDialog">
          <el-icon><Plus /></el-icon>
          Create Rule
        </el-button>
      </div>
    </div>

    <!-- Filters -->
    <el-card class="filter-card" shadow="hover">
      <el-form :inline="true" :model="filters">
        <el-form-item label="Status">
          <el-select v-model="filters.enabled" placeholder="All" clearable style="width: 120px">
            <el-option label="All" :value="undefined" />
            <el-option label="Enabled" :value="true" />
            <el-option label="Disabled" :value="false" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Match Type">
          <el-select v-model="filters.matchType" placeholder="All" clearable style="width: 130px">
            <el-option label="All" :value="undefined" />
            <el-option label="Pattern" value="pattern" />
            <el-option label="Threshold" value="threshold" />
            <el-option label="Composite" value="composite" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Severity">
          <el-select v-model="filters.severity" placeholder="All" clearable style="width: 120px">
            <el-option label="All" :value="undefined" />
            <el-option label="Critical" value="critical" />
            <el-option label="High" value="high" />
            <el-option label="Warning" value="warning" />
            <el-option label="Info" value="info" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Search">
          <el-input
            v-model="filters.search"
            placeholder="Search by name..."
            clearable
            style="width: 200px"
            @keyup.enter="fetchRules"
          >
            <template #prefix>
              <el-icon><Search /></el-icon>
            </template>
          </el-input>
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="fetchRules">Search</el-button>
          <el-button @click="filters.search = ''; filters.enabled = undefined; filters.matchType = undefined; filters.severity = undefined; fetchRules()">
            Reset
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Rules Table -->
    <el-card class="table-card" shadow="hover">
      <el-table v-loading="loading" :data="rules" stripe>
        <el-table-column prop="name" label="Name" min-width="180">
          <template #default="{ row }">
            <div class="rule-name-cell">
              <el-switch 
                :model-value="row.enabled" 
                @change="toggleEnabled(row)"
                size="small"
                style="margin-right: 8px"
              />
              <span class="rule-name">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column label="Match Type" width="120">
          <template #default="{ row }">
            <el-tag type="info" size="small">{{ getMatchTypeLabel(row.matchType) }}</el-tag>
          </template>
        </el-table-column>
        
        <el-table-column label="Pattern" min-width="200">
          <template #default="{ row }">
            <code class="pattern-preview">{{ getPatternPreview(row.matchConfig) }}</code>
          </template>
        </el-table-column>
        
        <el-table-column label="Severity" width="120">
          <template #default="{ row }">
            <AlertSeverityBadge :severity="row.severity" size="small" />
          </template>
        </el-table-column>
        
        <el-table-column label="Triggers" width="100">
          <template #default="{ row }">
            {{ row.triggerCount || 0 }}
          </template>
        </el-table-column>
        
        <el-table-column label="Last Triggered" width="160">
          <template #default="{ row }">
            {{ formatTime(row.lastTriggeredAt) }}
          </template>
        </el-table-column>
        
        <el-table-column label="Actions" width="200" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" text size="small" @click="openEditDialog(row)">
              Edit
            </el-button>
            <el-button type="primary" text size="small" @click="cloneRule(row)">
              Clone
            </el-button>
            <el-button type="danger" text size="small" @click="deleteRule(row)">
              Delete
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      
      <!-- Pagination -->
      <div class="table-footer">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="filters.limit"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @current-change="fetchRules"
          @size-change="fetchRules"
        />
      </div>
    </el-card>

    <!-- Create/Edit Dialog -->
    <el-dialog
      v-model="showDialog"
      :title="dialogMode === 'create' ? 'Create Log Alert Rule' : 'Edit Log Alert Rule'"
      width="800px"
      destroy-on-close
    >
      <!-- Steps -->
      <el-steps :active="currentStep" finish-status="success" class="wizard-steps">
        <el-step 
          v-for="(step, index) in steps" 
          :key="index" 
          :title="step.title" 
          :description="step.description" 
        />
      </el-steps>

      <div class="step-content">
        <!-- Step 1: Basic Info -->
        <div v-show="currentStep === 0">
          <el-form ref="formRef" :model="form" label-width="120px">
            <el-form-item label="Name" required>
              <el-input v-model="form.name" placeholder="Enter rule name" />
            </el-form-item>
            
            <el-form-item label="Description">
              <el-input 
                v-model="form.description" 
                type="textarea" 
                :rows="2"
                placeholder="Enter description" 
              />
            </el-form-item>
            
            <el-form-item label="Cluster" required>
              <el-select v-model="form.clusterName" placeholder="Select cluster" style="width: 100%">
                <el-option 
                  v-for="cluster in clusterStore.clusters" 
                  :key="cluster" 
                  :label="cluster" 
                  :value="cluster" 
                />
              </el-select>
            </el-form-item>
            
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="Severity">
                  <el-select v-model="form.severity" style="width: 100%">
                    <el-option label="Critical" value="critical" />
                    <el-option label="High" value="high" />
                    <el-option label="Warning" value="warning" />
                    <el-option label="Info" value="info" />
                  </el-select>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Priority">
                  <el-slider v-model="form.priority" :min="1" :max="100" show-input />
                </el-form-item>
              </el-col>
            </el-row>
            
            <el-form-item label="Enabled">
              <el-switch v-model="form.enabled" />
            </el-form-item>
          </el-form>
        </div>

        <!-- Step 2: Match Config -->
        <div v-show="currentStep === 1">
          <el-form label-width="120px">
            <el-form-item label="Match Type">
              <el-radio-group v-model="form.matchType" @change="handleMatchTypeChange">
                <el-radio-button value="pattern">Pattern</el-radio-button>
                <el-radio-button value="threshold">Threshold</el-radio-button>
              </el-radio-group>
            </el-form-item>

            <!-- Pattern Config -->
            <template v-if="form.matchType === 'pattern'">
              <el-form-item label="Regex Pattern">
                <el-input 
                  v-model="(form.matchConfig as PatternConfig).pattern" 
                  type="textarea"
                  :rows="3"
                  placeholder="Enter regex pattern, e.g. NCCL.*(error|warn)"
                />
              </el-form-item>
              
              <el-form-item label="Options">
                <el-checkbox v-model="(form.matchConfig as PatternConfig).flags!.caseInsensitive">
                  Case Insensitive
                </el-checkbox>
                <el-checkbox v-model="(form.matchConfig as PatternConfig).flags!.multiLine">
                  Multi-line Mode
                </el-checkbox>
              </el-form-item>
            </template>

            <!-- Threshold Config -->
            <template v-if="form.matchType === 'threshold'">
              <el-form-item label="Pattern">
                <el-input 
                  v-model="(form.matchConfig as ThresholdConfig).pattern"
                  placeholder="Pattern to match"
                />
              </el-form-item>
              
              <el-row :gutter="16">
                <el-col :span="12">
                  <el-form-item label="Count">
                    <el-input-number 
                      v-model="(form.matchConfig as ThresholdConfig).threshold.count" 
                      :min="1"
                    />
                  </el-form-item>
                </el-col>
                <el-col :span="12">
                  <el-form-item label="Window">
                    <el-input 
                      v-model="(form.matchConfig as ThresholdConfig).threshold.window"
                      placeholder="e.g. 10m, 1h"
                    />
                  </el-form-item>
                </el-col>
              </el-row>
            </template>

            <!-- Label Selectors -->
            <el-divider content-position="left">Label Selectors (Optional)</el-divider>
            
            <div class="label-selectors">
              <div 
                v-for="(selector, index) in form.labelSelectors" 
                :key="index"
                class="selector-row"
              >
                <el-input v-model="selector.key" placeholder="Key" style="width: 120px" />
                <el-select v-model="selector.operator" style="width: 80px">
                  <el-option label="=" value="=" />
                  <el-option label="!=" value="!=" />
                  <el-option label="=~" value="=~" />
                  <el-option label="!~" value="!~" />
                </el-select>
                <el-input v-model="selector.value as string" placeholder="Value" style="flex: 1" />
                <el-button type="danger" text @click="removeLabelSelector(index)">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </div>
              <el-button type="primary" text @click="addLabelSelector">
                <el-icon><Plus /></el-icon>
                Add Selector
              </el-button>
            </div>

            <!-- Test Pattern -->
            <el-divider content-position="left">Test Pattern</el-divider>
            
            <el-form-item label="Sample Log">
              <el-input 
                v-model="testSampleLog"
                type="textarea"
                :rows="2"
                placeholder="Enter sample log to test pattern"
              />
            </el-form-item>
            
            <el-form-item>
              <el-button type="primary" @click="testPattern">Test Pattern</el-button>
              <span v-if="testResult" class="test-result" :class="{ 'is-match': testResult.matched }">
                {{ testResult.matched ? '✅ Pattern matches' : '❌ Pattern does not match' }}
              </span>
            </el-form-item>
          </el-form>
        </div>

        <!-- Step 3: Alert Template -->
        <div v-show="currentStep === 2">
          <el-form label-width="120px">
            <el-form-item label="Alert Title">
              <el-input 
                v-model="form.alertTemplate!.title"
                placeholder="Alert title template"
              />
            </el-form-item>
            
            <el-form-item label="Summary">
              <el-input 
                v-model="form.alertTemplate!.summary"
                type="textarea"
                :rows="2"
                placeholder="Alert summary"
              />
            </el-form-item>
            
            <el-form-item label="Description">
              <el-input 
                v-model="form.alertTemplate!.description"
                type="textarea"
                :rows="3"
                placeholder="Detailed description"
              />
            </el-form-item>
            
            <el-row :gutter="16">
              <el-col :span="12">
                <el-form-item label="Group Wait">
                  <el-input-number v-model="form.groupWait" :min="0" />
                  <span class="unit-label">seconds</span>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="Repeat Interval">
                  <el-input-number v-model="form.repeatInterval" :min="0" />
                  <span class="unit-label">seconds</span>
                </el-form-item>
              </el-col>
            </el-row>
          </el-form>
        </div>

        <!-- Step 4: Review -->
        <div v-show="currentStep === 3">
          <el-descriptions :column="2" border>
            <el-descriptions-item label="Name">{{ form.name }}</el-descriptions-item>
            <el-descriptions-item label="Cluster">{{ form.clusterName }}</el-descriptions-item>
            <el-descriptions-item label="Match Type">{{ getMatchTypeLabel(form.matchType || '') }}</el-descriptions-item>
            <el-descriptions-item label="Severity">
              <AlertSeverityBadge v-if="form.severity" :severity="form.severity" size="small" />
            </el-descriptions-item>
            <el-descriptions-item label="Enabled">
              <el-tag :type="form.enabled ? 'success' : 'info'" size="small">
                {{ form.enabled ? 'Enabled' : 'Disabled' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Priority">{{ form.priority }}</el-descriptions-item>
            <el-descriptions-item label="Pattern" :span="2">
              <code>{{ (form.matchConfig as PatternConfig)?.pattern || '-' }}</code>
            </el-descriptions-item>
          </el-descriptions>
        </div>
      </div>
      
      <template #footer>
        <el-button @click="showDialog = false">Cancel</el-button>
        <el-button v-if="currentStep > 0" @click="prevStep">Previous</el-button>
        <el-button v-if="currentStep < steps.length - 1" type="primary" @click="nextStep">
          Next
        </el-button>
        <el-button v-if="currentStep === steps.length - 1" type="primary" @click="submitForm">
          {{ dialogMode === 'create' ? 'Create' : 'Save' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style lang="scss" scoped>
.log-alert-rules {
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
    color: var(--el-color-success);
  }
}

.filter-card {
  margin-bottom: 16px;
  border-radius: 12px;
}

.table-card {
  border-radius: 12px;
}

.rule-name-cell {
  display: flex;
  align-items: center;
  
  .rule-name {
    font-weight: 500;
  }
}

.pattern-preview {
  font-size: 12px;
  background: var(--el-fill-color-light);
  padding: 4px 8px;
  border-radius: 4px;
  color: var(--el-text-color-regular);
}

.table-footer {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

// Dialog styles
.wizard-steps {
  margin-bottom: 24px;
}

.step-content {
  min-height: 300px;
  padding: 16px 0;
}

.label-selectors {
  .selector-row {
    display: flex;
    gap: 8px;
    margin-bottom: 8px;
    align-items: center;
  }
}

.test-result {
  margin-left: 16px;
  font-weight: 500;
  
  &.is-match {
    color: var(--el-color-success);
  }
  
  &:not(.is-match) {
    color: var(--el-color-danger);
  }
}

.unit-label {
  margin-left: 8px;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}
</style>

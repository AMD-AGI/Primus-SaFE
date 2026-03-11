<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  metricAlertRulesApi,
  notificationChannelsApi,
  type MetricAlertRule,
  type VMRuleGroup,
  type VMRule,
  type ListMetricRulesParams,
  type NotificationChannel
} from '@/services/alerts'
import { useClusterStore } from '@/stores/cluster'

const clusterStore = useClusterStore()

// Notification channels state
const notificationChannels = ref<NotificationChannel[]>([])
const loadingChannels = ref(false)

// State
const loading = ref(false)
const rules = ref<MetricAlertRule[]>([])
const total = ref(0)
const showDialog = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const syncing = ref(false)

// Filters
const filters = reactive<ListMetricRulesParams>({
  enabled: undefined,
  syncStatus: undefined,
  search: '',
  offset: 0,
  limit: 20
})

// Alert routing config interface
interface AlertRoutingForm {
  enabled: boolean
  receivers: {
    name: string
    channelIds: number[]
  }[]
}

// Form
const formRef = ref()
const form = reactive<Partial<MetricAlertRule> & { alertRouting?: AlertRoutingForm }>({
  name: '',
  clusterName: '',
  enabled: true,
  description: '',
  groups: [],
  labels: {},
  alertRouting: {
    enabled: false,
    receivers: []
  }
})

const formRules = {
  name: [{ required: true, message: 'Name is required', trigger: 'blur' }],
  clusterName: [{ required: true, message: 'Cluster is required', trigger: 'change' }]
}

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
    const params: ListMetricRulesParams = {
      ...filters,
      cluster: clusterStore.currentCluster || undefined
    }
    
    const response = await metricAlertRulesApi.list(params)
    rules.value = response?.data || []
    total.value = response?.total || 0
  } catch (error) {
    console.error('Failed to fetch metric alert rules:', error)
    ElMessage.error('Failed to fetch rules')
  } finally {
    loading.value = false
  }
}

// Fetch notification channels
async function fetchChannels() {
  loadingChannels.value = true
  try {
    const response = await notificationChannelsApi.list({ enabled: true, limit: 100 })
    notificationChannels.value = response?.data || []
  } catch (error) {
    console.error('Failed to fetch notification channels:', error)
  } finally {
    loadingChannels.value = false
  }
}

// Dialog actions
function openCreateDialog() {
  dialogMode.value = 'create'
  resetForm()
  form.clusterName = clusterStore.currentCluster || ''
  showDialog.value = true
}

function openEditDialog(rule: MetricAlertRule) {
  dialogMode.value = 'edit'
  // Parse alert_routing from rule if exists
  const alertRouting = (rule as any).alert_routing || { enabled: false, receivers: [] }
  Object.assign(form, {
    id: rule.id,
    name: rule.name,
    clusterName: rule.clusterName,
    enabled: rule.enabled,
    description: rule.description,
    groups: JSON.parse(JSON.stringify(rule.groups)),
    labels: { ...rule.labels },
    alertRouting: {
      enabled: alertRouting.enabled || false,
      receivers: (alertRouting.receivers || []).map((r: any) => ({
        name: r.name,
        channelIds: r.channel_ids || []
      }))
    }
  })
  showDialog.value = true
}

function resetForm() {
  form.id = undefined
  form.name = ''
  form.clusterName = ''
  form.enabled = true
  form.description = ''
  form.groups = []
  form.labels = {}
  form.alertRouting = {
    enabled: false,
    receivers: []
  }
}

// Receiver management
function addReceiver() {
  if (!form.alertRouting) {
    form.alertRouting = { enabled: false, receivers: [] }
  }
  form.alertRouting.receivers.push({
    name: `receiver_${form.alertRouting.receivers.length + 1}`,
    channelIds: []
  })
}

function removeReceiver(index: number) {
  form.alertRouting?.receivers.splice(index, 1)
}

// Get channel name by ID
function getChannelName(id: number): string {
  const channel = notificationChannels.value.find(c => c.id === id)
  return channel?.name || `Channel #${id}`
}

// Get channel type icon color
function getChannelTypeColor(id: number): string {
  const channel = notificationChannels.value.find(c => c.id === id)
  const colors: Record<string, string> = {
    email: '#409eff',
    webhook: '#67c23a',
    dingtalk: '#5a9cf8',
    wechat: '#07c160',
    slack: '#4a154b',
    alertmanager: '#e6522c'
  }
  return channel ? colors[channel.type] || '#909399' : '#909399'
}

async function submitForm() {
  if (!formRef.value) return
  
  await formRef.value.validate(async (valid: boolean) => {
    if (!valid) return
    
    try {
      // Prepare the payload with alert_routing in the correct format
      const payload: any = {
        name: form.name,
        cluster_name: form.clusterName,
        enabled: form.enabled,
        description: form.description,
        groups: form.groups,
        labels: form.labels
      }
      
      // Add alert_routing if enabled
      if (form.alertRouting?.enabled && form.alertRouting.receivers.length > 0) {
        payload.alert_routing = {
          enabled: true,
          receivers: form.alertRouting.receivers.map(r => ({
            name: r.name,
            channel_ids: r.channelIds
          }))
        }
      }
      
      if (dialogMode.value === 'create') {
        await metricAlertRulesApi.create(payload)
        ElMessage.success('Rule created successfully')
      } else {
        await metricAlertRulesApi.update(form.id!, payload)
        ElMessage.success('Rule updated successfully')
      }
      
      showDialog.value = false
      fetchRules()
    } catch (error) {
      console.error('Failed to save rule:', error)
      ElMessage.error('Failed to save rule')
    }
  })
}

// Rule Group Management
function addGroup() {
  if (!form.groups) form.groups = []
  form.groups.push({
    name: `group_${form.groups.length + 1}`,
    interval: '30s',
    rules: []
  })
}

function removeGroup(index: number) {
  form.groups?.splice(index, 1)
}

function addRuleToGroup(group: VMRuleGroup) {
  group.rules.push({
    alert: '',
    expr: '',
    for: '5m',
    labels: { severity: 'warning' },
    annotations: { summary: '', description: '' }
  })
}

function removeRuleFromGroup(group: VMRuleGroup, index: number) {
  group.rules.splice(index, 1)
}

// Actions
async function deleteRule(rule: MetricAlertRule) {
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
    
    await metricAlertRulesApi.delete(rule.id)
    ElMessage.success('Rule deleted successfully')
    fetchRules()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to delete rule')
    }
  }
}

async function cloneRule(rule: MetricAlertRule) {
  try {
    await metricAlertRulesApi.clone(rule.id)
    ElMessage.success('Rule cloned successfully')
    fetchRules()
  } catch (error) {
    ElMessage.error('Failed to clone rule')
  }
}

async function toggleEnabled(rule: MetricAlertRule) {
  try {
    await metricAlertRulesApi.update(rule.id, { enabled: !rule.enabled })
    rule.enabled = !rule.enabled
    ElMessage.success(`Rule ${rule.enabled ? 'enabled' : 'disabled'}`)
  } catch (error) {
    ElMessage.error('Failed to update rule')
  }
}

async function syncAllRules() {
  syncing.value = true
  try {
    const result = await metricAlertRulesApi.sync({
      cluster: clusterStore.currentCluster || undefined
    })
    ElMessage.success(`Synced ${result.synced} rules, ${result.failed} failed`)
    fetchRules()
  } catch (error) {
    ElMessage.error('Failed to sync rules')
  } finally {
    syncing.value = false
  }
}

// Utility
function getSyncStatusType(status: string) {
  switch (status) {
    case 'synced': return 'success'
    case 'pending': return 'warning'
    case 'failed': return 'danger'
    default: return 'info'
  }
}

function formatTime(timestamp?: string) {
  if (!timestamp) return '-'
  return new Date(timestamp).toLocaleString()
}

function getRuleCount(rule: MetricAlertRule) {
  return rule.groups?.reduce((sum, g) => sum + (g.rules?.length || 0), 0) || 0
}

function getGroupCount(rule: MetricAlertRule) {
  return rule.groups?.length || 0
}

// Watch
watch(() => clusterStore.currentCluster, () => {
  filters.offset = 0
  fetchRules()
})

onMounted(() => {
  fetchRules()
  fetchChannels()
})
</script>

<template>
  <div class="metric-alert-rules">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">
          <el-icon class="title-icon"><DataLine /></el-icon>
          Metric Alert Rules
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
        
        <el-form-item label="Sync Status">
          <el-select v-model="filters.syncStatus" placeholder="All" clearable style="width: 130px">
            <el-option label="All" :value="undefined" />
            <el-option label="Synced" value="synced" />
            <el-option label="Pending" value="pending" />
            <el-option label="Failed" value="failed" />
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
          <el-button @click="filters.search = ''; filters.enabled = undefined; filters.syncStatus = undefined; fetchRules()">
            Reset
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Rules Table -->
    <el-card class="table-card" shadow="hover">
      <el-table v-loading="loading" :data="rules" stripe>
        <el-table-column prop="name" label="Name" min-width="200">
          <template #default="{ row }">
            <div class="rule-name-cell">
              <span class="rule-name">{{ row.name }}</span>
              <span class="rule-meta">
                {{ getRuleCount(row) }} rules in {{ getGroupCount(row) }} groups
              </span>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="clusterName" label="Cluster" width="150" />
        
        <el-table-column prop="enabled" label="Status" width="100">
          <template #default="{ row }">
            <el-switch 
              :model-value="row.enabled" 
              @change="toggleEnabled(row)"
              inline-prompt
              active-text="On"
              inactive-text="Off"
            />
          </template>
        </el-table-column>
        
        <el-table-column label="Sync Status" width="130">
          <template #default="{ row }">
            <el-tag :type="getSyncStatusType(row.syncStatus)" size="small">
              {{ row.syncStatus }}
            </el-tag>
          </template>
        </el-table-column>
        
        <el-table-column label="Last Sync" width="160">
          <template #default="{ row }">
            {{ formatTime(row.lastSyncAt) }}
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
      
      <!-- Footer -->
      <div class="table-footer">
        <el-button 
          type="primary" 
          :loading="syncing"
          @click="syncAllRules"
        >
          <el-icon><Refresh /></el-icon>
          Sync All Rules to Cluster
        </el-button>
        
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
      :title="dialogMode === 'create' ? 'Create Metric Alert Rule' : 'Edit Metric Alert Rule'"
      width="900px"
      destroy-on-close
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-width="120px"
        label-position="top"
      >
        <!-- Basic Info -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <span class="section-title">Basic Info</span>
          </template>
          
          <el-row :gutter="16">
            <el-col :span="12">
              <el-form-item label="Name" prop="name">
                <el-input v-model="form.name" placeholder="Enter rule name" />
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="Cluster" prop="clusterName">
                <el-select v-model="form.clusterName" placeholder="Select cluster" style="width: 100%">
                  <el-option 
                    v-for="cluster in clusterStore.clusters" 
                    :key="cluster" 
                    :label="cluster" 
                    :value="cluster" 
                  />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>
          
          <el-form-item label="Description">
            <el-input 
              v-model="form.description" 
              type="textarea" 
              :rows="2"
              placeholder="Enter description" 
            />
          </el-form-item>
          
          <el-form-item label="Enabled">
            <el-switch v-model="form.enabled" />
          </el-form-item>
        </el-card>

        <!-- Alert Groups -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <span class="section-title">Alert Groups</span>
              <el-button type="primary" size="small" @click="addGroup">
                <el-icon><Plus /></el-icon>
                Add Group
              </el-button>
            </div>
          </template>
          
          <div v-if="form.groups && form.groups.length > 0">
            <div 
              v-for="(group, groupIndex) in form.groups" 
              :key="groupIndex"
              class="rule-group"
            >
              <div class="group-header">
                <el-input 
                  v-model="group.name" 
                  placeholder="Group name" 
                  style="width: 200px"
                />
                <el-input 
                  v-model="group.interval" 
                  placeholder="Interval" 
                  style="width: 100px; margin-left: 12px"
                />
                <el-button 
                  type="danger" 
                  text 
                  @click="removeGroup(groupIndex)"
                  style="margin-left: auto"
                >
                  Remove Group
                </el-button>
              </div>
              
              <div class="group-rules">
                <div 
                  v-for="(rule, ruleIndex) in group.rules" 
                  :key="ruleIndex"
                  class="rule-item"
                >
                  <div class="rule-item-header">
                    <el-input 
                      v-model="rule.alert" 
                      placeholder="Alert name" 
                      style="width: 200px"
                    />
                    <el-button 
                      type="danger" 
                      text 
                      size="small"
                      @click="removeRuleFromGroup(group, ruleIndex)"
                    >
                      <el-icon><Delete /></el-icon>
                    </el-button>
                  </div>
                  
                  <el-form-item label="Expression">
                    <el-input 
                      v-model="rule.expr" 
                      type="textarea" 
                      :rows="2"
                      placeholder="PromQL expression"
                    />
                  </el-form-item>
                  
                  <el-row :gutter="16">
                    <el-col :span="8">
                      <el-form-item label="For">
                        <el-input v-model="rule.for" placeholder="5m" />
                      </el-form-item>
                    </el-col>
                    <el-col :span="8">
                      <el-form-item label="Severity">
                        <el-select v-model="rule.labels!.severity" style="width: 100%">
                          <el-option label="Critical" value="critical" />
                          <el-option label="High" value="high" />
                          <el-option label="Warning" value="warning" />
                          <el-option label="Info" value="info" />
                        </el-select>
                      </el-form-item>
                    </el-col>
                  </el-row>
                  
                  <el-form-item label="Summary">
                    <el-input 
                      v-model="rule.annotations!.summary" 
                      placeholder="Alert summary"
                    />
                  </el-form-item>
                </div>
                
                <el-button 
                  type="primary" 
                  text 
                  @click="addRuleToGroup(group)"
                  class="add-rule-btn"
                >
                  <el-icon><Plus /></el-icon>
                  Add Rule
                </el-button>
              </div>
            </div>
          </div>
          
          <el-empty v-else description="No groups. Click 'Add Group' to start." :image-size="60" />
        </el-card>

        <!-- Alert Routing -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <div class="section-title-group">
                <span class="section-title">Alert Routing</span>
                <el-switch 
                  v-model="form.alertRouting!.enabled" 
                  size="small"
                  style="margin-left: 12px"
                />
              </div>
              <el-button 
                type="primary" 
                size="small" 
                @click="addReceiver"
                :disabled="!form.alertRouting?.enabled"
              >
                <el-icon><Plus /></el-icon>
                Add Receiver
              </el-button>
            </div>
          </template>
          
          <template v-if="form.alertRouting?.enabled">
            <div v-if="form.alertRouting.receivers && form.alertRouting.receivers.length > 0">
              <div 
                v-for="(receiver, index) in form.alertRouting.receivers" 
                :key="index"
                class="receiver-item"
              >
                <div class="receiver-header">
                  <el-form-item label="Receiver Name" class="receiver-name-input">
                    <el-input v-model="receiver.name" placeholder="e.g., ops-team" />
                  </el-form-item>
                  <el-button 
                    type="danger" 
                    text 
                    @click="removeReceiver(index)"
                  >
                    <el-icon><Delete /></el-icon>
                  </el-button>
                </div>
                
                <el-form-item label="Notification Channels">
                  <el-select 
                    v-model="receiver.channelIds" 
                    multiple 
                    placeholder="Select notification channels"
                    style="width: 100%"
                    :loading="loadingChannels"
                  >
                    <el-option 
                      v-for="channel in notificationChannels" 
                      :key="channel.id" 
                      :label="channel.name" 
                      :value="channel.id"
                    >
                      <div class="channel-option">
                        <span 
                          class="channel-type-dot" 
                          :style="{ backgroundColor: getChannelTypeColor(channel.id) }"
                        />
                        <span>{{ channel.name }}</span>
                        <el-tag size="small" type="info" style="margin-left: auto">
                          {{ channel.type }}
                        </el-tag>
                      </div>
                    </el-option>
                  </el-select>
                  <div class="selected-channels" v-if="receiver.channelIds.length > 0">
                    <el-tag 
                      v-for="channelId in receiver.channelIds" 
                      :key="channelId"
                      size="small"
                      closable
                      @close="receiver.channelIds = receiver.channelIds.filter(id => id !== channelId)"
                      :style="{ borderColor: getChannelTypeColor(channelId) }"
                    >
                      {{ getChannelName(channelId) }}
                    </el-tag>
                  </div>
                </el-form-item>
              </div>
            </div>
            
            <div v-else class="no-receivers">
              <el-empty description="No receivers configured. Click 'Add Receiver' to start." :image-size="40">
                <router-link to="/alerts/channels" class="manage-channels-link">
                  <el-button type="primary" text size="small">
                    <el-icon><Setting /></el-icon>
                    Manage Notification Channels
                  </el-button>
                </router-link>
              </el-empty>
            </div>
          </template>
          
          <div v-else class="routing-disabled">
            <el-text type="info">
              Enable alert routing to configure notification channels for this rule.
            </el-text>
          </div>
        </el-card>
      </el-form>
      
      <template #footer>
        <el-button @click="showDialog = false">Cancel</el-button>
        <el-button type="primary" @click="submitForm">
          {{ dialogMode === 'create' ? 'Create' : 'Save' }} & Sync
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style lang="scss" scoped>
.metric-alert-rules {
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
    color: var(--el-color-primary);
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
  flex-direction: column;
  
  .rule-name {
    font-weight: 500;
  }
  
  .rule-meta {
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }
}

.table-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 16px;
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

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.rule-group {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
  
  &:last-child {
    margin-bottom: 0;
  }
}

.group-header {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.group-rules {
  padding-left: 16px;
}

.rule-item {
  background: var(--el-fill-color-light);
  padding: 16px;
  border-radius: 8px;
  margin-bottom: 12px;
  
  &:last-of-type {
    margin-bottom: 8px;
  }
}

.rule-item-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.add-rule-btn {
  margin-top: 8px;
}

// Alert Routing styles
.section-title-group {
  display: flex;
  align-items: center;
}

.receiver-item {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 12px;
  
  &:last-child {
    margin-bottom: 0;
  }
}

.receiver-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  
  .receiver-name-input {
    flex: 1;
    margin-right: 16px;
    margin-bottom: 12px;
  }
}

.channel-option {
  display: flex;
  align-items: center;
  gap: 8px;
  
  .channel-type-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }
}

.selected-channels {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
}

.no-receivers {
  text-align: center;
  padding: 20px;
  
  .manage-channels-link {
    display: inline-block;
    margin-top: 8px;
  }
}

.routing-disabled {
  text-align: center;
  padding: 20px;
  color: var(--el-text-color-secondary);
}
</style>

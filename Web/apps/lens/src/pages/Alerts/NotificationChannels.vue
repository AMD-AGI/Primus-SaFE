<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  notificationChannelsApi,
  type NotificationChannel,
  type NotificationChannelType,
  type ChannelTypeInfo,
  type ListChannelsParams
} from '@/services/alerts'

// State
const loading = ref(false)
const channels = ref<NotificationChannel[]>([])
const channelTypes = ref<ChannelTypeInfo[]>([])
const total = ref(0)
const showDialog = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const testing = ref(false)

// Filters
const filters = reactive<ListChannelsParams>({
  type: undefined,
  enabled: undefined,
  search: '',
  offset: 0,
  limit: 20
})

// Form
const formRef = ref()
const form = reactive<Partial<NotificationChannel>>({
  name: '',
  type: 'email',
  enabled: true,
  description: '',
  config: {}
})

const formRules = {
  name: [{ required: true, message: 'Name is required', trigger: 'blur' }],
  type: [{ required: true, message: 'Type is required', trigger: 'change' }]
}

// Pagination
const currentPage = computed({
  get: () => Math.floor((filters.offset || 0) / (filters.limit || 20)) + 1,
  set: (val: number) => {
    filters.offset = (val - 1) * (filters.limit || 20)
  }
})

// Type icons and colors
const typeConfig: Record<NotificationChannelType, { icon: string; color: string; label: string }> = {
  email: { icon: 'Message', color: '#409eff', label: 'Email' },
  webhook: { icon: 'Link', color: '#67c23a', label: 'Webhook' },
  dingtalk: { icon: 'ChatDotRound', color: '#5a9cf8', label: 'DingTalk' },
  wechat: { icon: 'ChatLineRound', color: '#07c160', label: 'WeChat' },
  slack: { icon: 'Comment', color: '#4a154b', label: 'Slack' },
  alertmanager: { icon: 'Bell', color: '#e6522c', label: 'AlertManager' }
}

// Fetch channels
async function fetchChannels() {
  loading.value = true
  try {
    const response = await notificationChannelsApi.list(filters)
    channels.value = response?.data || []
    total.value = response?.total || 0
  } catch (error) {
    console.error('Failed to fetch notification channels:', error)
    ElMessage.error('Failed to fetch channels')
  } finally {
    loading.value = false
  }
}

// Fetch channel types
async function fetchChannelTypes() {
  try {
    channelTypes.value = await notificationChannelsApi.getTypes()
  } catch (error) {
    console.error('Failed to fetch channel types:', error)
  }
}

// Dialog actions
function openCreateDialog() {
  dialogMode.value = 'create'
  resetForm()
  showDialog.value = true
}

function openEditDialog(channel: NotificationChannel) {
  dialogMode.value = 'edit'
  Object.assign(form, {
    id: channel.id,
    name: channel.name,
    type: channel.type,
    enabled: channel.enabled,
    description: channel.description,
    config: JSON.parse(JSON.stringify(channel.config))
  })
  showDialog.value = true
}

function resetForm() {
  form.id = undefined
  form.name = ''
  form.type = 'email'
  form.enabled = true
  form.description = ''
  form.config = getDefaultConfig('email')
}

function getDefaultConfig(type: NotificationChannelType): Record<string, any> {
  switch (type) {
    case 'email':
      return { smtp_host: '', smtp_port: 587, from: '', use_starttls: true }
    case 'webhook':
      return { url: '', method: 'POST', timeout: 30 }
    case 'dingtalk':
      return { webhook_url: '' }
    case 'slack':
      return { webhook_url: '' }
    case 'wechat':
      return { corp_id: '', agent_id: 0, secret: '' }
    case 'alertmanager':
      return { url: '' }
    default:
      return {}
  }
}

function onTypeChange(type: NotificationChannelType) {
  form.config = getDefaultConfig(type)
}

async function submitForm() {
  if (!formRef.value) return
  
  await formRef.value.validate(async (valid: boolean) => {
    if (!valid) return
    
    try {
      if (dialogMode.value === 'create') {
        await notificationChannelsApi.create(form)
        ElMessage.success('Channel created successfully')
      } else {
        await notificationChannelsApi.update(form.id!, form)
        ElMessage.success('Channel updated successfully')
      }
      
      showDialog.value = false
      fetchChannels()
    } catch (error) {
      console.error('Failed to save channel:', error)
      ElMessage.error('Failed to save channel')
    }
  })
}

// Actions
async function deleteChannel(channel: NotificationChannel) {
  try {
    await ElMessageBox.confirm(
      `Are you sure to delete channel "${channel.name}"?`,
      'Confirm Delete',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    
    await notificationChannelsApi.delete(channel.id)
    ElMessage.success('Channel deleted successfully')
    fetchChannels()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to delete channel')
    }
  }
}

async function toggleEnabled(channel: NotificationChannel) {
  try {
    await notificationChannelsApi.update(channel.id, { enabled: !channel.enabled })
    channel.enabled = !channel.enabled
    ElMessage.success(`Channel ${channel.enabled ? 'enabled' : 'disabled'}`)
  } catch (error) {
    ElMessage.error('Failed to update channel')
  }
}

async function testChannel(channel: NotificationChannel) {
  testing.value = true
  try {
    const result = await notificationChannelsApi.test(channel.id)
    if (result.success) {
      ElMessage.success('Test notification sent successfully')
    } else {
      ElMessage.warning(result.message || 'Test failed')
    }
  } catch (error) {
    ElMessage.error('Failed to test channel')
  } finally {
    testing.value = false
  }
}

// Utility
function formatTime(timestamp?: string) {
  if (!timestamp) return '-'
  return new Date(timestamp).toLocaleString()
}

function getConfigSummary(channel: NotificationChannel): string {
  const config = channel.config
  switch (channel.type) {
    case 'email':
      return `${config.smtp_host}:${config.smtp_port}`
    case 'webhook':
      return config.url ? new URL(config.url).hostname : '-'
    case 'dingtalk':
    case 'slack':
      return config.webhook_url ? 'Webhook configured' : '-'
    case 'wechat':
      return config.corp_id || '-'
    case 'alertmanager':
      return config.url ? new URL(config.url).hostname : '-'
    default:
      return '-'
  }
}

onMounted(() => {
  fetchChannels()
  fetchChannelTypes()
})
</script>

<template>
  <div class="notification-channels">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">
          <el-icon class="title-icon"><Bell /></el-icon>
          Notification Channels
        </h1>
        <p class="page-subtitle">Manage reusable notification configurations for alerts</p>
      </div>
      <div class="header-right">
        <el-button type="primary" @click="openCreateDialog">
          <el-icon><Plus /></el-icon>
          Create Channel
        </el-button>
      </div>
    </div>

    <!-- Filters -->
    <el-card class="filter-card" shadow="hover">
      <el-form :inline="true" :model="filters">
        <el-form-item label="Type">
          <el-select v-model="filters.type" placeholder="All Types" clearable style="width: 140px">
            <el-option label="All Types" :value="undefined" />
            <el-option v-for="t in channelTypes" :key="t.type" :label="t.name" :value="t.type" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Status">
          <el-select v-model="filters.enabled" placeholder="All" clearable style="width: 120px">
            <el-option label="All" :value="undefined" />
            <el-option label="Enabled" :value="true" />
            <el-option label="Disabled" :value="false" />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Search">
          <el-input
            v-model="filters.search"
            placeholder="Search by name..."
            clearable
            style="width: 200px"
            @keyup.enter="fetchChannels"
          >
            <template #prefix>
              <el-icon><Search /></el-icon>
            </template>
          </el-input>
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="fetchChannels">Search</el-button>
          <el-button @click="filters.search = ''; filters.type = undefined; filters.enabled = undefined; fetchChannels()">
            Reset
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Channels Grid -->
    <div class="channels-grid" v-loading="loading">
      <el-card 
        v-for="channel in channels" 
        :key="channel.id" 
        class="channel-card"
        shadow="hover"
      >
        <template #header>
          <div class="channel-header">
            <div class="channel-info">
              <div class="channel-type" :style="{ backgroundColor: typeConfig[channel.type]?.color + '20' }">
                <el-icon :style="{ color: typeConfig[channel.type]?.color }">
                  <component :is="typeConfig[channel.type]?.icon" />
                </el-icon>
              </div>
              <div class="channel-name-wrapper">
                <span class="channel-name">{{ channel.name }}</span>
                <el-tag :type="channel.enabled ? 'success' : 'info'" size="small">
                  {{ channel.enabled ? 'Enabled' : 'Disabled' }}
                </el-tag>
              </div>
            </div>
            <el-dropdown trigger="click">
              <el-button type="text" circle>
                <el-icon><MoreFilled /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item @click="openEditDialog(channel)">
                    <el-icon><Edit /></el-icon>Edit
                  </el-dropdown-item>
                  <el-dropdown-item @click="testChannel(channel)">
                    <el-icon><VideoPlay /></el-icon>Test
                  </el-dropdown-item>
                  <el-dropdown-item @click="toggleEnabled(channel)">
                    <el-icon><Switch /></el-icon>{{ channel.enabled ? 'Disable' : 'Enable' }}
                  </el-dropdown-item>
                  <el-dropdown-item divided @click="deleteChannel(channel)" style="color: var(--el-color-danger)">
                    <el-icon><Delete /></el-icon>Delete
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </template>
        
        <div class="channel-content">
          <div class="channel-type-label">
            <el-tag size="small" effect="plain">{{ typeConfig[channel.type]?.label }}</el-tag>
          </div>
          
          <div class="channel-config">
            <span class="config-label">Configuration:</span>
            <span class="config-value">{{ getConfigSummary(channel) }}</span>
          </div>
          
          <div class="channel-description" v-if="channel.description">
            {{ channel.description }}
          </div>
          
          <div class="channel-meta">
            <span>Created: {{ formatTime(channel.created_at) }}</span>
          </div>
        </div>
      </el-card>
      
      <!-- Empty State -->
      <div v-if="channels.length === 0 && !loading" class="empty-state">
        <el-empty description="No notification channels found">
          <el-button type="primary" @click="openCreateDialog">Create Channel</el-button>
        </el-empty>
      </div>
    </div>

    <!-- Pagination -->
    <div class="pagination-wrapper" v-if="total > filters.limit!">
      <el-pagination
        v-model:current-page="currentPage"
        :page-size="filters.limit"
        :total="total"
        layout="total, prev, pager, next"
        @current-change="fetchChannels"
      />
    </div>

    <!-- Create/Edit Dialog -->
    <el-dialog
      v-model="showDialog"
      :title="dialogMode === 'create' ? 'Create Notification Channel' : 'Edit Notification Channel'"
      width="600px"
      destroy-on-close
    >
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="120px">
        <el-form-item label="Name" prop="name">
          <el-input v-model="form.name" placeholder="e.g., ops-email, dev-webhook" />
        </el-form-item>
        
        <el-form-item label="Type" prop="type">
          <el-select 
            v-model="form.type" 
            placeholder="Select type"
            :disabled="dialogMode === 'edit'"
            @change="onTypeChange"
          >
            <el-option v-for="t in channelTypes" :key="t.type" :label="t.name" :value="t.type">
              <div class="type-option">
                <el-icon :style="{ color: typeConfig[t.type as NotificationChannelType]?.color }">
                  <component :is="typeConfig[t.type as NotificationChannelType]?.icon" />
                </el-icon>
                <span>{{ t.name }}</span>
              </div>
            </el-option>
          </el-select>
        </el-form-item>
        
        <el-form-item label="Enabled">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        
        <el-form-item label="Description">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="Optional description" />
        </el-form-item>
        
        <el-divider content-position="left">Configuration</el-divider>
        
        <!-- Email Config -->
        <template v-if="form.type === 'email'">
          <el-form-item label="SMTP Host" required>
            <el-input v-model="form.config!.smtp_host" placeholder="smtp.example.com" />
          </el-form-item>
          <el-form-item label="SMTP Port" required>
            <el-input-number v-model="form.config!.smtp_port" :min="1" :max="65535" />
          </el-form-item>
          <el-form-item label="Username">
            <el-input v-model="form.config!.username" placeholder="Optional" />
          </el-form-item>
          <el-form-item label="Password">
            <el-input v-model="form.config!.password" type="password" show-password placeholder="Optional" />
          </el-form-item>
          <el-form-item label="From" required>
            <el-input v-model="form.config!.from" placeholder="alerts@example.com" />
          </el-form-item>
          <el-form-item label="From Name">
            <el-input v-model="form.config!.from_name" placeholder="Primus-Lens Alerts" />
          </el-form-item>
          <el-form-item label="Use STARTTLS">
            <el-switch v-model="form.config!.use_starttls" />
          </el-form-item>
        </template>
        
        <!-- Webhook Config -->
        <template v-if="form.type === 'webhook'">
          <el-form-item label="URL" required>
            <el-input v-model="form.config!.url" placeholder="https://hooks.example.com/webhook" />
          </el-form-item>
          <el-form-item label="Method">
            <el-select v-model="form.config!.method">
              <el-option label="POST" value="POST" />
              <el-option label="PUT" value="PUT" />
            </el-select>
          </el-form-item>
          <el-form-item label="Timeout (s)">
            <el-input-number v-model="form.config!.timeout" :min="1" :max="300" />
          </el-form-item>
        </template>
        
        <!-- DingTalk Config -->
        <template v-if="form.type === 'dingtalk'">
          <el-form-item label="Webhook URL" required>
            <el-input v-model="form.config!.webhook_url" placeholder="https://oapi.dingtalk.com/robot/send?access_token=..." />
          </el-form-item>
          <el-form-item label="Secret">
            <el-input v-model="form.config!.secret" placeholder="SECxxx (optional)" />
          </el-form-item>
        </template>
        
        <!-- Slack Config -->
        <template v-if="form.type === 'slack'">
          <el-form-item label="Webhook URL" required>
            <el-input v-model="form.config!.webhook_url" placeholder="https://hooks.slack.com/services/xxx" />
          </el-form-item>
          <el-form-item label="Channel">
            <el-input v-model="form.config!.channel" placeholder="#alerts" />
          </el-form-item>
          <el-form-item label="Username">
            <el-input v-model="form.config!.username" placeholder="Primus-Lens" />
          </el-form-item>
        </template>
        
        <!-- WeChat Config -->
        <template v-if="form.type === 'wechat'">
          <el-form-item label="Corp ID" required>
            <el-input v-model="form.config!.corp_id" />
          </el-form-item>
          <el-form-item label="Agent ID" required>
            <el-input-number v-model="form.config!.agent_id" :min="0" />
          </el-form-item>
          <el-form-item label="Secret" required>
            <el-input v-model="form.config!.secret" type="password" show-password />
          </el-form-item>
          <el-form-item label="To User">
            <el-input v-model="form.config!.to_user" placeholder="@all or user IDs" />
          </el-form-item>
        </template>
        
        <!-- AlertManager Config -->
        <template v-if="form.type === 'alertmanager'">
          <el-form-item label="URL" required>
            <el-input v-model="form.config!.url" placeholder="http://alertmanager:9093" />
          </el-form-item>
          <el-form-item label="API Path">
            <el-input v-model="form.config!.api_path" placeholder="/api/v2/alerts" />
          </el-form-item>
          <el-form-item label="Timeout (s)">
            <el-input-number v-model="form.config!.timeout" :min="1" :max="300" />
          </el-form-item>
        </template>
      </el-form>
      
      <template #footer>
        <el-button @click="showDialog = false">Cancel</el-button>
        <el-button type="primary" @click="submitForm">
          {{ dialogMode === 'create' ? 'Create' : 'Save' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.notification-channels {
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 20px;
  
  .header-left {
    .page-title {
      display: flex;
      align-items: center;
      gap: 10px;
      margin: 0;
      font-size: 24px;
      font-weight: 600;
      
      .title-icon {
        color: var(--el-color-primary);
      }
    }
    
    .page-subtitle {
      margin: 8px 0 0 34px;
      color: var(--el-text-color-secondary);
      font-size: 14px;
    }
  }
}

.filter-card {
  margin-bottom: 20px;
  
  :deep(.el-card__body) {
    padding: 15px 20px;
  }
  
  :deep(.el-form-item) {
    margin-bottom: 0;
  }
}

.channels-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 20px;
  min-height: 200px;
}

.channel-card {
  .channel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    
    .channel-info {
      display: flex;
      align-items: center;
      gap: 12px;
      
      .channel-type {
        width: 40px;
        height: 40px;
        border-radius: 10px;
        display: flex;
        align-items: center;
        justify-content: center;
        
        .el-icon {
          font-size: 20px;
        }
      }
      
      .channel-name-wrapper {
        display: flex;
        flex-direction: column;
        gap: 4px;
        
        .channel-name {
          font-weight: 600;
          font-size: 16px;
        }
      }
    }
  }
  
  .channel-content {
    .channel-type-label {
      margin-bottom: 12px;
    }
    
    .channel-config {
      display: flex;
      gap: 8px;
      margin-bottom: 10px;
      font-size: 13px;
      
      .config-label {
        color: var(--el-text-color-secondary);
      }
      
      .config-value {
        font-family: monospace;
        color: var(--el-text-color-primary);
      }
    }
    
    .channel-description {
      color: var(--el-text-color-secondary);
      font-size: 13px;
      margin-bottom: 12px;
      line-height: 1.5;
    }
    
    .channel-meta {
      font-size: 12px;
      color: var(--el-text-color-placeholder);
    }
  }
}

.empty-state {
  grid-column: 1 / -1;
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 300px;
}

.pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 20px;
}

.type-option {
  display: flex;
  align-items: center;
  gap: 8px;
}

:deep(.el-divider__text) {
  font-weight: 500;
  color: var(--el-text-color-secondary);
}
</style>

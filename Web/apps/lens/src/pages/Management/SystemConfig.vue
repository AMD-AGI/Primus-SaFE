<template>
  <div class="system-config-page">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h3 class="page-title">System Configuration</h3>
        <p class="page-description">Manage system-wide configuration settings</p>
      </div>
      <div class="header-right">
        <el-button @click="handleRefresh" :icon="Refresh" :loading="loading">
          Refresh
        </el-button>
        <el-button type="primary" @click="showAddDialog" :icon="Plus">
          Add Config
        </el-button>
      </div>
    </div>

    <!-- Registry Config Card -->
    <el-card class="config-card registry-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <div class="header-info">
            <el-icon class="header-icon"><Box /></el-icon>
            <div>
              <h4>Container Registry</h4>
              <span class="header-description">Configure image registry for TraceLens and Perfetto pods</span>
            </div>
          </div>
          <el-button type="primary" size="small" @click="showRegistryDialog" :icon="Setting">
            Configure
          </el-button>
        </div>
      </template>
      
      <div class="registry-info" v-if="registryConfig">
        <div class="info-grid">
          <div class="info-item">
            <label>Registry:</label>
            <el-tag>{{ registryConfig.registry || 'docker.io' }}</el-tag>
          </div>
          <div class="info-item">
            <label>Namespace:</label>
            <el-tag type="info">{{ registryConfig.namespace || 'primussafe' }}</el-tag>
          </div>
          <div class="info-item" v-if="registryConfig.harborExternalUrl">
            <label>Harbor URL:</label>
            <span>{{ registryConfig.harborExternalUrl }}</span>
          </div>
        </div>
        
        <div class="image-versions" v-if="registryConfig.imageVersions && Object.keys(registryConfig.imageVersions).length">
          <h5>Image Versions:</h5>
          <div class="version-tags">
            <el-tag 
              v-for="(version, image) in registryConfig.imageVersions" 
              :key="image"
              type="success"
              class="version-tag"
            >
              {{ image }}: {{ version }}
            </el-tag>
          </div>
        </div>
      </div>
      <el-empty v-else description="No registry configuration" :image-size="60" />
    </el-card>

    <!-- All Configs Table -->
    <el-card class="config-card" shadow="hover">
      <template #header>
        <div class="card-header">
          <div class="header-info">
            <el-icon class="header-icon"><Setting /></el-icon>
            <div>
              <h4>All Configurations</h4>
              <span class="header-description">View and manage all system configuration entries</span>
            </div>
          </div>
          <el-select 
            v-model="selectedCategory" 
            placeholder="Filter by category"
            style="width: 180px"
            clearable
            @change="handleCategoryChange"
          >
            <el-option
              v-for="cat in CONFIG_CATEGORIES"
              :key="cat.value"
              :label="cat.label"
              :value="cat.value"
            />
          </el-select>
        </div>
      </template>
      
      <el-table 
        :data="configs" 
        v-loading="loading"
        stripe
        style="width: 100%"
      >
        <el-table-column prop="key" label="Key" min-width="200">
          <template #default="{ row }">
            <div class="config-key">
              <span class="key-text">{{ row.key }}</span>
              <el-tag v-if="row.isReadonly" size="small" type="info">readonly</el-tag>
            </div>
          </template>
        </el-table-column>
        
        <el-table-column prop="category" label="Category" width="140">
          <template #default="{ row }">
            <el-tag v-if="row.category" type="info" size="small">{{ row.category }}</el-tag>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        
        <el-table-column prop="description" label="Description" min-width="200">
          <template #default="{ row }">
            <span v-if="row.description">{{ row.description }}</span>
            <span v-else class="text-muted">No description</span>
          </template>
        </el-table-column>
        
        <el-table-column prop="version" label="Version" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small">v{{ row.version }}</el-tag>
          </template>
        </el-table-column>
        
        <el-table-column prop="updatedAt" label="Last Updated" width="180">
          <template #default="{ row }">
            <el-tooltip :content="formatFullTime(row.updatedAt)" placement="top">
              <span>{{ formatRelativeTime(row.updatedAt) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        
        <el-table-column label="Actions" width="150" fixed="right">
          <template #default="{ row }">
            <el-button-group>
              <el-button size="small" @click="handleView(row)" :icon="View">
                View
              </el-button>
              <el-button 
                size="small" 
                type="primary" 
                @click="handleEdit(row)" 
                :icon="Edit"
                :disabled="row.isReadonly"
              >
                Edit
              </el-button>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Registry Config Dialog -->
    <el-dialog 
      v-model="registryDialogVisible" 
      title="Container Registry Configuration"
      width="600px"
    >
      <el-form :model="registryForm" label-position="top">
        <el-form-item label="Registry Host">
          <el-input 
            v-model="registryForm.registry" 
            placeholder="e.g., harbor.example.com or docker.io"
          />
          <div class="form-hint">Default: docker.io</div>
        </el-form-item>
        
        <el-form-item label="Namespace">
          <el-input 
            v-model="registryForm.namespace" 
            placeholder="e.g., primussafe"
          />
          <div class="form-hint">Default: primussafe</div>
        </el-form-item>
        
        <el-form-item label="Harbor External URL (optional)">
          <el-input 
            v-model="registryForm.harborExternalUrl" 
            placeholder="https://harbor.example.com"
          />
        </el-form-item>
        
        <el-divider>Image Versions</el-divider>
        
        <el-form-item label="TraceLens Version">
          <el-input 
            v-model="registryForm.tracelensVersion" 
            placeholder="e.g., latest or 202501051200"
          />
        </el-form-item>
        
        <el-form-item label="Perfetto Viewer Version">
          <el-input 
            v-model="registryForm.perfettoVersion" 
            placeholder="e.g., latest or 202501051200"
          />
        </el-form-item>
        
        <el-divider>Quick Sync</el-divider>
        
        <el-form-item label="Sync from Harbor">
          <el-input 
            v-model="harborSyncUrl" 
            placeholder="Paste Harbor URL to auto-fill registry host"
          >
            <template #append>
              <el-button @click="handleSyncFromHarbor" :loading="syncing">
                Sync
              </el-button>
            </template>
          </el-input>
        </el-form-item>
      </el-form>
      
      <template #footer>
        <el-button @click="registryDialogVisible = false">Cancel</el-button>
        <el-button type="primary" @click="saveRegistryConfig" :loading="saving">
          Save
        </el-button>
      </template>
    </el-dialog>

    <!-- View/Edit Config Dialog -->
    <el-dialog 
      v-model="configDialogVisible" 
      :title="isEditing ? 'Edit Configuration' : (isAdding ? 'Add Configuration' : 'View Configuration')"
      width="700px"
    >
      <el-form :model="configForm" label-position="top">
        <el-form-item label="Key" :required="isAdding">
          <el-input 
            v-model="configForm.key" 
            :disabled="!isAdding"
            placeholder="e.g., my_config_key"
          />
        </el-form-item>
        
        <el-form-item label="Category">
          <el-select v-model="configForm.category" :disabled="!isEditing && !isAdding" style="width: 100%">
            <el-option
              v-for="cat in CONFIG_CATEGORIES.filter(c => c.value)"
              :key="cat.value"
              :label="cat.label"
              :value="cat.value"
            />
          </el-select>
        </el-form-item>
        
        <el-form-item label="Description">
          <el-input 
            v-model="configForm.description" 
            :disabled="!isEditing && !isAdding"
            placeholder="Description of this configuration"
          />
        </el-form-item>
        
        <el-form-item label="Value (JSON)">
          <el-input 
            v-model="configForm.valueJson" 
            type="textarea"
            :rows="10"
            :disabled="!isEditing && !isAdding"
            placeholder='{"key": "value"}'
            class="json-editor"
          />
          <div class="form-hint" v-if="jsonError">
            <el-text type="danger">{{ jsonError }}</el-text>
          </div>
        </el-form-item>
      </el-form>
      
      <template #footer>
        <el-button @click="configDialogVisible = false">
          {{ isEditing || isAdding ? 'Cancel' : 'Close' }}
        </el-button>
        <el-button 
          v-if="isEditing || isAdding" 
          type="primary" 
          @click="saveConfig" 
          :loading="saving"
          :disabled="!!jsonError"
        >
          Save
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Plus, Setting, Box, View, Edit } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import { useGlobalCluster } from '@/composables/useGlobalCluster'
import {
  listSystemConfigs,
  getSystemConfig,
  setSystemConfig,
  getRegistryConfig,
  setRegistryConfig,
  syncFromHarbor,
  CONFIG_CATEGORIES,
  type SystemConfig,
  type RegistryConfig
} from '@/services/system-config'

dayjs.extend(relativeTime)

const { selectedCluster } = useGlobalCluster()

// State
const loading = ref(false)
const saving = ref(false)
const syncing = ref(false)
const configs = ref<SystemConfig[]>([])
const registryConfig = ref<RegistryConfig | null>(null)
const selectedCategory = ref('')

// Registry dialog
const registryDialogVisible = ref(false)
const registryForm = ref({
  registry: '',
  namespace: '',
  harborExternalUrl: '',
  tracelensVersion: '',
  perfettoVersion: ''
})
const harborSyncUrl = ref('')

// Config dialog
const configDialogVisible = ref(false)
const isEditing = ref(false)
const isAdding = ref(false)
const configForm = ref({
  key: '',
  category: '',
  description: '',
  valueJson: ''
})

// Computed
const jsonError = computed(() => {
  if (!configForm.value.valueJson) return ''
  try {
    JSON.parse(configForm.value.valueJson)
    return ''
  } catch (e: any) {
    return `Invalid JSON: ${e.message}`
  }
})

// Methods
function formatRelativeTime(time: string): string {
  if (!time) return '-'
  return dayjs(time).fromNow()
}

function formatFullTime(time: string): string {
  if (!time) return '-'
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

async function loadConfigs() {
  loading.value = true
  try {
    const [configList, regConfig] = await Promise.all([
      listSystemConfigs(selectedCluster.value, selectedCategory.value || undefined),
      getRegistryConfig(selectedCluster.value).catch(() => null)
    ])
    configs.value = configList || []
    if (regConfig) {
      registryConfig.value = regConfig.config
    }
  } catch (error: any) {
    console.error('Failed to load configs:', error)
    ElMessage.error('Failed to load configurations')
  } finally {
    loading.value = false
  }
}

function handleRefresh() {
  loadConfigs()
}

function handleCategoryChange() {
  loadConfigs()
}

function showRegistryDialog() {
  if (registryConfig.value) {
    registryForm.value = {
      registry: registryConfig.value.registry || '',
      namespace: registryConfig.value.namespace || '',
      harborExternalUrl: registryConfig.value.harborExternalUrl || '',
      tracelensVersion: registryConfig.value.imageVersions?.tracelens || '',
      perfettoVersion: registryConfig.value.imageVersions?.['perfetto-viewer'] || ''
    }
  } else {
    registryForm.value = {
      registry: '',
      namespace: '',
      harborExternalUrl: '',
      tracelensVersion: '',
      perfettoVersion: ''
    }
  }
  harborSyncUrl.value = ''
  registryDialogVisible.value = true
}

async function handleSyncFromHarbor() {
  if (!harborSyncUrl.value) {
    ElMessage.warning('Please enter a Harbor URL')
    return
  }
  
  syncing.value = true
  try {
    const result = await syncFromHarbor(harborSyncUrl.value, selectedCluster.value)
    registryForm.value.registry = result.config.registry
    registryForm.value.namespace = result.config.namespace || 'primussafe'
    registryForm.value.harborExternalUrl = result.config.harborExternalUrl || harborSyncUrl.value
    ElMessage.success('Synced from Harbor')
  } catch (error: any) {
    ElMessage.error(`Failed to sync: ${error.message}`)
  } finally {
    syncing.value = false
  }
}

async function saveRegistryConfig() {
  saving.value = true
  try {
    const imageVersions: Record<string, string> = {}
    if (registryForm.value.tracelensVersion) {
      imageVersions.tracelens = registryForm.value.tracelensVersion
    }
    if (registryForm.value.perfettoVersion) {
      imageVersions['perfetto-viewer'] = registryForm.value.perfettoVersion
    }
    
    const config: RegistryConfig = {
      registry: registryForm.value.registry || 'docker.io',
      namespace: registryForm.value.namespace || 'primussafe',
      harborExternalUrl: registryForm.value.harborExternalUrl,
      imageVersions: Object.keys(imageVersions).length ? imageVersions : undefined
    }
    
    await setRegistryConfig(config, selectedCluster.value)
    ElMessage.success('Registry configuration saved')
    registryDialogVisible.value = false
    loadConfigs()
  } catch (error: any) {
    ElMessage.error(`Failed to save: ${error.message}`)
  } finally {
    saving.value = false
  }
}

function handleView(row: SystemConfig) {
  isEditing.value = false
  isAdding.value = false
  configForm.value = {
    key: row.key,
    category: row.category || '',
    description: row.description || '',
    valueJson: JSON.stringify(row.value, null, 2)
  }
  configDialogVisible.value = true
}

function handleEdit(row: SystemConfig) {
  isEditing.value = true
  isAdding.value = false
  configForm.value = {
    key: row.key,
    category: row.category || '',
    description: row.description || '',
    valueJson: JSON.stringify(row.value, null, 2)
  }
  configDialogVisible.value = true
}

function showAddDialog() {
  isEditing.value = false
  isAdding.value = true
  configForm.value = {
    key: '',
    category: 'general',
    description: '',
    valueJson: '{}'
  }
  configDialogVisible.value = true
}

async function saveConfig() {
  if (jsonError.value) {
    ElMessage.error('Please fix JSON errors first')
    return
  }
  
  if (isAdding.value && !configForm.value.key) {
    ElMessage.error('Key is required')
    return
  }
  
  saving.value = true
  try {
    const value = JSON.parse(configForm.value.valueJson)
    await setSystemConfig(
      configForm.value.key,
      value,
      selectedCluster.value,
      {
        description: configForm.value.description,
        category: configForm.value.category
      }
    )
    ElMessage.success('Configuration saved')
    configDialogVisible.value = false
    loadConfigs()
  } catch (error: any) {
    ElMessage.error(`Failed to save: ${error.message}`)
  } finally {
    saving.value = false
  }
}

// Watch cluster change
watch(() => selectedCluster.value, () => {
  loadConfigs()
})

onMounted(() => {
  loadConfigs()
})
</script>

<style scoped lang="scss">
.system-config-page {
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 24px;
    
    .header-left {
      .page-title {
        margin: 0 0 4px 0;
        font-size: 20px;
        font-weight: 600;
        color: var(--el-text-color-primary);
      }
      
      .page-description {
        margin: 0;
        font-size: 14px;
        color: var(--el-text-color-secondary);
      }
    }
    
    .header-right {
      display: flex;
      gap: 12px;
    }
  }
  
  .config-card {
    margin-bottom: 20px;
    
    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      
      .header-info {
        display: flex;
        align-items: center;
        gap: 12px;
        
        .header-icon {
          font-size: 24px;
          color: var(--el-color-primary);
        }
        
        h4 {
          margin: 0;
          font-size: 16px;
          font-weight: 600;
        }
        
        .header-description {
          font-size: 13px;
          color: var(--el-text-color-secondary);
        }
      }
    }
  }
  
  .registry-card {
    .registry-info {
      .info-grid {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
        gap: 16px;
        margin-bottom: 16px;
        
        .info-item {
          display: flex;
          align-items: center;
          gap: 8px;
          
          label {
            font-size: 13px;
            color: var(--el-text-color-secondary);
          }
        }
      }
      
      .image-versions {
        h5 {
          margin: 0 0 8px 0;
          font-size: 14px;
          color: var(--el-text-color-regular);
        }
        
        .version-tags {
          display: flex;
          flex-wrap: wrap;
          gap: 8px;
          
          .version-tag {
            font-family: 'SF Mono', Monaco, monospace;
          }
        }
      }
    }
  }
  
  .config-key {
    display: flex;
    align-items: center;
    gap: 8px;
    
    .key-text {
      font-family: 'SF Mono', Monaco, monospace;
      font-size: 13px;
    }
  }
  
  .text-muted {
    color: var(--el-text-color-placeholder);
  }
  
  .form-hint {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-top: 4px;
  }
  
  .json-editor {
    :deep(textarea) {
      font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
      font-size: 13px;
    }
  }
}
</style>


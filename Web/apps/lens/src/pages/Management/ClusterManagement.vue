<template>
  <div class="cluster-management-page">
    <div class="page-header">
      <h1>Cluster Management</h1>
      <p class="subtitle">Manage registered clusters and their configurations</p>
    </div>

    <!-- Actions Bar -->
    <div class="actions-bar">
      <div class="actions-left">
        <el-button type="primary" @click="showAddClusterDialog">
          <el-icon><Plus /></el-icon>
          Add Cluster
        </el-button>
        <el-button @click="refreshClusters" :loading="loading">
          <el-icon><Refresh /></el-icon>
          Refresh
        </el-button>
      </div>
      <div class="actions-right">
        <el-tag type="info" size="large" class="installer-tag">
          <el-icon><Box /></el-icon>
          Installer: {{ installerImage || 'loading...' }}
        </el-tag>
        <el-button @click="showInstallerConfigDialog">
          <el-icon><Setting /></el-icon>
          Settings
        </el-button>
      </div>
    </div>

    <!-- Clusters Table -->
    <el-card class="clusters-card">
      <el-table
        :data="clusters"
        v-loading="loading"
        stripe
        style="width: 100%"
      >
        <el-table-column prop="cluster_name" label="Cluster Name" min-width="150">
          <template #default="{ row }">
            <div class="cluster-name-cell">
              <span class="name">{{ row.cluster_name }}</span>
              <el-tag v-if="row.is_default" size="small" type="success">Default</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="display_name" label="Display Name" min-width="120" />
        <el-table-column label="K8S" width="80" align="center">
          <template #default="{ row }">
            <el-icon :class="row.k8s_configured ? 'status-ok' : 'status-na'">
              <CircleCheck v-if="row.k8s_configured" />
              <CircleClose v-else />
            </el-icon>
          </template>
        </el-table-column>
        <el-table-column label="Postgres" width="100" align="center">
          <template #default="{ row }">
            <el-tooltip :content="getStorageTooltip(row, 'postgres')" placement="top">
              <el-icon :class="isStorageConfigured(row, 'postgres') ? 'status-ok' : 'status-na'">
                <CircleCheck v-if="isStorageConfigured(row, 'postgres')" />
                <CircleClose v-else />
              </el-icon>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="Prometheus" width="110" align="center">
          <template #default="{ row }">
            <el-tooltip :content="getStorageTooltip(row, 'prometheus')" placement="top">
              <el-icon :class="isStorageConfigured(row, 'prometheus') ? 'status-ok' : 'status-na'">
                <CircleCheck v-if="isStorageConfigured(row, 'prometheus')" />
                <CircleClose v-else />
              </el-icon>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="OpenSearch" width="110" align="center">
          <template #default="{ row }">
            <el-tooltip :content="getStorageTooltip(row, 'opensearch')" placement="top">
              <el-icon :class="isStorageConfigured(row, 'opensearch') ? 'status-ok' : 'status-na'">
                <CircleCheck v-if="isStorageConfigured(row, 'opensearch')" />
                <CircleClose v-else />
              </el-icon>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="storage_mode" label="Storage Mode" width="130">
          <template #default="{ row }">
            <el-tag :type="row.storage_mode === 'lens-managed' ? 'success' : 'info'" size="small">
              {{ row.storage_mode || 'external' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Infrastructure" width="150">
          <template #default="{ row }">
            <div class="infra-status-cell">
              <el-tag :type="getInfraStatusType(row.infrastructure_status)" size="small">
                {{ row.infrastructure_status || 'not_initialized' }}
              </el-tag>
              <el-button 
                v-if="row.infrastructure_status === 'not_initialized' || row.infrastructure_status === 'failed'"
                size="small" 
                type="primary"
                @click="initializeInfrastructure(row)"
                :loading="initializingCluster === row.cluster_name"
              >
                Init
              </el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="dataplane_status" label="Dataplane" width="120">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.dataplane_status)" size="small">
              {{ row.dataplane_status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="source" label="Source" width="100">
          <template #default="{ row }">
            <el-tag type="info" size="small">{{ row.source }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Manual Mode" width="130" align="center">
          <template #default="{ row }">
            <div class="manual-mode-badges">
              <el-tooltip v-if="row.k8s_manual_mode" content="K8S config is manually managed" placement="top">
                <el-tag size="small" type="warning">K8S</el-tag>
              </el-tooltip>
              <el-tooltip v-if="row.storage_manual_mode" content="Storage config is manually managed" placement="top">
                <el-tag size="small" type="warning">Storage</el-tag>
              </el-tooltip>
              <span v-if="!row.k8s_manual_mode && !row.storage_manual_mode" class="auto-mode">Auto</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="250" fixed="right">
          <template #default="{ row }">
            <el-button-group>
              <el-tooltip content="Edit Cluster" placement="top">
                <el-button size="small" @click="editCluster(row)">
                  <el-icon><Edit /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="Test Connection" placement="top">
                <el-button size="small" @click="testConnection(row)" :loading="testingCluster === row.cluster_name">
                  <el-icon><Connection /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="View Tasks" placement="top">
                <el-button size="small" @click="viewTasks(row)">
                  <el-icon><Document /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="Set as Default" placement="top">
                <el-button size="small" @click="setAsDefault(row)" :disabled="row.is_default">
                  <el-icon><Star /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="Delete Cluster" placement="top">
                <el-button size="small" type="danger" @click="confirmDelete(row)">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </el-tooltip>
            </el-button-group>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Add/Edit Cluster Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? 'Edit Cluster' : 'Add Cluster'"
      width="700px"
      destroy-on-close
    >
      <el-form :model="clusterForm" :rules="formRules" ref="formRef" label-width="160px">
        <el-tabs v-model="activeTab">
          <el-tab-pane label="Basic Info" name="basic">
            <el-form-item label="Cluster Name" prop="cluster_name">
              <el-input v-model="clusterForm.cluster_name" :disabled="isEditing" placeholder="e.g., prod-cluster-1" />
            </el-form-item>
            <el-form-item label="Display Name" prop="display_name">
              <el-input v-model="clusterForm.display_name" placeholder="Human-readable name" />
            </el-form-item>
            <el-form-item label="Description" prop="description">
              <el-input v-model="clusterForm.description" type="textarea" :rows="2" placeholder="Optional description" />
            </el-form-item>
            <el-form-item label="Storage Mode" prop="storage_mode">
              <el-radio-group v-model="clusterForm.storage_mode">
                <el-radio label="external">External (pre-configured)</el-radio>
                <el-radio label="lens-managed">Lens Managed</el-radio>
              </el-radio-group>
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane label="Kubernetes" name="k8s">
            <el-form-item label="Manual Mode">
              <el-switch v-model="clusterForm.k8s_manual_mode" />
              <span class="form-hint">When enabled, K8S config won't be overwritten by Primus-SaFE sync</span>
            </el-form-item>
            <el-form-item label="Skip TLS Verify">
              <el-switch v-model="clusterForm.k8s_insecure_skip_verify" />
              <span class="form-hint">Skip TLS certificate verification (not recommended for production)</span>
            </el-form-item>
            <el-form-item label="API Endpoint" prop="k8s_endpoint">
              <el-input v-model="clusterForm.k8s_endpoint" placeholder="https://kubernetes.example.com:6443" />
            </el-form-item>
            <el-form-item label="CA Certificate">
              <el-input v-model="clusterForm.k8s_ca_data" type="textarea" :rows="3" placeholder="Base64 encoded CA certificate" />
            </el-form-item>
            <el-divider>Authentication (choose one)</el-divider>
            <el-form-item label="Bearer Token">
              <el-input v-model="clusterForm.k8s_token" type="password" show-password placeholder="Leave empty to keep existing" />
            </el-form-item>
            <el-form-item label="Client Certificate">
              <el-input v-model="clusterForm.k8s_cert_data" type="textarea" :rows="3" placeholder="Base64 encoded certificate (leave empty to keep existing)" />
            </el-form-item>
            <el-form-item label="Client Key">
              <el-input v-model="clusterForm.k8s_key_data" type="textarea" :rows="3" placeholder="Base64 encoded key (leave empty to keep existing)" />
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane label="PostgreSQL" name="postgres" v-if="clusterForm.storage_mode === 'external'">
            <el-form-item label="Storage Manual Mode">
              <el-switch v-model="clusterForm.storage_manual_mode" />
              <span class="form-hint">When enabled, storage config won't be overwritten by config sync job</span>
            </el-form-item>
            <el-divider />
            <el-form-item label="Host" prop="postgres_host">
              <el-input v-model="clusterForm.postgres_host" placeholder="postgres.example.com" />
            </el-form-item>
            <el-form-item label="Port">
              <el-input-number v-model="clusterForm.postgres_port" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item label="Username">
              <el-input v-model="clusterForm.postgres_username" placeholder="postgres" />
            </el-form-item>
            <el-form-item label="Password">
              <el-input v-model="clusterForm.postgres_password" type="password" show-password placeholder="Leave empty to keep existing" />
            </el-form-item>
            <el-form-item label="Database Name">
              <el-input v-model="clusterForm.postgres_db_name" placeholder="lens" />
            </el-form-item>
            <el-form-item label="SSL Mode">
              <el-select v-model="clusterForm.postgres_ssl_mode" placeholder="Select SSL mode">
                <el-option label="disable" value="disable" />
                <el-option label="require" value="require" />
                <el-option label="verify-ca" value="verify-ca" />
                <el-option label="verify-full" value="verify-full" />
              </el-select>
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane label="Prometheus" name="prometheus" v-if="clusterForm.storage_mode === 'external'">
            <el-form-item label="Read Host">
              <el-input v-model="clusterForm.prometheus_read_host" placeholder="vmselect.example.com" />
            </el-form-item>
            <el-form-item label="Read Port">
              <el-input-number v-model="clusterForm.prometheus_read_port" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item label="Write Host">
              <el-input v-model="clusterForm.prometheus_write_host" placeholder="vminsert.example.com" />
            </el-form-item>
            <el-form-item label="Write Port">
              <el-input-number v-model="clusterForm.prometheus_write_port" :min="1" :max="65535" />
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane label="OpenSearch" name="opensearch" v-if="clusterForm.storage_mode === 'external'">
            <el-form-item label="Host">
              <el-input v-model="clusterForm.opensearch_host" placeholder="opensearch.example.com" />
            </el-form-item>
            <el-form-item label="Port">
              <el-input-number v-model="clusterForm.opensearch_port" :min="1" :max="65535" />
            </el-form-item>
            <el-form-item label="Scheme">
              <el-select v-model="clusterForm.opensearch_scheme" placeholder="Select scheme">
                <el-option label="https" value="https" />
                <el-option label="http" value="http" />
              </el-select>
            </el-form-item>
            <el-form-item label="Username">
              <el-input v-model="clusterForm.opensearch_username" placeholder="admin" />
            </el-form-item>
            <el-form-item label="Password">
              <el-input v-model="clusterForm.opensearch_password" type="password" show-password placeholder="Leave empty to keep existing" />
            </el-form-item>
          </el-tab-pane>

          <el-tab-pane label="Managed Storage" name="managed" v-if="clusterForm.storage_mode === 'lens-managed'">
            <el-alert type="info" :closable="false" style="margin-bottom: 20px">
              Lens will automatically provision and manage storage components in this cluster.
            </el-alert>
            <el-form-item label="Storage Class">
              <el-input v-model="managedConfig.storage_class" placeholder="e.g., standard, gp3" />
            </el-form-item>
            <el-divider>PostgreSQL</el-divider>
            <el-form-item label="Enable PostgreSQL">
              <el-switch v-model="managedConfig.postgres_enabled" />
            </el-form-item>
            <el-form-item label="Storage Size" v-if="managedConfig.postgres_enabled">
              <el-input v-model="managedConfig.postgres_size" placeholder="e.g., 10Gi" />
            </el-form-item>
            <el-divider>OpenSearch</el-divider>
            <el-form-item label="Enable OpenSearch">
              <el-switch v-model="managedConfig.opensearch_enabled" />
            </el-form-item>
            <el-form-item label="Storage Size" v-if="managedConfig.opensearch_enabled">
              <el-input v-model="managedConfig.opensearch_size" placeholder="e.g., 10Gi" />
            </el-form-item>
            <el-form-item label="Replicas" v-if="managedConfig.opensearch_enabled">
              <el-input-number v-model="managedConfig.opensearch_replicas" :min="1" :max="5" />
            </el-form-item>
            <el-divider>VictoriaMetrics</el-divider>
            <el-form-item label="Enable VictoriaMetrics">
              <el-switch v-model="managedConfig.victoriametrics_enabled" />
            </el-form-item>
            <el-form-item label="Storage Size" v-if="managedConfig.victoriametrics_enabled">
              <el-input v-model="managedConfig.victoriametrics_size" placeholder="e.g., 50Gi" />
            </el-form-item>
          </el-tab-pane>
        </el-tabs>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">Cancel</el-button>
        <el-button type="primary" @click="saveCluster" :loading="saving">
          {{ isEditing ? 'Update' : 'Create' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Tasks Dialog -->
    <el-dialog
      v-model="tasksDialogVisible"
      :title="`Installation Tasks - ${selectedClusterForTasks}`"
      width="1000px"
      destroy-on-close
    >
      <el-table :data="tasks" v-loading="tasksLoading" stripe>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="install_scope" label="Scope" width="120">
          <template #default="{ row }">
            <el-tag :type="getScopeTagType(row.install_scope)" size="small">{{ row.install_scope }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="task_type" label="Type" width="100" />
        <el-table-column prop="current_stage" label="Stage" width="150" />
        <el-table-column prop="status" label="Status" width="100">
          <template #default="{ row }">
            <el-tag :type="getTaskStatusType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="error_message" label="Error" show-overflow-tooltip />
        <el-table-column prop="created_at" label="Created" width="160">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="100" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="viewTaskLogs(row)" :disabled="!row.job_name">
              Logs
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- Logs Dialog -->
    <el-dialog
      v-model="logsDialogVisible"
      :title="`Task Logs - Task #${selectedTask?.id || ''}`"
      width="1100px"
      destroy-on-close
    >
      <div v-loading="logsLoading" class="logs-container">
        <pre>{{ taskLogs }}</pre>
      </div>
      <template #footer>
        <el-button @click="logsDialogVisible = false">Close</el-button>
        <el-button type="primary" @click="refreshLogs">Refresh</el-button>
      </template>
    </el-dialog>

    <!-- Settings Dialog -->
    <el-dialog
      v-model="settingsDialogVisible"
      title="Installer Settings"
      width="500px"
      destroy-on-close
    >
      <el-form :model="installerConfig" label-width="120px">
        <el-form-item label="Repository">
          <el-input v-model="installerConfig.repository" placeholder="e.g., primussafe/primus-lens-installer" />
        </el-form-item>
        <el-form-item label="Tag">
          <el-input v-model="installerConfig.tag" placeholder="e.g., latest, v1.0.0" />
        </el-form-item>
        <el-form-item label="Preview">
          <el-tag type="info">{{ installerConfig.repository }}:{{ installerConfig.tag }}</el-tag>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="settingsDialogVisible = false">Cancel</el-button>
        <el-button type="primary" @click="saveInstallerConfig" :loading="savingConfig">
          Save
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus, Refresh, Edit, Delete, Connection, Star,
  CircleCheck, CircleClose, Document, Setting, Box
} from '@element-plus/icons-vue'
import axios from 'axios'

interface ClusterConfig {
  id: number
  cluster_name: string
  display_name: string
  description: string
  source: string
  k8s_endpoint: string
  k8s_configured: boolean
  k8s_insecure_skip_verify: boolean
  k8s_manual_mode: boolean
  postgres_host: string
  postgres_port: number
  postgres_configured: boolean
  prometheus_read_host: string
  prometheus_read_port: number
  prometheus_write_host: string
  prometheus_write_port: number
  prometheus_configured: boolean
  opensearch_host: string
  opensearch_port: number
  opensearch_scheme: string
  opensearch_configured: boolean
  storage_manual_mode: boolean
  storage_mode: string
  infrastructure_status: string
  dataplane_status: string
  dataplane_version: string
  is_default: boolean
  created_at: string
  updated_at: string
  managed_storage_config?: ManagedStorageConfig
}

interface InstallTask {
  id: number
  cluster_name: string
  task_type: string
  install_scope: string
  current_stage: string
  storage_mode: string
  status: string
  error_message: string
  retry_count: number
  max_retries: number
  job_name: string
  job_namespace: string
  created_at: string
  updated_at: string
  started_at: string
  completed_at: string
}

interface ManagedStorageConfig {
  storage_class: string
  postgres_enabled: boolean
  postgres_size: string
  opensearch_enabled: boolean
  opensearch_size: string
  opensearch_replicas: number
  victoriametrics_enabled: boolean
  victoriametrics_size: string
}

const loading = ref(false)
const saving = ref(false)
const testingCluster = ref('')
const initializingCluster = ref('')
const clusters = ref<ClusterConfig[]>([])
const dialogVisible = ref(false)
const isEditing = ref(false)
const activeTab = ref('basic')
const formRef = ref()

// Tasks dialog state
const tasksDialogVisible = ref(false)
const tasksLoading = ref(false)

// Settings dialog state
const settingsDialogVisible = ref(false)
const savingConfig = ref(false)
const installerImage = ref('')
const installerConfig = reactive({
  repository: '',
  tag: ''
})
const selectedClusterForTasks = ref('')
const tasks = ref<InstallTask[]>([])

// Logs dialog state
const logsDialogVisible = ref(false)
const logsLoading = ref(false)
const selectedTask = ref<InstallTask | null>(null)
const taskLogs = ref('')

const clusterForm = reactive({
  cluster_name: '',
  display_name: '',
  description: '',
  storage_mode: 'external',
  k8s_endpoint: '',
  k8s_ca_data: '',
  k8s_cert_data: '',
  k8s_key_data: '',
  k8s_token: '',
  k8s_insecure_skip_verify: false,
  k8s_manual_mode: false,
  postgres_host: '',
  postgres_port: 5432,
  postgres_username: '',
  postgres_password: '',
  postgres_db_name: '',
  postgres_ssl_mode: 'require',
  prometheus_read_host: '',
  prometheus_read_port: 8481,
  prometheus_write_host: '',
  prometheus_write_port: 8480,
  opensearch_host: '',
  opensearch_port: 9200,
  opensearch_scheme: 'https',
  opensearch_username: '',
  opensearch_password: '',
  storage_manual_mode: false,
})

const managedConfig = reactive<ManagedStorageConfig>({
  storage_class: '',
  postgres_enabled: true,
  postgres_size: '10Gi',
  opensearch_enabled: true,
  opensearch_size: '10Gi',
  opensearch_replicas: 1,
  victoriametrics_enabled: true,
  victoriametrics_size: '50Gi',
})

const formRules = {
  cluster_name: [
    { required: true, message: 'Cluster name is required', trigger: 'blur' },
    { pattern: /^[a-z0-9][a-z0-9-]*[a-z0-9]$/, message: 'Must be lowercase alphanumeric with hyphens', trigger: 'blur' }
  ],
}

const getStatusType = (status: string) => {
  const map: Record<string, string> = {
    'deployed': 'success',
    'deploying': 'warning',
    'pending': 'info',
    'failed': 'danger',
  }
  return map[status] || 'info'
}

// Check if storage component is configured based on storage mode
const isStorageConfigured = (row: ClusterConfig, component: string): boolean => {
  if (row.storage_mode === 'lens-managed') {
    // For lens-managed mode, check if infrastructure is ready and component is enabled
    const infraReady = row.infrastructure_status === 'ready'
    const config = row.managed_storage_config
    if (!config) return false
    
    switch (component) {
      case 'postgres':
        return infraReady && config.postgres_enabled
      case 'prometheus':
        return infraReady && config.victoriametrics_enabled
      case 'opensearch':
        return infraReady && config.opensearch_enabled
      default:
        return false
    }
  } else {
    // For external mode, check if host/connection is configured
    switch (component) {
      case 'postgres':
        return row.postgres_configured
      case 'prometheus':
        return row.prometheus_configured
      case 'opensearch':
        return row.opensearch_configured
      default:
        return false
    }
  }
}

// Get tooltip for storage status
const getStorageTooltip = (row: ClusterConfig, component: string): string => {
  if (row.storage_mode === 'lens-managed') {
    const infraStatus = row.infrastructure_status || 'not_initialized'
    const config = row.managed_storage_config
    
    let enabled = false
    switch (component) {
      case 'postgres':
        enabled = config?.postgres_enabled ?? false
        break
      case 'prometheus':
        enabled = config?.victoriametrics_enabled ?? false
        break
      case 'opensearch':
        enabled = config?.opensearch_enabled ?? false
        break
    }
    
    if (!enabled) return `${component} is disabled in managed storage config`
    if (infraStatus === 'ready') return `Lens-managed ${component} is ready`
    if (infraStatus === 'initializing') return `${component} is being initialized`
    return `Infrastructure not initialized (status: ${infraStatus})`
  } else {
    const configured = isStorageConfigured(row, component)
    if (configured) {
      switch (component) {
        case 'postgres':
          return `External PostgreSQL: ${row.postgres_host}:${row.postgres_port}`
        case 'prometheus':
          return `External Prometheus: ${row.prometheus_read_host || row.prometheus_write_host}`
        case 'opensearch':
          return `External OpenSearch: ${row.opensearch_host}:${row.opensearch_port}`
      }
    }
    return `External ${component} not configured`
  }
}

const getInfraStatusType = (status: string) => {
  const map: Record<string, string> = {
    'ready': 'success',
    'initializing': 'warning',
    'not_initialized': 'info',
    'failed': 'danger',
  }
  return map[status] || 'info'
}

const getScopeTagType = (scope: string) => {
  const map: Record<string, string> = {
    'infrastructure': 'primary',
    'apps': 'success',
    'full': 'info',
  }
  return map[scope] || 'info'
}

const getTaskStatusType = (status: string) => {
  const map: Record<string, string> = {
    'completed': 'success',
    'running': 'warning',
    'pending': 'info',
    'failed': 'danger',
  }
  return map[status] || 'info'
}

const formatDate = (dateStr: string) => {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  return date.toLocaleString()
}

const fetchClusters = async () => {
  loading.value = true
  try {
    const response = await axios.get('/lens/v1/management/clusters')
    clusters.value = response.data
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || 'Failed to fetch clusters')
  } finally {
    loading.value = false
  }
}

const refreshClusters = () => {
  fetchClusters()
}

const showAddClusterDialog = () => {
  isEditing.value = false
  activeTab.value = 'basic'
  resetForm()
  dialogVisible.value = true
}

const editCluster = async (cluster: ClusterConfig) => {
  isEditing.value = true
  activeTab.value = 'basic'
  
  // Fetch full cluster details
  try {
    const response = await axios.get(`/lens/v1/management/clusters/${cluster.cluster_name}`)
    const data = response.data
    
    Object.assign(clusterForm, {
      cluster_name: data.cluster_name,
      display_name: data.display_name,
      description: data.description,
      storage_mode: data.storage_mode || 'external',
      k8s_endpoint: data.k8s_endpoint,
      k8s_ca_data: '',
      k8s_cert_data: '',
      k8s_key_data: '',
      k8s_token: '',
      k8s_insecure_skip_verify: data.k8s_insecure_skip_verify || false,
      k8s_manual_mode: data.k8s_manual_mode || false,
      postgres_host: data.postgres_host,
      postgres_port: data.postgres_port || 5432,
      postgres_username: '',
      postgres_password: '',
      postgres_db_name: '',
      postgres_ssl_mode: data.postgres_ssl_mode || 'require',
      prometheus_read_host: data.prometheus_read_host,
      prometheus_read_port: data.prometheus_read_port || 8481,
      prometheus_write_host: data.prometheus_write_host,
      prometheus_write_port: data.prometheus_write_port || 8480,
      opensearch_host: data.opensearch_host,
      opensearch_port: data.opensearch_port || 9200,
      opensearch_scheme: data.opensearch_scheme || 'https',
      opensearch_username: '',
      opensearch_password: '',
      storage_manual_mode: data.storage_manual_mode || false,
    })
    
    if (data.managed_storage_config) {
      Object.assign(managedConfig, data.managed_storage_config)
    }
    
    dialogVisible.value = true
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || 'Failed to fetch cluster details')
  }
}

const resetForm = () => {
  Object.assign(clusterForm, {
    cluster_name: '',
    display_name: '',
    description: '',
    storage_mode: 'external',
    k8s_endpoint: '',
    k8s_ca_data: '',
    k8s_cert_data: '',
    k8s_key_data: '',
    k8s_token: '',
    k8s_insecure_skip_verify: false,
    k8s_manual_mode: false,
    postgres_host: '',
    postgres_port: 5432,
    postgres_username: '',
    postgres_password: '',
    postgres_db_name: '',
    postgres_ssl_mode: 'require',
    prometheus_read_host: '',
    prometheus_read_port: 8481,
    prometheus_write_host: '',
    prometheus_write_port: 8480,
    opensearch_host: '',
    opensearch_port: 9200,
    opensearch_scheme: 'https',
    opensearch_username: '',
    opensearch_password: '',
    storage_manual_mode: false,
  })
  Object.assign(managedConfig, {
    storage_class: '',
    postgres_enabled: true,
    postgres_size: '10Gi',
    victoriametrics_enabled: true,
    victoriametrics_size: '50Gi',
  })
}

const saveCluster = async () => {
  if (!formRef.value) return
  
  try {
    await formRef.value.validate()
  } catch {
    ElMessage.warning('Please check form fields')
    return
  }
  
  saving.value = true
  try {
    const payload: any = { ...clusterForm }
    
    if (clusterForm.storage_mode === 'lens-managed') {
      payload.managed_storage_config = managedConfig
    }
    
    if (isEditing.value) {
      await axios.put(`/lens/v1/management/clusters/${clusterForm.cluster_name}`, payload)
      ElMessage.success('Cluster updated successfully')
    } else {
      await axios.post('/lens/v1/management/clusters', payload)
      ElMessage.success('Cluster created successfully')
    }
    
    dialogVisible.value = false
    fetchClusters()
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || 'Failed to save cluster')
  } finally {
    saving.value = false
  }
}

const testConnection = async (cluster: ClusterConfig) => {
  testingCluster.value = cluster.cluster_name
  try {
    const response = await axios.post(`/lens/v1/management/clusters/${cluster.cluster_name}/test`)
    const result = response.data
    
    if (result.k8s_connected) {
      ElMessage.success(`K8S connected (version: ${result.k8s_version || 'unknown'})`)
    } else {
      ElMessage.warning(`K8S connection failed: ${result.k8s_error || result.error || 'Unknown error'}`)
    }
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || 'Connection test failed')
  } finally {
    testingCluster.value = ''
  }
}

const setAsDefault = async (cluster: ClusterConfig) => {
  try {
    await axios.put(`/lens/v1/management/clusters/${cluster.cluster_name}/default`)
    ElMessage.success(`${cluster.cluster_name} set as default cluster`)
    fetchClusters()
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || 'Failed to set default cluster')
  }
}

const confirmDelete = (cluster: ClusterConfig) => {
  ElMessageBox.confirm(
    `Are you sure you want to delete cluster "${cluster.cluster_name}"? This action cannot be undone.`,
    'Delete Cluster',
    {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    }
  ).then(async () => {
    try {
      await axios.delete(`/lens/v1/management/clusters/${cluster.cluster_name}`)
      ElMessage.success('Cluster deleted successfully')
      fetchClusters()
    } catch (error: any) {
      ElMessage.error(error.response?.data?.error || 'Failed to delete cluster')
    }
  }).catch(() => {})
}

// Infrastructure initialization
const initializeInfrastructure = async (cluster: ClusterConfig) => {
  try {
    await ElMessageBox.confirm(
      `Initialize infrastructure for cluster "${cluster.cluster_name}"? This will set up storage and database components.`,
      'Initialize Infrastructure',
      {
        confirmButtonText: 'Initialize',
        cancelButtonText: 'Cancel',
        type: 'info',
      }
    )

    initializingCluster.value = cluster.cluster_name
    const payload: any = {
      storage_mode: cluster.storage_mode || 'external',
    }
    
    if (cluster.storage_mode === 'lens-managed') {
      payload.managed_storage = {
        storage_class: managedConfig.storage_class,
        postgres_enabled: managedConfig.postgres_enabled,
        postgres_size: managedConfig.postgres_size,
        victoriametrics_enabled: managedConfig.victoriametrics_enabled,
        victoriametrics_size: managedConfig.victoriametrics_size,
      }
    }

    const response = await axios.post(`/lens/v1/management/clusters/${cluster.cluster_name}/initialize`, payload)
    ElMessage.success(`Infrastructure initialization started (Task ID: ${response.data.task_id})`)
    fetchClusters()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error(error.response?.data?.error || 'Failed to initialize infrastructure')
    }
  } finally {
    initializingCluster.value = ''
  }
}

// Tasks management
const viewTasks = async (cluster: ClusterConfig) => {
  selectedClusterForTasks.value = cluster.cluster_name
  tasksDialogVisible.value = true
  tasksLoading.value = true
  
  try {
    const response = await axios.get(`/lens/v1/management/clusters/${cluster.cluster_name}/tasks?limit=20`)
    tasks.value = response.data.data || []
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || 'Failed to fetch tasks')
  } finally {
    tasksLoading.value = false
  }
}

const viewTaskLogs = async (task: InstallTask) => {
  selectedTask.value = task
  logsDialogVisible.value = true
  logsLoading.value = true
  taskLogs.value = ''
  
  try {
    const response = await axios.get(`/lens/v1/management/clusters/${task.cluster_name}/tasks/${task.id}/logs?tail=1000`)
    taskLogs.value = response.data.logs || 'No logs available'
  } catch (error: any) {
    taskLogs.value = `Failed to fetch logs: ${error.response?.data?.error || error.message}`
  } finally {
    logsLoading.value = false
  }
}

const refreshLogs = () => {
  if (selectedTask.value) {
    viewTaskLogs(selectedTask.value)
  }
}

// Installer config methods
const fetchInstallerConfig = async () => {
  try {
    const response = await axios.get('/lens/v1/management/config/installer')
    installerImage.value = response.data.full_image || ''
    installerConfig.repository = response.data.repository || ''
    installerConfig.tag = response.data.tag || ''
  } catch (error: any) {
    installerImage.value = 'primussafe/primus-lens-installer:latest'
    installerConfig.repository = 'primussafe/primus-lens-installer'
    installerConfig.tag = 'latest'
  }
}

const showInstallerConfigDialog = () => {
  fetchInstallerConfig()
  settingsDialogVisible.value = true
}

const saveInstallerConfig = async () => {
  savingConfig.value = true
  try {
    await axios.put('/lens/v1/management/config/installer.image', {
      value: {
        repository: installerConfig.repository,
        tag: installerConfig.tag
      },
      description: 'Dataplane installer image configuration',
      category: 'installer'
    })
    installerImage.value = `${installerConfig.repository}:${installerConfig.tag}`
    ElMessage.success('Installer configuration saved')
    settingsDialogVisible.value = false
  } catch (error: any) {
    ElMessage.error(error.response?.data?.error || 'Failed to save configuration')
  } finally {
    savingConfig.value = false
  }
}

onMounted(() => {
  fetchClusters()
  fetchInstallerConfig()
})
</script>

<style scoped lang="scss">
.cluster-management-page {
  padding: 24px;
  max-width: 1400px;
  margin: 0 auto;
}

.page-header {
  margin-bottom: 24px;
  
  h1 {
    font-size: 24px;
    font-weight: 600;
    margin: 0 0 8px 0;
  }
  
  .subtitle {
    color: var(--el-text-color-secondary);
    margin: 0;
  }
}

.actions-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
  
  .actions-left {
    display: flex;
    gap: 12px;
  }
  
  .actions-right {
    display: flex;
    align-items: center;
    gap: 12px;
  }
}

.installer-tag {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
}

.clusters-card {
  :deep(.el-card__body) {
    padding: 0;
  }
}

.cluster-name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  
  .name {
    font-weight: 500;
  }
}

.status-ok {
  color: var(--el-color-success);
  font-size: 18px;
}

.status-na {
  color: var(--el-color-info-light-3);
  font-size: 18px;
}

:deep(.el-dialog__body) {
  padding-top: 10px;
}

:deep(.el-tabs__content) {
  padding: 20px 0;
}

:deep(.el-divider__text) {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.manual-mode-badges {
  display: flex;
  gap: 4px;
  justify-content: center;
  
  .auto-mode {
    color: var(--el-text-color-secondary);
    font-size: 12px;
  }
}

.form-hint {
  margin-left: 10px;
  color: var(--el-text-color-secondary);
  font-size: 12px;
}

.infra-status-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.logs-container {
  background-color: #1e1e1e;
  border-radius: 4px;
  padding: 16px;
  max-height: 500px;
  overflow: auto;
  
  pre {
    margin: 0;
    font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
    font-size: 12px;
    line-height: 1.5;
    color: #d4d4d4;
    white-space: pre-wrap;
    word-wrap: break-word;
  }
}
</style>

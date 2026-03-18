<template>
  <div class="release-management-page">
    <!-- Header -->
    <div class="page-header">
      <div class="header-left">
        <h3 class="page-title">Release Management</h3>
        <p class="page-description">Manage release versions and cluster deployments</p>
      </div>
      <div class="header-right">
        <el-tag type="info" size="large" class="installer-tag">
          <el-icon><Box /></el-icon>
          Installer: {{ installerImage || 'loading...' }}
        </el-tag>
        <el-button @click="handleRefresh" :icon="Refresh" :loading="loading">
          Refresh
        </el-button>
        <el-button type="primary" @click="showCreateVersionDialog" :icon="Plus">
          New Version
        </el-button>
      </div>
    </div>

    <!-- Tabs -->
    <el-tabs v-model="activeTab" class="release-tabs">
      <!-- Versions Tab -->
      <el-tab-pane label="Versions" name="versions">
        <el-card shadow="hover">
          <template #header>
            <div class="card-header">
              <div class="header-info">
                <el-icon class="header-icon"><Box /></el-icon>
                <div>
                  <h4>Release Versions</h4>
                  <span class="header-description">Manage available release versions</span>
                </div>
              </div>
              <el-select v-model="versionFilter.channel" placeholder="Filter by channel" clearable style="width: 150px">
                <el-option label="Stable" value="stable" />
                <el-option label="Beta" value="beta" />
                <el-option label="Canary" value="canary" />
              </el-select>
            </div>
          </template>

          <el-table :data="filteredVersions" v-loading="loading" stripe>
            <el-table-column prop="version_name" label="Version" width="150">
              <template #default="{ row }">
                <el-tag :type="getChannelTagType(row.channel)">{{ row.version_name }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="channel" label="Channel" width="100">
              <template #default="{ row }">
                <el-tag size="small" :type="getChannelTagType(row.channel)">{{ row.channel }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="chart_version" label="Chart Version" width="120" />
            <el-table-column prop="image_tag" label="Image Tag" width="120" />
            <el-table-column prop="status" label="Status" width="100">
              <template #default="{ row }">
                <el-tag :type="getStatusTagType(row.status)" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="Created" width="160">
              <template #default="{ row }">
                {{ formatDate(row.created_at) }}
              </template>
            </el-table-column>
            <el-table-column label="Actions" width="200" fixed="right">
              <template #default="{ row }">
                <el-button-group size="small">
                  <el-button @click="editVersion(row)" :icon="Edit">Edit</el-button>
                  <el-dropdown trigger="click">
                    <el-button :icon="MoreFilled" />
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item v-if="row.status === 'draft'" @click="activateVersion(row)">
                          <el-icon><CircleCheck /></el-icon> Activate
                        </el-dropdown-item>
                        <el-dropdown-item v-if="row.status === 'active'" @click="deprecateVersion(row)">
                          <el-icon><Warning /></el-icon> Deprecate
                        </el-dropdown-item>
                        <el-dropdown-item divided @click="deleteVersion(row)" style="color: var(--el-color-danger)">
                          <el-icon><Delete /></el-icon> Delete
                        </el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                </el-button-group>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <!-- Clusters Tab -->
      <el-tab-pane label="Clusters" name="clusters">
        <el-card shadow="hover">
          <template #header>
            <div class="card-header">
              <div class="header-info">
                <el-icon class="header-icon"><Monitor /></el-icon>
                <div>
                  <h4>Cluster Deployments</h4>
                  <span class="header-description">View and manage cluster release configurations</span>
                </div>
              </div>
            </div>
          </template>

          <el-table :data="clusterConfigs" v-loading="loading" stripe>
            <el-table-column prop="cluster_name" label="Cluster" width="150" />
            <el-table-column label="Target Version" width="150">
              <template #default="{ row }">
                <el-tag v-if="row.release_version" type="primary">
                  {{ row.release_version.version_name }}
                </el-tag>
                <span v-else class="text-muted">Not configured</span>
              </template>
            </el-table-column>
            <el-table-column label="Deployed Version" width="150">
              <template #default="{ row }">
                <el-tag v-if="row.deployed_version" type="success">
                  {{ row.deployed_version.version_name }}
                </el-tag>
                <span v-else class="text-muted">Never deployed</span>
              </template>
            </el-table-column>
            <el-table-column prop="sync_status" label="Status" width="120">
              <template #default="{ row }">
                <el-tag :type="getSyncStatusTagType(row.sync_status)" size="small">
                  {{ row.sync_status }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="deployed_at" label="Last Deployed" width="160">
              <template #default="{ row }">
                {{ row.deployed_at ? formatDate(row.deployed_at) : '-' }}
              </template>
            </el-table-column>
            <el-table-column label="Default" width="80">
              <template #default="{ row }">
                <el-tag v-if="row.cluster_name === defaultClusterName" type="success" size="small">
                  <el-icon><Star /></el-icon> Default
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="Actions" width="350" fixed="right">
              <template #default="{ row }">
                <el-button-group size="small">
                  <el-button 
                    v-if="row.cluster_name !== defaultClusterName"
                    @click="setAsDefault(row)" 
                    :icon="Star"
                    title="Set as Default"
                  >
                    Set Default
                  </el-button>
                  <el-button @click="configureCluster(row)" :icon="Setting">Configure</el-button>
                  <el-button 
                    type="primary" 
                    @click="deployCluster(row)" 
                    :icon="Upload"
                    :disabled="!row.release_version_id || row.sync_status === 'synced'"
                  >
                    Deploy
                  </el-button>
                  <el-button 
                    @click="viewHistory(row)" 
                    :icon="Document"
                  >
                    History
                  </el-button>
                </el-button-group>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- Create/Edit Version Dialog -->
    <el-dialog 
      v-model="versionDialogVisible" 
      :title="editingVersion ? 'Edit Version' : 'Create Version'"
      width="700px"
    >
      <el-form :model="versionForm" label-width="140px" ref="versionFormRef">
        <el-form-item label="Version Name" prop="version_name" required>
          <el-input v-model="versionForm.version_name" placeholder="e.g., v0.5.0" />
        </el-form-item>
        <el-form-item label="Channel" prop="channel">
          <el-select v-model="versionForm.channel" style="width: 100%">
            <el-option label="Stable" value="stable" />
            <el-option label="Beta" value="beta" />
            <el-option label="Canary" value="canary" />
          </el-select>
        </el-form-item>
        <el-divider content-position="left">Chart Configuration</el-divider>
        <el-form-item label="Chart Repository" prop="chart_repo">
          <el-input v-model="versionForm.chart_repo" placeholder="oci://docker.io/primussafe" />
        </el-form-item>
        <el-form-item label="Chart Version" prop="chart_version" required>
          <el-input v-model="versionForm.chart_version" placeholder="e.g., 0.5.0" />
        </el-form-item>
        <el-divider content-position="left">Image Configuration</el-divider>
        <el-form-item label="Image Registry" prop="image_registry">
          <el-input v-model="versionForm.image_registry" placeholder="docker.io/primussafe" />
        </el-form-item>
        <el-form-item label="Image Tag" prop="image_tag" required>
          <el-input v-model="versionForm.image_tag" placeholder="e.g., v0.5.0" />
        </el-form-item>
        <el-divider content-position="left">Default Values</el-divider>
        <el-form-item label="Default Values">
          <el-input 
            v-model="versionForm.default_values_json" 
            type="textarea" 
            :rows="8"
            placeholder="YAML or JSON format"
          />
        </el-form-item>
        <el-form-item label="Release Notes">
          <el-input 
            v-model="versionForm.release_notes" 
            type="textarea" 
            :rows="3"
            placeholder="Describe changes in this version..."
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="versionDialogVisible = false">Cancel</el-button>
        <el-button type="primary" @click="saveVersion" :loading="saving">
          {{ editingVersion ? 'Update' : 'Create' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Configure Cluster Dialog -->
    <el-dialog 
      v-model="clusterDialogVisible" 
      :title="`Configure ${selectedCluster?.cluster_name || 'Cluster'}`"
      width="700px"
    >
      <el-form :model="clusterForm" label-width="140px">
        <el-form-item label="Target Version" required>
          <el-select v-model="clusterForm.release_version_id" style="width: 100%">
            <el-option 
              v-for="v in activeVersions" 
              :key="v.id" 
              :label="`${v.version_name} (${v.channel})`" 
              :value="v.id"
            />
          </el-select>
        </el-form-item>
        <el-divider content-position="left">Values Override</el-divider>
        <el-form-item label="Override Values">
          <el-input 
            v-model="clusterForm.values_override_json" 
            type="textarea" 
            :rows="10"
            placeholder="YAML or JSON format - these values will override the version defaults"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="clusterDialogVisible = false">Cancel</el-button>
        <el-button type="primary" @click="saveClusterConfig" :loading="saving">
          Save Configuration
        </el-button>
      </template>
    </el-dialog>

    <!-- History Dialog -->
    <el-dialog 
      v-model="historyDialogVisible" 
      :title="`Deployment History - ${selectedCluster?.cluster_name || ''}`"
      width="900px"
    >
      <el-table :data="releaseHistory" v-loading="historyLoading" stripe>
        <el-table-column prop="action" label="Action" width="100">
          <template #default="{ row }">
            <el-tag :type="getActionTagType(row.action)" size="small">{{ row.action }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Version" width="120">
          <template #default="{ row }">
            {{ row.release_version?.version_name || '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="status" label="Status" width="100">
          <template #default="{ row }">
            <el-tag :type="getHistoryStatusTagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="triggered_by" label="Triggered By" width="120" />
        <el-table-column prop="created_at" label="Started" width="160">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column prop="completed_at" label="Completed" width="160">
          <template #default="{ row }">
            {{ row.completed_at ? formatDate(row.completed_at) : '-' }}
          </template>
        </el-table-column>
        <el-table-column prop="error_message" label="Error" show-overflow-tooltip />
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  Refresh, Plus, Edit, Delete, Box, Monitor, Setting, 
  Upload, Document, MoreFilled, CircleCheck, Warning, Star 
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'

// Types
interface ReleaseVersion {
  id: number
  version_name: string
  channel: string
  chart_repo: string
  chart_version: string
  image_registry: string
  image_tag: string
  default_values: Record<string, any>
  values_schema: Record<string, any>
  status: string
  release_notes: string
  created_at: string
  updated_at: string
}

interface ClusterReleaseConfig {
  id: number
  cluster_name: string
  release_version_id: number | null
  values_override: Record<string, any>
  deployed_version_id: number | null
  deployed_values: Record<string, any>
  deployed_at: string | null
  sync_status: string
  release_version?: ReleaseVersion
  deployed_version?: ReleaseVersion
}

interface ReleaseHistory {
  id: number
  cluster_name: string
  release_version_id: number
  action: string
  triggered_by: string
  values_snapshot: Record<string, any>
  status: string
  error_message: string
  created_at: string
  completed_at: string | null
  release_version?: ReleaseVersion
}

// State
const loading = ref(false)
const saving = ref(false)
const historyLoading = ref(false)
const activeTab = ref('versions')
const installerImage = ref('')

const versions = ref<ReleaseVersion[]>([])
const clusterConfigs = ref<ClusterReleaseConfig[]>([])
const releaseHistory = ref<ReleaseHistory[]>([])
const defaultClusterName = ref<string | null>(null)

const versionFilter = ref({ channel: '' })
const versionDialogVisible = ref(false)
const clusterDialogVisible = ref(false)
const historyDialogVisible = ref(false)

const editingVersion = ref<ReleaseVersion | null>(null)
const selectedCluster = ref<ClusterReleaseConfig | null>(null)

const versionForm = ref({
  version_name: '',
  channel: 'stable',
  chart_repo: 'oci://docker.io/primussafe',
  chart_version: '',
  image_registry: 'docker.io/primussafe',
  image_tag: '',
  default_values_json: '{}',
  release_notes: ''
})

const clusterForm = ref({
  release_version_id: null as number | null,
  values_override_json: '{}'
})

// Computed
const filteredVersions = computed(() => {
  if (!versionFilter.value.channel) return versions.value
  return versions.value.filter(v => v.channel === versionFilter.value.channel)
})

const activeVersions = computed(() => {
  return versions.value.filter(v => v.status === 'active')
})

// Methods
const formatDate = (date: string) => {
  return dayjs(date).format('YYYY-MM-DD HH:mm')
}

const getChannelTagType = (channel: string) => {
  switch (channel) {
    case 'stable': return 'success'
    case 'beta': return 'warning'
    case 'canary': return 'danger'
    default: return 'info'
  }
}

const getStatusTagType = (status: string) => {
  switch (status) {
    case 'active': return 'success'
    case 'draft': return 'info'
    case 'deprecated': return 'warning'
    default: return 'info'
  }
}

const getSyncStatusTagType = (status: string) => {
  switch (status) {
    case 'synced': return 'success'
    case 'out_of_sync': return 'warning'
    case 'upgrading': return 'primary'
    case 'failed': return 'danger'
    default: return 'info'
  }
}

const getActionTagType = (action: string) => {
  switch (action) {
    case 'install': return 'success'
    case 'upgrade': return 'primary'
    case 'rollback': return 'warning'
    default: return 'info'
  }
}

const getHistoryStatusTagType = (status: string) => {
  switch (status) {
    case 'completed': return 'success'
    case 'running': return 'primary'
    case 'pending': return 'info'
    case 'failed': return 'danger'
    default: return 'info'
  }
}

const fetchVersions = async () => {
  try {
    const response = await fetch('/lens/v1/releases/versions')
    const data = await response.json()
    versions.value = data.data || []
  } catch (error) {
    console.error('Failed to fetch versions:', error)
    ElMessage.error('Failed to fetch versions')
  }
}

const fetchClusterConfigs = async () => {
  try {
    const response = await fetch('/lens/v1/releases/clusters')
    const data = await response.json()
    clusterConfigs.value = data.data || []
  } catch (error) {
    console.error('Failed to fetch cluster configs:', error)
    ElMessage.error('Failed to fetch cluster configs')
  }
}

const fetchDefaultCluster = async () => {
  try {
    const response = await fetch('/lens/v1/releases/clusters/default')
    const data = await response.json()
    defaultClusterName.value = data.data?.default_cluster || null
  } catch (error) {
    console.error('Failed to fetch default cluster:', error)
  }
}

const setAsDefault = async (cluster: ClusterReleaseConfig) => {
  try {
    await ElMessageBox.confirm(
      `Set "${cluster.cluster_name}" as the default cluster? This will be used when no cluster is specified in API requests.`,
      'Set Default Cluster',
      { type: 'info' }
    )

    const response = await fetch('/lens/v1/releases/clusters/default', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ cluster_name: cluster.cluster_name })
    })

    if (!response.ok) throw new Error('Failed to set default cluster')

    ElMessage.success(`${cluster.cluster_name} is now the default cluster`)
    defaultClusterName.value = cluster.cluster_name
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to set default cluster')
    }
  }
}

const handleRefresh = async () => {
  loading.value = true
  try {
    await Promise.all([fetchVersions(), fetchClusterConfigs(), fetchDefaultCluster()])
    ElMessage.success('Data refreshed')
  } finally {
    loading.value = false
  }
}

const showCreateVersionDialog = () => {
  editingVersion.value = null
  versionForm.value = {
    version_name: '',
    channel: 'stable',
    chart_repo: 'oci://docker.io/primussafe',
    chart_version: '',
    image_registry: 'docker.io/primussafe',
    image_tag: '',
    default_values_json: '{}',
    release_notes: ''
  }
  versionDialogVisible.value = true
}

const editVersion = (version: ReleaseVersion) => {
  editingVersion.value = version
  versionForm.value = {
    version_name: version.version_name,
    channel: version.channel,
    chart_repo: version.chart_repo,
    chart_version: version.chart_version,
    image_registry: version.image_registry,
    image_tag: version.image_tag,
    default_values_json: JSON.stringify(version.default_values || {}, null, 2),
    release_notes: version.release_notes || ''
  }
  versionDialogVisible.value = true
}

const saveVersion = async () => {
  saving.value = true
  try {
    let defaultValues = {}
    try {
      defaultValues = JSON.parse(versionForm.value.default_values_json)
    } catch (e) {
      ElMessage.error('Invalid JSON in default values')
      return
    }

    const payload = {
      version_name: versionForm.value.version_name,
      channel: versionForm.value.channel,
      chart_repo: versionForm.value.chart_repo,
      chart_version: versionForm.value.chart_version,
      image_registry: versionForm.value.image_registry,
      image_tag: versionForm.value.image_tag,
      default_values: defaultValues,
      release_notes: versionForm.value.release_notes
    }

    const url = editingVersion.value 
      ? `/lens/v1/releases/versions/${editingVersion.value.id}`
      : '/lens/v1/releases/versions'
    const method = editingVersion.value ? 'PUT' : 'POST'

    const response = await fetch(url, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    })

    if (!response.ok) throw new Error('Failed to save version')

    ElMessage.success(editingVersion.value ? 'Version updated' : 'Version created')
    versionDialogVisible.value = false
    await fetchVersions()
  } catch (error) {
    ElMessage.error('Failed to save version')
  } finally {
    saving.value = false
  }
}

const activateVersion = async (version: ReleaseVersion) => {
  try {
    await fetch(`/lens/v1/releases/versions/${version.id}/status`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: 'active' })
    })
    ElMessage.success('Version activated')
    await fetchVersions()
  } catch (error) {
    ElMessage.error('Failed to activate version')
  }
}

const deprecateVersion = async (version: ReleaseVersion) => {
  try {
    await fetch(`/lens/v1/releases/versions/${version.id}/status`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: 'deprecated' })
    })
    ElMessage.success('Version deprecated')
    await fetchVersions()
  } catch (error) {
    ElMessage.error('Failed to deprecate version')
  }
}

const deleteVersion = async (version: ReleaseVersion) => {
  try {
    await ElMessageBox.confirm(
      `Are you sure you want to delete version "${version.version_name}"?`,
      'Delete Version',
      { type: 'warning' }
    )
    await fetch(`/lens/v1/releases/versions/${version.id}`, { method: 'DELETE' })
    ElMessage.success('Version deleted')
    await fetchVersions()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to delete version')
    }
  }
}

const configureCluster = (cluster: ClusterReleaseConfig) => {
  selectedCluster.value = cluster
  clusterForm.value = {
    release_version_id: cluster.release_version_id,
    values_override_json: JSON.stringify(cluster.values_override || {}, null, 2)
  }
  clusterDialogVisible.value = true
}

const saveClusterConfig = async () => {
  if (!selectedCluster.value) return
  saving.value = true
  try {
    let valuesOverride = {}
    try {
      valuesOverride = JSON.parse(clusterForm.value.values_override_json)
    } catch (e) {
      ElMessage.error('Invalid JSON in values override')
      return
    }

    await fetch(`/lens/v1/releases/clusters/${selectedCluster.value.cluster_name}/version`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        release_version_id: clusterForm.value.release_version_id,
        values_override: valuesOverride
      })
    })

    ElMessage.success('Cluster configuration saved')
    clusterDialogVisible.value = false
    await fetchClusterConfigs()
  } catch (error) {
    ElMessage.error('Failed to save cluster configuration')
  } finally {
    saving.value = false
  }
}

const deployCluster = async (cluster: ClusterReleaseConfig) => {
  try {
    // Check infrastructure status first
    const infraResponse = await fetch(`/lens/v1/management/clusters/${cluster.cluster_name}/infrastructure/status`)
    if (infraResponse.ok) {
      const infraData = await infraResponse.json()
      if (!infraData.initialized && infraData.status !== 'ready') {
        ElMessage.warning('Please initialize infrastructure first in Cluster Management')
        return
      }
    }

    await ElMessageBox.confirm(
      `Are you sure you want to deploy to cluster "${cluster.cluster_name}"?`,
      'Deploy',
      { type: 'warning' }
    )
    
    // Deploy with apps scope only
    const response = await fetch(`/lens/v1/releases/clusters/${cluster.cluster_name}/deploy`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ scope: 'apps' })
    })

    if (!response.ok) throw new Error('Failed to trigger deployment')

    ElMessage.success('Deployment triggered')
    await fetchClusterConfigs()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to trigger deployment')
    }
  }
}

const viewHistory = async (cluster: ClusterReleaseConfig) => {
  selectedCluster.value = cluster
  historyDialogVisible.value = true
  historyLoading.value = true
  try {
    const response = await fetch(`/lens/v1/releases/clusters/${cluster.cluster_name}/history?limit=20`)
    const data = await response.json()
    releaseHistory.value = data.data || []
  } catch (error) {
    ElMessage.error('Failed to fetch history')
  } finally {
    historyLoading.value = false
  }
}

const fetchInstallerConfig = async () => {
  try {
    const response = await fetch('/lens/v1/management/config/installer')
    const data = await response.json()
    installerImage.value = data.full_image || 'primussafe/primus-lens-installer:latest'
  } catch {
    installerImage.value = 'primussafe/primus-lens-installer:latest'
  }
}

onMounted(async () => {
  loading.value = true
  try {
    await Promise.all([fetchVersions(), fetchClusterConfigs(), fetchDefaultCluster(), fetchInstallerConfig()])
  } finally {
    loading.value = false
  }
})
</script>

<style scoped lang="scss">
.release-management-page {
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 24px;

    .header-left {
      .page-title {
        margin: 0 0 8px 0;
        font-size: 24px;
        font-weight: 600;
      }
      .page-description {
        margin: 0;
        color: var(--el-text-color-secondary);
      }
    }

    .header-right {
      display: flex;
      align-items: center;
      gap: 12px;
      
      .installer-tag {
        display: flex;
        align-items: center;
        gap: 6px;
        padding: 8px 12px;
      }
    }
  }

  .release-tabs {
    :deep(.el-tabs__content) {
      padding: 0;
    }
  }

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
      }

      .header-description {
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }
  }

  .text-muted {
    color: var(--el-text-color-secondary);
    font-style: italic;
  }
}
</style>

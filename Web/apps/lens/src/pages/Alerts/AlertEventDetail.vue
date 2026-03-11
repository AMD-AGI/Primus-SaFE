<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { 
  alertEventsApi,
  alertSilencesApi,
  type AlertEvent,
  type AlertCorrelation
} from '@/services/alerts'
import AlertSeverityBadge from './components/AlertSeverityBadge.vue'
import AlertStatusTag from './components/AlertStatusTag.vue'
import AlertSourceIcon from './components/AlertSourceIcon.vue'

const route = useRoute()
const router = useRouter()

// State
const loading = ref(false)
const alert = ref<AlertEvent | null>(null)
const correlations = ref<AlertCorrelation[]>([])
const activeTab = ref('overview')

const alertId = computed(() => route.params.id as string)

// Fetch alert detail
async function fetchAlertDetail() {
  if (!alertId.value) return
  
  loading.value = true
  try {
    const [alertRes, correlationsRes] = await Promise.all([
      alertEventsApi.get(alertId.value),
      alertEventsApi.getCorrelations(alertId.value)
    ])
    
    alert.value = alertRes
    correlations.value = correlationsRes || []
  } catch (error) {
    console.error('Failed to fetch alert detail:', error)
    ElMessage.error('Failed to load alert details')
  } finally {
    loading.value = false
  }
}

// Actions
function goBack() {
  router.push('/alerts/events')
}

async function silenceAlert() {
  if (!alert.value) return
  
  try {
    await ElMessageBox.prompt(
      'Enter reason for silencing this alert:',
      'Silence Alert',
      {
        confirmButtonText: 'Create Silence',
        cancelButtonText: 'Cancel',
        inputPattern: /.+/,
        inputErrorMessage: 'Reason is required'
      }
    ).then(async ({ value: reason }) => {
      await alertSilencesApi.create({
        name: `Silence ${alert.value!.alertName}`,
        silenceType: 'alert_name',
        alertNames: [alert.value!.alertName],
        startsAt: new Date().toISOString(),
        endsAt: new Date(Date.now() + 2 * 60 * 60 * 1000).toISOString(),
        reason,
        enabled: true
      })
      
      ElMessage.success('Silence created successfully')
      fetchAlertDetail()
    })
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('Failed to create silence')
    }
  }
}

function copyAlertId() {
  if (!alert.value) return
  navigator.clipboard.writeText(alert.value.id)
  ElMessage.success('Alert ID copied to clipboard')
}

function shareLink() {
  const url = window.location.href
  navigator.clipboard.writeText(url)
  ElMessage.success('Link copied to clipboard')
}

// Utility functions
function formatTime(timestamp?: string) {
  if (!timestamp) return '-'
  return new Date(timestamp).toLocaleString()
}

function formatDuration(startsAt: string, endsAt?: string) {
  const start = new Date(startsAt)
  const end = endsAt ? new Date(endsAt) : new Date()
  const diff = Math.floor((end.getTime() - start.getTime()) / 1000)
  
  if (diff < 60) return `${diff} seconds`
  if (diff < 3600) return `${Math.floor(diff / 60)} minutes`
  if (diff < 86400) return `${Math.floor(diff / 3600)} hours ${Math.floor((diff % 3600) / 60)} minutes`
  return `${Math.floor(diff / 86400)} days ${Math.floor((diff % 86400) / 3600)} hours`
}

function getCorrelationTypeLabel(type: string) {
  const labels: Record<string, string> = {
    time: 'Time-based Correlation',
    entity: 'Same Entity',
    causal: 'Causal Relationship',
    cross_source: 'Cross-Source Correlation'
  }
  return labels[type] || type
}

function goToWorkload() {
  if (alert.value?.workloadId) {
    router.push({
      path: '/workload/detail',
      query: { uid: alert.value.workloadId }
    })
  }
}

function goToCorrelatedAlert(alertItem: AlertEvent) {
  router.push(`/alerts/events/${alertItem.id}`)
}

onMounted(() => {
  fetchAlertDetail()
})
</script>

<template>
  <div class="alert-event-detail" v-loading="loading">
    <!-- Back Button -->
    <div class="back-nav">
      <el-button text @click="goBack">
        <el-icon><ArrowLeft /></el-icon>
        Back to Events
      </el-button>
    </div>

    <template v-if="alert">
      <!-- Alert Header -->
      <el-card class="alert-header-card" shadow="hover">
        <div class="alert-header">
          <div class="header-left">
            <AlertSeverityBadge :severity="alert.severity" size="large" />
            <div class="alert-title-section">
              <h1 class="alert-title">{{ alert.alertName }}</h1>
              <p class="alert-summary">{{ alert.annotations?.summary || alert.annotations?.description }}</p>
            </div>
          </div>
          <div class="header-right">
            <AlertStatusTag :status="alert.status" size="large" />
            <AlertSourceIcon :source="alert.source" show-label />
          </div>
        </div>
        
        <div class="alert-meta">
          <div class="meta-item">
            <span class="meta-label">Started:</span>
            <span class="meta-value">{{ formatTime(alert.startsAt) }}</span>
          </div>
          <div class="meta-item" v-if="alert.endsAt">
            <span class="meta-label">Ended:</span>
            <span class="meta-value">{{ formatTime(alert.endsAt) }}</span>
          </div>
          <div class="meta-item">
            <span class="meta-label">Duration:</span>
            <span class="meta-value">{{ formatDuration(alert.startsAt, alert.endsAt) }}</span>
          </div>
        </div>
        
        <div class="header-actions">
          <el-button 
            type="warning" 
            @click="silenceAlert"
            :disabled="alert.status === 'silenced'"
          >
            <el-icon><MuteNotification /></el-icon>
            Silence Alert
          </el-button>
          <el-button @click="copyAlertId">
            <el-icon><DocumentCopy /></el-icon>
            Copy Alert ID
          </el-button>
          <el-button @click="shareLink">
            <el-icon><Link /></el-icon>
            Share Link
          </el-button>
        </div>
      </el-card>

      <!-- Tabs -->
      <el-tabs v-model="activeTab" class="detail-tabs">
        <el-tab-pane label="Overview" name="overview">
          <div class="tab-content">
            <!-- Resource Context -->
            <el-card class="section-card" shadow="hover">
              <template #header>
                <div class="section-header">
                  <el-icon><Location /></el-icon>
                  Resource Context
                </div>
              </template>
              <div class="resource-grid">
                <div class="resource-item">
                  <span class="resource-label">Cluster</span>
                  <span class="resource-value">{{ alert.clusterName || '-' }}</span>
                </div>
                <div class="resource-item" v-if="alert.nodeName">
                  <span class="resource-label">Node</span>
                  <span class="resource-value">{{ alert.nodeName }}</span>
                </div>
                <div class="resource-item" v-if="alert.workloadName || alert.workloadId">
                  <span class="resource-label">Workload</span>
                  <span class="resource-value link" @click="goToWorkload">
                    {{ alert.workloadName || alert.workloadId }}
                    <el-icon><ArrowRight /></el-icon>
                  </span>
                </div>
                <div class="resource-item" v-if="alert.podName">
                  <span class="resource-label">Pod</span>
                  <span class="resource-value">{{ alert.podName }}</span>
                </div>
              </div>
              
              <!-- Enriched Data -->
              <template v-if="alert.enrichedData">
                <el-divider />
                <div class="enriched-section">
                  <h4>Enriched Information</h4>
                  <div class="resource-grid">
                    <template v-if="alert.enrichedData.workloadInfo">
                      <div class="resource-item">
                        <span class="resource-label">Namespace</span>
                        <span class="resource-value">{{ alert.enrichedData.workloadInfo.namespace }}</span>
                      </div>
                      <div class="resource-item">
                        <span class="resource-label">Kind</span>
                        <span class="resource-value">{{ alert.enrichedData.workloadInfo.kind }}</span>
                      </div>
                      <div class="resource-item">
                        <span class="resource-label">GPU Request</span>
                        <span class="resource-value">{{ alert.enrichedData.workloadInfo.gpuRequest }}</span>
                      </div>
                    </template>
                    <template v-if="alert.enrichedData.nodeInfo">
                      <div class="resource-item">
                        <span class="resource-label">GPU Model</span>
                        <span class="resource-value">{{ alert.enrichedData.nodeInfo.gpuModel }}</span>
                      </div>
                      <div class="resource-item">
                        <span class="resource-label">GPU Count</span>
                        <span class="resource-value">{{ alert.enrichedData.nodeInfo.gpuCount }}</span>
                      </div>
                    </template>
                  </div>
                </div>
              </template>
            </el-card>

            <!-- Annotations -->
            <el-card class="section-card" shadow="hover">
              <template #header>
                <div class="section-header">
                  <el-icon><Document /></el-icon>
                  Annotations
                </div>
              </template>
              <div class="annotations-list">
                <div 
                  v-for="(value, key) in alert.annotations" 
                  :key="key"
                  class="annotation-item"
                >
                  <span class="annotation-key">{{ key }}:</span>
                  <span class="annotation-value" v-if="key === 'runbook_url' || key === 'runbookUrl'">
                    <a :href="value" target="_blank" rel="noopener">{{ value }}</a>
                  </span>
                  <span class="annotation-value" v-else>{{ value }}</span>
                </div>
                <el-empty v-if="!alert.annotations || Object.keys(alert.annotations).length === 0" 
                  description="No annotations" 
                  :image-size="60" 
                />
              </div>
            </el-card>
          </div>
        </el-tab-pane>

        <el-tab-pane label="Labels & Annotations" name="labels">
          <div class="tab-content">
            <el-card class="section-card" shadow="hover">
              <template #header>
                <div class="section-header">
                  <el-icon><PriceTag /></el-icon>
                  Labels
                </div>
              </template>
              <div class="labels-container">
                <el-tag 
                  v-for="(value, key) in alert.labels" 
                  :key="key"
                  class="label-tag"
                  type="info"
                >
                  {{ key }}={{ value }}
                </el-tag>
                <el-empty v-if="!alert.labels || Object.keys(alert.labels).length === 0" 
                  description="No labels" 
                  :image-size="60" 
                />
              </div>
            </el-card>

            <el-card class="section-card" shadow="hover">
              <template #header>
                <div class="section-header">
                  <el-icon><Memo /></el-icon>
                  Annotations
                </div>
              </template>
              <el-descriptions :column="1" border>
                <el-descriptions-item 
                  v-for="(value, key) in alert.annotations" 
                  :key="key"
                  :label="key"
                >
                  <template v-if="key === 'runbook_url' || key === 'runbookUrl'">
                    <a :href="value" target="_blank" rel="noopener">{{ value }}</a>
                  </template>
                  <template v-else>{{ value }}</template>
                </el-descriptions-item>
              </el-descriptions>
              <el-empty v-if="!alert.annotations || Object.keys(alert.annotations).length === 0" 
                description="No annotations" 
                :image-size="60" 
              />
            </el-card>
          </div>
        </el-tab-pane>

        <el-tab-pane label="Correlations" name="correlations">
          <div class="tab-content">
            <el-card class="section-card" shadow="hover">
              <template #header>
                <div class="section-header">
                  <el-icon><Connection /></el-icon>
                  Related Alerts ({{ correlations.reduce((sum, c) => sum + c.alerts.length, 0) }})
                </div>
              </template>
              
              <div v-if="correlations.length > 0">
                <div 
                  v-for="correlation in correlations" 
                  :key="correlation.correlationId"
                  class="correlation-group"
                >
                  <div class="correlation-header">
                    <span class="correlation-type">{{ getCorrelationTypeLabel(correlation.correlationType) }}</span>
                    <span v-if="correlation.reason" class="correlation-reason">{{ correlation.reason }}</span>
                    <el-tag v-if="correlation.correlationScore" type="info" size="small">
                      Score: {{ (correlation.correlationScore * 100).toFixed(0) }}%
                    </el-tag>
                  </div>
                  
                  <div class="correlated-alerts">
                    <div 
                      v-for="correlatedAlert in correlation.alerts" 
                      :key="correlatedAlert.id"
                      class="correlated-alert-item"
                      @click="goToCorrelatedAlert(correlatedAlert)"
                    >
                      <AlertSeverityBadge :severity="correlatedAlert.severity" size="small" />
                      <span class="correlated-name">{{ correlatedAlert.alertName }}</span>
                      <span class="correlated-resource">{{ correlatedAlert.podName || correlatedAlert.nodeName }}</span>
                      <span class="correlated-time">{{ formatTime(correlatedAlert.startsAt) }}</span>
                      <el-button type="primary" text size="small">View</el-button>
                    </div>
                  </div>
                </div>
              </div>
              
              <el-empty v-else description="No correlated alerts found" :image-size="80" />
            </el-card>
          </div>
        </el-tab-pane>

        <el-tab-pane label="Raw Data" name="raw">
          <div class="tab-content">
            <el-card class="section-card" shadow="hover">
              <template #header>
                <div class="section-header">
                  <el-icon><Document /></el-icon>
                  Raw Alert Data
                </div>
              </template>
              <pre class="raw-data">{{ JSON.stringify(alert, null, 2) }}</pre>
            </el-card>
          </div>
        </el-tab-pane>
      </el-tabs>
    </template>

    <el-empty v-else-if="!loading" description="Alert not found" />
  </div>
</template>

<style lang="scss" scoped>
.alert-event-detail {
  padding: 0;
}

.back-nav {
  margin-bottom: 16px;
}

.alert-header-card {
  margin-bottom: 24px;
  border-radius: 12px;
}

.alert-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
}

.header-left {
  display: flex;
  align-items: flex-start;
  gap: 16px;
}

.alert-title-section {
  .alert-title {
    font-size: 24px;
    font-weight: 600;
    margin: 0 0 8px 0;
    color: var(--el-text-color-primary);
  }
  
  .alert-summary {
    font-size: 14px;
    color: var(--el-text-color-secondary);
    margin: 0;
    max-width: 600px;
  }
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.alert-meta {
  display: flex;
  gap: 32px;
  padding: 16px 0;
  border-top: 1px solid var(--el-border-color-lighter);
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.meta-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.meta-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.meta-value {
  font-size: 14px;
  font-weight: 500;
  color: var(--el-text-color-primary);
}

.header-actions {
  display: flex;
  gap: 12px;
  margin-top: 16px;
}

.detail-tabs {
  :deep(.el-tabs__header) {
    margin-bottom: 20px;
  }
}

.tab-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.section-card {
  border-radius: 12px;
}

.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
}

.resource-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 16px;
}

.resource-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.resource-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.resource-value {
  font-size: 14px;
  font-weight: 500;
  color: var(--el-text-color-primary);
  
  &.link {
    color: var(--el-color-primary);
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 4px;
    
    &:hover {
      text-decoration: underline;
    }
  }
}

.enriched-section {
  h4 {
    margin: 0 0 12px 0;
    font-size: 14px;
    color: var(--el-text-color-secondary);
  }
}

.annotations-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.annotation-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.annotation-key {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.annotation-value {
  font-size: 14px;
  color: var(--el-text-color-regular);
  word-break: break-word;
  
  a {
    color: var(--el-color-primary);
    text-decoration: none;
    
    &:hover {
      text-decoration: underline;
    }
  }
}

.labels-container {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.label-tag {
  font-family: monospace;
}

.correlation-group {
  margin-bottom: 24px;
  
  &:last-child {
    margin-bottom: 0;
  }
}

.correlation-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.correlation-type {
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.correlation-reason {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.correlated-alerts {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.correlated-alert-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
  cursor: pointer;
  transition: background 0.2s;
  
  &:hover {
    background: var(--el-fill-color);
  }
}

.correlated-name {
  font-weight: 500;
  flex: 1;
}

.correlated-resource {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.correlated-time {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.raw-data {
  background: var(--el-fill-color-dark);
  padding: 16px;
  border-radius: 8px;
  overflow-x: auto;
  font-family: 'Fira Code', monospace;
  font-size: 13px;
  line-height: 1.5;
  color: var(--el-text-color-primary);
  margin: 0;
}
</style>

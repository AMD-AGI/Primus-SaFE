<template>
  <div class="workload-detail-page" v-loading="pageLoading">
    <!-- Show skeleton when loading -->
    <el-row :gutter="24" v-if="pageLoading">
      <!-- Skeleton for tree -->
      <el-col :span="6">
        <el-card class="tree-panel">
          <template #header>
            <el-skeleton :rows="0" animated>
              <template #template>
                <el-skeleton-item variant="text" style="width: 60%" />
              </template>
            </el-skeleton>
          </template>
          <el-skeleton :rows="8" animated />
        </el-card>
      </el-col>

      <!-- Skeleton for details -->
      <el-col :span="18">
        <el-card class="detail-panel">
          <template #header>
            <el-skeleton :rows="0" animated>
              <template #template>
                <div style="display: flex; justify-content: space-between; align-items: center;">
                  <el-skeleton-item variant="text" style="width: 200px" />
                  <el-skeleton-item variant="button" style="width: 80px" />
                </div>
              </template>
            </el-skeleton>
          </template>
          <el-skeleton :rows="6" animated />
        </el-card>

        <el-card class="grafana-panel" style="margin-top: 20px;">
          <template #header>
            <el-skeleton :rows="0" animated>
              <template #template>
                <el-skeleton-item variant="text" style="width: 100px" />
              </template>
            </el-skeleton>
          </template>
          <el-skeleton-item variant="rect" style="height: 400px" />
        </el-card>
      </el-col>
    </el-row>

    <!-- Actual content -->
    <el-row :gutter="24" v-else>
      <!-- Left Panel - Tree -->
      <el-col :span="6">
        <el-card class="tree-panel">
          <template #header>
            <div class="panel-header">
              <i i="ep-folder-opened" class="header-icon" />
              <span>Workload Hierarchy</span>
            </div>
          </template>
          <!-- Wrapped in a container to avoid el-tree issues -->
          <div v-if="!treeLoading && treeData && treeData.length > 0" class="tree-wrapper">
            <el-tree
              :key="treeKey"
              :data="safeTreeData"
              :props="treeProps"
              @node-click="handleNodeClick"
              class="workload-tree"
              :highlight-current="true"
              :default-expand-all="true"
              node-key="uid"
              :current-node-key="detailData?.uid"
            >
              <template #default="{ node, data }">
                <div class="tree-node">
                  <i :class="getNodeIcon(data)" class="node-icon" />
                  <span class="node-label">{{ node.label || 'Unknown' }}</span>
                </div>
              </template>
            </el-tree>
          </div>
          <div v-else-if="!treeLoading && (!treeData || treeData.length === 0)" class="tree-empty">
            <el-empty description="No hierarchy data available" />
          </div>
          <div v-else-if="treeLoading" class="tree-loading">
            <el-skeleton :rows="5" animated />
          </div>
        </el-card>
      </el-col>

      <!-- Right Panel - Details -->
      <el-col :span="18">
        <!-- Add Tabs Container -->
        <el-tabs v-model="activeTab" class="workload-tabs">
          <!-- Overview Tab -->
          <el-tab-pane label="Overview" name="overview">
            <el-card class="detail-panel">
              <template #header>
                <div class="detail-header">
                  <h3 class="detail-title">{{ detailData?.name || '-' }}</h3>
                  <el-tag :type="getKindType(detailData?.kind) || 'info'" size="default">
                    {{ detailData?.kind || '-' }}
                  </el-tag>
                </div>
              </template>

              <div class="detail-grid">
                <div class="detail-item">
                  <label>API Version</label>
                  <span>{{ detailData?.apiVersion || '-' }}</span>
                </div>
                <div class="detail-item">
                  <label>Namespace</label>
                  <span>{{ detailData?.namespace || '-' }}</span>
                </div>
                <div class="detail-item">
                  <label>UID</label>
                  <span class="uid-text">{{ detailData?.uid || '-' }}</span>
                </div>
                <div class="detail-item">
                  <label>Start Time</label>
                  <span>{{ detailData?.startTime ? dayjs(detailData.startTime * 1000).format('YYYY-MM-DD HH:mm:ss') : '-' }}</span>
                </div>
                <div class="detail-item">
                  <label>End Time</label>
                  <span>{{ detailData?.endTime ? dayjs(detailData.endTime * 1000).format('YYYY-MM-DD HH:mm:ss') : '-' }}</span>
                </div>
                <div class="detail-item">
                  <label>Duration</label>
                  <span>{{ getDuration(detailData?.startTime, detailData?.endTime) }}</span>
                </div>
              </div>
            </el-card>

            <!-- Grafana Panel -->
            <el-card class="grafana-panel" v-if="detailData && detailData.uid">
              <template #header>
                <div class="panel-header">
                  <i i="ep-data-line" class="header-icon" />
                  <span>Metrics</span>
                </div>
              </template>
              <GrafanaIframe
                path="/grafana/d/workload-metrics/workload-metrics"
                :orgId="1"
                datasource=""
                varKey="var-workload_uid"
                :varValue="detailData?.uid"
                :time="defaultTime"
                theme="dark"
                kiosk
                refresh="30s"
                height="800px"
              />
            </el-card>
          </el-tab-pane>
          <!-- Profiler Files Tab -->
          <el-tab-pane label="Profiler Files" name="profiler-files" lazy>
            <ProfilerFilesList
              v-if="detailData?.uid"
              :workload-uid="detailData.uid"
            />
          </el-tab-pane>

          <!-- Py-Spy Profiler Tab -->
          <el-tab-pane label="Py-Spy Profiler" name="pyspy-profiler" lazy>
            <PySpyProfilerTab
              v-if="detailData?.uid"
              :workload-uid="detailData.uid"
              :workload-status="workloadStatus"
              :pods="workloadPods"
              :cluster="selectedCluster"
            />
          </el-tab-pane>
        </el-tabs>
      </el-col>
    </el-row>
  </div>
</template>
<script setup lang="ts">
import {computed, onMounted, ref, onBeforeUnmount} from 'vue'
import { useRoute } from 'vue-router'
import dayjs from 'dayjs'
import {getWorkloadsTree, getWorkloadsDetail, getWorkloadsMetrics} from '@/services/dashboard/index'
import { ElMessage } from 'element-plus'
import GrafanaIframe from '@/components/base/GrafanaIframe.vue'
import ProfilerFilesList from '@/components/tracelens/ProfilerFilesList.vue'
import PySpyProfilerTab from '@/components/pyspy/PySpyProfilerTab.vue'
import { useClusterSync } from '@/composables/useClusterSync'

const route = useRoute()
const { syncFromUrl, updateUrlWithCluster, selectedCluster } = useClusterSync()
const kind = computed(() => route.query.kind as string | undefined)
const name = computed(() => route.query.name as string | undefined)
const uid = ref<string>()


const treeLoading = ref(false)
const treeData = ref<any[]>([])
const treeKey = ref(0) // Used to force re-render el-tree
const curName = ref('')
const detailLoading = ref(false)
const detailData = ref<any>({})
const pageLoading = ref(true) // Full page loading state
const activeTab = ref('overview') // Currently active tab

const ONE_DAY = 24 * 60 * 60 * 1000


// Tree component property configuration
const treeProps = {
  label: 'name',
  children: 'children',
  isLeaf: (data: any) => !data.children || data.children.length === 0
}

// Safe tree data
const safeTreeData = computed(() => {
  if (!treeData.value || treeData.value.length === 0) {
    return []
  }

  try {
    // Deep clone and ensure correct data structure
    const ensureSafe = (nodes: any[]): any[] => {
      if (!Array.isArray(nodes)) return []

      return nodes.map(node => {
        const safeNode: any = {
          uid: node?.uid || `node_${Math.random().toString(36).substr(2, 9)}`,
          name: node?.name || 'Unknown',
          kind: node?.kind || '',
          namespace: node?.namespace || '',
          ...node
        }
        // Ensure children field exists and is an array
        safeNode.children = node?.children ? ensureSafe(node.children) : []
        return safeNode
      })
    }

    return ensureSafe(treeData.value)
  } catch (error) {
    console.error('[WorkloadDetail] Error processing tree data:', error)
    return []
  }
})

const getTreeData = async() => {
    if(!kind.value || !name.value) {
        ElMessage.error('Workload kind and name are required')
        pageLoading.value = false
        return
    }

    treeLoading.value = true

    try {
        // Call hierarchy API with kind and name parameters
        const res = await getWorkloadsTree({ kind: kind.value, name: name.value })

        // Ensure response data has correct structure, handle children field
        const normalizeTreeData = (node: any): any => {
            if (!node) return null
            // Ensure every node has required fields
            const normalized: any = {
                uid: node.uid || '',
                name: node.name || '',
                kind: node.kind || '',
                namespace: node.namespace || '',
                ...node
            }
            // Ensure children is an array, even if empty
            normalized.children = Array.isArray(node.children)
                ? node.children.map(normalizeTreeData).filter(Boolean)
                : []
            return normalized
        }

        const normalizedData = normalizeTreeData(res)
        // Ensure treeData is always an array
        treeData.value = normalizedData ? [normalizedData] : []
        // Force re-render el-tree component
        treeKey.value++

        // Extract uid from the response for later use
        uid.value = res?.uid || ''
        curName.value = res?.name || ''

        // Auto select first node
        if (treeData.value.length > 0 && treeData.value[0]) {
            await getDetailByTree(treeData.value[0].uid)
        }
    } catch (error) {
        ElMessage.error(error || 'Failed to load workload data')
        pageLoading.value = false
    } finally {
        treeLoading.value = false
    }
}

const getDetailByTree = async(curUid: string) => {
    detailLoading.value = true
    try {
        detailData.value = await getWorkloadsDetail(curUid)
    } catch (error) {
        ElMessage.error('Failed to load workload details')
    } finally {
        detailLoading.value = false
        pageLoading.value = false // Initial load complete
    }
}

const handleNodeClick = (data: any) => {
    if (data && data.uid) {
        getDetailByTree(data.uid)
        curName.value = data.name || ''
    }
}

const defaultTime = computed<[Date, Date] | null>(() => {
  const s = detailData.value?.startTime
  const e = detailData.value?.endTime
  if (!s) return null
  const start = new Date(Number(s) * 1000)
  const end = e ? new Date(Number(e) * 1000) : new Date()
  return end >= start ? [start, end] : [start, start]
})

// Get icon for tree node based on type
const getNodeIcon = (data: any) => {
  const kind = data.kind?.toLowerCase() || ''
  if (kind.includes('deployment')) return 'i-ep-box'
  if (kind.includes('pod')) return 'i-ep-cpu'
  if (kind.includes('service')) return 'i-ep-connection'
  if (kind.includes('job')) return 'i-ep-timer'
  if (kind.includes('statefulset')) return 'i-ep-coin'
  if (kind.includes('daemonset')) return 'i-ep-monitor'
  return 'i-ep-document'
}

// Get tag type based on workload kind
const getKindType = (kind: string) => {
  const k = kind?.toLowerCase() || ''
  if (k.includes('deployment')) return 'success'
  if (k.includes('pod')) return 'info'
  if (k.includes('service')) return 'warning'
  if (k.includes('job')) return 'danger'
  return ''
}

// Text overflow detection for tooltips
const nodeRefs = new Map<string, HTMLElement>()

const setNodeRef = (el: HTMLElement | null, label: string) => {
  if (el) {
    nodeRefs.set(label, el)
  } else {
    nodeRefs.delete(label)
  }
}

const isTextOverflow = (label: string) => {
  const el = nodeRefs.get(label)
  if (!el) return false
  return el.scrollWidth > el.clientWidth
}

// Calculate duration between start and end time
const getDuration = (startTime: number, endTime: number) => {
  if (!startTime) return '-'
  if (!endTime) return 'Running'

  const duration = endTime - startTime
  const hours = Math.floor(duration / 3600)
  const minutes = Math.floor((duration % 3600) / 60)
  const seconds = duration % 60

  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m ${seconds}s`
  return `${seconds}s`
}

// Get workload status for Py-Spy feature - MUST be computed to react to changes
const workloadStatus = computed<'running' | 'completed' | 'failed' | 'pending'>(() => {
  // Priority 1: Use status from URL query (passed from list/statistics page)
  const queryStatus = route.query.status as string | undefined
  if (queryStatus) {
    const status = queryStatus.toLowerCase()
    if (status === 'running') return 'running'
    if (status === 'done' || status === 'completed') return 'completed'
    if (status === 'failed') return 'failed'
    if (status === 'pending') return 'pending'
  }

  // Priority 2: Use status from detailData API
  if (detailData.value?.status) {
    const status = detailData.value.status.toLowerCase()
    if (status === 'running') return 'running'
    if (status === 'done' || status === 'completed') return 'completed'
    if (status === 'failed') return 'failed'
    if (status === 'pending') return 'pending'
  }

  // Fallback: assume pending
  return 'pending'
})

// Get pods from workload detail API response
const workloadPods = computed(() => {
  if (!detailData.value?.pods || !Array.isArray(detailData.value.pods)) {
    return []
  }

  // Map API pods to PodInfo format
  return detailData.value.pods.map((pod: any) => ({
    uid: pod.podUid || pod.uid,
    name: pod.podName || pod.name,
    namespace: pod.podNamespace || pod.namespace || 'default',
    nodeName: pod.nodeName || pod.node_name || 'unknown',
    status: pod.phase || pod.status || (pod.running ? 'Running' : 'Unknown'),
    ip: pod.ip,
    gpuAllocated: pod.gpuAllocated,
    createdAt: pod.createdAt,
    updatedAt: pod.updatedAt,
    containerStatuses: pod.containerStatuses || []
  }))
})

onMounted(() => {
    // Sync cluster from URL to global state
    syncFromUrl()
    // Ensure URL contains cluster parameter
    updateUrlWithCluster()
    getTreeData()
})
</script>

<style scoped lang="scss">
.workload-detail-page {
  padding: 0 20px 20px 20px;
  // Remove background color and min-height to avoid extra dark area
}

// Tabs Styles
.workload-tabs {
  :deep(.el-tabs__header) {
    margin-bottom: 20px;

    .el-tabs__item {
      font-size: 14px;
      font-weight: 500;

      &.is-active {
        color: var(--el-color-primary);
      }
    }
  }
}

// Tree Panel Styles
.tree-panel {
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);

  :deep(.el-card__header) {
    border-bottom: 1px solid var(--el-border-color-lighter);
    padding: 16px 20px;
  }

  :deep(.el-card__body) {
    padding: 0;
    min-height: 400px;
    max-height: 600px;
    overflow-y: auto;

    &::-webkit-scrollbar {
      width: 6px;
    }

    &::-webkit-scrollbar-thumb {
      background-color: rgba(144, 147, 153, 0.3);
      border-radius: 3px;

      &:hover {
        background-color: rgba(144, 147, 153, 0.5);
      }
    }
  }
}

.panel-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 16px;
  font-weight: 600;
  color: var(--el-text-color-primary);

  .header-icon {
    font-size: 20px;
    color: var(--el-color-primary);
  }
}

// Tree Component Styles
.workload-tree {
  font-size: 15px;
  width: 100%;

  :deep(.el-tree-node__content) {
    height: 40px;
    padding: 0 20px;
    transition: all 0.2s ease;
    overflow: hidden; // Prevent content overflow

    &:hover {
      background-color: var(--el-fill-color-light);
    }
  }

  :deep(.el-tree-node.is-current > .el-tree-node__content) {
    background-color: var(--el-color-primary-light-9);
    color: var(--el-color-primary);
    font-weight: 500;
  }

  :deep(.el-tree-node__expand-icon) {
    font-size: 16px;
    color: var(--el-text-color-regular);
    flex-shrink: 0; // Prevent icon from shrinking
  }

  // Fix for long content in tree nodes
  :deep(.el-tree-node__label) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 100%;
  }
}

.tree-node {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-width: 0; // Allow flex item to shrink below content size

  .node-icon {
    font-size: 18px;
    color: var(--el-color-primary);
    transition: transform 0.2s ease;
    flex-shrink: 0; // Prevent icon from shrinking
  }

  .node-label {
    font-size: 14px;
    color: var(--el-text-color-primary);
    flex: 1;
    min-width: 0; // Critical for text-overflow to work in flex container
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &:hover .node-icon {
    transform: scale(1.1);
  }

  // Add tooltip for long content
  &:hover .node-label {
    cursor: pointer;
  }
}

// Detail Panel Styles
.detail-panel {
  margin-bottom: 20px;
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);

  :deep(.el-card__header) {
    border-bottom: 1px solid var(--el-border-color-lighter);
    padding: 20px;
  }

  :deep(.el-card__body) {
    padding: 24px;
  }
}

.detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;

  .detail-title {
    font-size: 20px;
    font-weight: 600;
    margin: 0;
    color: var(--el-text-color-primary);
  }
}

.detail-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 24px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 8px;

  label {
    font-size: 13px;
    font-weight: 500;
    color: var(--el-text-color-secondary);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  span {
    font-size: 15px;
    color: var(--el-text-color-primary);
    font-weight: 500;
  }

  .uid-text {
    font-family: 'Consolas', 'Monaco', monospace;
    font-size: 14px;
    background-color: var(--el-fill-color-light);
    padding: 4px 8px;
    border-radius: 4px;
    word-break: break-all;
  }
}

// Grafana Panel
.grafana-panel {
  border-radius: 12px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
  margin-top: 20px;

  :deep(.el-card__header) {
    border-bottom: 1px solid var(--el-border-color-lighter);
    padding: 16px 20px;
  }

  :deep(.el-card__body) {
    padding: 0;
    background-color: #1a1a1a;
    border-bottom-left-radius: 12px;
    border-bottom-right-radius: 12px;
    overflow: hidden;
  }
}

// Dark Mode Adjustments
.dark {
  .tree-panel,
  .detail-panel,
  .grafana-panel {
    background-color: var(--el-bg-color);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
  }

  .workload-tree {
    :deep(.el-tree-node__content:hover) {
      background-color: rgba(255, 255, 255, 0.05);
    }

    :deep(.el-tree-node.is-current > .el-tree-node__content) {
      background-color: rgba(64, 158, 255, 0.15);
    }
  }

  .detail-item {
    .uid-text {
      background-color: rgba(255, 255, 255, 0.05);
    }
  }
}
</style>

<template>
  <div v-loading="loading">
    <div v-if="detail" class="mb-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <el-button :icon="ArrowLeft" @click="handleBack" class="mr-0"> Back </el-button>
          <div>
            <div class="flex items-center gap-3">
              <h2 class="text-xl font-semibold">Deployment Request #{{ detail.id }}</h2>
              <el-tag :type="getStatusType(detail.status)">
                {{ detail.status }}
              </el-tag>
            </div>
            <div class="text-sm text-gray-500 mt-1 flex items-center gap-2">
              <span>Initiated by {{ detail.deploy_name }}</span>
              <span>•</span>
              <span>{{ formatTimeStr(detail.created_at) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="mt-4">
      <el-tab-pane label="Overview" name="overview">
        <el-card class="mt-2 safe-card" shadow="never">
          <div class="flex items-center mb-4">
            <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
            <span class="textx-15 font-medium">Deployment Details</span>
          </div>
          <el-descriptions v-if="detail" :column="2" border>
            <el-descriptions-item label="ID">{{ detail.id }}</el-descriptions-item>
            <el-descriptions-item label="Status">
              <el-tag :type="getStatusType(detail.status)">{{ detail.status }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="Type">
              {{ detail.deploy_type || 'safe' }}
            </el-descriptions-item>
            <el-descriptions-item label="Deploy Name" :span="2">
              {{ detail.deploy_name || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Description" :span="2">
              {{ detail.description || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Branch" v-if="isLens">
              {{ detail.branch || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Snapshot ID" v-if="isLens && detail.snapshot_id">
              {{ detail.snapshot_id }}
            </el-descriptions-item>
            <el-descriptions-item label="Rollback From ID" v-if="detail.rollback_from_id">
              {{ detail.rollback_from_id }}
            </el-descriptions-item>
            <el-descriptions-item label="Approval Result" v-if="detail.approval_result">
              <el-tag
                :type="detail.approval_result === 'approved' ? 'success' : 'danger'"
                size="small"
              >
                {{ detail.approval_result }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item
              label="Rejection Reason"
              :span="2"
              v-if="detail.approval_result === 'rejected' && detail.rejection_reason"
            >
              {{ detail.rejection_reason }}
            </el-descriptions-item>
            <el-descriptions-item label="Created At">
              {{ formatTimeStr(detail.created_at) }}
            </el-descriptions-item>
            <el-descriptions-item label="Updated At">
              {{ formatTimeStr(detail.updated_at) }}
            </el-descriptions-item>
            <el-descriptions-item label="Approver" v-if="detail.approver_name">
              {{ detail.approver_name }}
            </el-descriptions-item>
            <el-descriptions-item label="Approved At" v-if="detail.approved_at">
              {{ formatTimeStr(detail.approved_at) }}
            </el-descriptions-item>
            <el-descriptions-item label="Workload ID" v-if="detail.workload_id" :span="2">
              {{ detail.workload_id }}
            </el-descriptions-item>
          </el-descriptions>

          <!-- Image Versions -->
          <div
            v-if="
              !isLens && detail?.image_versions && Object.keys(detail.image_versions).length > 0
            "
            class="mt-6"
          >
            <div class="flex items-center mb-4">
              <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
              <span class="textx-15 font-medium">Image Versions</span>
            </div>
            <el-table :data="imageVersionsTable" border>
              <el-table-column prop="component" label="Component" min-width="150" />
              <el-table-column prop="version" label="Version" min-width="200" />
            </el-table>
          </div>

          <!-- Environment Config -->
          <div v-if="!isLens && detail?.env_file_config" class="mt-6">
            <div class="flex items-center mb-4">
              <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
              <span class="textx-15 font-medium">Environment Config</span>
            </div>
            <el-input
              :model-value="detail?.env_file_config"
              type="textarea"
              :rows="10"
              readonly
              class="config-textarea"
            />
          </div>

          <!-- Lens Configs -->
          <div v-if="isLens && detail?.control_plane_config" class="mt-6">
            <div class="flex items-center mb-4">
              <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
              <span class="textx-15 font-medium">Control Plane Config</span>
            </div>
            <el-input
              :model-value="detail?.control_plane_config"
              type="textarea"
              :rows="10"
              readonly
              class="config-textarea"
            />
          </div>

          <div v-if="isLens && detail?.data_plane_config" class="mt-6">
            <div class="flex items-center mb-4">
              <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
              <span class="textx-15 font-medium">Data Plane Config</span>
            </div>
            <el-input
              :model-value="detail?.data_plane_config"
              type="textarea"
              :rows="10"
              readonly
              class="config-textarea"
            />
          </div>
        </el-card>
      </el-tab-pane>

      <el-tab-pane
        label="Diff"
        name="diff"
        lazy
        v-if="isLens && (detail?.control_plane_diff || detail?.data_plane_diff)"
      >
        <el-card class="mt-2 safe-card" shadow="never">
          <el-collapse v-model="diffActive" accordion>
            <el-collapse-item
              v-if="detail?.control_plane_diff"
              name="control"
              title="Control Plane Diff"
            >
              <div class="diff-view diff-view-full">
                <div
                  v-for="(line, idx) in controlPlaneDiffLines"
                  :key="`c-${idx}`"
                  class="diff-line"
                  :class="`diff-${line.kind}`"
                >
                  {{ line.text }}
                </div>
              </div>
            </el-collapse-item>

            <el-collapse-item v-if="detail?.data_plane_diff" name="data" title="Data Plane Diff">
              <div class="diff-view diff-view-full">
                <div
                  v-for="(line, idx) in dataPlaneDiffLines"
                  :key="`d-${idx}`"
                  class="diff-line"
                  :class="`diff-${line.kind}`"
                >
                  {{ line.text }}
                </div>
              </div>
            </el-collapse-item>
          </el-collapse>
        </el-card>
      </el-tab-pane>

      <el-tab-pane
        label="Logs"
        name="logs"
        lazy
        v-if="detail?.workload_id && userStore.envs?.enableLog"
      >
        <LogTable
          v-if="workloadDetail"
          :wlid="detail.workload_id"
          :dispatchCount="workloadDetail.dispatchCount"
          :nodes="workloadDetail.nodes"
          :ranks="workloadDetail.ranks"
          :failedNodes="workloadDetail.failedNodes"
          :isDownload="userStore.envs?.enableLogDownload || false"
        />
        <el-card v-else v-loading="workloadLoading" class="mt-2 safe-card" shadow="never">
          <el-empty description="Loading workload information..." />
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ArrowLeft } from '@element-plus/icons-vue'
import { getDeploymentDetail } from '@/services/deploy'
import { getWorkloadDetail } from '@/services/workload'
import { formatTimeStr } from '@/utils'
import { useUserStore } from '@/stores/user'
import type { DeploymentRequest } from '@/services/deploy/type'
import LogTable from '@/pages/CICD/Components/LogTable.vue'
import { ElMessage } from 'element-plus'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

interface WorkloadDetail {
  dispatchCount?: number
  nodes?: string[][]
  ranks?: string[][]
  failedNodes?: string[]
}

const activeTab = ref('overview')
const diffActive = ref<'control' | 'data'>('control')
const loading = ref(false)
const workloadLoading = ref(false)
const detail = ref<DeploymentRequest | null>(null)
const workloadDetail = ref<WorkloadDetail | null>(null)

const deploymentType = computed(() => detail.value?.deploy_type || 'safe')
const isLens = computed(() => deploymentType.value === 'lens')

watch(
  () => detail.value,
  (d) => {
    if (!d) return
    // Initialize diff: default expand the one that exists
    if (!d.control_plane_diff && d.data_plane_diff) diffActive.value = 'data'
    else diffActive.value = 'control'
  },
  { immediate: true },
)

type DiffLine = { kind: 'add' | 'del' | 'hunk' | 'meta' | 'context'; text: string }
const parseDiff = (diff?: string): DiffLine[] => {
  if (!diff) return []
  return diff.split('\n').map((line) => {
    if (line.startsWith('@@')) return { kind: 'hunk', text: line }
    if (line.startsWith('+++') || line.startsWith('---')) return { kind: 'meta', text: line }
    if (line.startsWith('+')) return { kind: 'add', text: line }
    if (line.startsWith('-')) return { kind: 'del', text: line }
    return { kind: 'context', text: line }
  })
}

const controlPlaneDiffLines = computed(() => parseDiff(detail.value?.control_plane_diff))
const dataPlaneDiffLines = computed(() => parseDiff(detail.value?.data_plane_diff))

const imageVersionsTable = computed(() => {
  if (!detail.value?.image_versions) return []
  return Object.entries(detail.value.image_versions).map(([component, version]) => ({
    component,
    version,
  }))
})

const getStatusType = (status?: string) => {
  const typeMap: Record<string, string> = {
    pending_approval: 'warning',
    approved: 'info',
    rejected: 'danger',
    deploying: 'primary',
    deployed: 'success',
    failed: 'danger',
  }
  return typeMap[status || ''] || ''
}

const fetchDetail = async () => {
  const id = route.query.id as string
  if (!id) {
    ElMessage.error('Missing deployment ID')
    router.push('/deploy')
    return
  }

  try {
    loading.value = true
    detail.value = await getDeploymentDetail(id)
  } catch (error) {
    console.error('Failed to fetch deployment detail:', error)
    ElMessage.error('Failed to fetch deployment detail')
  } finally {
    loading.value = false
  }
}

const handleBack = () => {
  const fromType = route.query.type as string | undefined
  if (fromType) {
    router.push({ path: '/deploy', query: { type: fromType } })
  } else {
    router.push('/deploy')
  }
}

const fetchWorkloadDetail = async () => {
  if (!detail.value?.workload_id) return

  try {
    workloadLoading.value = true
    workloadDetail.value = await getWorkloadDetail(detail.value.workload_id)
  } catch (error) {
    console.error('Failed to fetch workload detail:', error)
    ElMessage.error('Failed to fetch workload detail')
  } finally {
    workloadLoading.value = false
  }
}

// Watch detail to fetch workload detail when available
watch(
  () => detail.value?.workload_id,
  (workloadId) => {
    if (workloadId) {
      fetchWorkloadDetail()
    }
  },
  { immediate: true },
)

// Fetch deployment detail on mount
fetchDetail()
userStore.fetchEnvs()
</script>

<style scoped>
.config-textarea :deep(.el-textarea__inner) {
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.6;
}

.diff-view {
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre;
}
.diff-view-full {
  height: calc(100vh - 380px);
  overflow: auto;
  border: 1px solid var(--el-border-color);
  border-radius: 6px;
}
.diff-line {
  padding: 0 10px;
}
.diff-add {
  background: rgba(46, 160, 67, 0.12);
}
.diff-del {
  background: rgba(248, 81, 73, 0.12);
}
.diff-hunk,
.diff-meta {
  background: rgba(175, 184, 193, 0.18);
}
</style>

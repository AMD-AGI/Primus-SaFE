<template>
  <div v-loading="runLoading" :element-loading-text="$loadingText">
    <!-- Header -->
    <div v-if="run" class="flex items-center justify-between flex-wrap gap-3">
      <div class="flex items-center gap-3">
        <el-button :icon="ArrowLeft" text @click="router.push('/posttrain')" />
        <el-text class="textx-18 font-600" tag="b">{{ run.displayName }}</el-text>
        <el-tag :type="run.trainType === 'sft' ? 'success' : 'warning'" effect="plain" size="small">
          {{ run.trainType.toUpperCase() }}
        </el-tag>
        <el-tag :type="statusTagType(run.status)" :effect="isDark ? 'plain' : 'light'" size="small">
          {{ run.status }}
        </el-tag>
      </div>
      <div class="flex items-center gap-2">
        <el-button type="primary" plain size="default" @click="goWorkload">
          <el-icon class="mr-1"><Monitor /></el-icon>
          View Workload
        </el-button>
        <el-button type="danger" plain size="default" @click="handleDelete">
          <el-icon class="mr-1"><Delete /></el-icon>
          Delete Record
        </el-button>
      </div>
    </div>

    <template v-if="run">
      <!-- Basic Info -->
      <el-card class="mt-4 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Basic Info</span>
        </div>
        <el-descriptions class="m-t-4" border :column="4" direction="vertical">
          <el-descriptions-item label="Run Name">{{ run.displayName }}</el-descriptions-item>
          <el-descriptions-item label="Run ID">
            <span class="text-xs break-all">{{ run.runId }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="Train Type">
            <el-tag size="small" :type="run.trainType === 'sft' ? 'success' : 'warning'" effect="plain">
              {{ run.trainType.toUpperCase() }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Strategy">{{ run.strategy }}</el-descriptions-item>
          <el-descriptions-item label="Status">
            <el-tag size="small" :type="statusTagType(run.status)">{{ run.status }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Workspace">{{ run.workspace }}</el-descriptions-item>
          <el-descriptions-item label="Cluster">{{ run.cluster || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Owner">{{ run.userName || run.userId || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Workload ID" :span="2">
            <el-link type="primary" :underline="false" @click="goWorkload">
              {{ run.workloadId }}
              <el-icon class="ml-1"><Right /></el-icon>
            </el-link>
          </el-descriptions-item>
          <el-descriptions-item label="Created">{{ formatTimeStr(run.createdAt) }}</el-descriptions-item>
          <el-descriptions-item label="Duration">{{ run.duration || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Base Model" :span="2">{{ run.baseModelName || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Dataset" :span="2">{{ run.datasetName || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Image" :span="4">
            <span class="text-xs break-all">{{ run.image || '-' }}</span>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <!-- Resource -->
      <el-card class="mt-4 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Resource</span>
        </div>
        <div class="grid gap-3 mt-4 sm:grid-cols-2 lg:grid-cols-3">
          <StatCard label="Nodes" :value="run.nodeCount ?? '-'" :icon="DataLine" />
          <StatCard label="GPU / Node" :value="run.gpuPerNode ?? '-'" :icon="Monitor" />
          <StatCard label="CPU" :value="run.cpu || '-'" :icon="Cpu" />
          <StatCard label="Memory" :value="run.memory || '-'" :icon="Box" />
          <StatCard label="Shared Memory" :value="run.sharedMemory || '-'" :icon="Collection" />
          <StatCard label="Ephemeral" :value="run.ephemeralStorage || '-'" :icon="Collection" />
        </div>
      </el-card>

      <!-- Parameters -->
      <el-card class="mt-4 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Parameters</span>
        </div>
        <template v-if="run.parameterSnapshot && Object.keys(run.parameterSnapshot).length">
          <el-descriptions class="m-t-4" border :column="4" direction="vertical">
            <el-descriptions-item
              v-for="(val, key) in run.parameterSnapshot"
              :key="String(key)"
              :label="String(key)"
            >
              {{ formatParamValue(val) }}
            </el-descriptions-item>
          </el-descriptions>
        </template>
        <div v-else-if="run.parameterSummary" class="param-summary mt-4">
          {{ run.parameterSummary }}
        </div>
        <div v-else class="text-gray-400 text-sm mt-4">
          No parameter data available
        </div>
      </el-card>

      <!-- Output / Registration -->
      <el-card class="mt-4 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Output / Registration</span>
        </div>
        <el-descriptions class="m-t-4" border :column="3" direction="vertical">
          <el-descriptions-item label="Export">
            <el-tag size="small" :type="run.exportModel ? 'success' : 'info'" effect="plain">
              {{ run.exportModel ? 'Enabled' : 'Disabled' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Output Path">
            <span class="text-xs break-all">{{ run.outputPath || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item v-if="run.modelId" label="Model">
            <el-link type="primary" :underline="false" @click="goModel">
              {{ run.modelDisplayName || run.modelId }}
              <el-tag v-if="run.modelPhase" size="small" class="ml-1" effect="plain">{{ run.modelPhase }}</el-tag>
            </el-link>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <!-- Metrics -->
      <el-card class="mt-4 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Metrics</span>
        </div>
        <div v-if="metricsLoading" class="text-center p-y-4 mt-4">
          <el-icon class="is-loading"><Loading /></el-icon>
        </div>
        <template v-else-if="metrics">
          <el-descriptions class="m-t-4" border :column="2" direction="vertical">
            <el-descriptions-item label="Latest Loss">
              {{ metrics.latestLoss != null ? Number(metrics.latestLoss).toFixed(4) : '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Source">{{ metrics.source || '-' }}</el-descriptions-item>
          </el-descriptions>
          <div v-if="metrics.availableMetrics?.length" class="text-xs text-gray-400 mt-2">
            Available: {{ metrics.availableMetrics.join(', ') }}
          </div>
          <div v-if="lossChartData.length" class="loss-chart mt-3">
            <div class="chart-label">Loss Curve</div>
            <div class="sparkline">
              <svg :viewBox="`0 0 ${lossChartData.length} 100`" preserveAspectRatio="none">
                <polyline
                  :points="sparklinePoints"
                  fill="none"
                  stroke="var(--el-color-primary)"
                  stroke-width="1.5"
                  vector-effect="non-scaling-stroke"
                />
              </svg>
            </div>
          </div>
        </template>
        <div v-else class="text-gray-400 text-sm mt-4">No structured metrics available</div>
      </el-card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ArrowLeft, Monitor, Delete, Right, Loading,
  Cpu, Box, Collection, DataLine,
} from '@element-plus/icons-vue'
import { useDark } from '@vueuse/core'
import { formatTimeStr } from '@/utils'
import { getPostTrainRunDetail, getPostTrainRunMetrics, deletePostTrainRun } from '@/services/posttrain'
import type { PostTrainRunItem, PostTrainMetricsResp } from '@/services/posttrain'
import StatCard from '@/components/Base/StatCard.vue'

const route = useRoute()
const router = useRouter()
const isDark = useDark()

const runId = computed(() => route.query.runId as string)

const runLoading = ref(false)
const metricsLoading = ref(false)

const run = ref<PostTrainRunItem | null>(null)
const metrics = ref<PostTrainMetricsResp | null>(null)

const statusTagType = (status: string) => {
  const map: Record<string, string> = {
    Running: 'primary', Succeeded: 'success', Failed: 'danger', Pending: 'info', Stopped: 'info',
  }
  return map[status] || 'info'
}

const formatParamValue = (val: unknown): string => {
  if (typeof val === 'boolean') return val ? 'Yes' : 'No'
  if (val == null) return '-'
  return String(val)
}

const lossChartData = computed(() => {
  return metrics.value?.series?.loss ?? metrics.value?.series?.train_loss ?? []
})

const sparklinePoints = computed(() => {
  const data = lossChartData.value
  if (!data.length) return ''
  const maxVal = Math.max(...data.map((p) => p.value))
  const minVal = Math.min(...data.map((p) => p.value))
  const range = maxVal - minVal || 1
  return data
    .map((p, i) => `${i},${100 - ((p.value - minVal) / range) * 90 - 5}`)
    .join(' ')
})

const loadRun = async () => {
  if (!runId.value) return
  runLoading.value = true
  try {
    run.value = (await getPostTrainRunDetail(runId.value)) as unknown as PostTrainRunItem
  } catch {
    run.value = null
    ElMessage.error('Failed to load run detail')
  } finally {
    runLoading.value = false
  }
}

const loadMetrics = async () => {
  if (!runId.value) return
  metricsLoading.value = true
  try {
    metrics.value = (await getPostTrainRunMetrics(runId.value)) as unknown as PostTrainMetricsResp
  } catch {
    metrics.value = null
  } finally {
    metricsLoading.value = false
  }
}

const goWorkload = () => {
  if (!run.value) return
  const path = run.value.trainType === 'rl' ? '/rayjob/detail' : '/training/detail'
  router.push({ path, query: { id: run.value.workloadId } })
}

const goModel = () => {
  if (run.value?.modelId) {
    router.push(`/model-square/detail/${run.value.modelId}`)
  }
}

const handleDelete = async () => {
  if (!run.value) return
  const msg = h('span', null, [
    'Delete training record ',
    h('b', null, run.value.displayName),
    '? This only removes the record, not the actual workload.',
  ])
  try {
    await ElMessageBox.confirm(msg, 'Delete Record', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
    await deletePostTrainRun(run.value.runId)
    ElMessage.success('Record deleted')
    router.push('/posttrain')
  } catch (err) {
    if (err !== 'cancel' && err !== 'close') {
      ElMessage.error('Failed to delete record')
    }
  }
}

watch(runId, () => {
  loadRun()
  loadMetrics()
}, { immediate: true })

defineOptions({ name: 'PostTrainDetail' })
</script>

<style scoped lang="scss">
.param-summary {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  word-break: break-all;
}

.loss-chart {
  .chart-label {
    font-size: 12px;
    color: var(--el-text-color-secondary);
    margin-bottom: 4px;
  }

  .sparkline {
    height: 80px;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 6px;
    padding: 4px;

    svg {
      width: 100%;
      height: 100%;
    }
  }
}
</style>

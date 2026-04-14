<template>
  <el-drawer
    :model-value="visible"
    title="Run Detail"
    size="620"
    @close="emit('update:visible', false)"
    destroy-on-close
  >
    <div v-if="loading" class="text-center p-y-8">
      <el-icon class="is-loading" :size="24"><Loading /></el-icon>
    </div>

    <template v-else-if="run">
      <!-- Basic Info -->
      <section class="drawer-section">
        <div class="section-title">Basic Info</div>
        <el-descriptions border :column="2" size="small">
          <el-descriptions-item label="Run Name">{{ run.displayName }}</el-descriptions-item>
          <el-descriptions-item label="Status">
            <el-tag size="small" :type="statusTagType(run.status)">{{ run.status }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Train Type">
            <el-tag size="small" :type="run.trainType === 'sft' ? 'success' : 'warning'" effect="plain">
              {{ run.trainType.toUpperCase() }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Strategy">{{ run.strategy }}</el-descriptions-item>
          <el-descriptions-item label="Workspace">{{ run.workspace }}</el-descriptions-item>
          <el-descriptions-item label="Owner">{{ run.userName || run.userId || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Workload ID" :span="2">
            <el-link type="primary" :underline="false" @click="goWorkload">
              {{ run.workloadId }}
              <el-icon class="ml-1"><Right /></el-icon>
            </el-link>
          </el-descriptions-item>
          <el-descriptions-item label="Image" :span="2">
            <span class="text-xs break-all">{{ run.image || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="Created">{{ formatTimeStr(run.createdAt) }}</el-descriptions-item>
          <el-descriptions-item label="Duration">{{ run.duration || '-' }}</el-descriptions-item>
        </el-descriptions>
      </section>

      <!-- Resource Snapshot -->
      <section class="drawer-section">
        <div class="section-title">Resource Snapshot</div>
        <el-descriptions border :column="3" size="small">
          <el-descriptions-item label="Nodes">{{ run.nodeCount ?? '-' }}</el-descriptions-item>
          <el-descriptions-item label="GPU/Node">{{ run.gpuPerNode ?? '-' }}</el-descriptions-item>
          <el-descriptions-item label="CPU">{{ run.cpu || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Memory">{{ run.memory || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Shared Mem">{{ run.sharedMemory || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Ephemeral">{{ run.ephemeralStorage || '-' }}</el-descriptions-item>
        </el-descriptions>
      </section>

      <!-- Parameter Snapshot -->
      <section class="drawer-section">
        <div class="section-title">Parameter Snapshot</div>
        <div v-if="run.parameterSummary" class="param-summary">
          {{ run.parameterSummary }}
        </div>
        <el-collapse v-if="run.parameterSnapshot">
          <el-collapse-item title="Full Parameters (JSON)" name="params">
            <pre class="param-json">{{ JSON.stringify(run.parameterSnapshot, null, 2) }}</pre>
          </el-collapse-item>
        </el-collapse>
        <div v-if="!run.parameterSummary && !run.parameterSnapshot" class="text-gray-400 text-sm">
          No parameter data available
        </div>
      </section>

      <!-- Output / Registration -->
      <section class="drawer-section">
        <div class="section-title">Output / Registration</div>
        <el-descriptions border :column="2" size="small">
          <el-descriptions-item label="Export">
            <el-tag size="small" :type="run.exportModel ? 'success' : 'info'" effect="plain">
              {{ run.exportModel ? 'Enabled' : 'Disabled' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Output Path">
            <span class="text-xs break-all">{{ run.outputPath || '-' }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="Model" v-if="run.modelId">
            <el-link type="primary" :underline="false" @click="goModel">
              {{ run.modelDisplayName || run.modelId }}
              <el-tag v-if="run.modelPhase" size="small" class="ml-1" effect="plain">{{ run.modelPhase }}</el-tag>
            </el-link>
          </el-descriptions-item>
        </el-descriptions>
      </section>

      <!-- Metrics -->
      <section class="drawer-section">
        <div class="section-title">Metrics</div>
        <div v-if="metricsLoading" class="text-center p-y-4">
          <el-icon class="is-loading"><Loading /></el-icon>
        </div>
        <template v-else-if="metrics">
          <el-descriptions border :column="2" size="small" class="m-b-3">
            <el-descriptions-item label="Latest Loss">
              {{ metrics.latestLoss != null ? Number(metrics.latestLoss).toFixed(4) : '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Source">{{ metrics.source || '-' }}</el-descriptions-item>
          </el-descriptions>
          <div v-if="metrics.availableMetrics?.length" class="text-xs text-gray-400">
            Available: {{ metrics.availableMetrics.join(', ') }}
          </div>
          <div v-if="lossChartData.length" class="loss-chart">
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
        <div v-else class="text-gray-400 text-sm">No structured metrics available</div>
      </section>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Loading, Right } from '@element-plus/icons-vue'
import { formatTimeStr } from '@/utils'
import { getPostTrainRunDetail, getPostTrainRunMetrics } from '@/services/posttrain'
import type { PostTrainRunItem, PostTrainMetricsResp } from '@/services/posttrain'

const props = defineProps<{
  visible: boolean
  runId: string
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

const router = useRouter()
const loading = ref(false)
const metricsLoading = ref(false)
const run = ref<PostTrainRunItem | null>(null)
const metrics = ref<PostTrainMetricsResp | null>(null)

const statusTagType = (status: string) => {
  const map: Record<string, string> = {
    Running: 'primary', Succeeded: 'success', Failed: 'danger', Pending: 'info', Stopped: 'info',
  }
  return map[status] || 'info'
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

const goWorkload = () => {
  if (run.value?.workloadId) {
    router.push({ path: '/training/detail', query: { id: run.value.workloadId } })
  }
}

const goModel = () => {
  if (run.value?.modelId) {
    router.push(`/model-square/detail/${run.value.modelId}`)
  }
}

const loadDetail = async () => {
  if (!props.runId) return
  loading.value = true
  try {
    run.value = (await getPostTrainRunDetail(props.runId)) as unknown as PostTrainRunItem
  } catch {
    run.value = null
  } finally {
    loading.value = false
  }
}

const loadMetrics = async () => {
  if (!props.runId) return
  metricsLoading.value = true
  try {
    metrics.value = (await getPostTrainRunMetrics(props.runId)) as unknown as PostTrainMetricsResp
  } catch {
    metrics.value = null
  } finally {
    metricsLoading.value = false
  }
}

watch(
  () => props.visible,
  (val) => {
    if (val && props.runId) {
      loadDetail()
      loadMetrics()
    } else {
      run.value = null
      metrics.value = null
    }
  },
)
</script>

<style scoped lang="scss">
.drawer-section {
  margin-bottom: 24px;

  .section-title {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 12px;
    padding-left: 8px;
    border-left: 3px solid var(--el-color-primary);
  }
}

.param-summary {
  font-family: monospace;
  font-size: 13px;
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  margin-bottom: 8px;
  word-break: break-all;
}

.param-json {
  font-size: 12px;
  line-height: 1.5;
  max-height: 300px;
  overflow: auto;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}

.loss-chart {
  margin-top: 12px;

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

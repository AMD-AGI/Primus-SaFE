<template>
  <div ref="wrapperRef" class="node-detail-wrapper" :style="{ width: containerWidth }">
    <el-card class="safe-card" shadow="never" v-loading="loading">
      <div v-if="summary.modelPath" class="mb-3 text-sm text-gray-500">
        <div>Model path: <span class="text-gray-700 dark:text-gray-200">{{ summary.modelPath }}</span></div>
        <div v-if="summary.message">Message: {{ summary.message }}</div>
      </div>
      <el-table :data="nodes" size="small" :max-height="360">
        <el-table-column label="Node" prop="node" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.node || row.adminNodeId || '-' }}
          </template>
        </el-table-column>
        <el-table-column label="Phase" prop="phase" width="120">
          <template #default="{ row }">
            <el-tag :type="phaseTagType(row.phase)" size="small">{{ row.phase || 'Pending' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Duration" width="110">
          <template #default="{ row }">
            {{ row.durationSeconds != null ? `${row.durationSeconds}s` : '-' }}
          </template>
        </el-table-column>
        <el-table-column label="Bytes Read" width="120">
          <template #default="{ row }">
            {{ formatBytes(row.bytesRead) }}
          </template>
        </el-table-column>
        <el-table-column label="Message" prop="message" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.message || '-' }}
          </template>
        </el-table-column>
      </el-table>
      <div class="mt-2 text-xs text-gray-400">
        Total: {{ summary.nodesTotal || nodes.length }}
        | Succeeded: {{ summary.nodesSucceeded || succeededCount }}
        | Failed: {{ summary.nodesFailed || failedCount }}
        | Pending: {{ summary.nodesPending || pendingCount }}
      </div>
    </el-card>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { getOpsjobsDetail } from '@/services'
import { ElMessage } from 'element-plus'
import {
  formatBytes,
  parseModelPrewarmOutputs,
  type ModelPrewarmNodeDetail,
} from '../utils/modelPrewarm'

const props = defineProps<{ jobId: string }>()

const loading = ref(false)
const nodes = ref<ModelPrewarmNodeDetail[]>([])
const summary = ref({
  modelPath: '',
  message: '',
  nodesTotal: '',
  nodesSucceeded: '',
  nodesFailed: '',
  nodesPending: '',
})
const wrapperRef = ref<HTMLElement>()
const containerWidth = ref('100%')

const succeededCount = computed(
  () => nodes.value.filter((n) => n.phase === 'Succeeded').length,
)
const failedCount = computed(() => nodes.value.filter((n) => n.phase === 'Failed').length)
const pendingCount = computed(
  () => nodes.value.filter((n) => !n.phase || n.phase === 'Pending' || n.phase === 'Running').length,
)

const phaseTagType = (phase?: string) => {
  const map: Record<string, string> = {
    Succeeded: 'success',
    Running: 'warning',
    Pending: 'info',
    Failed: 'danger',
  }
  return map[phase || ''] || 'info'
}

let resizeObserver: ResizeObserver | null = null

const calcWidth = () => {
  const scrollWrap = wrapperRef.value?.closest('.el-scrollbar__wrap')
  if (scrollWrap) {
    containerWidth.value = `${scrollWrap.clientWidth}px`
  }
}

const fetchDetail = async () => {
  loading.value = true
  try {
    const detail = await getOpsjobsDetail(props.jobId)
    const outputs = parseModelPrewarmOutputs(detail.outputs || [])
    summary.value = {
      modelPath: (detail.inputs || []).find((i: { name?: string }) => i.name === 'model.path')?.value || '',
      message: outputs.message || detail.conditions?.[0]?.message || '',
      nodesTotal: outputs.nodesTotal || '',
      nodesSucceeded: outputs.nodesSucceeded || '',
      nodesFailed: outputs.nodesFailed || '',
      nodesPending: outputs.nodesPending || '',
    }
    nodes.value = outputs.nodesDetail
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to load prewarm detail')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await nextTick()
  calcWidth()
  const scrollWrap = wrapperRef.value?.closest('.el-scrollbar__wrap')
  if (scrollWrap) {
    resizeObserver = new ResizeObserver(calcWidth)
    resizeObserver.observe(scrollWrap)
  }
  await fetchDetail()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
})

defineExpose({ refresh: fetchDetail })
</script>

<style scoped>
.node-detail-wrapper {
  padding: 8px 16px 16px;
}
</style>

<template>
  <div v-if="task" class="w-header">
    <div class="w-row">
      <div class="w-left">
        <el-button @click="router.back()" :icon="ArrowLeft" text type="primary" class="mr-2 mt-1">
          Back
        </el-button>
        <h1 class="w-name">{{ task.displayName || task.id }}</h1>
        <TaskStatusTag :status="liveStatus || task.status" class="ml-2" />
      </div>
      <div class="w-right">
        <el-button
          v-if="isRunning"
          type="warning"
          plain
          :icon="VideoPause"
          @click="handleInterrupt"
        >Interrupt</el-button>
        <el-button
          v-if="canRetry"
          type="primary"
          plain
          :icon="RefreshRight"
          @click="handleRetry"
        >Retry</el-button>
      </div>
    </div>
    <div class="w-meta">
      <span class="item"><span class="label">Model</span>{{ task.modelId }}</span>
      <span class="item"><span class="label">Workspace</span>{{ task.workspace }}</span>
      <span class="item"><span class="label">Created</span>{{ formatTimeStr(task.createdAt) }}</span>
    </div>
  </div>

  <!-- Phase Timeline -->
  <el-card v-if="phases.length" class="mt-4 safe-card" shadow="never">
    <div class="section-header">
      <span class="section-bar" />
      <span class="section-title">Phase Timeline</span>
    </div>
    <el-steps :active="activePhaseIdx" finish-status="success" process-status="process" align-center>
      <el-step
        v-for="p in uniquePhases"
        :key="p.phase"
        :title="p.phaseName"
        :status="phaseStatus(p)"
      />
    </el-steps>
  </el-card>

  <!-- Tabs -->
  <el-tabs v-model="activeTab" class="mt-4">
    <!-- Benchmark -->
    <el-tab-pane label="Benchmark" name="benchmark">
      <el-card class="safe-card" shadow="never">
        <el-empty v-if="!benchmarks.length" description="No benchmark data yet" />
        <el-table v-else :data="benchmarks" size="small">
          <el-table-column prop="label" label="Label" min-width="180" />
          <el-table-column prop="outputTokensPerSec" label="Output tok/s" width="130">
            <template #default="{ row }">{{ row.outputTokensPerSec != null ? Number(row.outputTokensPerSec).toFixed(1) : '-' }}</template>
          </el-table-column>
          <el-table-column prop="tpotMs" label="TPOT (ms)" width="110">
            <template #default="{ row }">{{ row.tpotMs != null ? Number(row.tpotMs).toFixed(2) : '-' }}</template>
          </el-table-column>
          <el-table-column prop="ttftMs" label="TTFT (ms)" width="110">
            <template #default="{ row }">{{ row.ttftMs != null ? Number(row.ttftMs).toFixed(2) : '-' }}</template>
          </el-table-column>
          <el-table-column prop="concurrency" label="Concurrency" width="110" />
          <el-table-column prop="framework" label="Framework" width="100" />
        </el-table>
      </el-card>
    </el-tab-pane>

    <!-- Kernel -->
    <el-tab-pane label="Kernels" name="kernel">
      <el-card class="safe-card" shadow="never">
        <el-empty v-if="!kernels.length" description="No kernel events yet" />
        <el-table v-else :data="kernels" size="small">
          <el-table-column prop="name" label="Kernel" min-width="220" />
          <el-table-column prop="backend" label="Backend" width="130" />
          <el-table-column prop="status" label="Status" width="120">
            <template #default="{ row }">
              <el-tag
                :type="row.status === 'patched' ? 'success' : row.status === 'failed' ? 'danger' : 'info'"
                size="small"
              >{{ row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Baseline (us)" width="120">
            <template #default="{ row }">{{ row.baselineUs != null ? Number(row.baselineUs).toFixed(1) : '-' }}</template>
          </el-table-column>
          <el-table-column label="Optimized (us)" width="130">
            <template #default="{ row }">{{ row.optimizedUs != null ? Number(row.optimizedUs).toFixed(1) : '-' }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </el-tab-pane>

    <!-- Logs -->
    <el-tab-pane label="Logs" name="log">
      <el-card class="safe-card" shadow="never">
        <el-empty v-if="!logs.length" description="No logs yet" />
        <div v-else ref="logContainer" class="log-box">
          <p v-for="(l, i) in logs" :key="i" :class="['log-line', `log-${l.level}`]">
            <span class="log-src">[{{ l.source }}]</span> {{ l.message }}
          </p>
        </div>
      </el-card>
    </el-tab-pane>

    <!-- Artifacts -->
    <el-tab-pane label="Artifacts" name="artifact">
      <el-card class="safe-card" shadow="never">
        <el-empty v-if="!artifacts.length" description="No artifacts yet" />
        <el-table v-else :data="artifacts" size="small">
          <el-table-column prop="path" label="Path" min-width="400" />
          <el-table-column label="Actions" width="120">
            <template #default="{ row }">
              <el-link type="primary" @click="handleDownload(row.path)">Download</el-link>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </el-tab-pane>
  </el-tabs>

  <el-skeleton v-if="detailLoading" :rows="8" animated class="mt-4" />
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, VideoPause, RefreshRight } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getOptimizationTask,
  interruptOptimizationTask,
  retryOptimizationTask,
  listOptimizationArtifacts,
  downloadOptimizationArtifact,
} from '@/services/model-optimization'
import type { OptimizationTask, ArtifactItem, PhasePayload } from '@/services/model-optimization/type'
import { formatTimeStr } from '@/utils'
import TaskStatusTag from './components/TaskStatusTag.vue'
import { useOptimizationEvents } from './composables/useOptimizationEvents'

const route = useRoute()
const router = useRouter()
const taskId = computed(() => (route.params.id as string) || '')

const task = ref<OptimizationTask | null>(null)
const detailLoading = ref(false)
const artifacts = ref<ArtifactItem[]>([])
const activeTab = ref('benchmark')
const logContainer = ref<HTMLElement | null>(null)

const {
  phases,
  benchmarks,
  kernels,
  logs,
  taskStatus: liveStatus,
  isDone,
  sseError,
  connect,
  close: closeSSE,
  reset: resetSSE,
} = useOptimizationEvents(taskId.value)

const isRunning = computed(() => (liveStatus.value || task.value?.status) === 'Running')
const canRetry = computed(() => {
  const s = liveStatus.value || task.value?.status
  return s === 'Failed' || s === 'Interrupted'
})

const uniquePhases = computed(() => {
  const map = new Map<number, PhasePayload>()
  for (const p of phases.value) map.set(p.phase, p)
  return [...map.values()].sort((a, b) => a.phase - b.phase)
})

const activePhaseIdx = computed(() => {
  const arr = uniquePhases.value
  let lastIdx = -1
  let lastItem: PhasePayload | null = null
  for (let i = arr.length - 1; i >= 0; i--) {
    if (arr[i].status === 'started' || arr[i].status === 'completed') {
      lastIdx = i
      lastItem = arr[i]
      break
    }
  }
  if (!lastItem) return 0
  return lastItem.status === 'completed' ? lastIdx + 1 : lastIdx
})

const phaseStatus = (p: PhasePayload) => {
  if (p.status === 'completed') return 'success'
  if (p.status === 'started') return 'process'
  if (p.status === 'failed') return 'error'
  return 'wait'
}

const fetchDetail = async () => {
  if (!taskId.value) return
  detailLoading.value = true
  try {
    task.value = await getOptimizationTask(taskId.value)
  } finally {
    detailLoading.value = false
  }
}

const fetchArtifacts = async () => {
  if (!taskId.value) return
  try {
    artifacts.value = await listOptimizationArtifacts(taskId.value)
  } catch {
    artifacts.value = []
  }
}

const handleDownload = (path: string) => {
  downloadOptimizationArtifact(taskId.value, path)
}

const handleInterrupt = async () => {
  try {
    await ElMessageBox.confirm('Interrupt this task?', 'Interrupt', {
      confirmButtonText: 'Interrupt',
      type: 'warning',
    })
    await interruptOptimizationTask(taskId.value)
    ElMessage.success('Task interrupted')
    fetchDetail()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
  }
}

const handleRetry = async () => {
  try {
    await ElMessageBox.confirm('Retry this task?', 'Retry', {
      confirmButtonText: 'Retry',
      type: 'info',
    })
    await retryOptimizationTask(taskId.value)
    ElMessage.success('Task retried')
    resetSSE()
    fetchDetail()
    connect()
  } catch (e) {
    if (e === 'cancel' || e === 'close') return
  }
}

watch(logs, async () => {
  await nextTick()
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}, { deep: true })

watch(isDone, (v) => {
  if (v) {
    fetchDetail()
    fetchArtifacts()
  }
})

onMounted(async () => {
  await fetchDetail()
  fetchArtifacts()
  const status = task.value?.status
  if (status === 'Running' || status === 'Pending') {
    connect()
  }
})
</script>

<style scoped>
.w-header { margin-bottom: 8px; }
.w-row { display: flex; justify-content: space-between; align-items: center; }
.w-left { display: flex; align-items: center; gap: 4px; }
.w-right { display: flex; gap: 8px; }
.w-name { font-size: 20px; font-weight: 600; margin: 0; }
.w-meta { display: flex; gap: 24px; margin-top: 8px; font-size: 13px; color: var(--el-text-color-secondary); }
.w-meta .label { font-weight: 500; margin-right: 6px; }

.section-header { display: flex; align-items: center; gap: 8px; margin-bottom: 16px; }
.section-bar { width: 3px; height: 16px; border-radius: 2px; background: var(--safe-primary, var(--el-color-primary)); }
.section-title { font-weight: 600; font-size: 14px; }

.log-box {
  max-height: calc(100vh - 380px);
  overflow-y: auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  background: #111;
  color: #ddd;
  padding: 12px;
  border-radius: 6px;
}
.log-line { margin: 0; padding: 1px 0; }
.log-src { color: #888; margin-right: 6px; }
.log-info { color: #0f0; }
.log-warn { color: #ff0; }
.log-error { color: #f44; }
</style>

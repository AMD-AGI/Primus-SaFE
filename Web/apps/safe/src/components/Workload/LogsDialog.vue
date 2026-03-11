<template>
  <el-dialog
    :model-value="visible"
    title="pod-log"
    width="70%"
    :close-on-click-modal="false"
    @close="emit('update:visible', false)"
    :draggable="true"
    class="resizable-dialog"
  >
    <div class="flex items-center justify-between">
      <el-space wrap>
        tailLines:
        <el-input v-model.number="tailLines" type="number" :max="10000" style="width: 300px" />
        <el-divider direction="vertical" />
        <el-switch v-model="autoRefresh" active-text="Auto" inactive-text="Manual" />
        <template v-if="autoRefresh">
          <span class="text-[12px] text-gray-500">Refresh in {{ remain }}s</span>
          <el-input-number v-model="intervalSec" :min="2" :max="60" size="small" :step="1" />
        </template>
      </el-space>

      <el-button v-if="logResponse.logs?.length" :icon="Refresh" @click="manualRefresh">
        Refresh
      </el-button>
    </div>

    <el-skeleton v-if="loading" class="m-t-2" :rows="10" animated />
    <div v-else-if="logResponse.logs?.length" ref="logContainer" class="log-box">
      <p v-for="(line, index) in logResponse.logs" :key="index">
        {{ line }}
      </p>
    </div>
    <div v-else>No Data</div>
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, ref, onMounted, watch, nextTick, onUnmounted } from 'vue'
import { getWorkloadLogsByPod } from '@/services/workload/index'
import type { GetWorkloadPodLogResponse } from '@/services'
import { Refresh } from '@element-plus/icons-vue'
// ===== Auto-refresh related =====
const autoRefresh = ref(true)
const intervalSec = ref(30) // Default 30 seconds
const remain = ref(intervalSec.value)
let timer: number | null = null

function clearTimer() {
  if (timer) {
    window.clearInterval(timer)
    timer = null
  }
}

function resetCountdown() {
  remain.value = intervalSec.value
}

function startTimer() {
  clearTimer()
  resetCountdown()
  timer = window.setInterval(() => {
    // Only decrement when dialog is visible, not loading, and auto-refresh is enabled
    if (!props.visible || loading.value || !autoRefresh.value) return
    remain.value -= 1
    if (remain.value <= 0) {
      // Trigger refresh and reset countdown
      getLogs()
      resetCountdown()
    }
  }, 1000)
}

// Manual refresh button: reset countdown after successful refresh
const manualRefresh = async () => {
  await getLogs()
  resetCountdown()
}

const tailLines = ref(1000)
const logResponse = ref<GetWorkloadPodLogResponse>({
  workloadId: '',
  namespace: '',
  podId: '',
  logs: [],
})
const loading = ref(false)
const logContainer = ref<HTMLElement | null>(null)

const getLogs = async () => {
  if (!props.wlid || !props.podid) return
  try {
    loading.value = true
    logResponse.value = await getWorkloadLogsByPod(props.wlid, props.podid, {
      tailLines: tailLines.value,
    })
  } catch (err) {
    console.error(err)
  } finally {
    loading.value = false
  }
}

const props = defineProps<{
  visible: boolean
  wlid?: string
  podid?: string
}>()
const emit = defineEmits<{ (e: 'update:visible', val: boolean): void }>()

onMounted(() => {
  if (props.visible) getLogs()
  startTimer()
})

onUnmounted(() => {
  clearTimer()
})

watch(
  () => logResponse.value.logs,
  async () => {
    await nextTick()
    if (logContainer.value) {
      logContainer.value.scrollTop = logContainer.value.scrollHeight
    }
  },
  { deep: true },
)

// Switch pod: fetch immediately and reset countdown
watch(
  () => props.podid,
  () => {
    getLogs()
    resetCountdown()
  },
)

// Dialog visibility: reset and start timer on show; timer is kept but won't decrement when hidden
watch(
  () => props.visible,
  (v) => {
    if (v) {
      getLogs()
      startTimer()
    } else {
      resetCountdown()
    }
  },
)

// Changes to tailLines / interval / toggle: all reset countdown
watch(tailLines, resetCountdown)
watch(intervalSec, () => startTimer())
watch(autoRefresh, () => startTimer())
</script>
<style scoped>
.log-box {
  flex: 1; /* Adaptive height */
  overflow-y: auto;
  font-family: monospace;
  white-space: pre-wrap;
  background: #111;
  color: #0f0;
  padding: 10px;
  border-radius: 6px;
  margin: 20px 0;
}
.log-box p {
  margin: 0; /* Remove default margin */
  padding: 10px 4px; /* Add some line spacing */
  line-height: 1.4;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08); /* Thin separator line */
}

/* No border for the last line */
.log-box p:last-child {
  border-bottom: none;
}

/* Highlight current line on hover */
.log-box p:hover {
  background-color: rgba(255, 255, 255, 0.08);
}
</style>
<style>
.el-dialog.resizable-dialog {
  display: flex;
  flex-direction: column;
  resize: both;
  overflow: hidden;
  min-width: 420px;
  min-height: 260px;
  height: 800px;
  max-width: calc(100vw - 48px);
  max-height: calc(100vh - 48px);
}

.el-dialog.resizable-dialog .el-dialog__body {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
</style>

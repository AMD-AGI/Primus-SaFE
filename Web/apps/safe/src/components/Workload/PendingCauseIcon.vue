<template>
  <span class="pending-cause" @click.stop="analyze">
    <el-tooltip content="Pending Cause Analysis" placement="top">
      <el-icon
        class="pending-cause-icon"
        :class="{ 'pulse-scale': pulse && !loading }"
        :size="18"
      >
        <Loading v-if="loading" class="is-loading" />
        <InfoFilled v-else />
      </el-icon>
    </el-tooltip>
  </span>
  <el-dialog
    v-model="visible"
    title="Pending Cause Analysis"
    width="520px"
    append-to-body
  >
    <div class="pending-cause-body">
      <template v-if="loading">
        <el-icon class="is-loading"><Loading /></el-icon>
        <span>Analyzing...</span>
      </template>
      <span v-else>{{ text }}</span>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { InfoFilled, Loading } from '@element-plus/icons-vue'
import { getWorkloadPendingReason } from '@/services/workload'

const props = withDefaults(
  defineProps<{
    workloadId?: string
    pulse?: boolean
  }>(),
  { pulse: false },
)

const visible = ref(false)
const loading = ref(false)
const text = ref('')

const errorText = (err: unknown) => {
  const body = (err as { response?: { data?: { errorMessage?: string; message?: string } } })
    ?.response?.data
  if (body?.errorMessage) return body.errorMessage
  if (body?.message) return body.message
  const message = (err as { message?: string })?.message
  return message || 'Failed to load the pending cause.'
}

const analyze = async () => {
  if (!props.workloadId || loading.value) return
  visible.value = true
  loading.value = true
  text.value = ''
  try {
    const res = await getWorkloadPendingReason(props.workloadId)
    text.value = res?.reason?.trim() || 'No pending cause.'
  } catch (err) {
    text.value = errorText(err)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.pending-cause {
  display: inline-flex;
  align-items: center;
  vertical-align: middle;
}
.pending-cause-icon {
  cursor: pointer;
  color: var(--el-color-warning);
  flex: none;
  transition:
    color 0.2s ease,
    transform 0.2s ease;
}
.pending-cause-icon:hover {
  color: var(--el-color-primary);
  transform: scale(1.2);
}
.pending-cause-body {
  display: flex;
  align-items: center;
  gap: 8px;
  line-height: 1.6;
  word-break: break-word;
}
.pulse-scale {
  animation: pulse-scale 1.4s ease-in-out infinite;
}
@keyframes pulse-scale {
  0%,
  100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.2);
  }
}
</style>

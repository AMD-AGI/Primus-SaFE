<template>
  <el-timeline size="large" class="m-t-4">
    <el-timeline-item
      v-for="(c, i) in conditions ?? []"
      :key="c.lastTransitionTime || i"
      placement="top"
      :icon="statusOf(c.type).icon"
      :color="statusOf(c.type).color"
    >
      <div class="flex items-center gap-2 textx-14">
        <span class="font-medium">{{ formatTimeStr(c.lastTransitionTime) }}</span>
        <span class="font-medium">{{ c.type }}</span>
      </div>

      <div
        v-if="c.reason || c.message"
        class="mt-1 textx-12.5 leading-snug break-all"
        style="color: var(--el-text-color-secondary)"
      >
        <span v-if="c.reason" class="mr-1">{{ c.reason }}:</span>
        {{ c.message || '—' }}
      </div>
    </el-timeline-item>
  </el-timeline>
</template>

<script setup lang="ts">
import { Clock, CircleClose, CircleCheck, InfoFilled } from '@element-plus/icons-vue'
import type { Component } from 'vue'
import { formatTimeStr } from '@/utils'

defineProps<{
  conditions?: any[]
}>()

type StatusConf = { color: string; icon: Component }
const STATUS_MAP: Record<string, StatusConf> = {
  K8sRunning: { color: '#f59e0b', icon: Clock },
  K8sFailed: { color: '#ef4444', icon: CircleClose },
  K8sSucceeded: { color: '#22c55e', icon: CircleCheck },
}
const DEFAULT_STATUS: StatusConf = { color: '#9ca3af', icon: InfoFilled }

const statusOf = (t?: string): StatusConf => STATUS_MAP[t ?? ''] ?? DEFAULT_STATUS
</script>

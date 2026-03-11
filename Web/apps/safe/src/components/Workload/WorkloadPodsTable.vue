<template>
  <el-card class="mt-2 safe-card" shadow="never">
    <el-table v-if="pods && pods.length" :data="pods">
      <el-table-column prop="podId" label="PodId" min-width="260" show-overflow-tooltip />
      <el-table-column prop="adminNodeName" label="NodeName" min-width="220">
        <template #default="{ row }">
          <div class="flex flex-col items-start" v-if="row.adminNodeName">
            <div class="text-sm">{{ row.adminNodeName }}</div>
            <div class="text-[13px] text-gray-400">
              {{ row.hostIP }}
            </div>
          </div>
          <div v-else>-</div>
        </template>
      </el-table-column>
      <el-table-column prop="phase" min-width="180">
        <template #header>
          <div class="flex items-center">
            <span>Phase</span>
            <el-icon
              class="ml-1 cursor-pointer hover:text-[var(--el-color-primary)] transition"
              :class="{ 'is-loading': refreshLoading }"
              @click="emit('refresh')"
            >
              <Refresh />
            </el-icon>
          </div>
        </template>
        <template #default="{ row }">
          <el-tag :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'">
            {{ row.phase }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="startTime" label="StartTime" min-width="160">
        <template #default="{ row }">
          {{ formatTimeStr(row.startTime) }}
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="140" fixed="right">
        <template #default="{ row }">
          <el-tooltip
            effect="dark"
            content="Only available when Running"
            :disabled="workloadPhase === 'Running'"
            placement="top"
          >
            <span style="display: inline-block">
              <el-button
                type="text"
                size="small"
                :disabled="row.phase !== 'Running'"
                @click="emit('openLog', row.podId)"
              >
                Logs
              </el-button>
            </span>
          </el-tooltip>
          <el-tooltip
            v-if="showSsh"
            effect="dark"
            content="Only available when Running"
            :disabled="workloadPhase === 'Running'"
            placement="top"
          >
            <span style="display: inline-block">
              <el-button
                type="text"
                size="small"
                :disabled="row.phase !== 'Running'"
                @click="emit('openSsh', row.podId, row.sshCommand)"
              >
                SSH
              </el-button>
            </span>
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import { Refresh } from '@element-plus/icons-vue'
import { WorkloadPhaseButtonType } from '@/services'
import { formatTimeStr } from '@/utils'

defineProps<{
  pods?: any[]
  workloadPhase?: string
  showSsh?: boolean
  refreshLoading?: boolean
}>()

const emit = defineEmits<{
  (e: 'openLog', podId: string): void
  (e: 'openSsh', podId: string, sshCommand?: string): void
  (e: 'refresh'): void
}>()
</script>


<template>
  <el-table :data="rows" size="small" empty-text="No running workloads">
    <el-table-column label="Workload" min-width="160" show-overflow-tooltip>
      <template #default="{ row }">{{ row.displayName || row.workloadId }}</template>
    </el-table-column>
    <el-table-column prop="userName" label="User" min-width="100" show-overflow-tooltip />
    <el-table-column label="GPUs" width="80" align="right">
      <template #default="{ row }">{{ fmtNum(row.allocated) }}</template>
    </el-table-column>
    <el-table-column label="GPU Util" width="100" align="right">
      <template #default="{ row }">
        <span :class="`usage-tone--${utilizationTone(toRatio(row.gpuUtilization))}`">
          {{ fmtPercent(row.gpuUtilization) }}
        </span>
      </template>
    </el-table-column>
  </el-table>
</template>

<script setup lang="ts">
import type { UsageWorkloadItem } from '@/services/usage/type'
import { fmtNum, fmtPercent, utilizationTone } from '../usageUtils'

defineProps<{ rows: UsageWorkloadItem[] }>()

// Workload utilization is a 0..100 percentage; the tone thresholds work on a 0..1 ratio.
const toRatio = (percent: number | null) => (percent == null ? null : percent / 100)
</script>

<style scoped>
.usage-tone--danger {
  color: var(--el-color-danger);
}
.usage-tone--warning {
  color: var(--el-color-warning);
}
.usage-tone--success {
  color: var(--el-color-success);
}
</style>

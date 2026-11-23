<template>
  <div class="usage-summary">
    <StatCard label="Occupancy">
      {{ fmtRatio(summary?.occupancy) }}
    </StatCard>
    <StatCard label="Utilization">
      <span :class="`usage-tone--${utilizationTone(summary?.utilization)}`">
        {{ fmtRatio(summary?.utilization) }}
      </span>
    </StatCard>
    <StatCard label="Effective Utilization">
      {{ fmtRatio(summary?.effectiveUtilization) }}
    </StatCard>
    <StatCard label="Allocated GPU Hours">
      {{ fmtNum(summary?.allocatedHours) }}
    </StatCard>
    <StatCard label="Queued Avg / Max" suffix="GPUs">
      {{ fmtNum(summary?.queuedAvg) }} / {{ fmtNum(summary?.queuedMax) }}
    </StatCard>
    <StatCard label="Now Allocated / Quota" suffix="GPUs">
      <template v-if="current"
        >{{ fmtNum(current.allocated) }} / {{ fmtNum(current.quota) }}</template
      >
      <template v-else>-</template>
    </StatCard>
  </div>
</template>

<script setup lang="ts">
import StatCard from '@/components/Base/StatCard.vue'
import type { UsagePoint, UsageSummary } from '@/services/usage/type'
import { fmtNum, fmtRatio, utilizationTone } from '../usageUtils'

defineProps<{
  summary: UsageSummary | null
  /** Latest sample; null renders "-" in the Now card. */
  current: UsagePoint | null
}>()
</script>

<style scoped>
.usage-summary {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;
}
.usage-tone--danger {
  color: var(--el-color-danger);
}
.usage-tone--warning {
  color: var(--el-color-warning);
}
.usage-tone--success {
  color: var(--el-color-success);
}
@media (max-width: 1400px) {
  .usage-summary {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}
</style>

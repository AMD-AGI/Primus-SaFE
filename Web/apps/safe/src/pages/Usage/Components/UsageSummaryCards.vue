<template>
  <div class="usage-summary">
    <div class="usage-summary__grid">
      <StatCard flat label="Occupancy" caption="allocated / quota">
        {{ fmtRatio(summary?.occupancy) }}
      </StatCard>
      <StatCard flat label="Utilization" caption="busy / allocated">
        <span :class="`usage-tone--${utilizationTone(summary?.utilization)}`">
          {{ fmtRatio(summary?.utilization) }}
        </span>
      </StatCard>
      <StatCard flat label="Effective Utilization" caption="busy / quota">
        {{ fmtRatio(summary?.effectiveUtilization) }}
      </StatCard>
      <StatCard flat label="Allocated GPU Hours" :value="fmtNum(summary?.allocatedHours)" />
      <StatCard flat label="Queued Avg / Max" caption="GPUs">
        {{ fmtNum(summary?.queuedAvg) }} / {{ fmtNum(summary?.queuedMax) }}
      </StatCard>
      <StatCard flat label="Now Allocated / Quota" caption="GPUs">
        <template v-if="current">
          {{ fmtNum(current.allocated) }} / {{ fmtNum(current.quota) }}
        </template>
        <template v-else>-</template>
      </StatCard>
    </div>
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
/* Sized by its own width so it also fits the narrower Homepage column. */
.usage-summary {
  container-type: inline-size;
}
.usage-summary__grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;
}
@container (max-width: 1100px) {
  .usage-summary__grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
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
</style>

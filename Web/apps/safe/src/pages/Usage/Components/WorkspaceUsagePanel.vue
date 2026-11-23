<template>
  <div v-loading="loading" class="workspace-usage">
    <UsageSummaryCards :summary="data?.summary ?? null" :current="data?.current ?? null" />

    <el-alert
      v-if="lowCoverage"
      type="warning"
      show-icon
      :closable="false"
      title="GPU metrics cover less than 90% of the allocated time in part of this range; utilization there is indicative."
    />

    <el-card shadow="never" class="safe-card">
      <SectionHeader title="Allocated vs Busy GPUs" />
      <UsageTrendChart :series="series" :height="chartHeight" />
    </el-card>

    <el-card shadow="never" class="safe-card">
      <SectionHeader title="Queued GPUs by Reason" />
      <UsageQueuedChart :series="series" />
    </el-card>

    <div class="workspace-usage__tables">
      <el-card shadow="never" class="safe-card">
        <SectionHeader title="Top Allocated Workloads" />
        <UsageWorkloadTable :rows="data?.topAllocated ?? []" />
      </el-card>
      <el-card shadow="never" class="safe-card">
        <SectionHeader title="Lowest Utilization Workloads" />
        <UsageWorkloadTable :rows="data?.lowestUtilization ?? []" />
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import SectionHeader from '@/components/Base/SectionHeader.vue'
import type { GetUsageWorkspaceResp } from '@/services/usage/type'
import { COVERAGE_WARN_THRESHOLD } from '../usageUtils'
import UsageQueuedChart from './UsageQueuedChart.vue'
import UsageSummaryCards from './UsageSummaryCards.vue'
import UsageTrendChart from './UsageTrendChart.vue'
import UsageWorkloadTable from './UsageWorkloadTable.vue'

const props = withDefaults(
  defineProps<{
    data: GetUsageWorkspaceResp | null
    loading?: boolean
    chartHeight?: number
  }>(),
  { loading: false, chartHeight: 300 },
)

const series = computed(() => props.data?.series ?? [])
const lowCoverage = computed(() =>
  series.value.some((p) => p.coverage != null && p.coverage < COVERAGE_WARN_THRESHOLD),
)
</script>

<style scoped>
.workspace-usage {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.workspace-usage__tables {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}
@media (max-width: 1100px) {
  .workspace-usage__tables {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>

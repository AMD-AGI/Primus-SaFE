<template>
  <div v-loading="loading" class="cluster-cards">
    <div v-if="items.length" class="cluster-cards__grid" :style="gridStyle">
      <el-card
        v-for="item in items"
        :key="item.cluster"
        shadow="never"
        class="safe-card cluster-cards__card"
        @click="emit('select', item.cluster)"
      >
        <SectionHeader :title="item.cluster">
          <template #actions>
            <el-link type="primary" :underline="false">
              View workspaces<el-icon class="ml-1"><ArrowRight /></el-icon>
            </el-link>
          </template>
        </SectionHeader>
        <div class="cluster-cards__meta">
          {{ item.workspaces }} workspaces · quota {{ fmtNum(item.current?.quota) }} GPUs
        </div>
        <div class="cluster-cards__kpis">
          <StatCard flat label="Occupancy" :value="fmtRatio(item.summary.occupancy)" />
          <StatCard flat label="Utilization">
            <span :class="`usage-tone--${utilizationTone(item.summary.utilization)}`">
              {{ fmtRatio(item.summary.utilization) }}
            </span>
          </StatCard>
          <StatCard flat label="Queued Max" :value="fmtNum(item.summary.queuedMax)" />
        </div>
        <UsageTrendChart :series="item.series" :height="56" compact />
      </el-card>
    </div>
    <el-empty v-else-if="!loading" description="No usage data in this range" />
    <div v-if="items.length" class="cluster-cards__legend">
      <span class="cluster-cards__legend-item">
        <i class="cluster-cards__swatch" :style="{ borderColor: USAGE_COLORS.allocated }" />
        Allocated
      </span>
      <span class="cluster-cards__legend-item">
        <i class="cluster-cards__swatch" :style="{ borderColor: USAGE_COLORS.busy }" />
        Busy
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight } from '@element-plus/icons-vue'
import SectionHeader from '@/components/Base/SectionHeader.vue'
import StatCard from '@/components/Base/StatCard.vue'
import type { UsageClusterItem } from '@/services/usage/type'
import { fmtNum, fmtRatio, utilizationTone } from '../usageUtils'
import { USAGE_COLORS } from '../useUsageChart'
import UsageTrendChart from './UsageTrendChart.vue'

const props = defineProps<{ items: UsageClusterItem[]; loading: boolean }>()
const emit = defineEmits<{ (e: 'select', cluster: string): void }>()

// Three cards per row; with fewer clusters the cards share the whole row.
const gridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${Math.min(3, Math.max(1, props.items.length))}, minmax(0, 1fr))`,
}))
</script>

<style scoped>
.cluster-cards {
  min-height: 160px;
}
.cluster-cards__grid {
  display: grid;
  gap: 16px;
}
.cluster-cards__card {
  cursor: pointer;
  transition: border-color 0.16s ease-out;
}
.cluster-cards__card:hover {
  border-color: var(--safe-primary);
}
.cluster-cards__meta {
  margin-bottom: 12px;
  color: var(--safe-muted);
  font-size: var(--fs-caption);
}
.cluster-cards__kpis {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin-bottom: 12px;
}
.cluster-cards__legend {
  display: flex;
  gap: 16px;
  margin-top: 10px;
  color: var(--safe-muted);
  font-size: var(--fs-caption);
}
.cluster-cards__legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.cluster-cards__swatch {
  width: 14px;
  border-top: 2px solid;
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
@media (max-width: 960px) {
  .cluster-cards__grid {
    grid-template-columns: minmax(0, 1fr) !important;
  }
}
</style>

<template>
  <div v-loading="loading" class="cluster-cards">
    <el-card
      v-for="item in items"
      :key="item.cluster"
      shadow="never"
      class="safe-card cluster-cards__card"
      @click="emit('select', item.cluster)"
    >
      <template #header>
        <div class="cluster-cards__head">
          <span class="cluster-cards__name">{{ item.cluster }}</span>
          <el-link type="primary" :underline="false">
            View workspaces<el-icon class="ml-1"><ArrowRight /></el-icon>
          </el-link>
        </div>
      </template>
      <div class="cluster-cards__meta">
        {{ item.workspaces }} workspaces · quota {{ fmtNum(item.current?.quota) }} GPUs
      </div>
      <div class="cluster-cards__kpis">
        <StatCard label="Occupancy" :value="fmtRatio(item.summary.occupancy)" />
        <StatCard label="Utilization">
          <span :class="`usage-tone--${utilizationTone(item.summary.utilization)}`">
            {{ fmtRatio(item.summary.utilization) }}
          </span>
        </StatCard>
        <StatCard label="Queued Max" :value="fmtNum(item.summary.queuedMax)" />
      </div>
      <UsageTrendChart :series="item.series" :height="48" compact />
    </el-card>
    <el-empty
      v-if="!loading && !items.length"
      class="cluster-cards__empty"
      description="No usage data in this range"
    />
  </div>
</template>

<script setup lang="ts">
import { ArrowRight } from '@element-plus/icons-vue'
import StatCard from '@/components/Base/StatCard.vue'
import type { UsageClusterItem } from '@/services/usage/type'
import { fmtNum, fmtRatio, utilizationTone } from '../usageUtils'
import UsageTrendChart from './UsageTrendChart.vue'

defineProps<{ items: UsageClusterItem[]; loading: boolean }>()
const emit = defineEmits<{ (e: 'select', cluster: string): void }>()
</script>

<style scoped>
.cluster-cards {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 12px;
  min-height: 120px;
}
.cluster-cards__card {
  margin-bottom: 0;
  cursor: pointer;
}
.cluster-cards__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.cluster-cards__name {
  font-weight: 600;
  font-size: var(--fs-subtitle);
}
.cluster-cards__meta {
  margin-bottom: 10px;
  color: var(--safe-muted);
  font-size: var(--fs-caption);
}
.cluster-cards__kpis {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-bottom: 10px;
}
.cluster-cards__empty {
  grid-column: 1 / -1;
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

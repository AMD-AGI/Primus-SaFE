<template>
  <div v-loading="loading" class="cluster-cards">
    <div v-if="items.length" class="cluster-cards__grid" :style="gridStyle">
      <el-card v-for="item in items" :key="item.cluster" shadow="never" class="safe-card">
        <SectionHeader :title="item.cluster" clickable @click="emit('select', item.cluster)">
          <template #actions>
            <el-link type="primary" :underline="false">
              View workspaces<el-icon class="ml-1"><ArrowRight /></el-icon>
            </el-link>
          </template>
        </SectionHeader>
        <div class="cluster-cards__meta">
          {{ item.workspaces }} workspaces · quota {{ fmtNum(item.current?.quota) }} GPUs
        </div>
        <UsageSummaryCards :summary="item.summary" :current="item.current" />
        <UsageTrendChart :series="item.series" :height="200" class="cluster-cards__chart" />
      </el-card>
    </div>
    <el-empty v-else-if="!loading" description="No usage data in this range" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ArrowRight } from '@element-plus/icons-vue'
import SectionHeader from '@/components/Base/SectionHeader.vue'
import type { UsageClusterItem } from '@/services/usage/type'
import { fmtNum } from '../usageUtils'
import UsageSummaryCards from './UsageSummaryCards.vue'
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
.cluster-cards__meta {
  margin-bottom: 12px;
  color: var(--safe-muted);
  font-size: var(--fs-caption);
}
.cluster-cards__chart {
  margin-top: 16px;
}
@media (max-width: 960px) {
  .cluster-cards__grid {
    grid-template-columns: minmax(0, 1fr) !important;
  }
}
</style>

<template>
  <div class="cluster-usage">
    <UsageSummaryCards :summary="summary" :current="current" />

    <el-card shadow="never" class="safe-card">
      <SectionHeader title="Allocated vs Busy GPUs" />
      <UsageTrendChart :series="series" :height="300" />
    </el-card>

    <el-card shadow="never" class="safe-card">
      <SectionHeader title="Workspaces">
        <template #actions>
          <div class="cluster-usage__sort">
            <span class="cluster-usage__sort-label">Sort by</span>
            <el-segmented v-model="sort" size="small" :options="sortOptions" />
          </div>
        </template>
      </SectionHeader>
      <el-table v-loading="loading" :data="items" empty-text="No workspaces">
        <el-table-column label="Workspace" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            <el-link type="primary" :underline="false" @click="emit('select', row.workspace)">
              {{ row.workspace }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="Quota" width="90" align="right">
          <template #default="{ row }">{{ fmtNum(row.current?.quota) }}</template>
        </el-table-column>
        <el-table-column label="Occupancy" min-width="160">
          <template #default="{ row }">
            <UsageRatioBar :value="row.summary.occupancy" />
          </template>
        </el-table-column>
        <el-table-column label="Utilization" min-width="160">
          <template #default="{ row }">
            <UsageRatioBar
              :value="row.summary.utilization"
              :tone="utilizationTone(row.summary.utilization)"
            />
          </template>
        </el-table-column>
        <el-table-column label="Effective" width="100" align="right">
          <template #default="{ row }">{{ fmtRatio(row.summary.effectiveUtilization) }}</template>
        </el-table-column>
        <el-table-column label="GPU Hours" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.summary.allocatedHours) }}</template>
        </el-table-column>
        <el-table-column label="Queued Max" width="110" align="right">
          <template #default="{ row }">{{ fmtNum(row.summary.queuedMax) }}</template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import SectionHeader from '@/components/Base/SectionHeader.vue'
import type { UsagePoint, UsageSummary, UsageWorkspaceItem } from '@/services/usage/type'
import { fmtNum, fmtRatio, utilizationTone, type WorkspaceSort } from '../usageUtils'
import UsageRatioBar from './UsageRatioBar.vue'
import UsageSummaryCards from './UsageSummaryCards.vue'
import UsageTrendChart from './UsageTrendChart.vue'

defineProps<{
  summary: UsageSummary | null
  current: UsagePoint | null
  series: UsagePoint[]
  items: UsageWorkspaceItem[]
  loading: boolean
}>()

const sort = defineModel<WorkspaceSort>('sort', { required: true })
const emit = defineEmits<{ (e: 'select', workspace: string): void }>()

const sortOptions = [
  { label: 'Occupancy', value: 'occupancy' },
  { label: 'Utilization', value: 'utilization' },
  { label: 'Effective', value: 'effectiveUtilization' },
  { label: 'GPU hours', value: 'allocatedHours' },
  { label: 'Name', value: 'name' },
]
</script>

<style scoped>
.cluster-usage {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.cluster-usage__sort {
  display: flex;
  align-items: center;
  gap: 8px;
}
.cluster-usage__sort-label {
  color: var(--safe-muted);
  font-size: var(--fs-caption);
}
</style>

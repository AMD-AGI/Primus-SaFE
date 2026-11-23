<template>
  <el-card shadow="never" class="safe-card">
    <SectionHeader title="Clusters" />
    <el-table v-loading="loading" :data="items" empty-text="No usage data in this range">
      <el-table-column label="Cluster" min-width="140" show-overflow-tooltip>
        <template #default="{ row }">
          <el-link type="primary" :underline="false" @click="emit('select', row.cluster)">
            {{ row.cluster }}
          </el-link>
        </template>
      </el-table-column>
      <el-table-column label="Workspaces" prop="workspaces" width="110" align="right" />
      <el-table-column label="Quota" width="90" align="right">
        <template #default="{ row }">{{ fmtNum(row.current?.quota) }}</template>
      </el-table-column>
      <el-table-column label="Occupancy" min-width="150">
        <template #default="{ row }">
          <UsageRatioBar :value="row.summary.occupancy" />
        </template>
      </el-table-column>
      <el-table-column label="Utilization" min-width="150">
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
      <el-table-column min-width="200">
        <template #header>
          <span :style="{ color: USAGE_COLORS.allocated }">Allocated</span>
          /
          <span :style="{ color: USAGE_COLORS.busy }">Busy</span>
        </template>
        <template #default="{ row }">
          <UsageTrendChart :series="row.series" :height="36" compact />
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script setup lang="ts">
import SectionHeader from '@/components/Base/SectionHeader.vue'
import type { UsageClusterItem } from '@/services/usage/type'
import { fmtNum, fmtRatio, utilizationTone } from '../usageUtils'
import { USAGE_COLORS } from '../useUsageChart'
import UsageRatioBar from './UsageRatioBar.vue'
import UsageTrendChart from './UsageTrendChart.vue'

defineProps<{ items: UsageClusterItem[]; loading: boolean }>()
const emit = defineEmits<{ (e: 'select', cluster: string): void }>()
</script>

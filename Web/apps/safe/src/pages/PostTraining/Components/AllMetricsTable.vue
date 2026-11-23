<template>
  <div>
    <div class="flex items-center justify-between mb-2">
      <span class="font-500">All metrics</span>
      <el-input v-model="search" placeholder="Search metrics" clearable style="width: 220px" />
    </div>
    <el-table :data="rows" size="small" row-key="name">
      <el-table-column type="expand">
        <template #default="{ row }">
          <MetricsPanel :title="row.name" :series="[row.series]" :smoothing="smoothing" />
        </template>
      </el-table-column>
      <el-table-column label="Metric" min-width="240">
        <template #default="{ row }">
          {{ row.name }}
          <el-tag v-if="row.unpaneled" size="small" type="info" effect="plain" class="ml-1">
            Not in a panel
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Latest" width="150">
        <template #default="{ row }">{{ row.latest }}</template>
      </el-table-column>
      <el-table-column label="Min" width="110">
        <template #default="{ row }">{{ fmt(row.min) }}</template>
      </el-table-column>
      <el-table-column label="Max" width="110">
        <template #default="{ row }">{{ fmt(row.max) }}</template>
      </el-table-column>
      <el-table-column label="Best" width="150">
        <template #default="{ row }">{{ row.best }}</template>
      </el-table-column>
      <el-table-column label="Points" width="90" prop="count" />
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { betterOf, type MetricSeries } from '@/services/postTraining'
import MetricsPanel from './MetricsPanel.vue'

const props = withDefaults(
  defineProps<{
    series: MetricSeries[]
    better?: Record<string, 'lower' | 'higher'>
    /** The metrics no panel draws; they are listed first. */
    unassigned?: string[]
    smoothing?: number
  }>(),
  { better: undefined, unassigned: () => [], smoothing: 0.6 },
)

const search = ref('')

const fmt = (v?: number) =>
  v === undefined
    ? '—'
    : Math.abs(v) >= 1000 || (Math.abs(v) < 0.001 && v !== 0)
      ? v.toExponential(3)
      : v.toPrecision(4)

/**
 * One row per metric: the latest value and point count from the API, the min and max over the points, which keep
 * every bucket's extremes when downsampled, and the best value and its step when the registry gives a direction.
 */
const rows = computed(() => {
  const q = search.value.trim().toLowerCase()
  const unpaneled = new Set(props.unassigned)
  return props.series
    .filter((s) => !q || s.name.toLowerCase().includes(q))
    .map((s) => {
      const values = s.points.map((p) => p[1])
      const min = values.length ? Math.min(...values) : undefined
      const max = values.length ? Math.max(...values) : undefined
      const dir = betterOf(props.better, s.name)
      const bestPoint = dir
        ? s.points.reduce<[number, number] | undefined>(
            (b, p) => (!b || (dir === 'lower' ? p[1] < b[1] : p[1] > b[1]) ? p : b),
            undefined,
          )
        : undefined
      return {
        name: s.name,
        series: s,
        unpaneled: unpaneled.has(s.name),
        latest: s.last ? `${fmt(s.last[1])} @ ${s.last[0]}` : '—',
        min,
        max,
        best: bestPoint ? `${fmt(bestPoint[1])} @ ${bestPoint[0]}` : '—',
        count: s.count,
      }
    })
    .sort((a, b) => Number(b.unpaneled) - Number(a.unpaneled) || a.name.localeCompare(b.name))
})
</script>

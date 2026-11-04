<template>
  <div>
    <div class="flex items-center gap-2 mb-2">
      <span class="font-500">System</span>
      <el-tag size="small" type="info" effect="plain">Time axis</el-tag>
      <span v-if="hasData" class="text-[12px] text-[var(--safe-muted)]"
        >{{ countText }} · every 30 s</span
      >
      <el-link
        v-if="workloadId"
        type="primary"
        :underline="false"
        class="ml-auto"
        @click="router.push({ path: '/rayjob/detail', query: { id: workloadId } })"
      >
        Open in Grafana
      </el-link>
    </div>
    <div v-if="!hasData" class="text-[13px] text-[var(--safe-muted)]">
      No GPU metrics: the launcher found no GPU it could read in this run's containers, or the image
      has neither the ROCm SMI library nor amd-smi. The training curves above are not affected.
    </div>
    <div v-else class="pt-grid">
      <MetricsPanel
        title="GPU utilization (%) · minutes since training started"
        :series="utilSeries"
        :colors="roleColors"
        :bands="utilBands"
        :smoothing="0"
      />
      <MetricsPanel
        title="GPU memory (GiB) · minutes since training started"
        :series="memSeries"
        :colors="memColors"
        :smoothing="0"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import type { MetricSeries } from '@/services/postTraining'
import MetricsPanel from './MetricsPanel.vue'

const props = defineProps<{
  /** The run's system/* series, whose step counts seconds since training first started. */
  series: MetricSeries[]
  /** The GPUs the run asked for, and how many train when training and rollout are separate. */
  layout?: { nodes?: number; gpusPerNode?: number; colocate?: boolean; actorGpus?: number }
  workloadId?: string
}>()

const router = useRouter()

/** The role groups the launcher reports: one for all GPUs, or train and rollout when they are separate. */
const roles = computed(() => (props.layout?.colocate === false ? ['train/', 'rollout/'] : ['']))
const byName = computed(() => new Map(props.series.map((s) => [s.name, s])))

/** A series on the time axis in minutes, renamed. */
const inMinutes = (s: MetricSeries | undefined, name: string): MetricSeries | undefined =>
  s && { ...s, name, points: s.points.map(([x, y]) => [x / 60, y]) }

const label = (role: string) => (role ? role.slice(0, -1) : 'all GPUs')
const get = (role: string, metric: string) => byName.value.get(`system/${role}${metric}`)

const hasData = computed(() => roles.value.some((r) => get(r, 'gpu_util_mean')))
const roleColors = ['#0d9488', '#6366f1']

const utilSeries = computed(() =>
  roles.value
    .map((r) => inMinutes(get(r, 'gpu_util_mean'), `mean · ${label(r)}`))
    .filter((s): s is MetricSeries => !!s),
)
const utilBands = computed(() =>
  roles.value.flatMap((r, i) => {
    const low = inMinutes(get(r, 'gpu_util_min'), `min · ${label(r)}`)
    const high = inMinutes(get(r, 'gpu_util_max'), `max · ${label(r)}`)
    return low && high ? [{ low, high, color: roleColors[i] }] : []
  }),
)

const memSeries = computed(() =>
  roles.value.flatMap((r) =>
    [
      inMinutes(get(r, 'gpu_mem_used_mean_gb'), `used mean · ${label(r)}`),
      inMinutes(get(r, 'gpu_mem_used_max_gb'), `used max · ${label(r)}`),
      inMinutes(get(r, 'gpu_mem_total_gb'), `total · ${label(r)}`),
    ].filter((s): s is MetricSeries => !!s),
  ),
)
const memColors = computed(() =>
  roles.value.flatMap((_, i) =>
    i === 0 ? ['#0d9488', '#f59e0b', '#9aa7bd'] : ['#6366f1', '#e11d48', '#cbd5e1'],
  ),
)

/** How many GPUs the last sample counted, against how many the run asked for. */
const countText = computed(() => {
  const counted = roles.value.reduce((n, r) => n + (get(r, 'gpu_count')?.last?.[1] ?? 0), 0)
  const expected = (props.layout?.nodes ?? 0) * (props.layout?.gpusPerNode ?? 0)
  if (expected && counted < expected) return `${counted} of ${expected} GPUs`
  return `${counted} GPUs`
})
</script>

<style scoped>
.pt-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
</style>

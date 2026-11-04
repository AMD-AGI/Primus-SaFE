<template>
  <svg :width="width" :height="height" class="shrink-0" aria-hidden="true">
    <polyline :points="path" fill="none" :stroke="color" stroke-width="1.5" />
  </svg>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SummaryPoint } from '@/services/postTraining'

defineOptions({ name: 'MetricSparkline' })

const props = withDefaults(
  defineProps<{ points?: SummaryPoint[]; width?: number; height?: number; color?: string }>(),
  { points: () => [], width: 60, height: 20, color: 'var(--safe-primary)' },
)

/** The polyline through the points, scaled to the box with a 2px margin top and bottom. */
const path = computed(() => {
  const points = props.points
  if (!points.length) return ''
  const values = points.map((p) => p.value)
  const lo = Math.min(...values)
  const span = Math.max(...values) - lo || 1
  const dx = points.length > 1 ? props.width / (points.length - 1) : 0
  const inner = props.height - 4
  return points
    .map(
      (p, i) =>
        `${(i * dx).toFixed(1)},${(props.height - 2 - ((p.value - lo) / span) * inner).toFixed(1)}`,
    )
    .join(' ')
})
</script>

<template>
  <div class="pt-stage-bar">
    <div class="pt-stage-segments">
      <el-tooltip
        v-for="seg in segments"
        :key="seg.stage"
        :content="`${seg.label} · ${stateText[seg.state]}`"
        placement="top"
      >
        <span class="pt-stage-seg" :class="`is-${seg.state}`" />
      </el-tooltip>
    </div>
    <div v-if="caption" class="pt-stage-caption">{{ caption }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  STAGE_LABELS,
  stageSegments,
  type PostTrainingRun,
  type SegmentState,
  type StageSegment,
} from '@/services/postTraining'

const props = defineProps<{
  run: Pick<
    PostTrainingRun,
    'status' | 'stage' | 'failedStage' | 'evalEnabled' | 'convertWorkloadId'
  >
  /** The latest training step, shown next to the Train stage. */
  step?: number
  /** The segments and the caption of a run whose stages are not SaFE's, such as an imported run. */
  stages?: StageSegment[]
  label?: string
}>()

const stateText: Record<SegmentState, string> = {
  done: 'done',
  active: 'in progress',
  failed: 'failed',
  skipped: 'skipped',
  stopped: 'stopped',
  pending: 'not started',
}

const segments = computed(() => props.stages ?? stageSegments(props.run))

/** The current stage under the bar, with the step while training. */
const caption = computed(() => {
  if (props.label != null) return props.label
  const stage =
    props.run.status === 'Failed' && props.run.failedStage ? props.run.failedStage : props.run.stage
  const label = STAGE_LABELS[stage as keyof typeof STAGE_LABELS] ?? stage
  if (stage === 'train' && props.step != null) return `${label} · step ${props.step}`
  return label
})
</script>

<style scoped>
.pt-stage-segments {
  display: flex;
  gap: 2px;
}
.pt-stage-seg {
  flex: 1;
  height: 6px;
  border-radius: 2px;
  background: var(--safe-border);
}
.pt-stage-seg.is-done {
  background: color-mix(in srgb, var(--safe-primary) 62%, transparent);
}
.pt-stage-seg.is-active {
  background: var(--safe-primary);
  animation: pt-breathe 1.6s ease-in-out infinite;
}
.pt-stage-seg.is-failed {
  background: var(--el-color-danger);
}
.pt-stage-seg.is-stopped {
  background: var(--safe-muted);
}
.pt-stage-seg.is-skipped {
  background: repeating-linear-gradient(
    135deg,
    var(--safe-border) 0,
    var(--safe-border) 2px,
    transparent 2px,
    transparent 4px
  );
}
.pt-stage-caption {
  margin-top: 4px;
  font-size: 12px;
  color: var(--safe-muted);
}
@keyframes pt-breathe {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.45;
  }
}
</style>

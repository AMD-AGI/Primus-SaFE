<template>
  <div class="pt-timeline">
    <el-tooltip v-for="s in stages" :key="s.stage" placement="top" :show-after="200">
      <template #content>
        <div>{{ s.label }}</div>
        <div v-if="s.start">Started {{ formatTime(s.start) }}</div>
        <div v-if="s.end && s.state !== 'active'">Ended {{ formatTime(s.end) }}</div>
        <div v-if="s.detail">{{ s.detail }}</div>
      </template>
      <div
        class="pt-stage"
        :class="[
          `is-${s.state}`,
          { 'is-stalled': s.stage === 'train' && s.state === 'active' && stalled },
        ]"
      >
        <div class="pt-dot" :class="{ dashed: s.dashed }">
          <el-icon v-if="s.state === 'done'" :size="12"><Check /></el-icon>
          <el-icon v-else-if="s.state === 'failed'" :size="12"><Close /></el-icon>
        </div>
        <div class="text-[13px] font-500 mt-1">{{ s.label }}</div>
        <div class="text-[12px] text-[var(--safe-muted)]">{{ s.duration || '—' }}</div>
        <div v-if="s.detail" class="text-[12px] text-[var(--safe-muted)] truncate max-w-[140px]">
          {{ s.detail }}
        </div>
        <el-progress
          v-if="s.stage === 'train' && s.state === 'active' && progress !== undefined"
          :percentage="progress"
          :show-text="false"
          :stroke-width="3"
          class="w-[90px] mt-1"
        />
      </div>
    </el-tooltip>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Check, Close } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import {
  STAGE_LABELS,
  formatDuration,
  stageDurations,
  stageSegments,
  stallNote,
  timeOf,
  type RunDetail,
  type SegmentState,
} from '@/services/postTraining'

const props = defineProps<{ run: RunDetail; now: number }>()

const formatTime = (ms: number) => dayjs(ms).format('MM-DD HH:mm:ss')

type Recipe = {
  convert?: { cacheHit?: boolean; nodes?: number; separateJob?: boolean; hfDirect?: boolean }
  output?: { exportHf?: boolean; peftAdapter?: boolean }
  choices?: { numRollout?: number }
}

const recipe = computed(() => (props.run.recipe ?? {}) as Recipe)
/** A recipe without a convert block trains from the HF weights and has no convert stage. */
const loadsHF = computed(() => !!props.run.recipe && !recipe.value.convert)
const stalled = computed(() => !!stallNote(props.run, props.now))

/** The share of the planned steps done, when the recipe sets a step count rather than epochs. */
const progress = computed(() => {
  const step = props.run.metricSummary?.main?.step
  const total = recipe.value.choices?.numRollout
  if (step === undefined || !total) return undefined
  return Math.min(100, Math.round((step / total) * 100))
})

/** The one-line detail shown under a stage. */
const detailOf = (stage: string, state: SegmentState): string => {
  const r = recipe.value
  switch (stage) {
    case 'convert':
      if (loadsHF.value) return 'HF weights'
      if (r.convert?.cacheHit) return 'Cache hit'
      if (r.convert?.hfDirect) return 'FP8 weights'
      return r.convert?.separateJob ? `${r.convert.nodes ?? 1}-node job` : ''
    case 'train': {
      const step = props.run.metricSummary?.main?.step
      const unit = props.run.trainType === 'rl' ? 'rollout' : 'step'
      const resumes = (props.run.stageHistory ?? []).filter((e) => e.resumeStep !== undefined)
      const last = resumes[resumes.length - 1]
      const parts = [step !== undefined ? `${unit} ${step}` : '']
      if (last) {
        parts.push(
          resumes.length > 1
            ? `Resumed ${resumes.length} times`
            : `Resumed from step ${last.resumeStep}`,
        )
      }
      return parts.filter(Boolean).join(' · ')
    }
    case 'export':
      if (r.output?.exportHf === false) return 'Not enabled'
      return r.output?.peftAdapter ? 'HF + PEFT' : 'HF model'
    case 'register':
      return state === 'done' ? (props.run.outputModel?.displayName ?? '') : ''
    case 'eval':
      return props.run.evalEnabled ? '' : 'Not enabled'
    default:
      return state === 'failed' ? (props.run.message ?? '') : ''
  }
}

const stages = computed(() => {
  const r = props.run
  const finished = timeOf(r.endTime)
  const until = finished ? Date.parse(finished) : props.now
  const times = stageDurations(r.stageHistory ?? [], until)
  const segments = stageSegments(r).map((s) =>
    s.stage === 'convert' && loadsHF.value ? { ...s, state: 'skipped' as SegmentState } : s,
  )
  const queuedState: SegmentState =
    r.stage === 'queued'
      ? r.status === 'Failed'
        ? 'failed'
        : r.status === 'Stopped'
          ? 'stopped'
          : 'active'
      : 'done'
  return [{ stage: 'queued', label: STAGE_LABELS.queued, state: queuedState }, ...segments].map(
    (s) => {
      const t = times.get(s.stage)
      const seconds = t && s.state !== 'pending' ? Math.max(0, (t.end - t.start) / 1000) : undefined
      const duration =
        seconds === undefined
          ? ''
          : s.state === 'active' && s.stage === 'queued'
            ? `${formatDuration(seconds)} so far`
            : formatDuration(seconds)
      const dashed =
        s.state === 'skipped' ||
        (s.stage === 'convert' && !!recipe.value.convert?.cacheHit) ||
        (s.stage === 'export' && recipe.value.output?.exportHf === false)
      return {
        ...s,
        start: t?.start,
        end: t?.end,
        duration,
        dashed,
        detail: detailOf(s.stage, s.state),
      }
    },
  )
})
</script>

<style scoped>
.pt-timeline {
  display: grid;
  grid-template-columns: repeat(8, minmax(0, 1fr));
  gap: 8px;
}
.pt-stage {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}
.pt-dot {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  border: 2px solid var(--safe-border);
  display: flex;
  align-items: center;
  justify-content: center;
}
.pt-dot.dashed {
  border-style: dashed;
}
.is-done .pt-dot {
  background: var(--safe-primary);
  border-color: var(--safe-primary);
  color: #000;
}
.is-active .pt-dot {
  border-color: var(--safe-primary);
  animation: pt-breathe 1.6s ease-in-out infinite;
}
.is-stalled .pt-dot {
  border-color: var(--safe-warning);
  background: var(--safe-warning);
  animation: none;
}
.is-failed .pt-dot {
  background: var(--el-color-danger);
  border-color: var(--el-color-danger);
  color: #fff;
}
.is-stopped .pt-dot {
  border-color: var(--el-color-info);
  background: var(--el-color-info-light-7);
}
@keyframes pt-breathe {
  0%,
  100% {
    box-shadow: 0 0 0 0 rgba(0, 229, 229, 0.45);
  }
  50% {
    box-shadow: 0 0 0 6px rgba(0, 229, 229, 0);
  }
}
</style>

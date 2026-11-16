<template>
  <div class="min-w-0">
    <SectionCard title="Configuration" subtitle="What the run trains, on which data and resources">
      <el-descriptions :column="1" size="small">
        <el-descriptions-item label="Task">{{ taskText }}</el-descriptions-item>
        <el-descriptions-item label="Framework">{{ frameworkText }}</el-descriptions-item>
        <el-descriptions-item label="Base model">
          <el-link
            v-if="run.baseModelId"
            type="primary"
            :underline="false"
            @click="router.push(`/model-square/detail/${run.baseModelId}`)"
          >
            {{ run.baseModelName || run.baseModelId }}
          </el-link>
          <span v-else>—</span>
        </el-descriptions-item>
        <el-descriptions-item label="Training data">
          <span :title="data.train">{{ baseName(data.train) }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="Validation">
          <span :title="data.eval">{{ validationText }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="Resources">{{ resourcesText }}</el-descriptions-item>
        <el-descriptions-item label="Parallelism">{{ parallelText }}</el-descriptions-item>
        <el-descriptions-item label="Hyperparameters">{{ hyperText }}</el-descriptions-item>
        <el-descriptions-item label="Image">
          <span :title="run.image">{{ baseName(run.image) }}</span>
        </el-descriptions-item>
        <el-descriptions-item label="Scheduling">{{ schedulingText }}</el-descriptions-item>
      </el-descriptions>
    </SectionCard>

    <SectionCard
      title="Efficiency & storage"
      subtitle="Time, throughput, GPU use and disk of the run"
    >
      <el-descriptions :column="1" size="small">
        <el-descriptions-item label="Total time">{{ totalTime }}</el-descriptions-item>
        <el-descriptions-item label="Training">{{ trainTime }}</el-descriptions-item>
        <el-descriptions-item label="Throughput">{{ throughputText }}</el-descriptions-item>
        <el-descriptions-item label="Step time">{{ stepTimeText }}</el-descriptions-item>
        <el-descriptions-item label="GPU utilization">{{ gpuUtilText }}</el-descriptions-item>
        <el-descriptions-item label="GPU memory">{{ gpuMemText }}</el-descriptions-item>
        <el-descriptions-item label="Checkpoints">{{ checkpointText }}</el-descriptions-item>
        <el-descriptions-item label="Storage">{{ storageText }}</el-descriptions-item>
      </el-descriptions>
    </SectionCard>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import SectionCard from '@/components/Base/SectionCard.vue'
import { formatBytes } from '@/utils'
import {
  checkpointBytes,
  formatDuration,
  parseJSON,
  timeOf,
  type Choices,
  type DiskUsage,
  type MetricsResp,
  type RunDetail,
} from '@/services/postTraining'

const props = defineProps<{
  run: RunDetail
  metrics?: MetricsResp
  checkpoints: { step: number; best: boolean; deletedBy: string }[]
  now: number
}>()
const router = useRouter()

type RecipeView = {
  data?: { train?: string; eval?: string }
  layout?: {
    nodes?: number
    gpusPerNode?: number
    colocate?: boolean
    actorGpus?: number
    rolloutGpus?: number
  }
  choices?: Choices
}
const recipe = computed(() => (props.run.recipe ?? {}) as RecipeView)
const data = computed(() => recipe.value.data ?? {})
const choices = computed(() => recipe.value.choices ?? ({} as Choices))
const rl = computed(() => props.run.trainType === 'rl')

/** The last element of a path or an image reference. */
const baseName = (p?: string) => (p ? (p.replace(/\/+$/, '').split('/').pop() ?? p) : '—')

/** A learning rate in the short form the recipe uses, 1e-5 rather than 0.00001. */
const rate = (v: number) => (v !== 0 && Math.abs(v) < 1e-3 ? v.toExponential() : String(v))

const taskText = computed(() => {
  const r = props.run
  const algorithm = rl.value && r.algorithm ? ` · ${r.algorithm.toUpperCase()}` : ''
  return `${r.trainType.toUpperCase()} · ${r.method === 'lora' ? 'LoRA' : 'Full'}${algorithm}`
})

const frameworkText = computed(
  () => [props.run.framework, props.run.frameworkVersion].filter(Boolean).join(' ') || '—',
)

const validationText = computed(() => {
  if (data.value.eval) return baseName(data.value.eval)
  const split = choices.value.valSplit
  return split ? `${Math.round(split * 100)}% of the training data` : 'None'
})

const resourcesText = computed(() => {
  const l = recipe.value.layout
  const nodes = l?.nodes ?? props.run.nodeCount
  const gpus = l?.gpusPerNode ?? props.run.gpuPerNode
  const base = nodes && gpus ? `${nodes} × ${gpus} GPUs` : '—'
  if (!rl.value || !l) return base
  return l.colocate
    ? `${base} · colocated`
    : `${base} · ${l.actorGpus ?? 0} train + ${l.rolloutGpus ?? 0} rollout`
})

const parallelText = computed(() => {
  const c = choices.value
  const parts = [`TP ${c.tp ?? 1}`, `PP ${c.pp ?? 1}`, `CP ${c.cp ?? 1}`, `EP ${c.ep ?? 1}`]
  if (c.sequenceParallel) parts.push('SP')
  return parts.join(' · ')
})

const hyperText = computed(() => {
  const c = choices.value
  const parts: string[] = []
  if (rl.value) {
    if (c.numRollout) parts.push(`${c.numRollout} rollouts`)
    if (c.batchSize) parts.push(`${c.batchSize} prompts × ${c.nSamplesPerPrompt ?? 1} samples`)
    if (c.maxResponseLen) parts.push(`max response ${c.maxResponseLen}`)
  } else {
    if (c.numRollout) parts.push(`${c.numRollout} steps`)
    else if (c.numEpoch) parts.push(`${c.numEpoch} epoch${c.numEpoch === 1 ? '' : 's'}`)
    if (c.batchSize) parts.push(`global batch ${c.batchSize}`)
    if (c.maxTokensPerGpu) parts.push(`${c.maxTokensPerGpu} tokens/GPU`)
  }
  if (c.lr !== undefined) parts.push(`lr ${rate(c.lr)}`)
  if (c.saveInterval) parts.push(`save every ${c.saveInterval}`)
  return parts.join(' · ') || 'Spec defaults'
})

const PRIORITIES = ['Low', 'Medium', 'High']
const schedulingText = computed(() => {
  const priority = PRIORITIES[props.run.priority] ?? String(props.run.priority)
  const timeout =
    props.run.timeout > 0 ? `timeout ${formatDuration(props.run.timeout)}` : 'no timeout'
  return `${priority} priority · ${timeout}`
})

/** The stage entries in time order, without the events within a stage. */
const stages = computed(() =>
  (props.run.stageHistory ?? [])
    .filter((e) => !e.event)
    .sort((a, b) => Date.parse(a.time) - Date.parse(b.time)),
)

/** Whether the run still moves: queued, training, or evaluating after it succeeded. */
const moving = computed(
  () =>
    props.run.status === 'Pending' || props.run.status === 'Running' || props.run.stage === 'eval',
)

const totalTime = computed(() => {
  const start = timeOf(props.run.createdAt)
  if (!start) return '—'
  const last = stages.value[stages.value.length - 1]
  const end = moving.value || !last ? props.now : Date.parse(last.time)
  return formatDuration((end - Date.parse(start)) / 1000)
})

const trainTime = computed(() => {
  const list = stages.value
  const first = list.findIndex((e) => e.stage === 'train')
  if (first < 0) return '—'
  let lastTrain = first
  list.forEach((e, i) => {
    if (e.stage === 'train') lastTrain = i
  })
  const next = list[lastTrain + 1]
  const end = next ? Date.parse(next.time) : moving.value ? props.now : undefined
  return end ? formatDuration((end - Date.parse(list[first].time)) / 1000) : '—'
})

/** The points of a metric series, or none. */
const points = (name: string) =>
  props.metrics?.series.find((s) => s.name === name)?.points.map((p) => p[1]) ?? []
const mean = (v: number[]) => (v.length ? v.reduce((a, b) => a + b, 0) / v.length : undefined)
const peak = (v: number[]) => (v.length ? Math.max(...v) : undefined)

const throughputText = computed(() => {
  const tokens = mean(points('perf/actor_train_tok_per_s'))
  const tflops = mean(points('perf/actor_train_tflops'))
  if (tokens === undefined) return '—'
  const flops = tflops !== undefined ? ` · ${tflops.toFixed(1)} TFLOPS` : ''
  return `avg ${Math.round(tokens).toLocaleString()} tokens/s${flops}`
})

const stepTimeText = computed(() => {
  const s = mean(points('perf/step_time'))
  return s === undefined ? '—' : `avg ${s.toFixed(1)} s`
})

const gpuUtilText = computed(() => {
  const avg = mean(points('system/gpu_util_mean'))
  const max = peak(points('system/gpu_util_max'))
  if (avg === undefined) return '—'
  return `avg ${avg.toFixed(0)}%${max !== undefined ? ` · peak ${max.toFixed(0)}%` : ''}`
})

const gpuMemText = computed(() => {
  const used = peak(points('system/gpu_mem_used_max_gb'))
  const total = peak(points('system/gpu_mem_total_gb'))
  if (used === undefined) return '—'
  return `peak ${used.toFixed(1)}${total !== undefined ? ` of ${total.toFixed(0)}` : ''} GiB per GPU`
})

const checkpointText = computed(() => {
  const all = props.checkpoints
  if (!all.length) return '—'
  const kept = all.filter((c) => !c.deletedBy).length
  const best = all.find((c) => c.best)
  return `${kept} kept of ${all.length}${best ? ` · best at step ${best.step}` : ''}`
})

const storageText = computed(() => {
  const d = parseJSON<DiskUsage>(props.run.diskUsage)
  if (!d) return '—'
  const outputs = (d.finalModel ?? 0) + (d.finalAdapter ?? 0) + (d.exports ?? 0)
  return `checkpoints ${formatBytes(checkpointBytes(d), 1)} · outputs ${formatBytes(outputs, 1)}`
})
</script>

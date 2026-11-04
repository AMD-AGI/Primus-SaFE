<template>
  <div>
    <el-card v-if="checkpointRows.length >= 2" class="safe-card mb-3" shadow="never">
      <div class="font-500 mb-2">
        Scores by checkpoint
        <span class="text-[13px] text-[var(--safe-muted)]">{{ checkpointRows.length }} models</span>
      </div>
      <el-table :data="checkpointRows" size="small">
        <el-table-column label="Checkpoint" width="200">
          <template #default="{ row }">
            {{ row.checkpoint }}
            <el-tag v-if="row.output" size="small" effect="plain">Output</el-tag>
            <el-tag v-if="row.best" size="small" type="warning" effect="plain"
              >Best val loss</el-tag
            >
          </template>
        </el-table-column>
        <el-table-column label="Model" min-width="200">
          <template #default="{ row }">
            <el-link type="primary" @click="router.push(`/model-square/detail/${row.modelId}`)">
              {{ row.model }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column v-for="b in benchmarks" :key="b" :label="b" width="110">
          <template #default="{ row }">
            <span
              :class="{ 'font-700': row.scores[b] !== undefined && row.scores[b] === bestScore[b] }"
            >
              {{ row.scores[b] === undefined ? '—' : row.scores[b].toFixed(1) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="Evaluated" width="130" prop="evaluated" />
      </el-table>
      <div class="text-[12px] text-[var(--safe-muted)] mt-2">
        Bold marks the best score in each column. Export a checkpoint and evaluate it to add a row.
      </div>
    </el-card>

    <el-card v-if="trainingRows.length" class="safe-card mb-3" shadow="never">
      <div class="font-500 mb-2">Evaluation during training</div>
      <el-table :data="trainingRows" size="small">
        <el-table-column label="Rollout" width="110" prop="step" />
        <el-table-column
          v-for="name in trainingSets"
          :key="name"
          :label="name.slice(5)"
          width="140"
        >
          <template #default="{ row }">
            <span
              :class="{
                'font-700':
                  row.values[name] !== undefined && row.values[name] === bestTraining[name],
              }"
            >
              {{ row.values[name] === undefined ? '—' : row.values[name].toFixed(3) }}
            </span>
          </template>
        </el-table-column>
      </el-table>
      <div class="text-[12px] text-[var(--safe-muted)] mt-2">
        From the eval/&lt;set&gt; metrics. The same scores are drawn on the Reward panel.
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import dayjs from 'dayjs'
import {
  matchName,
  parseJSON,
  timeOf,
  type MetricsResp,
  type RunDetail,
} from '@/services/postTraining'

const props = defineProps<{ run: RunDetail; metrics?: MetricsResp }>()
const router = useRouter()

type Summary = { benchmarks?: Record<string, { accuracy?: number }> }

/** The latest scores of each evaluated model, in percent, with when they were evaluated. */
const latestScores = computed(() => {
  const out = new Map<string, { scores: Record<string, number>; time?: string }>()
  const tasks = [...(props.run.evaluations ?? [])].sort((a, b) =>
    (timeOf(a.CreationTime) ?? '').localeCompare(timeOf(b.CreationTime) ?? ''),
  )
  for (const t of tasks) {
    const modelId = t.ModelId?.Valid ? t.ModelId.String : ''
    const s = parseJSON<Summary>(t.ResultSummary?.Valid ? t.ResultSummary.String : '')
    if (!modelId || !s?.benchmarks) continue
    const scores: Record<string, number> = {}
    for (const [name, v] of Object.entries(s.benchmarks)) {
      if (v.accuracy !== undefined) scores[name] = v.accuracy * 100
    }
    out.set(modelId, { scores, time: timeOf(t.EndTime) ?? timeOf(t.CreationTime) })
  }
  return out
})

/** One row per evaluated model: the output model at the last step, or an export at its step. */
const checkpointRows = computed(() => {
  const r = props.run
  const best = r.metricSummary?.bestValLoss?.step
  const models: { modelId: string; model: string; step?: number; output: boolean }[] = []
  if (r.outputModel) {
    models.push({
      modelId: r.outputModel.id,
      model: r.outputModel.displayName || r.outputModel.id,
      step: r.metricSummary?.main?.step,
      output: true,
    })
  }
  for (const e of r.exports ?? []) {
    if (e.modelId)
      models.push({
        modelId: e.modelId,
        model: e.displayName || e.modelId,
        step: e.step,
        output: false,
      })
  }
  return models
    .filter((m) => latestScores.value.has(m.modelId))
    .map((m) => {
      const s = latestScores.value.get(m.modelId)!
      return {
        ...m,
        checkpoint: m.step !== undefined ? `Step ${m.step.toLocaleString()}` : 'Last step',
        best: m.step !== undefined && m.step === best,
        scores: s.scores,
        evaluated: s.time ? dayjs(s.time).format('MM-DD HH:mm') : '—',
      }
    })
    .sort((a, b) => (a.step ?? 0) - (b.step ?? 0))
})

const benchmarks = computed(() =>
  [...new Set(checkpointRows.value.flatMap((r) => Object.keys(r.scores)))].sort(),
)
const bestScore = computed(() => {
  const out: Record<string, number> = {}
  for (const b of benchmarks.value) {
    const values = checkpointRows.value.map((r) => r.scores[b]).filter((v) => v !== undefined)
    if (values.length) out[b] = Math.max(...values)
  }
  return out
})

/** The RL evaluation sets measured during training: eval/<set>, one level deep. */
const trainingSets = computed(() =>
  props.run.trainType !== 'rl'
    ? []
    : (props.metrics?.series ?? []).filter((s) => matchName('eval/*', s.name)).map((s) => s.name),
)

/** One row per evaluated rollout with the score of each set. */
const trainingRows = computed(() => {
  const byStep = new Map<number, Record<string, number>>()
  for (const name of trainingSets.value) {
    for (const [step, v] of props.metrics?.series.find((s) => s.name === name)?.points ?? []) {
      byStep.set(step, { ...(byStep.get(step) ?? {}), [name]: v })
    }
  }
  return [...byStep].sort((a, b) => a[0] - b[0]).map(([step, values]) => ({ step, values }))
})
const bestTraining = computed(() => {
  const out: Record<string, number> = {}
  for (const name of trainingSets.value) {
    const values = trainingRows.value.map((r) => r.values[name]).filter((v) => v !== undefined)
    if (values.length) out[name] = Math.max(...values)
  }
  return out
})
</script>

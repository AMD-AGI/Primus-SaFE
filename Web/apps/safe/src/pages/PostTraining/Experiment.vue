<template>
  <div class="pt-experiment">
    <el-result v-if="notFound" icon="warning" title="Experiment not found">
      <template #extra>
        <el-button round @click="router.push('/post-training')">Back to Post Training</el-button>
      </template>
    </el-result>

    <div v-else-if="!exp" v-loading="true" class="h-[240px]" />

    <template v-else>
      <div class="flex items-start justify-between gap-4">
        <div>
          <div class="flex items-center gap-2">
            <el-button link :icon="ArrowLeft" @click="router.push('/post-training')" />
            <span class="text-[20px] font-600">{{ exp.displayName }}</span>
            <el-tag size="small" type="info" effect="plain">A/B experiment</el-tag>
            <el-tag size="small" effect="plain">{{ exp.trainType.toUpperCase() }}</el-tag>
            <span class="text-[13px] text-[var(--safe-muted)]">{{ statusCounts }}</span>
          </div>
          <div class="pt-meta">
            <span>Owner {{ exp.userName || exp.userId }}</span>
            <span>Created {{ shortTime(timeOf(exp.createdAt)) }}</span>
            <span>Controls seed {{ exp.seed }} · same data order · same benchmarks</span>
          </div>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <el-button round @click="copyLink">Copy Link</el-button>
          <el-tooltip :content="stopTip" :disabled="!stopTip" placement="top">
            <span>
              <el-button round :disabled="!!stopTip" @click="stopAll">Stop All</el-button>
            </span>
          </el-tooltip>
          <el-tooltip :content="readOnlyTip" :disabled="!isReadonly" placement="top">
            <span>
              <el-button
                round
                plain
                type="danger"
                :disabled="isReadonly"
                @click="deleteDialog.visible = true"
              >
                Delete Experiment
              </el-button>
            </span>
          </el-tooltip>
        </div>
      </div>

      <el-alert v-if="noiseText" class="mt-3" type="info" :closable="false" show-icon>
        <template #title>
          <b>Evaluation noise.</b> {{ noiseText }} A difference is marked significant only when it
          is larger than this gap and than two standard errors for the benchmark size. Training
          randomness is not included; submit the same recipe as two variants to measure it.
        </template>
      </el-alert>

      <el-card class="safe-card mt-3" shadow="never">
        <div class="font-500 mb-2">Results</div>
        <el-table :data="rows" size="small">
          <el-table-column label="Variant" min-width="200">
            <template #default="{ row }">
              <div class="font-500">{{ row.variant }}</div>
              <el-link v-if="row.run" type="primary" @click="openRun(row.run.runId)">
                {{ row.run.displayName }}
              </el-link>
            </template>
          </el-table-column>
          <el-table-column label="Changes from Baseline" min-width="200">
            <template #default="{ row }">{{ row.changes }}</template>
          </el-table-column>
          <el-table-column label="Status" width="110">
            <template #default="{ row }">{{ row.run?.status ?? 'Not found' }}</template>
          </el-table-column>
          <el-table-column
            :label="exp.trainType === 'rl' ? 'Final Reward' : 'Best Val Loss'"
            width="130"
          >
            <template #default="{ row }">{{ row.result }}</template>
          </el-table-column>
          <template v-for="b in benchmarks" :key="b">
            <el-table-column :label="b" width="120">
              <template #default="{ row }">
                <template v-if="row.scores[b]?.length">
                  <div>{{ fmtScore(mean(row.scores[b])) }}</div>
                  <div
                    v-if="row.baseline && row.scores[b].length > 1"
                    class="text-[12px] text-[var(--safe-muted)]"
                  >
                    {{ row.scores[b].map(fmtScore).join(' · ') }}
                  </div>
                </template>
                <span v-else>{{ row.pending ? 'Pending' : '—' }}</span>
              </template>
            </el-table-column>
            <el-table-column label="vs Baseline" width="140">
              <template #default="{ row }">
                <template v-if="!row.baseline && compare(row, b)">
                  <el-tooltip :content="compare(row, b)!.tip" placement="top">
                    <span :class="compare(row, b)!.cls">{{ compare(row, b)!.text }}</span>
                  </el-tooltip>
                </template>
                <span v-else>—</span>
              </template>
            </el-table-column>
          </template>
        </el-table>
      </el-card>

      <el-card class="safe-card mt-3" shadow="never">
        <div class="flex items-center justify-between">
          <span class="font-500">Curves</span>
          <el-button
            v-if="router.hasRoute('PostTrainingCompare')"
            link
            type="primary"
            @click="compareCurves"
          >
            Open in Compare
          </el-button>
        </div>
        <div class="text-[13px] text-[var(--safe-muted)]">
          The variants' curves overlay on the compare page in variant order.
        </div>
      </el-card>

      <el-card class="safe-card mt-3" shadow="never">
        <div class="font-500 mb-2">
          Config Differences
          <span class="text-[13px] text-[var(--safe-muted)]"
            >{{ differing.length }} fields differ</span
          >
        </div>
        <el-table :data="differing" size="small">
          <el-table-column label="Field" min-width="200" prop="label" />
          <el-table-column
            v-for="(r, i) in details"
            :key="r.runId"
            :label="exp.runs[i]?.variantName || r.displayName"
            min-width="140"
          >
            <template #default="{ row }">
              <span :class="{ 'pt-changed': i > 0 && row.values[i] !== row.values[0] }">
                {{ row.values[i] }}
              </span>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <el-dialog v-model="deleteDialog.visible" title="Delete experiment" width="520px">
      <div v-if="exp">
        Delete experiment <b>{{ exp.displayName }}</b> and its runs? Unfinished runs are stopped
        first, and their run directories, checkpoints and curves are removed.
        <ul class="mt-2">
          <li v-for="r in exp.runs" :key="r.runId">{{ r.displayName }}</li>
        </ul>
      </div>
      <el-checkbox v-model="deleteDialog.deleteModel" class="mt-2">
        Also delete their output models and weights
      </el-checkbox>
      <div v-if="deleteDialog.failed.length" class="mt-2 text-[13px] text-[var(--el-color-danger)]">
        These runs failed to delete, so the experiment is kept:
        <div v-for="f in deleteDialog.failed" :key="f">{{ f }}</div>
      </div>
      <template #footer>
        <el-button @click="deleteDialog.visible = false">Cancel</el-button>
        <el-button type="danger" :loading="deleteDialog.loading" @click="confirmDelete"
          >Delete</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { useUserStore } from '@/stores/user'
import { copyText } from '@/utils/index'
import {
  VARIANT_FIELDS,
  configDiff,
  deletePostTrainingExperiment,
  flattenChanges,
  flattenRecipe,
  getPostTrainingExperiment,
  getPostTrainingRun,
  parseJSON,
  significance,
  stopPostTrainingExperiment,
  timeOf,
  type Experiment,
  type RunDetail,
} from '@/services/postTraining'

defineOptions({ name: 'PostTrainingExperimentPage' })

const readOnlyTip = 'Read-only access'
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const isReadonly = computed(() => userStore.isReadonlyManager)

const experimentId = computed(() => String(route.query.experimentId ?? ''))
const exp = ref<Experiment>()
const details = ref<RunDetail[]>([])
const notFound = ref(false)

const shortTime = (t?: string) => (t ? dayjs(t).format('MM-DD HH:mm') : '—')
const isActive = (status: string) => status === 'Pending' || status === 'Running'

/** Loads the experiment and the detail of each of its runs. */
const load = async () => {
  try {
    exp.value = (await getPostTrainingExperiment(experimentId.value, true)) as unknown as Experiment
  } catch (err) {
    notFound.value =
      (err as { response?: { status?: number } })?.response?.status === 404 || !experimentId.value
    return
  }
  const runs = await Promise.all(
    exp.value.runs.map((r) => getPostTrainingRun(r.runId, true).catch(() => undefined)),
  )
  details.value = runs.filter(Boolean) as unknown as RunDetail[]
}

const statusCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const r of exp.value?.runs ?? []) counts.set(r.status, (counts.get(r.status) ?? 0) + 1)
  return [...counts].map(([s, n]) => `${n} ${s}`).join(' · ')
})

type Summary = { benchmarks?: Record<string, { accuracy?: number; samples?: number }> }

/** The benchmark scores of a run, every evaluation in order, as fractions, with the benchmark sizes. */
const scoresOf = (r?: RunDetail) => {
  const scores: Record<string, number[]> = {}
  const sizes: Record<string, number> = {}
  const tasks = [...(r?.evaluations ?? [])].sort((a, b) =>
    (timeOf(a.CreationTime) ?? '').localeCompare(timeOf(b.CreationTime) ?? ''),
  )
  for (const t of tasks) {
    const s = parseJSON<Summary>(t.ResultSummary?.Valid ? t.ResultSummary.String : '')
    for (const [name, v] of Object.entries(s?.benchmarks ?? {})) {
      if (v.accuracy === undefined) continue
      scores[name] = [...(scores[name] ?? []), v.accuracy]
      if (v.samples) sizes[name] = v.samples
    }
  }
  return { scores, sizes }
}

const detailOf = (runId: string) => details.value.find((d) => d.runId === runId)
const baselineDetail = computed(() => {
  const run = exp.value?.runs.find((r) => r.variantName === exp.value?.baselineVariant)
  return run ? detailOf(run.runId) : undefined
})
const baselineScores = computed(() => scoresOf(baselineDetail.value))

/** The changes of a variant from the baseline, with the baseline's values. */
const changesText = (changes?: Record<string, unknown>) => {
  const flat = flattenChanges(changes)
  if (!Object.keys(flat).length) return 'No changes (repeat of baseline)'
  const base = flattenRecipe(baselineDetail.value?.recipe ?? {})
  return Object.entries(flat)
    .map(([key, value]) => {
      const field = VARIANT_FIELDS.find((f) => f.key === key)
      const before = field ? base[field.recipeKey] : undefined
      return `${field?.label ?? key}: ${before !== undefined ? `${before} → ` : ''}${value}`
    })
    .join('; ')
}

const rows = computed(() => {
  const e = exp.value
  if (!e) return []
  return e.runs.map((run) => {
    const d = detailOf(run.runId)
    const baseline = run.variantName === e.baselineVariant
    const variant = e.variants.find((v) => v.name === run.variantName)
    const summary = d?.metricSummary
    const result =
      e.trainType === 'rl'
        ? summary?.main?.value?.toFixed(3)
        : summary?.bestValLoss
          ? `${summary.bestValLoss.value.toFixed(3)} @ ${summary.bestValLoss.step}`
          : undefined
    return {
      variant: run.variantName,
      run,
      baseline,
      changes: baseline ? '—' : changesText(variant?.changes),
      result: result ?? '—',
      scores: scoresOf(d).scores,
      pending: isActive(run.status) || run.stage === 'eval',
    }
  })
})

const benchmarks = computed(() =>
  [...new Set(rows.value.flatMap((r) => Object.keys(r.scores)))].sort(),
)

const mean = (values: number[]) => values.reduce((a, b) => a + b, 0) / values.length
const fmtScore = (v: number) => v.toFixed(3)

/** A variant's difference from the baseline on a benchmark, colored by its significance. */
const compare = (row: (typeof rows.value)[number], benchmark: string) => {
  const base = baselineScores.value.scores[benchmark]
  const own = row.scores[benchmark]
  if (!base?.length || !own?.length) return undefined
  const n = baselineScores.value.sizes[benchmark]
  const s = significance(base, mean(own), n)
  const sign = s.diff >= 0 ? '+' : '−'
  const text = `${sign}${Math.abs(s.diff).toFixed(3)}${s.significant ? (s.diff >= 0 ? ' ▲' : ' ▼') : ' Not significant'}`
  const cls = !s.significant
    ? 'text-[var(--safe-muted)]'
    : s.diff >= 0
      ? 'text-[var(--el-color-success)]'
      : 'text-[var(--el-color-danger)]'
  const tip =
    `Significant when |difference| > ${s.threshold.toFixed(3)} (the larger of the baseline ` +
    `re-evaluation gap and two standard errors${n ? ` at n = ${n}` : ''}).`
  return { text, cls, tip }
}

/** How far the baseline's two evaluations were apart on each benchmark. */
const noiseText = computed(() => {
  const parts = Object.entries(baselineScores.value.scores)
    .filter(([, v]) => v.length > 1)
    .map(([name, v]) => `${name} differed by ${Math.abs(v[0] - v[1]).toFixed(3)}`)
  return parts.length ? `The baseline model was evaluated twice: ${parts.join(' and ')}.` : ''
})

const differing = computed(() =>
  configDiff(details.value.map((d) => d.recipe)).filter((d) => d.differs),
)

const unfinished = computed(() => (exp.value?.runs ?? []).filter((r) => isActive(r.status)))
const stopTip = computed(() => {
  if (isReadonly.value) return readOnlyTip
  return unfinished.value.length ? '' : 'Every run has ended.'
})

const openRun = (runId: string) => router.push({ path: '/post-training/detail', query: { runId } })

const compareCurves = () => {
  router.push({
    path: '/post-training/compare',
    query: { runIds: (exp.value?.runs ?? []).map((r) => r.runId).join(',') },
  })
}

const copyLink = () => {
  copyText(window.location.href)
  ElMessage.success('Link copied')
}

const stopAll = async () => {
  try {
    await ElMessageBox.confirm(
      `These runs stop now: ${unfinished.value.map((r) => r.displayName).join(', ')}. Saved checkpoints are kept.`,
      `Stop experiment ${exp.value?.displayName}?`,
      { confirmButtonText: 'Stop All', cancelButtonText: 'Cancel', type: 'warning' },
    )
  } catch {
    return
  }
  await stopPostTrainingExperiment(experimentId.value)
  ElMessage.success('Experiment stopped')
  load()
}

const deleteDialog = reactive({
  visible: false,
  loading: false,
  deleteModel: false,
  failed: [] as string[],
})

const confirmDelete = async () => {
  deleteDialog.loading = true
  deleteDialog.failed = []
  try {
    await deletePostTrainingExperiment(experimentId.value, deleteDialog.deleteModel)
    ElMessage.success('Experiment deleted')
    deleteDialog.visible = false
    router.push('/post-training')
  } catch (err) {
    const data = (
      err as { response?: { data?: { failed?: Record<string, string>; errorMessage?: string } } }
    )?.response?.data
    deleteDialog.failed = data?.failed
      ? Object.entries(data.failed).map(([id, why]) => `${id}: ${why}`)
      : [data?.errorMessage ?? String(err)]
  } finally {
    deleteDialog.loading = false
  }
}

watch(experimentId, load)
onMounted(load)
</script>

<style scoped>
.pt-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  margin-top: 6px;
  font-size: 13px;
  color: var(--safe-muted);
}
.pt-changed {
  padding: 0 4px;
  border-radius: 4px;
  background: var(--el-color-warning-light-8);
}
</style>

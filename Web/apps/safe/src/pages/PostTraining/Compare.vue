<template>
  <div class="pt-compare">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <el-button link :icon="ArrowLeft" @click="router.push('/post-training')" />
        <span class="text-[20px] font-600">Compare Runs</span>
        <el-tag v-if="trainType" size="small" effect="plain">{{ trainType.toUpperCase() }}</el-tag>
        <span class="text-[13px] text-[var(--safe-muted)]">{{ found.length }} runs</span>
      </div>
      <el-button round @click="copyLink">Copy Link</el-button>
    </div>

    <el-result
      v-if="mixedTypes.length"
      icon="error"
      title="Runs must have the same training type"
      :sub-title="mixedTypes.join('; ')"
    >
      <template #extra>
        <el-button round @click="router.push('/post-training')">Back to Post Training</el-button>
      </template>
    </el-result>

    <template v-else>
      <div class="flex flex-wrap items-center gap-2 mt-3">
        <el-tag
          v-for="(e, i) in entries"
          :key="e.runId"
          closable
          effect="plain"
          :type="e.run ? undefined : 'info'"
          @close="removeRun(e.runId)"
        >
          <span class="pt-dot" :style="{ background: e.run ? colors[i] : 'var(--safe-muted)' }" />
          {{ e.run ? e.run.displayName : `${e.runId} · Not found` }}
          <span v-if="e.run && isActive(e.run)" class="text-[12px]"> · {{ e.run.status }}</span>
        </el-tag>
        <el-tooltip content="Up to 5 runs." :disabled="entries.length < MAX_RUNS" placement="top">
          <span>
            <el-button size="small" round :disabled="entries.length >= MAX_RUNS" @click="openAdd">
              Add Run
            </el-button>
          </span>
        </el-tooltip>
      </div>

      <el-empty
        v-if="!loading && found.length < 2"
        description="Add at least one more run to compare."
        :image-size="60"
      />

      <template v-else-if="found.length >= 2">
        <div class="flex items-center gap-4 mt-3 text-[13px]">
          <span>X axis Step</span>
          <span>Smoothing</span>
          <el-slider v-model="smoothing" :min="0" :max="0.99" :step="0.01" class="w-[180px]" />
          <el-checkbox v-model="logY">Log Y</el-checkbox>
          <el-checkbox v-if="hasRepeats" v-model="groupByRecipe">Group by recipe</el-checkbox>
          <span v-if="hasRepeats && groupByRecipe" class="text-[var(--safe-muted)]">
            {{ recipeGroups.length }} groups · {{ found.length }} runs
          </span>
          <span class="text-[var(--safe-muted)]">Settings apply to every panel</span>
        </div>
        <div class="pt-grid mt-3">
          <MetricsPanel
            v-for="p in mainPanels"
            :key="p.name"
            :title="p.name"
            :series="p.series"
            :colors="p.colors"
            :bands="p.bands"
            :smoothing="smoothing"
            :log-y="logY"
            :legend="groupByRecipe && hasRepeats"
          />
        </div>
        <el-collapse v-if="otherPanels.length" class="mt-3">
          <el-collapse-item :title="`${otherPanels.length} more panels`" name="more">
            <div class="pt-grid">
              <MetricsPanel
                v-for="p in otherPanels"
                :key="p.name"
                :title="p.name"
                :series="p.series"
                :colors="p.colors"
                :bands="p.bands"
                :smoothing="smoothing"
                :log-y="logY"
                :legend="groupByRecipe && hasRepeats"
              />
            </div>
          </el-collapse-item>
        </el-collapse>

        <el-card class="safe-card mt-4" shadow="never">
          <div class="font-500 mb-2">Results</div>
          <el-table :data="results" size="small">
            <el-table-column label="Run" min-width="200">
              <template #default="{ row }">
                <span class="pt-dot" :style="{ background: row.color }" />
                <el-link type="primary" @click="openRun(row.runId)">{{ row.name }}</el-link>
              </template>
            </el-table-column>
            <el-table-column label="Status" prop="status" width="110" />
            <el-table-column label="Steps" prop="steps" width="130" />
            <el-table-column
              :label="trainType === 'rl' ? 'Final Reward' : 'Train Loss'"
              width="130"
            >
              <template #default="{ row }">
                <span :class="{ 'font-700': row.main === best.main }">{{ fmt(row.main) }}</span>
              </template>
            </el-table-column>
            <el-table-column v-if="trainType === 'sft'" label="Best Val Loss" width="190">
              <template #default="{ row }">
                <span :class="{ 'font-700': row.valLoss === best.valLoss }">
                  {{ row.valLoss === undefined ? '—' : `${fmt(row.valLoss)} @ ${row.valStep}` }}
                </span>
                <div v-if="row.rising" class="text-[12px] text-[var(--safe-warning)]">
                  Rising since step {{ row.valStep }}
                </div>
              </template>
            </el-table-column>
            <el-table-column v-for="b in benchmarks" :key="b" :label="b" width="110">
              <template #default="{ row }">
                <span
                  :class="{
                    'font-700': row.scores[b] !== undefined && row.scores[b] === best.scores[b],
                  }"
                >
                  {{ row.scores[b] === undefined ? row.pending : row.scores[b].toFixed(1) }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="Duration" prop="duration" width="110" />
          </el-table>
        </el-card>

        <el-card v-if="scatter" class="safe-card mt-4" shadow="never">
          <CompareScatter
            v-if="scatter.points"
            :field="scatter.field"
            :x-label="scatter.label"
            :points="scatter.points"
            :columns="scatterColumns"
          />
          <div v-else class="text-[13px] text-[var(--safe-muted)]">{{ scatter.message }}</div>
        </el-card>

        <el-card class="safe-card mt-4" shadow="never">
          <div class="flex items-center justify-between mb-2">
            <span class="font-500">Config Differences</span>
            <span v-if="!sameRecipe" class="text-[13px] text-[var(--safe-muted)]">
              {{ fieldsDiffer(differing.length) }} · {{ diff.length - differing.length }} identical
              ·
              <el-button link type="primary" @click="showAll = !showAll">
                {{ showAll ? 'Show differences' : 'Show all' }}
              </el-button>
            </span>
          </div>
          <div v-if="sameRecipe" class="text-[13px]">Same recipe</div>
          <el-table v-else :data="showAll ? diff : differing" size="small">
            <el-table-column label="Field" min-width="200">
              <template #default="{ row }">{{ row.label }}</template>
            </el-table-column>
            <el-table-column
              v-for="(r, i) in found"
              :key="r.runId"
              :label="r.displayName"
              min-width="150"
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
    </template>

    <el-dialog v-model="addDialog.visible" title="Add Run" width="480px">
      <el-select
        v-model="addDialog.runId"
        filterable
        remote
        :remote-method="searchRuns"
        :loading="addDialog.loading"
        placeholder="Search runs by name or ID"
        style="width: 100%"
      >
        <el-option
          v-for="r in addDialog.options"
          :key="r.runId"
          :label="r.displayName"
          :value="r.runId"
          :disabled="runIds.includes(r.runId)"
        />
      </el-select>
      <template #footer>
        <el-button @click="addDialog.visible = false">Cancel</el-button>
        <el-button type="primary" :disabled="!addDialog.runId" @click="confirmAdd">Add</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft } from '@element-plus/icons-vue'
import { useDark, useNow } from '@vueuse/core'
import dayjs from 'dayjs'
import { useWorkspaceStore } from '@/stores/workspace'
import { copyText } from '@/utils/index'
import {
  configDiff,
  fieldsDiffer,
  formatDuration,
  getPostTrainingMetrics,
  getPostTrainingRun,
  listPostTrainingRuns,
  parseJSON,
  timeOf,
  type MetricSeries,
  type MetricsResp,
  type PostTrainingRun,
  type RunDetail,
  type RunListResp,
} from '@/services/postTraining'
import MetricsPanel from './Components/MetricsPanel.vue'
import CompareScatter from './Components/CompareScatter.vue'

defineOptions({ name: 'PostTrainingComparePage' })

const MAX_RUNS = 5
const VAL_LOSS = 'eval/val/loss'

const route = useRoute()
const router = useRouter()
const wsStore = useWorkspaceStore()
const now = useNow({ interval: 1000 })
const isDark = useDark()

/** One color per run, in URL order; dark mode takes the lighter shade. */
const colors = computed(() =>
  isDark.value
    ? ['#2dd4bf', '#818cf8', '#fbbf24', '#fb7185', '#38bdf8']
    : ['#0d9488', '#6366f1', '#f59e0b', '#e11d48', '#0284c7'],
)

const runIds = computed(() =>
  String(route.query.runIds ?? '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
    .slice(0, MAX_RUNS),
)

type Entry = { runId: string; run?: RunDetail; metrics?: MetricsResp }
const entries = ref<Entry[]>([])
const loading = ref(true)
const smoothing = ref(0.6)
const logY = ref(false)
const showAll = ref(false)

/** Loads the detail and metrics of every run in the URL; a run that can't be read shows as not found. */
const load = async () => {
  loading.value = true
  entries.value = await Promise.all(
    runIds.value.map(async (runId) => {
      const [run, metrics] = await Promise.all([
        getPostTrainingRun(runId, true).catch(() => undefined),
        getPostTrainingMetrics(runId, {}, true).catch(() => undefined),
      ])
      return {
        runId,
        run: run as unknown as RunDetail | undefined,
        metrics: metrics as unknown as MetricsResp | undefined,
      }
    }),
  )
  loading.value = false
}
watch(runIds, load, { immediate: true })

const found = computed(() => entries.value.filter((e) => e.run).map((e) => e.run!))
const trainType = computed(() => found.value[0]?.trainType)

/** The runs whose training type differs from the first run's, described for the error page. */
const mixedTypes = computed(() => {
  const types = new Set(found.value.map((r) => r.trainType))
  if (types.size < 2) return []
  return found.value
    .filter((r) => r.trainType !== trainType.value)
    .map((r) => `${r.displayName} is ${r.trainType.toUpperCase()}`)
    .concat(`the other runs are ${trainType.value?.toUpperCase()}`)
})

const isActive = (r: PostTrainingRun | RunDetail) =>
  r.status === 'Pending' || r.status === 'Running'

type Band = { low: MetricSeries; high: MetricSeries; color: string }
type Panel = { name: string; series: MetricSeries[]; colors: string[]; bands?: Band[] }

const groupByRecipe = ref(false)

/** The runs grouped by recipe hash, in run order, each group in the color of its first run. */
const recipeGroups = computed(() => {
  const groups = new Map<string, { hash: string; entries: Entry[]; color: string }>()
  entries.value.forEach((e, i) => {
    if (!e.run) return
    const g = groups.get(e.run.recipeHash) ?? {
      hash: e.run.recipeHash,
      entries: [],
      color: colors.value[i],
    }
    g.entries.push(e)
    groups.set(e.run.recipeHash, g)
  })
  return [...groups.values()]
})
const hasRepeats = computed(() => recipeGroups.value.some((g) => g.entries.length > 1))

/** The mean, min and max of several series at each step one of them has. */
const aggregate = (list: MetricSeries[]) => {
  const byStep = new Map<number, number[]>()
  for (const s of list) for (const [x, y] of s.points) byStep.set(x, [...(byStep.get(x) ?? []), y])
  const steps = [...byStep.keys()].sort((a, b) => a - b)
  const of = (f: (v: number[]) => number): [number, number][] =>
    steps.map((x) => [x, f(byStep.get(x)!)])
  return {
    mean: of((v) => v.reduce((a, b) => a + b, 0) / v.length),
    min: of((v) => Math.min(...v)),
    max: of((v) => Math.max(...v)),
  }
}

/**
 * One panel per metric name: every run in its own color, or with Group by recipe one mean line per recipe hash and
 * a band from its lowest to its highest run.
 */
const panels = computed(() => {
  const byName = new Map<string, Panel>()
  if (groupByRecipe.value && hasRepeats.value) {
    const names = new Set(
      entries.value.flatMap((e) => (e.metrics?.series ?? []).map((s) => s.name)),
    )
    for (const name of names) {
      const panel: Panel = { name, series: [], colors: [], bands: [] }
      for (const g of recipeGroups.value) {
        const list = g.entries
          .map((e) => e.metrics?.series.find((s) => s.name === name))
          .filter((s): s is MetricSeries => !!s)
        if (!list.length) continue
        const agg = aggregate(list)
        const label = `${g.hash.slice(0, 8)} · ${g.entries.length} run${g.entries.length === 1 ? '' : 's'}`
        const base = { ...list[0], count: agg.mean.length }
        panel.series.push({ ...base, name: label, points: agg.mean })
        panel.colors.push(g.color)
        if (list.length > 1) {
          panel.bands!.push({
            low: { ...base, name: `${label} min`, points: agg.min },
            high: { ...base, name: `${label} max`, points: agg.max },
            color: g.color,
          })
        }
      }
      byName.set(name, panel)
    }
    return byName
  }
  entries.value.forEach((e, i) => {
    if (!e.run) return
    for (const s of e.metrics?.series ?? []) {
      const panel = byName.get(s.name) ?? { name: s.name, series: [], colors: [] }
      panel.series.push({ ...s, name: e.run.displayName })
      panel.colors.push(colors.value[i])
      byName.set(s.name, panel)
    }
  })
  return byName
})
const mainNames = computed(() =>
  trainType.value === 'rl' ? ['rollout/raw_reward', 'train/pg_loss'] : ['train/loss', VAL_LOSS],
)
const mainPanels = computed(() =>
  mainNames.value.map((n) => panels.value.get(n)).filter((p): p is Panel => !!p),
)
const otherPanels = computed(() =>
  [...panels.value.values()].filter((p) => !mainNames.value.includes(p.name)),
)

const fmt = (v?: number) =>
  v === undefined ? '—' : Math.abs(v) >= 100 ? v.toFixed(1) : v.toFixed(3)

type Summary = { benchmarks?: Record<string, { accuracy?: number }> }

/** The benchmark scores of a run's evaluations, the latest per benchmark, in percent. */
const scoresOf = (r: RunDetail) => {
  const out: Record<string, number> = {}
  const tasks = [...(r.evaluations ?? [])].sort((a, b) =>
    (timeOf(a.CreationTime) ?? '').localeCompare(timeOf(b.CreationTime) ?? ''),
  )
  for (const t of tasks) {
    const s = parseJSON<Summary>(t.ResultSummary?.Valid ? t.ResultSummary.String : '')
    for (const [name, v] of Object.entries(s?.benchmarks ?? {})) {
      if (v.accuracy !== undefined) out[name] = v.accuracy * 100
    }
  }
  return out
}

/** Whether validation loss kept rising after its best point: the last two points are both above the best. */
const risingAfterBest = (e: Entry, bestStep?: number) => {
  const points = e.metrics?.series.find((s) => s.name === VAL_LOSS)?.points ?? []
  const after = points.filter(([step]) => bestStep !== undefined && step > bestStep)
  return after.length >= 2 && after[after.length - 1][1] > after[after.length - 2][1]
}

const results = computed(() =>
  entries.value
    .map((e, i) => ({ e, color: colors.value[i] }))
    .filter(({ e }) => e.run)
    .map(({ e, color }) => {
      const r = e.run!
      const main = r.metricSummary?.main
      const total = (r.recipe as { choices?: { numRollout?: number } } | undefined)?.choices
        ?.numRollout
      const best = r.metricSummary?.bestValLoss
      const start = timeOf(r.startTime)
      const end = timeOf(r.endTime)
      const endMs = end ? dayjs(end).valueOf() : now.value.getTime()
      return {
        runId: r.runId,
        name: r.displayName,
        color,
        status: r.status,
        steps: main
          ? `${main.step.toLocaleString()}${total && isActive(r) ? ` / ${total.toLocaleString()}` : ''}`
          : '—',
        main: main?.value,
        valLoss: best?.value,
        valStep: best?.step,
        rising: !!best && risingAfterBest(e, best.step),
        scores: scoresOf(r),
        pending: isActive(r) || r.stage === 'eval' ? 'Pending' : '—',
        duration: start ? formatDuration((endMs - dayjs(start).valueOf()) / 1000) : '—',
      }
    }),
)

const benchmarks = computed(() =>
  [...new Set(results.value.flatMap((r) => Object.keys(r.scores)))].sort(),
)

/** The best value of each column: the lowest loss, the highest reward and score. */
const best = computed(() => {
  const pick = (values: (number | undefined)[], lower: boolean) => {
    const known = values.filter((v): v is number => v !== undefined)
    if (!known.length) return undefined
    return lower ? Math.min(...known) : Math.max(...known)
  }
  const scores: Record<string, number | undefined> = {}
  for (const b of benchmarks.value)
    scores[b] = pick(
      results.value.map((r) => r.scores[b]),
      false,
    )
  return {
    main: pick(
      results.value.map((r) => r.main),
      trainType.value !== 'rl',
    ),
    valLoss: pick(
      results.value.map((r) => r.valLoss),
      true,
    ),
    scores,
  }
})

const diff = computed(() => configDiff(found.value.map((r) => r.recipe)))
const differing = computed(() => diff.value.filter((d) => d.differs))

/** The result columns a scatter can plot: the train type's main result first, then the scores. */
const scatterColumns = computed(() => {
  const main = { key: 'main', label: trainType.value === 'rl' ? 'Final Reward' : 'Train Loss' }
  const val = { key: 'valLoss', label: 'Best Val Loss' }
  const first = trainType.value === 'rl' ? [main] : [val, main]
  return [...first, ...benchmarks.value.map((b) => ({ key: b, label: b }))]
})

/**
 * The single-parameter scatter: drawn when exactly one field besides the recipe hash differs and it is a number,
 * otherwise a note when several fields differ.
 */
const scatter = computed(() => {
  const fields = differing.value.filter((d) => d.field !== 'provenance.recipeHash')
  if (fields.length > 1) {
    return {
      message:
        `${fields.length} fields differ (${fields.map((f) => f.label).join(', ')}), so a single-parameter ` +
        `scatter can't be drawn.`,
    }
  }
  const field = fields[0]
  if (!field || !field.values.every((v) => v !== '—' && Number.isFinite(Number(v))))
    return undefined
  const byRun = new Map(results.value.map((r) => [r.runId, r]))
  const points = found.value.map((run, i) => {
    const r = byRun.get(run.runId)
    return {
      name: run.displayName,
      color: r?.color ?? colors.value[i],
      x: Number(field.values[i]),
      ended: !isActive(run),
      results: { main: r?.main, valLoss: r?.valLoss, ...(r?.scores ?? {}) } as Record<
        string,
        number | undefined
      >,
    }
  })
  return { field: field.field, label: field.label, points }
})
const sameRecipe = computed(() => new Set(found.value.map((r) => r.recipeHash)).size === 1)

/** Writes the run ids into the URL, which reloads the page's data. */
const setRunIds = (ids: string[]) => {
  router.replace({ query: { ...route.query, runIds: ids.join(',') } })
}

const removeRun = (runId: string) => setRunIds(runIds.value.filter((id) => id !== runId))
const openRun = (runId: string) => router.push({ path: '/post-training/detail', query: { runId } })

const copyLink = () => {
  copyText(window.location.href)
  ElMessage.success('Link copied')
}

const addDialog = reactive({
  visible: false,
  loading: false,
  runId: '',
  options: [] as PostTrainingRun[],
})

/** Lists runs of the same training type in the current workspace that match the query. */
const searchRuns = async (query: string) => {
  addDialog.loading = true
  try {
    const res = (await listPostTrainingRuns(
      {
        workspace: wsStore.currentWorkspaceId ?? '',
        trainType: trainType.value,
        search: query || undefined,
        limit: 20,
      },
      true,
    )) as unknown as RunListResp
    addDialog.options = res.items ?? []
  } catch {
    addDialog.options = []
  } finally {
    addDialog.loading = false
  }
}

const openAdd = () => {
  Object.assign(addDialog, { visible: true, runId: '', options: [] })
  searchRuns('')
}

const confirmAdd = () => {
  setRunIds([...runIds.value, addDialog.runId])
  addDialog.visible = false
}
</script>

<style scoped>
.pt-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.pt-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 4px;
  border-radius: 50%;
}
.pt-changed {
  padding: 0 4px;
  border-radius: 4px;
  background: var(--el-color-warning-light-8);
}
</style>

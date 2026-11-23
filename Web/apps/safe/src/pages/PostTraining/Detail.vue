<template>
  <div class="pt-detail">
    <el-result v-if="notFound" icon="warning" title="Run not found">
      <template #extra>
        <el-button round @click="backToRuns(router)">Back to runs</el-button>
      </template>
    </el-result>

    <el-result
      v-else-if="loadError"
      icon="error"
      title="Couldn't load the run"
      :sub-title="loadError"
    >
      <template #extra>
        <el-button round @click="load">Retry</el-button>
        <el-button round @click="backToRuns(router)">Back to runs</el-button>
      </template>
    </el-result>

    <div v-else-if="!run" v-loading="true" class="h-[240px]" />

    <template v-else>
      <div class="flex items-start justify-between gap-4">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <el-button link :icon="ArrowLeft" @click="backToRuns(router)" />
            <span class="text-[20px] font-600 truncate">{{ run.displayName }}</span>
            <el-tag size="small" effect="plain">{{ run.trainType.toUpperCase() }}</el-tag>
            <span v-if="run.algorithm" class="text-[12px] text-[var(--safe-muted)]">
              {{ run.algorithm.toUpperCase() }}
            </span>
            <el-tag size="small" :type="statusTagType(run.status)">{{ run.status }}</el-tag>
          </div>
          <div class="pt-meta">
            <span>
              Run ID {{ run.runId }}
              <el-icon class="cursor-pointer align-middle" @click="copyText(run.runId)">
                <CopyDocument />
              </el-icon>
            </span>
            <span>Base model {{ run.baseModelName || run.baseModelId }}</span>
            <span>Dataset {{ run.datasetName || run.datasetId }}</span>
            <span>Owner {{ run.userName || run.userId }}</span>
            <span>Created {{ shortTime(timeOf(run.createdAt)) }}</span>
            <span>Duration {{ duration }}</span>
            <span>
              Recipe
              <el-button link type="primary" @click="tab = 'recipe'">
                {{ run.recipeHash.slice(0, 7) }}
              </el-button>
            </span>
            <el-link
              v-if="run.metricSummary?.links?.wandb"
              :href="run.metricSummary.links.wandb"
              target="_blank"
              rel="noopener noreferrer"
              type="primary"
              :underline="false"
            >
              View in W&B
            </el-link>
          </div>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <el-tooltip
            v-if="isActive"
            :content="actionTip('Stop', isReadonly ? readOnlyTip : '')"
            placement="top"
          >
            <span>
              <el-button
                circle
                class="btn-danger-plain"
                :icon="CloseBold"
                :disabled="isReadonly"
                @click="handleStop"
              />
            </span>
          </el-tooltip>
          <el-tooltip :content="actionTip('Clone', cloneBlocked)" placement="top">
            <span>
              <el-button
                circle
                class="btn-success-plain"
                :icon="CopyDocument"
                :disabled="!!cloneBlocked"
                @click="copyAsNew"
              />
            </span>
          </el-tooltip>
          <el-tooltip :content="actionTip('Delete', isReadonly ? readOnlyTip : '')" placement="top">
            <span>
              <el-button
                circle
                class="btn-danger-plain"
                :icon="Delete"
                :disabled="isReadonly"
                @click="openDelete"
              />
            </span>
          </el-tooltip>
        </div>
      </div>

      <el-alert
        v-if="banner"
        :type="banner.type"
        :closable="false"
        show-icon
        class="mt-3"
        :title="banner.title"
      >
        <div class="whitespace-pre-wrap">{{ banner.text }}</div>
        <div v-if="banner.actions || banner.stall" class="mt-2 flex gap-3">
          <el-button
            v-if="banner.stall"
            link
            type="primary"
            :disabled="isReadonly"
            @click="handleStop"
          >
            Stop
          </el-button>
          <el-button
            v-if="banner.actions"
            link
            type="primary"
            :disabled="!!cloneBlocked"
            @click="copyAsNew"
          >
            Clone
          </el-button>
          <el-button link type="primary" @click="viewLogs">View logs</el-button>
        </div>
      </el-alert>

      <el-card class="safe-card mt-3" shadow="never">
        <div class="flex justify-end mb-2">
          <el-button link type="primary" :disabled="!run.workloadId" @click="viewLogs">
            View logs
          </el-button>
        </div>
        <StageTimeline :run="run" :now="now.getTime()" />
      </el-card>

      <el-tabs v-model="tab" class="mt-3">
        <el-tab-pane label="Overview" name="overview">
          <div class="pt-cards">
            <div v-for="c in cards" :key="c.label" class="pt-stat">
              <div class="text-[12px] text-[var(--safe-muted)]">{{ c.label }}</div>
              <div class="text-[20px] font-600 truncate" :title="c.value">{{ c.value }}</div>
              <el-button
                v-if="c.link"
                link
                type="primary"
                class="text-[12px]"
                @click="openCardLink(c.link)"
              >
                {{ c.link.text }}
              </el-button>
            </div>
          </div>
          <div class="pt-grid mt-3">
            <MetricsPanel
              v-for="p in overviewPanels"
              :key="p.title"
              :title="p.title"
              :series="p.series"
              :markers="metrics?.markers"
              :smoothing="smoothing"
            />
          </div>
          <el-empty v-if="!overviewPanels.length" description="No metrics yet" :image-size="60" />
          <RunSummary class="mt-3" :run="run" :metrics="metrics" :now="now.getTime()" />
        </el-tab-pane>

        <el-tab-pane label="Metrics" name="metrics">
          <div class="flex items-center gap-4 mb-3">
            <span class="text-[13px]">Smoothing</span>
            <el-slider v-model="smoothing" :min="0" :max="0.99" :step="0.01" class="w-[200px]" />
            <el-link :href="metricsCsvUrl(run.runId)" type="primary" :underline="false">
              Download CSV
            </el-link>
          </div>
          <div v-if="registryPanels.shown.length" class="pt-grid mb-4">
            <MetricsPanel
              v-for="p in registryPanels.shown"
              :key="p.panel.id"
              :title="PANEL_TITLES[p.panel.id] ?? p.panel.id"
              :series="seriesOf(p.metrics.series)"
              :band="[
                seriesByName.get(p.metrics.band[0] ?? ''),
                seriesByName.get(p.metrics.band[1] ?? ''),
              ]"
              :stack="p.panel.stack"
              :rest="p.metrics.rest ? seriesByName.get(p.metrics.rest) : undefined"
              :divisor="p.panel.per === 'prompts' ? promptsPerRollout : undefined"
              :log-y="p.panel.logY"
              :markers="metrics?.markers"
              :smoothing="p.panel.smooth ? smoothing : 0"
            />
          </div>
          <div v-for="g in metricGroups" v-else :key="g.name" class="mb-4">
            <div class="font-500 mb-2">{{ g.name }}</div>
            <div class="pt-grid">
              <MetricsPanel
                v-for="s in g.series"
                :key="s.name"
                :title="s.name"
                :series="[s]"
                :markers="metrics?.markers"
                :smoothing="smoothing"
              />
            </div>
          </div>
          <AllMetricsTable
            v-if="metrics?.series.length"
            :series="metrics.series"
            :better="metrics.better"
            :unassigned="registryPanels.unassigned"
            :smoothing="smoothing"
          />
          <el-empty v-else description="No metrics yet" :image-size="60" />
        </el-tab-pane>

        <el-tab-pane label="Outputs" name="outputs">
          <el-table :data="outputs" size="small">
            <el-table-column label="Model">
              <template #default="{ row }">
                <el-link type="primary" @click="router.push(`/model-square/detail/${row.modelId}`)">
                  {{ row.name }}
                </el-link>
              </template>
            </el-table-column>
            <el-table-column label="Status" min-width="200" prop="status" />
          </el-table>
          <div v-if="run.outputPath" class="text-[13px] mt-2">Path {{ run.outputPath }}</div>
        </el-tab-pane>

        <el-tab-pane label="Recipe" name="recipe">
          <div class="flex items-center gap-4 mb-1 text-[13px]">
            <span>
              Recipe hash
              <span class="text-[var(--safe-muted)]">{{ run.recipeHash }}</span>
            </span>
            <el-button link type="primary" @click="downloadRecipe">Download recipe</el-button>
          </div>
          <el-tabs v-model="recipeTab">
            <RecipePanes :recipe="run.recipe" :framework="run.framework" />
          </el-tabs>
        </el-tab-pane>
      </el-tabs>
    </template>

    <el-dialog v-model="deleteDialog.visible" title="Delete run" width="480px">
      <div v-if="run">
        Delete run <b>{{ run.displayName }}</b
        >?
        <span v-if="isActive">It is stopped first.</span>
        Its record and curves are removed. The run directory, with its checkpoints, stays on the
        workspace storage.
      </div>
      <el-checkbox v-if="run?.outputModelId" v-model="deleteDialog.deleteModel" class="mt-3">
        Also delete the output model {{ run.outputModelId }} and its weights
      </el-checkbox>
      <template #footer>
        <el-button @click="deleteDialog.visible = false">Cancel</el-button>
        <el-button type="danger" :loading="deleteDialog.loading" @click="confirmDelete">
          Delete
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, CloseBold, CopyDocument, Delete } from '@element-plus/icons-vue'
import { useDocumentVisibility, useIntervalFn, useNow } from '@vueuse/core'
import dayjs from 'dayjs'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import { copyText } from '@/utils/index'
import {
  LOST_MESSAGE,
  NOT_RETRIED_EVENT,
  STAGE_LABELS,
  backToRuns,
  curvesDue,
  deletePostTrainingRun,
  errorMessageOf,
  formatDuration,
  getPostTrainingMetrics,
  getPostTrainingModels,
  getPostTrainingRun,
  metricsCsvUrl,
  panelsOf,
  stallNote,
  statusTagType,
  stopPostTrainingRun,
  timeOf,
  type MetricSeries,
  type MetricsResp,
  type ModelsResp,
  type RunDetail,
  type RunStage,
} from '@/services/postTraining'
import StageTimeline from './Components/StageTimeline.vue'
import MetricsPanel from './Components/MetricsPanel.vue'
import AllMetricsTable from './Components/AllMetricsTable.vue'
import RunSummary from './Components/RunSummary.vue'
import RecipePanes from './Components/RecipePanes.vue'

defineOptions({ name: 'PostTrainingDetailPage' })

const TABS = ['overview', 'metrics', 'outputs', 'recipe']
const VAL_LOSS = 'eval/val/loss'
const readOnlyTip = 'Read-only access'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const wsStore = useWorkspaceStore()
const isReadonly = computed(() => userStore.isReadonlyManager)
const now = useNow({ interval: 1000 })
const visibility = useDocumentVisibility()

const runId = computed(() => String(route.query.runId ?? ''))
const tab = ref(TABS.includes(String(route.query.tab)) ? String(route.query.tab) : 'overview')
watch(tab, (t) => router.replace({ query: { ...route.query, tab: t } }))

const run = ref<RunDetail>()
const notFound = ref(false)
// Why the run could not be read, when it exists or that is unknown: no access, or a failing server.
const loadError = ref('')
// Replaced as a whole on every load; its points stay plain arrays for the charts.
const metrics = shallowRef<MetricsResp>()
const smoothing = ref(0.6)

const isActive = computed(() => run.value?.status === 'Pending' || run.value?.status === 'Running')

type Recipe = {
  choices?: { numRollout?: number }
}
const recipe = computed(() => (run.value?.recipe ?? {}) as Recipe)
const recipeText = computed(() => JSON.stringify(run.value?.recipe ?? {}, null, 2))
const recipeTab = ref('args')

/** Saves the run's recipe snapshot as a JSON file. */
const downloadRecipe = () => {
  const url = URL.createObjectURL(new Blob([recipeText.value], { type: 'application/json' }))
  const a = document.createElement('a')
  a.href = url
  a.download = `recipe-${run.value?.runId ?? 'run'}.json`
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/** A message ending with a full stop, so another sentence can follow it. */
const sentence = (s: string) => (s.trim() && !/[.!?]$/.test(s.trim()) ? `${s.trim()}.` : s.trim())
const shortTime = (t?: string) => (t ? dayjs(t).format('MM-DD HH:mm') : '—')
const stageLabel = (s?: string) => STAGE_LABELS[(s || 'train') as RunStage] ?? s

/**
 * Loads the run; a refresh keeps what is shown when it fails. The page reports a failure itself rather than through
 * the request's handler, whose rejection does not carry the status: a missing run shows as not found, and one that
 * cannot be read, forbidden or failing, as an error instead of loading forever.
 */
const fetchRun = async (refresh = false) => {
  const id = runId.value
  if (!id) {
    notFound.value = true
    return
  }
  try {
    const loaded = (await getPostTrainingRun(id, true)) as unknown as RunDetail
    // The page may show another run by now: the answer for the run it left is dropped.
    if (runId.value !== id) return
    run.value = loaded
    loadError.value = ''
  } catch (err) {
    if (runId.value !== id || (refresh && run.value)) return
    if ((err as { response?: { status?: number } })?.response?.status === 404) notFound.value = true
    else loadError.value = errorMessageOf(err, 'Reading the run')
  }
}

// When the curves were last asked for.
let curvesReadAt = 0
/** Loads all metric series and their markers. */
const fetchMetrics = async (quiet = false) => {
  const id = runId.value
  if (!id) return
  curvesReadAt = Date.now()
  try {
    const loaded = (await getPostTrainingMetrics(id, {}, quiet)) as unknown as MetricsResp
    // The curves of a run the page has left are dropped, as fetchRun drops its answer.
    if (runId.value !== id) return
    metrics.value = loaded
  } catch {
    /* the curves keep what they show */
  }
}

/** Loads the run, then its curves: a run that cannot be read has none to show, nor a second error to report. */
const load = async () => {
  const id = runId.value
  loadError.value = ''
  await fetchRun()
  if (run.value && runId.value === id) fetchMetrics()
}

// Why a clone of the run cannot be created in the workspace now, as the run list tells for its Clone.
const createBlocked = ref('')
/** Reads whether the workspace can create runs now; an answer for a workspace left meanwhile is dropped. */
const fetchAvailability = async () => {
  const workspace = wsStore.currentWorkspaceId
  if (!workspace) return
  let blocked = ''
  try {
    const res = (await getPostTrainingModels(workspace)) as unknown as ModelsResp
    blocked =
      res.unavailableReason ||
      (res.pausedReason ? `New runs are paused. ${res.pausedReason}` : '')
  } catch {
    /* the wizard reports what fails */
  }
  if (wsStore.currentWorkspaceId === workspace) createBlocked.value = blocked
}
const cloneBlocked = computed(() => (isReadonly.value ? readOnlyTip : createBlocked.value))

// The run is read every 10 seconds, and its curves every 30 while a tab shows them or their markers.
useIntervalFn(() => {
  if (!isActive.value) return
  if (visibility.value !== 'visible') return
  fetchRun(true)
  if (curvesDue(tab.value, curvesReadAt)) fetchMetrics(true)
}, 10000)

// The points a run wrote after the last read are read once more when it ends, whichever tab shows.
watch(isActive, (on, was) => {
  // Opening another run clears this one, which is not a run that ended.
  if (was && !on && run.value) fetchMetrics(true)
})

// A tab that was hidden reads its run once when it shows again: the run may have ended meanwhile, which only a
// read of the run tells.
watch(visibility, (state) => {
  if (state === 'visible' && run.value) fetchRun(true)
})

/** Training time from the start of the workload to its end, or to now while it runs. */
const duration = computed(() => {
  const start = timeOf(run.value?.startTime)
  if (!start || run.value?.status === 'Pending') return '—'
  const end = timeOf(run.value?.endTime)
  const endMs = end ? dayjs(end).valueOf() : now.value.getTime()
  return formatDuration((endMs - dayjs(start).valueOf()) / 1000)
})

type Banner = {
  type: 'error' | 'warning' | 'info'
  title: string
  text: string
  actions?: boolean
  stall?: boolean
}

/** The banner under the title: why a run failed, looks stalled, waits or was stopped. */
const banner = computed((): Banner | undefined => {
  const r = run.value
  if (!r) return undefined
  const position = r.queuePosition > 0 ? ` · #${r.queuePosition} in queue` : ''
  if (r.status === 'Failed') {
    const notRetried = (r.stageHistory ?? []).some((e) => e.event === NOT_RETRIED_EVENT)
    return {
      type: 'error',
      title: `Failed at ${stageLabel(r.failedStage)}.`,
      text: [
        sentence(r.message ?? ''),
        notRetried ? 'Not retried: retrying would fail the same way.' : '',
      ]
        .filter(Boolean)
        .join(' '),
      actions: true,
    }
  }
  const note = stallNote(r, now.value.getTime())
  if (note) {
    const since = Date.parse(timeOf(r.stalledSince) ?? '')
    const minutes = Math.max(1, Math.round((now.value.getTime() - since) / 60000))
    const text = r.message?.startsWith(LOST_MESSAGE)
      ? `The launcher hasn't reported for ${minutes} min, so the stall watchdog can't act. Check the logs, or stop the run.`
      : r.message || `${note}.`
    return { type: 'warning', title: '', text, stall: true }
  }
  if (r.status === 'Pending' || (r.status === 'Running' && r.queuePosition > 0)) {
    return {
      type: 'info',
      title: '',
      text: `${r.message || 'Waiting to be scheduled'}${position}`,
    }
  }
  if (r.status === 'Stopped') {
    const end = timeOf(r.endTime)
    return {
      type: 'info',
      title: '',
      text: `${r.message || 'Stopped'}${end ? ` at ${shortTime(end)}` : ''}`,
    }
  }
  return undefined
})

/** The estimated time left in Train, from the pace since the last resume or the start of training. */
const eta = computed(() => {
  const r = run.value
  const total = recipe.value.choices?.numRollout
  const step = r?.metricSummary?.main?.step
  if (!r || r.stage !== 'train' || r.status !== 'Running' || !total || step === undefined) return ''
  const history = r.stageHistory ?? []
  const resumes = history.filter((e) => e.resumeStep !== undefined)
  const last = resumes[resumes.length - 1]
  const start = history.find((e) => e.stage === 'train' && !e.event && e.resumeStep === undefined)
  const from = last
    ? { t: Date.parse(last.time), step: last.resumeStep ?? 0 }
    : start
      ? { t: Date.parse(start.time), step: 0 }
      : undefined
  if (!from || step <= from.step) return ''
  const perStep = (now.value.getTime() - from.t) / (step - from.step)
  return formatDuration(((total - step) * perStep) / 1000)
})

/** The tooltip of a header action: its name, and why it is unavailable when it is. */
const actionTip = (label: string, reason?: string) => (reason ? `${label} · ${reason}` : label)

type CardLink = { text: string; tab?: string; path?: string }

/** Opens what a card links to: another tab of this page or another page. */
const openCardLink = (link: CardLink) => {
  if (link.path) router.push(link.path)
  else if (link.tab) tab.value = link.tab
}

const cards = computed(() => {
  const r = run.value
  const main = r?.metricSummary?.main
  const total = recipe.value.choices?.numRollout
  const unit = r?.trainType === 'rl' ? 'Rollout' : 'Step'
  const out: { label: string; value: string; link?: CardLink }[] = [
    {
      label: unit,
      value: main
        ? `${main.step.toLocaleString()}${total ? ` / ${total.toLocaleString()}` : ''}`
        : '—',
    },
    { label: main?.name ?? 'Main metric', value: main ? main.value.toFixed(4) : '—' },
  ]
  if (r?.trainType === 'sft') {
    const best = r.metricSummary?.bestValLoss
    out.push({
      label: 'Best val loss',
      value: best ? `${best.value.toFixed(3)} @ ${best.step}` : '—',
    })
  }
  out.push({ label: 'Time left', value: eta.value || '—' })
  const model = r?.outputModel
  out.push({
    label: 'Output model',
    value: model
      ? `${model.displayName || model.id}${model.phase === 'Ready' ? '' : ` · ${model.phase}`}`
      : '—',
    link: model
      ? { text: 'Open in Model Square', path: `/model-square/detail/${model.id}` }
      : undefined,
  })
  return out
})

const seriesByName = computed(() => new Map((metrics.value?.series ?? []).map((s) => [s.name, s])))

/** The curves of the overview: loss and validation loss for SFT, the main metric and a loss for RL, and throughput. */
const overviewPanels = computed(() => {
  const r = run.value
  const all = metrics.value?.series ?? []
  const pick = (name?: string) => (name ? seriesByName.value.get(name) : undefined)
  const panels: { title: string; series: MetricSeries[] }[] = []
  if (r?.trainType === 'sft') {
    const loss = pick('train/loss') ?? pick(r.metricSummary?.main?.name)
    const val = pick(VAL_LOSS)
    if (loss) panels.push({ title: 'Loss', series: val ? [loss, val] : [loss] })
  } else {
    const main = pick(r?.metricSummary?.main?.name)
    if (main) panels.push({ title: main.name, series: [main] })
    const loss = all.find((s) => s.name !== main?.name && /loss/.test(s.name))
    if (loss) panels.push({ title: loss.name, series: [loss] })
  }
  const throughput = registryPanels.value.shown.find((p) => p.panel.id === 'throughput')
  const perf = throughput ? seriesOf(throughput.metrics.series) : []
  if (perf.length) panels.push({ title: PANEL_TITLES.throughput, series: perf })
  return panels
})

/** Panel titles by registry panel id. */
const PANEL_TITLES: Record<string, string> = {
  loss: 'Loss',
  lr: 'Learning rate',
  grad_norm: 'Grad norm',
  throughput: 'Throughput',
  reward: 'Reward',
  signal: 'Effective signal',
  passrate: 'Pass@k',
  policy: 'Policy loss & KL',
  clip_entropy: 'Clipping & entropy',
  consistency: 'Train–inference consistency',
  length: 'Response length & truncation',
  time: 'Time per rollout',
  engine: 'Inference engine',
  grad_perf: 'Grad norm & step time',
  staleness: 'Sample staleness',
  critic: 'Critic',
  mtp: 'MTP loss',
}

/** The registry's panels this run shows and the training metrics no panel draws. */
const registryPanels = computed(() => {
  const names = (metrics.value?.series ?? []).map((s) => s.name)
  const choices = (run.value?.recipe as { choices?: { schedule?: string } } | undefined)?.choices
  return panelsOf(metrics.value?.panels ?? [], names, {
    algorithm: run.value?.algorithm,
    schedule: choices?.schedule,
  })
})

const seriesOf = (names: string[]) =>
  names.map((n) => seriesByName.value.get(n)).filter((s): s is MetricSeries => !!s)

/** The prompts of one rollout, which turn the effective-signal counts into shares. */
const promptsPerRollout = computed(
  () => (run.value?.recipe as { choices?: { batchSize?: number } } | undefined)?.choices?.batchSize,
)

/** All series grouped by name prefix, for runs whose metrics come without the registry's panels. */
const metricGroups = computed(() => {
  const groups = new Map<string, MetricSeries[]>()
  for (const s of metrics.value?.series ?? []) {
    const prefix = s.name.includes('/') ? s.name.slice(0, s.name.indexOf('/')) : 'other'
    groups.set(prefix, [...(groups.get(prefix) ?? []), s])
  }
  return [...groups].map(([name, series]) => ({ name, series }))
})

/** The run's model: the output of its last step. */
const outputs = computed(() => {
  const m = run.value?.outputModel
  return m ? [{ name: m.displayName || m.id, modelId: m.id, status: m.phase }] : []
})

const viewLogs = () => {
  if (run.value?.workloadId) {
    router.push({ path: '/rayjob/detail', query: { id: run.value.workloadId } })
  }
}

const copyAsNew = () => {
  router.push({ path: '/post-training/create', query: { fromRun: runId.value } })
}

const handleStop = async () => {
  if (!run.value) return
  try {
    await ElMessageBox.confirm(
      'Training stops now. Checkpoints already saved are kept, and you can copy this run to continue later.',
      `Stop run ${run.value.displayName}?`,
      { confirmButtonText: 'Stop', cancelButtonText: 'Cancel', type: 'warning' },
    )
  } catch {
    return
  }
  try {
    await stopPostTrainingRun(run.value.runId)
    ElMessage.success('Run stopped')
  } catch {
    // The request's error handler has shown why; the run may have ended meanwhile, which the read shows.
  }
  fetchRun(true)
}

const deleteDialog = reactive({ visible: false, loading: false, deleteModel: false })

const openDelete = () => {
  Object.assign(deleteDialog, { visible: true, loading: false, deleteModel: false })
}

const confirmDelete = async () => {
  deleteDialog.loading = true
  try {
    await deletePostTrainingRun(runId.value, deleteDialog.deleteModel)
    ElMessage.success('Run deleted')
    deleteDialog.visible = false
    backToRuns(router)
  } finally {
    deleteDialog.loading = false
  }
}

watch(runId, () => {
  run.value = undefined
  metrics.value = undefined
  notFound.value = false
  load()
})

watch(() => wsStore.currentWorkspaceId, fetchAvailability)

onMounted(() => {
  load()
  fetchAvailability()
})
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
.pt-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}
.pt-stat {
  padding: 12px;
  border: 1px solid var(--safe-border);
  border-radius: 8px;
}
.pt-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
</style>

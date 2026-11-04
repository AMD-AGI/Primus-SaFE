<template>
  <div class="pt-detail">
    <el-result v-if="notFound" icon="warning" title="Run not found">
      <template #extra>
        <el-button round @click="router.push('/post-training')">Back to runs</el-button>
      </template>
    </el-result>

    <div v-else-if="!run" v-loading="true" class="h-[240px]" />

    <template v-else>
      <div class="flex items-start justify-between gap-4">
        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <el-button link :icon="ArrowLeft" @click="router.push('/post-training')" />
            <span class="text-[20px] font-600 truncate">{{ run.displayName }}</span>
            <el-tag size="small" effect="plain">{{ run.trainType.toUpperCase() }}</el-tag>
            <el-tag size="small" type="info" effect="plain">{{ methodLabel(run.method) }}</el-tag>
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
          <el-tooltip :content="readOnlyTip" :disabled="!isReadonly" placement="top">
            <span v-if="isActive">
              <el-button round :disabled="isReadonly" @click="handleStop">Stop</el-button>
            </span>
          </el-tooltip>
          <el-tooltip :content="readOnlyTip" :disabled="!isReadonly" placement="top">
            <span>
              <el-button round :disabled="isReadonly" @click="copyAsNew">Copy as New Run</el-button>
            </span>
          </el-tooltip>
          <el-tooltip :content="readOnlyTip" :disabled="!isReadonly" placement="top">
            <span>
              <el-button round :disabled="isReadonly" @click="saveTemplate = true">
                Save as Template
              </el-button>
            </span>
          </el-tooltip>
          <el-tooltip :content="readOnlyTip" :disabled="!isReadonly" placement="top">
            <span>
              <el-button round :disabled="isReadonly" @click="abDrawer = true">
                A/B from This Run
              </el-button>
            </span>
          </el-tooltip>
          <el-tooltip :content="evaluateTip" :disabled="!evaluateTip" placement="top">
            <span>
              <el-button round :disabled="!!evaluateTip" @click="openEvaluate">Evaluate</el-button>
            </span>
          </el-tooltip>
          <el-tooltip :content="readOnlyTip" :disabled="!isReadonly" placement="top">
            <span>
              <el-button round plain type="danger" :disabled="isReadonly" @click="openDelete">
                Delete
              </el-button>
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
            :disabled="isReadonly"
            @click="copyAsNew"
          >
            Copy as New Run
          </el-button>
          <el-button v-if="banner.cleanup" link type="primary" @click="tab = 'checkpoints'">
            Clean up checkpoints
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
              <div class="text-[20px] font-600">{{ c.value }}</div>
              <el-button
                v-if="c.link"
                link
                type="primary"
                class="text-[12px]"
                @click="tab = c.link.tab"
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
          <GpuPanel
            v-if="metrics?.series.length"
            class="mb-4"
            :series="systemSeries"
            :layout="runLayout"
            :workload-id="run.workloadId"
          />
          <AllMetricsTable
            v-if="metrics?.series.length"
            :series="metrics.series"
            :better="metrics.better"
            :unassigned="registryPanels.unassigned"
            :smoothing="smoothing"
          />
          <el-empty v-else description="No metrics yet" :image-size="60" />
        </el-tab-pane>

        <el-tab-pane label="Checkpoints" name="checkpoints">
          <div class="flex items-center justify-between mb-1">
            <div class="text-[13px]">
              Retention {{ retention }} · {{ checkpoints.length }} saved
              <span v-if="saveInterval"> · Every {{ saveInterval }} steps</span>
            </div>
            <el-tooltip
              v-if="!isActive"
              :content="cleanupTip"
              :disabled="!cleanupTip"
              placement="top"
            >
              <span>
                <el-button size="small" round :disabled="!!cleanupTip" @click="openCleanup">
                  Clean up checkpoints
                </el-button>
              </span>
            </el-tooltip>
          </div>
          <div v-if="disk" class="text-[12px] text-[var(--safe-muted)] mb-2">
            Storage · checkpoints {{ formatBytes(checkpointBytes(disk), 1) }} · outputs
            {{
              formatBytes(
                (disk.finalModel ?? 0) + (disk.finalAdapter ?? 0) + (disk.exports ?? 0),
                1,
              )
            }}
          </div>
          <el-alert
            v-if="hintBytes > 0"
            type="warning"
            :closable="false"
            class="mb-2"
            :title="`Ended ${endedDays} days ago. Clean up checkpoints to free ${formatBytes(hintBytes, 1)}.`"
          />
          <el-table :data="checkpoints" size="small">
            <el-table-column label="Step" width="200">
              <template #default="{ row }">
                {{ row.step.toLocaleString() }}
                <el-tag v-if="row.best" size="small" type="warning" effect="plain">Best</el-tag>
                <el-tag v-if="row.resumed" size="small" effect="plain">Resumed</el-tag>
              </template>
            </el-table-column>
            <el-table-column v-if="run.trainType === 'sft'" label="Val Loss" width="160">
              <template #default="{ row }">{{ row.valLoss?.toFixed(3) ?? '—' }}</template>
            </el-table-column>
            <el-table-column label="Status">
              <template #default="{ row }">
                <span :class="{ 'text-[var(--safe-muted)]': row.deletedBy }">
                  {{ row.deletedBy ? `Deleted by ${row.deletedBy}` : 'Kept' }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="Actions" width="170">
              <template #default="{ row }">
                <el-tooltip
                  v-if="!row.deletedBy"
                  :content="exportTip"
                  :disabled="!exportTip"
                  placement="top"
                >
                  <span>
                    <el-button link type="primary" :disabled="!!exportTip" @click="openExport(row)">
                      Export as Model
                    </el-button>
                  </span>
                </el-tooltip>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="isActive" class="text-[12px] text-[var(--safe-muted)] mt-2">
            Export becomes available when the run ends. The last step is exported automatically.
          </div>
        </el-tab-pane>

        <el-tab-pane label="Outputs" name="outputs">
          <el-table :data="outputs" size="small">
            <el-table-column label="Model">
              <template #default="{ row }">
                <el-link
                  v-if="row.modelId"
                  type="primary"
                  @click="router.push(`/model-square/detail/${row.modelId}`)"
                >
                  {{ row.name }}
                </el-link>
                <span v-else>{{ row.name }}</span>
              </template>
            </el-table-column>
            <el-table-column label="From" width="180">
              <template #default="{ row }">
                {{ row.from }}
                <el-tag v-if="row.best" size="small" type="warning" effect="plain">Best</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="Status" min-width="200">
              <template #default="{ row }">
                <span :class="{ 'text-[var(--el-color-danger)]': row.failed }">{{
                  row.status
                }}</span>
                <div v-if="row.failed && row.message" class="text-[12px] text-[var(--safe-muted)]">
                  {{ row.message }}
                </div>
                <el-button
                  v-if="row.failed && row.step !== undefined"
                  link
                  type="primary"
                  :disabled="!!exportTip"
                  @click="openExport({ step: row.step })"
                >
                  Retry
                </el-button>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="run.outputPath" class="text-[13px] mt-2">Path {{ run.outputPath }}</div>
        </el-tab-pane>

        <el-tab-pane label="Evaluation" name="evaluation">
          <EvalByCheckpoint :run="run" :metrics="metrics" />
          <el-table :data="run.evaluations" size="small">
            <el-table-column label="Task">
              <template #default="{ row }">
                <el-link type="primary" @click="router.push(`/evaluation/${row.TaskId}`)">
                  {{ row.TaskName || row.TaskId }}
                </el-link>
              </template>
            </el-table-column>
            <el-table-column label="Benchmarks">
              <template #default="{ row }">{{ benchmarkNames(row) }}</template>
            </el-table-column>
            <el-table-column label="Scores">
              <template #default="{ row }">{{ scores(row) }}</template>
            </el-table-column>
            <el-table-column label="Status" width="120" prop="Status" />
            <el-table-column label="Created" width="140">
              <template #default="{ row }">{{ shortTime(timeOf(row.CreationTime)) }}</template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="Recipe" name="recipe">
          <div class="flex justify-end mb-2">
            <el-button link type="primary" @click="copyText(recipeText)">Copy</el-button>
          </div>
          <pre class="pt-recipe">{{ recipeText }}</pre>
        </el-tab-pane>
      </el-tabs>
    </template>

    <VariantEditor v-if="templateRequest" v-model="abDrawer" :baseline="templateRequest" />

    <SaveTemplateDialog
      v-if="templateRequest"
      v-model="saveTemplate"
      :request="templateRequest"
      :spec="run ? specOfRun(run) : undefined"
      :model-label="run?.baseModelName || run?.baseModelId"
      :data-label="run?.datasetName || run?.datasetId"
      :source-run-id="run?.runId"
    />

    <el-dialog v-model="evalDialog.visible" title="Evaluate the output model" width="520px">
      <el-form label-position="top">
        <el-form-item label="Benchmarks" required>
          <el-select v-model="evalDialog.benchmarks" multiple filterable style="width: 100%">
            <el-option
              v-for="d in evalDialog.datasets"
              :key="d.datasetId"
              :label="d.displayName"
              :value="d.datasetId"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="Keep the inference service after evaluation">
          <el-switch v-model="evalDialog.keepService" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="evalDialog.visible = false">Cancel</el-button>
        <el-button
          type="primary"
          :disabled="!evalDialog.benchmarks.length"
          :loading="evalDialog.loading"
          @click="confirmEvaluate"
        >
          Evaluate
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="exportDialog.visible"
      :title="`Export Step ${exportDialog.step} as a Model`"
      width="520px"
    >
      <el-form label-position="top">
        <el-form-item label="Checkpoint">{{ exportDialog.summary }}</el-form-item>
        <el-form-item label="Model name" required>
          <el-input v-model="exportDialog.displayName" />
          <div class="text-[12px] text-[var(--safe-muted)]">
            Shown in Model Square. Defaults to the run name plus the step.
          </div>
        </el-form-item>
        <el-form-item v-if="run?.method === 'lora'">
          <el-checkbox v-model="exportDialog.peftAdapter">Also export a PEFT adapter</el-checkbox>
        </el-form-item>
      </el-form>
      <div class="text-[13px]">
        Runs as a separate 1-node job. The new model is registered as
        {{ run?.trainType === 'rl' ? 'rl_trained' : 'fine_tuned' }}; this run's status and outputs
        don't change.
      </div>
      <template #footer>
        <el-button @click="exportDialog.visible = false">Cancel</el-button>
        <el-button
          type="primary"
          :disabled="!exportDialog.displayName.trim()"
          :loading="exportDialog.loading"
          @click="confirmExport"
        >
          Export
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="cleanupDialog.visible" title="Clean up checkpoints" width="520px">
      <el-radio-group v-model="cleanupDialog.keep" class="flex flex-col items-start">
        <el-radio v-if="canKeepBest" value="best">Keep the best checkpoint</el-radio>
        <el-radio value="none">Delete all</el-radio>
      </el-radio-group>
      <div class="mt-3 text-[13px]">
        {{ cleanupFrees }} You can't export from a deleted checkpoint afterwards. Models already
        exported are not affected.
      </div>
      <template #footer>
        <el-button @click="cleanupDialog.visible = false">Cancel</el-button>
        <el-button type="danger" :loading="cleanupDialog.loading" @click="confirmCleanup">
          Clean up
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="deleteDialog.visible" title="Delete run" width="480px">
      <div v-if="run">
        Delete run <b>{{ run.displayName }}</b
        >?
        <span v-if="isActive">It is stopped first.</span>
        Its run directory, checkpoints and curves are removed.
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
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, CopyDocument } from '@element-plus/icons-vue'
import { useDocumentVisibility, useIntervalFn, useNow } from '@vueuse/core'
import dayjs from 'dayjs'
import { useUserStore } from '@/stores/user'
import { copyText, formatBytes } from '@/utils/index'
import { getDatasets } from '@/services/dataset'
import type { DatasetItem } from '@/services/dataset/type'
import {
  LOST_MESSAGE,
  NOT_RETRIED_EVENT,
  STAGE_LABELS,
  checkpointBytes,
  cleanupHintBytes,
  cleanupPostTrainingCheckpoints,
  deletePostTrainingRun,
  exportPostTrainingCheckpoint,
  evaluatePostTrainingRun,
  formatDuration,
  getPostTrainingMetrics,
  getPostTrainingRun,
  metricsCsvUrl,
  panelsOf,
  parseJSON,
  stallNote,
  statusTagType,
  stopPostTrainingRun,
  timeOf,
  type DiskUsage,
  type EvaluationTaskRow,
  type MetricSeries,
  type MetricsResp,
  type RunDetail,
  type RunStage,
} from '@/services/postTraining'
import StageTimeline from './Components/StageTimeline.vue'
import MetricsPanel from './Components/MetricsPanel.vue'
import AllMetricsTable from './Components/AllMetricsTable.vue'
import EvalByCheckpoint from './Components/EvalByCheckpoint.vue'
import GpuPanel from './Components/GpuPanel.vue'
import SaveTemplateDialog from './Components/SaveTemplateDialog.vue'
import VariantEditor from './Components/VariantEditor.vue'
import { requestFromRun, specOfRun } from './Create/wizard'

defineOptions({ name: 'PostTrainingDetailPage' })

const TABS = ['overview', 'metrics', 'checkpoints', 'outputs', 'evaluation', 'recipe']
const VAL_LOSS = 'eval/val/loss'
const DISK_FULL = /out of space|No space left on device/i
const readOnlyTip = 'Read-only access'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const isReadonly = computed(() => userStore.isReadonlyManager)
const now = useNow({ interval: 1000 })
const visibility = useDocumentVisibility()

const runId = computed(() => String(route.query.runId ?? ''))
const tab = ref(TABS.includes(String(route.query.tab)) ? String(route.query.tab) : 'overview')
watch(tab, (t) => router.replace({ query: { ...route.query, tab: t } }))

const run = ref<RunDetail>()
const notFound = ref(false)
const metrics = ref<MetricsResp>()
const smoothing = ref(0.6)

const isActive = computed(() => run.value?.status === 'Pending' || run.value?.status === 'Running')

const saveTemplate = ref(false)
const abDrawer = ref(false)
/** The request the run's recipe snapshot gives a template. */
const templateRequest = computed(() => (run.value ? requestFromRun(run.value) : undefined))

type Recipe = {
  choices?: { numRollout?: number; saveInterval?: number }
  checkpoint?: { keepLast?: number; keepBest?: boolean }
}
const recipe = computed(() => (run.value?.recipe ?? {}) as Recipe)
const recipeText = computed(() => JSON.stringify(run.value?.recipe ?? {}, null, 2))

const methodLabel = (m: string) => (m === 'lora' ? 'LoRA' : 'Full')
const shortTime = (t?: string) => (t ? dayjs(t).format('MM-DD HH:mm') : '—')
const stageLabel = (s?: string) => STAGE_LABELS[(s || 'train') as RunStage] ?? s

/** Loads the run; a quiet refresh keeps what is shown when it fails. */
const fetchRun = async (quiet = false) => {
  if (!runId.value) {
    notFound.value = true
    return
  }
  try {
    run.value = (await getPostTrainingRun(runId.value, quiet)) as unknown as RunDetail
  } catch (err) {
    if ((err as { response?: { status?: number } })?.response?.status === 404) notFound.value = true
  }
}

/** Loads all metric series and their markers. */
const fetchMetrics = async (quiet = false) => {
  if (!runId.value) return
  try {
    metrics.value = (await getPostTrainingMetrics(runId.value, {}, quiet)) as unknown as MetricsResp
  } catch {
    /* the curves keep what they show */
  }
}

/** Whether a checkpoint export is still running or its model is not ready yet. */
const exporting = computed(() =>
  (run.value?.exports ?? []).some((e) =>
    e.modelId ? e.modelPhase !== 'Ready' : e.status !== 'Failed' && e.status !== 'Stopped',
  ),
)

useIntervalFn(() => {
  if (
    !isActive.value &&
    run.value?.stage !== 'eval' &&
    !run.value?.cleanupRequest &&
    !exporting.value
  )
    return
  if (visibility.value !== 'visible') return
  fetchRun(true)
  fetchMetrics(true)
}, 10000)

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
  cleanup?: boolean
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
      text: `${r.message ?? ''}${notRetried ? ' Not retried: retrying would fail the same way.' : ''}`,
      actions: true,
      cleanup: DISK_FULL.test(r.message ?? ''),
    }
  }
  if (stallNote(r, now.value.getTime())) {
    const since = Date.parse(timeOf(r.stalledSince) ?? '')
    const minutes = Math.max(1, Math.round((now.value.getTime() - since) / 60000))
    const text = r.message?.startsWith(LOST_MESSAGE)
      ? `The launcher hasn't reported for ${minutes} min, so the stall watchdog can't act. Check the logs, or stop the run.`
      : (r.message ?? '')
    return { type: 'warning', title: '', text, stall: true }
  }
  if (r.status === 'Pending' || (r.status === 'Running' && r.queuePosition > 0)) {
    return {
      type: 'info',
      title: '',
      text: `${r.message || 'Waiting to be scheduled'}${position}`,
    }
  }
  if (r.status === 'Stopped') return { type: 'info', title: '', text: r.message || 'Stopped' }
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

const cards = computed(() => {
  const r = run.value
  const main = r?.metricSummary?.main
  const total = recipe.value.choices?.numRollout
  const unit = r?.trainType === 'rl' ? 'Rollout' : 'Step'
  const out: { label: string; value: string; link?: { text: string; tab: string } }[] = [
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
      link: best ? { text: 'View checkpoint', tab: 'checkpoints' } : undefined,
    })
  }
  out.push({ label: 'Time left', value: eta.value || '—' })
  return out
})

const seriesByName = computed(() => new Map((metrics.value?.series ?? []).map((s) => [s.name, s])))

/** The two curves of the overview: loss and validation loss for SFT, the main metric and a loss for RL. */
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
  gpu_util: 'GPU utilization',
  gpu_mem: 'GPU memory',
}

/** The registry's panels this run shows and the training metrics no panel draws; system metrics have their group. */
const registryPanels = computed(() => {
  const names = (metrics.value?.series ?? [])
    .map((s) => s.name)
    .filter((n) => !n.startsWith('system/'))
  const choices = (run.value?.recipe as { choices?: { schedule?: string } } | undefined)?.choices
  return panelsOf(metrics.value?.panels ?? [], names, {
    algorithm: run.value?.algorithm,
    schedule: choices?.schedule,
  })
})

/** The GPU layout the run's recipe asked for. */
const runLayout = computed(
  () =>
    (
      run.value?.recipe as
        | {
            layout?: {
              nodes?: number
              gpusPerNode?: number
              colocate?: boolean
              actorGpus?: number
            }
          }
        | undefined
    )?.layout,
)

/** The launcher's GPU samples, drawn in the System group on their own time axis. */
const systemSeries = computed(() =>
  (metrics.value?.series ?? []).filter((s) => s.name.startsWith('system/')),
)

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

const retention = computed(() => {
  const c = recipe.value.checkpoint
  if (!c?.keepLast) return 'Keep all'
  return `Keep last ${c.keepLast}${c.keepBest ? ' + best' : ''}`
})
const saveInterval = computed(() => recipe.value.choices?.saveInterval)

/** The saved checkpoints, best first and then newest first; a deleted one the retention keeps went in a cleanup. */
const checkpoints = computed(() => {
  const m = metrics.value?.markers
  if (!m) return []
  const resumed = new Set(m.resumes.map((r) => r.step))
  const best = m.bestCheckpoint?.step
  const c = recipe.value.checkpoint
  const n = m.checkpoints.length
  return m.checkpoints
    .map((cp, i) => {
      const retained = !c?.keepLast || i >= n - c.keepLast || (!!c.keepBest && cp.step === best)
      const deletedBy = !cp.deleted ? '' : retained ? 'cleanup' : 'retention'
      return { ...cp, best: cp.step === best, resumed: resumed.has(cp.step), deletedBy }
    })
    .sort((a, b) => Number(b.best) - Number(a.best) || b.step - a.step)
})

const disk = computed(() => parseJSON<DiskUsage>(run.value?.diskUsage))
const hintBytes = computed(() => (run.value ? cleanupHintBytes(run.value, now.value.getTime()) : 0))
const endedDays = computed(() => {
  const end = timeOf(run.value?.endTime)
  return end ? Math.floor((now.value.getTime() - Date.parse(end)) / 86400000) : 0
})

const cleanupTip = computed(() => {
  if (isReadonly.value) return readOnlyTip
  if (run.value?.cleanupRequest) return 'Cleanup in progress'
  if (disk.value && checkpointBytes(disk.value) === 0) return 'No checkpoints left'
  return ''
})

const canKeepBest = computed(() => {
  const best = checkpoints.value.find((cp) => cp.best)
  return run.value?.trainType === 'sft' && !!best && !best.deletedBy
})

const exportTip = computed(() => {
  if (isReadonly.value) return readOnlyTip
  if (isActive.value) return 'Export becomes available when the run ends.'
  if (run.value?.cleanupRequest) return 'Cleanup in progress'
  return ''
})

const exportDialog = reactive({
  visible: false,
  loading: false,
  step: 0,
  summary: '',
  displayName: '',
  peftAdapter: false,
})

/** Opens the export dialog for a checkpoint, with the run name plus the step as the model name. */
const openExport = (cp: { step: number; best?: boolean; valLoss?: number }) => {
  const parts = [`Step ${cp.step}`]
  if (cp.best) parts.push('best')
  if (cp.valLoss !== undefined) parts.push(`val loss ${cp.valLoss.toFixed(3)}`)
  Object.assign(exportDialog, {
    visible: true,
    loading: false,
    step: cp.step,
    summary: parts.join(' · '),
    displayName: `${run.value?.displayName ?? ''}-step${cp.step}`,
    peftAdapter: false,
  })
}

const confirmExport = async () => {
  exportDialog.loading = true
  try {
    await exportPostTrainingCheckpoint(runId.value, {
      step: exportDialog.step,
      displayName: exportDialog.displayName.trim(),
      peftAdapter: exportDialog.peftAdapter || undefined,
    })
    ElMessage.success(`Exporting step ${exportDialog.step}`)
    exportDialog.visible = false
    tab.value = 'outputs'
    fetchRun()
  } finally {
    exportDialog.loading = false
  }
}

/** The run's models: the last step's output and every checkpoint export with its job's state. */
const outputs = computed(() => {
  const r = run.value
  if (!r) return []
  const bestStep = r.metricSummary?.bestValLoss?.step
  const rows: {
    name: string
    modelId?: string
    from: string
    step?: number
    best: boolean
    status: string
    failed: boolean
    message?: string
  }[] = []
  if (r.outputModel) {
    rows.push({
      name: r.outputModel.displayName || r.outputModel.id,
      modelId: r.outputModel.id,
      from: 'Last step',
      best: false,
      status: r.outputModel.phase,
      failed: false,
    })
  }
  for (const e of r.exports ?? []) {
    const failed = !e.modelId && (e.status === 'Failed' || e.status === 'Stopped')
    rows.push({
      name: e.displayName || `${r.displayName}-step${e.step}`,
      modelId: e.modelId,
      from: `Step ${e.step.toLocaleString()}`,
      step: e.step,
      best: e.step === bestStep,
      status: e.modelId ? (e.modelPhase ?? '') : failed ? 'Export failed' : 'Exporting',
      failed,
      message: e.message,
    })
  }
  return rows
})

const cleanupDialog = reactive({ visible: false, loading: false, keep: 'none' as 'best' | 'none' })

/** The space a cleanup frees: all checkpoint bytes, or all but about one checkpoint's share when one is kept. */
const cleanupFrees = computed(() => {
  if (!disk.value) return ''
  const bytes = checkpointBytes(disk.value)
  const kept = checkpoints.value.filter((cp) => !cp.deletedBy).length
  const freed = cleanupDialog.keep === 'best' && kept > 0 ? (bytes * (kept - 1)) / kept : bytes
  return `Frees about ${formatBytes(freed, 1)}.`
})

const openCleanup = () => {
  Object.assign(cleanupDialog, {
    visible: true,
    loading: false,
    keep: canKeepBest.value ? 'best' : 'none',
  })
}

const confirmCleanup = async () => {
  cleanupDialog.loading = true
  try {
    await cleanupPostTrainingCheckpoints(runId.value, cleanupDialog.keep)
    ElMessage.success('Cleanup started')
    cleanupDialog.visible = false
    fetchRun()
  } finally {
    cleanupDialog.loading = false
  }
}

const benchmarkNames = (row: EvaluationTaskRow) =>
  (parseJSON<{ datasetName?: string; datasetId?: string }[]>(row.Benchmarks) ?? [])
    .map((b) => b.datasetName || b.datasetId)
    .join(', ')

const scores = (row: EvaluationTaskRow) => {
  type Summary = { benchmarks?: Record<string, { accuracy?: number }> }
  const s = parseJSON<Summary>(row.ResultSummary?.Valid ? row.ResultSummary.String : '')
  return (
    Object.entries(s?.benchmarks ?? {})
      .filter(([, v]) => v.accuracy !== undefined)
      .map(([k, v]) => `${k} ${((v.accuracy ?? 0) * 100).toFixed(1)}`)
      .join(' · ') || '—'
  )
}

const evaluateTip = computed(() => {
  if (isReadonly.value) return readOnlyTip
  if (run.value?.status !== 'Succeeded' || run.value.outputModel?.phase !== 'Ready')
    return 'Available after the output model is ready.'
  return ''
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
  await stopPostTrainingRun(run.value.runId)
  ElMessage.success('Run stopped')
  fetchRun()
}

const evalDialog = reactive({
  visible: false,
  loading: false,
  keepService: false,
  benchmarks: [] as string[],
  datasets: [] as DatasetItem[],
})

const openEvaluate = async () => {
  if (!run.value) return
  Object.assign(evalDialog, { visible: true, loading: false, keepService: false, benchmarks: [] })
  const res = await getDatasets({
    workspace: run.value.workspace,
    datasetType: 'evaluation',
    pageSize: 200,
  })
  evalDialog.datasets = res.items ?? []
}

const confirmEvaluate = async () => {
  evalDialog.loading = true
  try {
    await evaluatePostTrainingRun(runId.value, {
      benchmarks: evalDialog.benchmarks,
      keepService: evalDialog.keepService,
    })
    ElMessage.success('Evaluation requested')
    evalDialog.visible = false
    fetchRun()
  } finally {
    evalDialog.loading = false
  }
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
    router.push('/post-training')
  } finally {
    deleteDialog.loading = false
  }
}

watch(runId, () => {
  run.value = undefined
  metrics.value = undefined
  notFound.value = false
  fetchRun()
  fetchMetrics()
})

onMounted(() => {
  fetchRun()
  fetchMetrics()
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
  grid-template-columns: repeat(4, minmax(0, 1fr));
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
.pt-recipe {
  max-height: 600px;
  overflow: auto;
  padding: 12px;
  font-size: 12px;
  border: 1px solid var(--safe-border);
  border-radius: 8px;
}
</style>

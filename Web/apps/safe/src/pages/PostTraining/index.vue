<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <el-text class="block textx-18 font-500" tag="b">Post Training</el-text>

  <!-- Post Training is turned off on this cluster: its API answers 404. -->
  <el-empty v-if="notEnabled" class="mt-10" description=" ">
    <template #description>
      <div class="text-[16px] font-500">Post Training is not enabled on this cluster</div>
      <div class="mt-2 text-[13px] text-[var(--safe-muted)] max-w-[560px]">
        The cluster admin hasn't turned Post Training on (postTraining.enabled is off), so its API
        returns 404. Use Post Train for SFT and RL on cluster {{ clusterName }}.
      </div>
    </template>
    <el-button type="primary" round @click="router.push('/posttrain')">Open Post Train</el-button>
    <el-button round @click="router.push('/model-square')">Open Model Square</el-button>
  </el-empty>

  <template v-else>
    <el-alert
      v-if="unavailableReason"
      class="mt-3"
      type="warning"
      :closable="false"
      show-icon
      :title="`Post Training isn't available in this workspace. ${unavailableReason}`"
    >
      <el-link type="primary" @click="router.push('/posttrain')">Open Post Train</el-link>
    </el-alert>
    <el-alert
      v-if="pausedReason"
      class="mt-3"
      type="warning"
      :closable="false"
      show-icon
      :title="`New runs are paused on this cluster. ${pausedReason}`"
    />

    <div class="flex flex-wrap items-center justify-between gap-2 mt-4">
      <div class="flex flex-wrap items-center gap-2">
        <el-tooltip
          v-if="canCreate"
          :content="createBlocked"
          :disabled="!createBlocked"
          placement="top"
        >
          <span>
            <el-button
              type="primary"
              round
              :icon="Plus"
              class="text-black"
              :disabled="!!createBlocked"
              @click="router.push('/post-training/create')"
            >
              New Run
            </el-button>
          </span>
        </el-tooltip>
        <el-button
          v-if="router.hasRoute('PostTrainingTemplates')"
          round
          @click="router.push('/post-training/templates')"
        >
          Templates
        </el-button>
        <el-tooltip v-if="canCompare" :content="compareTip" :disabled="!compareTip" placement="top">
          <span>
            <el-button round :disabled="!!compareTip" @click="goCompare">
              Compare ({{ selected.length }})
            </el-button>
          </span>
        </el-tooltip>
        <span v-if="autoRefresh" class="text-[12px] text-[var(--safe-muted)]">
          {{ refreshFailed ? 'Refresh failed · retrying' : `Auto-refresh · ${refreshAgo}` }}
        </span>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <el-input
          v-model="filters.q"
          placeholder="Name or run ID"
          clearable
          style="width: 200px"
          @keyup.enter="onSearch(true)"
          @clear="onSearch(true)"
        />
        <el-select
          v-model="filters.baseModel"
          placeholder="Base Model"
          clearable
          filterable
          style="width: 170px"
          @change="onSearch(true)"
        >
          <el-option v-for="m in baseModelOptions" :key="m.id" :label="m.name" :value="m.id" />
        </el-select>
        <el-input
          v-model="filters.owner"
          placeholder="Owner"
          clearable
          style="width: 120px"
          @keyup.enter="onSearch(true)"
          @clear="onSearch(true)"
        />
        <el-tooltip content="Reset" placement="top">
          <el-button :icon="RefreshIcon" @click="resetFilters" />
        </el-tooltip>
        <el-tooltip content="Refresh" placement="top">
          <el-button :icon="Refresh" @click="fetchData()" />
        </el-tooltip>
      </div>
    </div>

    <el-card class="mt-4 safe-card" shadow="never">
      <el-table
        ref="tableRef"
        :data="tableRows"
        :height="tableHeight"
        size="large"
        row-key="runId"
        default-expand-all
        :tree-props="{ children: 'children' }"
        v-loading="loading"
        :element-loading-text="$loadingText"
        @filter-change="handleFilterChange"
        @selection-change="(list: PostTrainingRun[]) => (selected = list)"
      >
        <template #empty>
          <div v-if="loadError" class="py-6">
            <div class="text-[15px] font-500">Couldn't load runs</div>
            <div class="mt-1 text-[12px] text-[var(--safe-muted)]">{{ loadError }}</div>
            <el-button class="mt-3" round @click="fetchData()">Retry</el-button>
          </div>
          <div v-else-if="hasFilters" class="py-6">
            <div class="text-[15px] font-500">No runs match these filters</div>
            <div class="mt-1 text-[12px] text-[var(--safe-muted)]">{{ filterSummary }}</div>
            <el-button class="mt-3" round @click="resetFilters">Clear filters</el-button>
          </div>
          <div v-else-if="!loading" class="py-6">
            <div class="text-[15px] font-500">No Post Training runs yet</div>
            <div class="mt-1 text-[12px] text-[var(--safe-muted)]">
              Pick a base model in Model Square and choose Train with Post Training, or start from a
              template.
            </div>
            <el-button
              v-if="canCreate"
              class="mt-3"
              type="primary"
              round
              :disabled="!!createBlocked"
              @click="router.push('/post-training/create')"
            >
              New Run
            </el-button>
            <el-button class="mt-3" round @click="router.push('/model-square')"
              >Open Model Square</el-button
            >
          </div>
        </template>

        <el-table-column
          v-if="canCompare"
          type="selection"
          width="36"
          fixed="left"
          reserve-selection
          :selectable="selectable"
        />
        <el-table-column label="Run Name / ID" min-width="270" fixed="left">
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <div class="flex flex-col items-start min-w-0">
                <div class="flex items-center gap-1 max-w-full">
                  <el-link
                    v-if="canOpenDetail"
                    type="primary"
                    class="truncate"
                    @click.stop="goDetail(row)"
                  >
                    {{ row.displayName }}
                  </el-link>
                  <span v-else class="truncate">{{ row.displayName }}</span>
                  <el-tag v-if="row.variantName" size="small" type="info" round>{{
                    row.variantName
                  }}</el-tag>
                </div>
                <div class="flex items-center gap-1">
                  <span class="text-[12px] text-[var(--safe-muted)]">{{ row.runId }}</span>
                  <el-tooltip v-if="row.verifiedRuns" placement="top">
                    <template #content>
                      This recipe succeeded {{ row.verifiedRuns }}
                      {{ row.verifiedRuns === 1 ? 'time' : 'times' }} on this platform.
                      <el-button
                        v-if="canOpenDetail && row.latestVerifiedRun"
                        link
                        type="primary"
                        @click.stop="goDetail({ runId: row.latestVerifiedRun })"
                      >
                        Latest run
                      </el-button>
                    </template>
                    <el-tag size="small" type="primary" effect="light" round
                      >✓ Verified ×{{ row.verifiedRuns }}</el-tag
                    >
                  </el-tooltip>
                </div>
              </div>
            </template>
            <template v-else>
              <div class="flex items-center gap-2">
                <span class="font-500">{{ row.name }}</span>
                <span class="text-[12px] text-[var(--safe-muted)]">{{ groupSummary(row) }}</span>
                <el-link
                  v-if="router.hasRoute('PostTrainingExperiment')"
                  type="primary"
                  @click.stop="openExperiment(row.experimentId)"
                >
                  View experiment
                </el-link>
              </div>
            </template>
          </template>
        </el-table-column>

        <el-table-column
          label="Type / Method"
          width="130"
          column-key="type"
          :filters="typeFilters"
          :filter-multiple="false"
          :filtered-value="typeFilterValue"
          filter-placement="bottom-start"
          :filter-method="passAll"
        >
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <div class="flex items-center gap-1">
                <el-tag
                  size="small"
                  :type="row.trainType === 'sft' ? 'success' : 'warning'"
                  effect="plain"
                >
                  {{ row.trainType?.toUpperCase() }}
                </el-tag>
                <el-tag size="small" type="info" effect="plain">{{
                  methodLabel(row.method)
                }}</el-tag>
              </div>
              <div v-if="row.algorithm" class="text-[12px] text-[var(--safe-muted)]">
                {{ row.algorithm.toUpperCase() }}
              </div>
            </template></template
          >
        </el-table-column>

        <el-table-column label="Progress" width="150">
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <StageBar :run="row" :step="summaryOf(row)?.main?.step" /> </template
          ></template>
        </el-table-column>

        <el-table-column
          label="Status"
          width="190"
          column-key="status"
          :filters="statusFilters"
          :filter-multiple="true"
          :filtered-value="filters.status"
          :filter-method="passAll"
        >
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <el-tag
                size="small"
                :type="statusTagType(row.status)"
                :effect="isDark ? 'plain' : 'light'"
              >
                <span v-if="row.status === 'Running'" class="pt-dot" />{{ row.status }}
              </el-tag>
              <el-tooltip
                v-if="statusNote(row).text"
                :content="statusNote(row).text"
                placement="top"
              >
                <div class="text-[12px] truncate" :class="statusNote(row).cls">
                  {{ statusNote(row).text }}
                </div>
              </el-tooltip>
            </template></template
          >
        </el-table-column>

        <el-table-column label="Key Metric" width="150">
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <template v-if="summaryOf(row)?.main">
                <div class="flex items-center gap-2">
                  <Sparkline
                    :points="summaryOf(row)!.main!.spark"
                    :color="row.status === 'Stopped' ? 'var(--safe-muted)' : 'var(--safe-primary)'"
                  />
                  <span class="font-500">{{ formatValue(summaryOf(row)!.main!.value) }}</span>
                </div>
                <div class="text-[12px] text-[var(--safe-muted)]">
                  {{ summaryOf(row)!.main!.name }}
                </div>
              </template>
              <span v-else>—</span>
            </template></template
          >
        </el-table-column>

        <el-table-column label="Base Model" width="130" show-overflow-tooltip>
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <el-link
                type="primary"
                @click.stop="router.push(`/model-square/detail/${row.baseModelId}`)"
              >
                {{ row.baseModelName || row.baseModelId }}
              </el-link>
            </template></template
          >
        </el-table-column>

        <el-table-column label="Dataset" width="140" show-overflow-tooltip>
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">{{
              row.datasetName || row.datasetId || '—'
            }}</template></template
          >
        </el-table-column>

        <el-table-column label="Output" width="220" show-overflow-tooltip>
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <el-link
                v-if="row.outputModelId"
                type="primary"
                @click.stop="router.push(`/model-square/detail/${row.outputModelId}`)"
              >
                {{ row.outputModelId }}
              </el-link>
              <span v-else>—</span>
            </template></template
          >
        </el-table-column>

        <el-table-column label="Evaluation" width="180">
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">{{ evaluationText(row) }}</template></template
          >
        </el-table-column>

        <el-table-column label="Created" width="110">
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <el-tooltip :content="formatTimeStr(timeOf(row.createdAt))" placement="top">
                <span>{{ shortTime(timeOf(row.createdAt)) }}</span>
              </el-tooltip>
            </template></template
          >
        </el-table-column>

        <el-table-column label="Duration" width="90">
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">{{ durationOf(row) }}</template></template
          >
        </el-table-column>

        <el-table-column label="Owner" width="90" show-overflow-tooltip>
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">{{ row.userName || row.userId }}</template></template
          >
        </el-table-column>

        <el-table-column label="Actions" width="206" fixed="right">
          <template #default="{ row }"
            ><template v-if="!isGroup(row)">
              <el-button v-if="canOpenDetail" link type="primary" @click.stop="goDetail(row)"
                >Details</el-button
              >
              <el-tooltip
                v-if="isActive(row)"
                :content="readOnlyTip"
                :disabled="!isReadonly"
                placement="top"
              >
                <span>
                  <el-button
                    link
                    type="primary"
                    :disabled="isReadonly"
                    @click.stop="handleStop(row)"
                    >Stop</el-button
                  >
                </span>
              </el-tooltip>
              <el-tooltip
                v-if="canCreate"
                :content="isReadonly ? readOnlyTip : createBlocked"
                :disabled="!isReadonly && !createBlocked"
                placement="top"
              >
                <span>
                  <el-button
                    link
                    type="primary"
                    :disabled="isReadonly || !!createBlocked"
                    @click.stop="
                      router.push({ path: '/post-training/create', query: { fromRun: row.runId } })
                    "
                  >
                    Copy
                  </el-button>
                </span>
              </el-tooltip>
              <el-tooltip :content="readOnlyTip" :disabled="!isReadonly" placement="top">
                <span>
                  <el-button link type="danger" :disabled="isReadonly" @click.stop="openDelete(row)"
                    >Delete</el-button
                  >
                </span>
              </el-tooltip>
            </template></template
          >
        </el-table-column>
      </el-table>

      <el-pagination
        class="m-t-2"
        :current-page="pagination.page"
        :page-size="pagination.pageSize"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next"
        :page-sizes="[20, 50, 100]"
        @current-change="(p: number) => ((pagination.page = p), fetchData())"
        @size-change="
          (s: number) => ((pagination.pageSize = s), (pagination.page = 1), fetchData())
        "
      />
    </el-card>

    <el-dialog v-model="deleteDialog.visible" title="Delete run" width="480px">
      <div v-if="deleteDialog.run">
        Delete run <b>{{ deleteDialog.run.displayName }}</b
        >?
        <span v-if="isActive(deleteDialog.run)">It is stopped first.</span>
        Its run directory, checkpoints and curves are removed.
      </div>
      <el-checkbox
        v-if="deleteDialog.run?.outputModelId"
        v-model="deleteDialog.deleteModel"
        class="mt-3"
      >
        Also delete the output model {{ deleteDialog.run.outputModelId }} and its weights
      </el-checkbox>
      <template #footer>
        <el-button @click="deleteDialog.visible = false">Cancel</el-button>
        <el-button type="danger" :loading="deleteDialog.loading" @click="confirmDelete"
          >Delete</el-button
        >
      </template>
    </el-dialog>
  </template>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { useDark, useDocumentVisibility, useIntervalFn, useNow } from '@vueuse/core'
import dayjs from 'dayjs'
import RefreshIcon from '@/components/icons/ResetIcon.vue'
import StageBar from './Components/StageBar.vue'
import Sparkline from './Components/Sparkline.vue'
import { formatBytes, formatTimeStr } from '@/utils'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import { useClusterStore } from '@/stores/cluster'
import {
  cleanupHintBytes,
  deletePostTrainingRun,
  listPostTrainingExperiments,
  type ExperimentListResp,
  formatDuration,
  getPostTrainingModels,
  listPostTrainingRuns,
  parseJSON,
  stallNote,
  statusTagType,
  stopPostTrainingRun,
  timeOf,
  type MetricSummary,
  type PostTrainingRun,
  type RunListParams,
} from '@/services/postTraining'

defineOptions({ name: 'PostTrainingPage' })

const AUTO_REFRESH_MS = 10_000

const router = useRouter()
const isDark = useDark()
const wsStore = useWorkspaceStore()
const userStore = useUserStore()
const clusterStore = useClusterStore()

const isReadonly = computed(() => userStore.isReadonlyManager)
const readOnlyTip = 'Read-only access'
const canCreate = computed(() => router.hasRoute('PostTrainingCreate'))
const canOpenDetail = computed(() => router.hasRoute('PostTrainingDetail'))
const clusterName = computed(() => clusterStore.currentClusterId || '')

const loading = ref(false)
const loadError = ref('')
const rows = ref<PostTrainingRun[]>([])
const tableRef = ref()
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })

const notEnabled = ref(false)
const unavailableReason = ref('')
const pausedReason = ref('')
/** Why new runs cannot be created now, or an empty string. */
const createBlocked = computed(() =>
  isReadonly.value
    ? readOnlyTip
    : unavailableReason.value ||
      (pausedReason.value ? `New runs are paused. ${pausedReason.value}` : ''),
)

const filters = reactive({
  type: '',
  method: '',
  status: [] as string[],
  q: '',
  baseModel: '',
  owner: '',
})

const typeFilters = [
  { text: 'SFT', value: 'type:sft' },
  { text: 'RL', value: 'type:rl' },
  { text: 'Full', value: 'method:full' },
  { text: 'LoRA', value: 'method:lora' },
]
const statusFilters = ['Pending', 'Running', 'Succeeded', 'Failed', 'Stopped'].map((s) => ({
  text: s,
  value: s,
}))
const typeFilterValue = computed(() => [
  ...(filters.type ? [`type:${filters.type}`] : []),
  ...(filters.method ? [`method:${filters.method}`] : []),
])
const passAll = () => true

/** Applies the header filters; Type and Method each keep one value. */
const handleFilterChange = (columns: Record<string, string[]>) => {
  if ('type' in columns) {
    const picked = columns.type ?? []
    filters.type = picked.find((v) => v.startsWith('type:'))?.slice(5) ?? ''
    filters.method = picked.find((v) => v.startsWith('method:'))?.slice(7) ?? ''
  }
  if ('status' in columns) filters.status = columns.status ?? []
  onSearch(true)
}

const hasFilters = computed(
  () =>
    !!(
      filters.type ||
      filters.method ||
      filters.status.length ||
      filters.q ||
      filters.baseModel ||
      filters.owner
    ),
)
const filterSummary = computed(() => {
  const parts: string[] = []
  if (filters.type) parts.push(`Type: ${filters.type.toUpperCase()}`)
  if (filters.method) parts.push(`Method: ${methodLabel(filters.method)}`)
  if (filters.status.length) parts.push(`Status: ${filters.status.join(', ')}`)
  if (filters.q) parts.push(`Search: ${filters.q}`)
  if (filters.baseModel) parts.push(`Base Model: ${filters.baseModel}`)
  if (filters.owner) parts.push(`Owner: ${filters.owner}`)
  return parts.join(' · ')
})

/** The base models seen so far, as filter options. */
const knownBaseModels = reactive(new Map<string, string>())
const baseModelOptions = computed(() => [...knownBaseModels].map(([id, name]) => ({ id, name })))

/** Restores the filters and page from the URL. */
const readQuery = () => {
  const q = router.currentRoute.value.query
  filters.type = (q.type as string) || ''
  filters.method = (q.method as string) || ''
  filters.status = q.status ? String(q.status).split(',').filter(Boolean) : []
  filters.q = (q.q as string) || ''
  filters.baseModel = (q.baseModel as string) || ''
  filters.owner = (q.owner as string) || ''
  pagination.page = Number(q.page || 1)
  pagination.pageSize = Number(q.pageSize || pagination.pageSize)
}

/** Writes the filters and page into the URL, so a refresh or a shared link keeps them. */
const writeQuery = () => {
  const query: Record<string, string> = {}
  if (filters.type) query.type = filters.type
  if (filters.method) query.method = filters.method
  if (filters.status.length) query.status = filters.status.join(',')
  if (filters.q) query.q = filters.q
  if (filters.baseModel) query.baseModel = filters.baseModel
  if (filters.owner) query.owner = filters.owner
  if (pagination.page > 1) query.page = String(pagination.page)
  if (pagination.pageSize !== 20) query.pageSize = String(pagination.pageSize)
  router.replace({ query })
}

const lastRefresh = ref(0)
const refreshFailed = ref(false)
const now = useNow({ interval: 1000 })
const refreshAgo = computed(
  () => `${Math.max(0, Math.round((now.value.getTime() - lastRefresh.value) / 1000))}s ago`,
)

/** Loads one page of runs; a quiet refresh keeps the current rows and selection when it fails. */
const fetchData = async (quiet = false) => {
  const workspace = wsStore.currentWorkspaceId
  if (!workspace) return
  if (!quiet) {
    writeQuery()
    loading.value = true
  }
  const params: RunListParams = {
    workspace,
    offset: (pagination.page - 1) * pagination.pageSize,
    limit: pagination.pageSize,
  }
  if (filters.type) params.trainType = filters.type
  if (filters.method) params.method = filters.method
  if (filters.status.length) params.status = filters.status.join(',')
  if (filters.q) params.search = filters.q
  if (filters.baseModel) params.baseModelId = filters.baseModel
  if (filters.owner) params.owner = filters.owner
  try {
    const res = (await listPostTrainingRuns(params, quiet)) as unknown as {
      total: number
      items: PostTrainingRun[]
    }
    rows.value = res.items ?? []
    pagination.total = res.total ?? 0
    loadError.value = ''
    refreshFailed.value = false
    for (const r of rows.value) knownBaseModels.set(r.baseModelId, r.baseModelName || r.baseModelId)
  } catch (err) {
    if (quiet) {
      refreshFailed.value = true
    } else {
      rows.value = []
      loadError.value = `GET /post-training/runs failed: ${String(err)}`
    }
  } finally {
    lastRefresh.value = Date.now()
    loading.value = false
  }
}

/** Reads whether Post Training is on, and why the workspace cannot create runs now. */
const fetchAvailability = async () => {
  const workspace = wsStore.currentWorkspaceId
  if (!workspace) return
  try {
    const res = (await getPostTrainingModels(workspace)) as unknown as {
      unavailableReason?: string
      pausedReason?: string
    }
    notEnabled.value = false
    unavailableReason.value = res.unavailableReason ?? ''
    pausedReason.value = res.pausedReason ?? ''
  } catch (err) {
    notEnabled.value = (err as { response?: { status?: number } })?.response?.status === 404
  }
}

const onSearch = (resetPage: boolean) => {
  if (resetPage) pagination.page = 1
  fetchData()
}

const resetFilters = () => {
  Object.assign(filters, { type: '', method: '', status: [], q: '', baseModel: '', owner: '' })
  tableRef.value?.clearFilter()
  pagination.page = 1
  fetchData()
}

const isActive = (row: PostTrainingRun) => row.status === 'Pending' || row.status === 'Running'

/** Refreshes every 10 seconds while the page shows unfinished runs and the tab is visible. */
const visibility = useDocumentVisibility()
const autoRefresh = computed(() => rows.value.some(isActive))
useIntervalFn(() => {
  if (autoRefresh.value && visibility.value === 'visible' && !loading.value) fetchData(true)
}, AUTO_REFRESH_MS)

const summaryOf = (row: PostTrainingRun) => parseJSON<MetricSummary>(row.metricSummary)

/** Compare opens once its page is routed. */
const canCompare = router.hasRoute('PostTrainingCompare')
const MAX_COMPARE = 5
const selected = ref<PostTrainingRun[]>([])

/** Why the selection can't be compared, or an empty string. */
const compareTip = computed(() => {
  const types = new Set(selected.value.map((r) => r.trainType))
  if (selected.value.length < 2 || selected.value.length > MAX_COMPARE || types.size > 1)
    return 'Select 2–5 runs of the same type to compare.'
  return ''
})

/** Runs stay selectable until five are selected; experiment headers never are. */
const selectable = (row: PostTrainingRun | ExperimentGroup) =>
  !isGroup(row) &&
  (selected.value.length < MAX_COMPARE || selected.value.some((r) => r.runId === row.runId))

/** The header row of an A/B experiment's runs on this page. */
type ExperimentGroup = {
  runId: string
  isGroup: true
  experimentId: string
  name: string
  children: PostTrainingRun[]
}
const isGroup = (row: PostTrainingRun | ExperimentGroup): row is ExperimentGroup => 'isGroup' in row

const experimentNames = reactive(new Map<string, string>())

/** The page's runs with the runs of each A/B experiment folded under one header, where the first of them is. */
const tableRows = computed(() => {
  const out: (PostTrainingRun | ExperimentGroup)[] = []
  const groups = new Map<string, ExperimentGroup>()
  for (const r of rows.value) {
    if (!r.experimentId) {
      out.push(r)
      continue
    }
    let g = groups.get(r.experimentId)
    if (!g) {
      g = {
        runId: `experiment:${r.experimentId}`,
        isGroup: true,
        experimentId: r.experimentId,
        name: experimentNames.get(r.experimentId) ?? r.experimentId,
        children: [],
      }
      groups.set(r.experimentId, g)
      out.push(g)
    }
    g.children.push(r)
  }
  return out
})

/** The run count and status counts of an experiment's runs on this page. */
const groupSummary = (g: ExperimentGroup) => {
  const counts = new Map<string, number>()
  for (const r of g.children) counts.set(r.status, (counts.get(r.status) ?? 0) + 1)
  const parts = [...counts].map(([s, n]) => `${n} ${s}`)
  return [`${g.children.length} runs`, ...parts].join(' · ')
}

/** Reads the names of the experiments on this page that are not known yet. */
const loadExperimentNames = async () => {
  const ids = [
    ...new Set(rows.value.map((r) => r.experimentId).filter((id): id is string => !!id)),
  ].filter((id) => !experimentNames.has(id))
  if (!ids.length || !wsStore.currentWorkspaceId) return
  try {
    const res = (await listPostTrainingExperiments(
      { workspace: wsStore.currentWorkspaceId, experimentIds: ids.join(','), limit: ids.length },
      true,
    )) as unknown as ExperimentListResp
    for (const e of res.items ?? []) experimentNames.set(e.experimentId, e.displayName)
  } catch {
    /* the headers show the experiment ids */
  }
}
watch(rows, loadExperimentNames)

const openExperiment = (experimentId: string) => {
  router.push({ path: '/post-training/experiments/detail', query: { experimentId } })
}

const goCompare = () => {
  router.push({
    path: '/post-training/compare',
    query: { runIds: selected.value.map((r) => r.runId).join(',') },
  })
}

const formatValue = (v: number) => (Math.abs(v) >= 100 ? v.toFixed(1) : v.toPrecision(4))
const methodLabel = (m: string) => (m === 'lora' ? 'LoRA' : 'Full')
const shortTime = (t?: string) => (t ? dayjs(t).format('MM-DD HH:mm') : '—')

/**
 * The second line of the status: checkpoints left a week after the end, why a run waits, failed, was stopped or
 * looks stalled.
 */
const statusNote = (row: PostTrainingRun): { text: string; cls: string } => {
  const hint = cleanupHintBytes(row, now.value.getTime())
  if (hint > 0) {
    return {
      text: `Clean up checkpoints · ${formatBytes(hint, 1)}`,
      cls: 'text-[var(--safe-warning)]',
    }
  }
  if (row.cleanupRequest)
    return { text: 'Cleaning up checkpoints…', cls: 'text-[var(--safe-muted)]' }
  if (row.status === 'Failed')
    return { text: (row.message ?? '').split('\n')[0], cls: 'text-[var(--el-color-danger)]' }
  const stall = stallNote(row, now.value.getTime())
  if (stall) return { text: stall, cls: 'text-[var(--safe-warning)]' }
  if (row.status === 'Pending' || (row.status === 'Running' && row.queuePosition > 0)) {
    const position = row.queuePosition > 0 ? ` · #${row.queuePosition} in queue` : ''
    return {
      text: `${row.message || 'Waiting to be scheduled'}${position}`,
      cls: 'text-[var(--safe-muted)]',
    }
  }
  if (row.status === 'Stopped') return { text: row.message ?? '', cls: 'text-[var(--safe-muted)]' }
  return { text: '', cls: '' }
}

const evaluationText = (row: PostTrainingRun) => {
  if (!row.evalEnabled) return 'Not enabled'
  if (row.stage === 'eval') return 'Evaluating…'
  return '—'
}

/** Training time from the start of the workload to its end, or to now while it runs. */
const durationOf = (row: PostTrainingRun) => {
  const start = timeOf(row.startTime)
  if (!start || row.status === 'Pending') return '—'
  const end = timeOf(row.endTime)
  const endMs = end ? dayjs(end).valueOf() : now.value.getTime()
  return formatDuration((endMs - dayjs(start).valueOf()) / 1000)
}

const goDetail = (row: Pick<PostTrainingRun, 'runId'>) => {
  router.push({ path: '/post-training/detail', query: { runId: row.runId } })
}

const handleStop = async (row: PostTrainingRun) => {
  try {
    await ElMessageBox.confirm(
      'Training stops now. Checkpoints already saved are kept, and you can copy this run to continue later.',
      `Stop run ${row.displayName}?`,
      { confirmButtonText: 'Stop', cancelButtonText: 'Cancel', type: 'warning' },
    )
  } catch {
    return
  }
  await stopPostTrainingRun(row.runId)
  ElMessage.success('Run stopped')
  fetchData()
}

const deleteDialog = reactive({
  visible: false,
  loading: false,
  deleteModel: false,
  run: null as PostTrainingRun | null,
})

const openDelete = (row: PostTrainingRun) => {
  Object.assign(deleteDialog, { visible: true, loading: false, deleteModel: false, run: row })
}

const confirmDelete = async () => {
  if (!deleteDialog.run) return
  deleteDialog.loading = true
  try {
    await deletePostTrainingRun(deleteDialog.run.runId, deleteDialog.deleteModel)
    ElMessage.success('Run deleted')
    deleteDialog.visible = false
    fetchData()
  } finally {
    deleteDialog.loading = false
  }
}

const tableHeight = computed(() => `calc(100vh / var(--zoom) - 245px)`)

watch(
  () => wsStore.currentWorkspaceId,
  () => {
    knownBaseModels.clear()
    pagination.page = 1
    fetchAvailability()
    fetchData()
  },
)

onMounted(() => {
  readQuery()
  fetchAvailability()
  fetchData()
})

onBeforeUnmount(() => {
  deleteDialog.visible = false
})
</script>

<style scoped>
.pt-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: 4px;
  border-radius: 50%;
  background: currentColor;
  animation: pt-dot-breathe 1.6s ease-in-out infinite;
}
@keyframes pt-dot-breathe {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.35;
  }
}
</style>

<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <el-text class="block textx-18 font-500" tag="b">Post Training</el-text>

  <!-- Post Training is turned off on this cluster: its API answers 404. -->
  <el-empty v-if="notEnabled" class="mt-10" description=" ">
    <template #description>
      <div class="text-[16px] font-500">Post Training is not enabled on this cluster</div>
      <div class="mt-2 text-[13px] text-[var(--safe-muted)] max-w-[560px]">
        The cluster admin hasn't turned Post Training on (post_training.enabled is off), so its API
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
        <el-segmented v-model="scope" :options="['All', 'My Runs']" @change="onSearch(true)" />
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
        class="pt-run-table"
        :data="rows"
        :height="tableHeight"
        size="large"
        row-key="runId"
        v-loading="loading"
        :element-loading-text="$loadingText"
        @filter-change="handleFilterChange"
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
              Pick a base model in Model Square and choose Train with Post Training.
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

        <el-table-column label="Run Name / ID" min-width="270" fixed="left">
          <template #default="{ row }">
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
              </div>
              <div class="flex flex-wrap items-center gap-1">
                <span class="text-[12px] text-[var(--safe-muted)] whitespace-nowrap">{{
                  row.runId
                }}</span>
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
        </el-table-column>

        <el-table-column
          label="Type / Method"
          width="150"
          column-key="type"
          :filters="typeFilters"
          :filter-multiple="false"
          :filtered-value="typeFilterValue"
          filter-placement="bottom-start"
          :filter-method="passAll"
        >
          <template #default="{ row }">
            <div class="flex items-center gap-1">
              <el-tag
                size="small"
                :type="row.trainType === 'sft' ? 'success' : 'warning'"
                effect="plain"
              >
                {{ row.trainType?.toUpperCase() }}
              </el-tag>
              <el-tag size="small" type="info" effect="plain">{{ methodLabel(row.method) }}</el-tag>
            </div>
            <div v-if="row.algorithm" class="text-[12px] text-[var(--safe-muted)]">
              {{ row.algorithm.toUpperCase() }}
            </div>
          </template>
        </el-table-column>

        <el-table-column label="Progress" width="150">
          <template #default="{ row }">
            <StageBar :run="row" :step="summaryOf(row)?.main?.step"
          /></template>
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
          <template #default="{ row }">
            <el-tag
              size="small"
              :type="statusTagType(row.status)"
              :effect="isDark ? 'plain' : 'light'"
            >
              <span v-if="row.status === 'Running'" class="pt-dot" />{{ row.status }}
            </el-tag>
            <el-tooltip v-if="statusNote(row).text" :content="statusNote(row).text" placement="top">
              <div class="text-[12px] truncate" :class="statusNote(row).cls">
                {{ statusNote(row).text }}
              </div>
            </el-tooltip>
          </template>
        </el-table-column>

        <el-table-column label="Key Metric" width="150">
          <template #default="{ row }">
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
          </template>
        </el-table-column>

        <el-table-column label="Base Model" width="130" show-overflow-tooltip>
          <template #default="{ row }">
            <el-link
              type="primary"
              @click.stop="router.push(`/model-square/detail/${row.baseModelId}`)"
            >
              {{ row.baseModelName || row.baseModelId }}
            </el-link>
          </template>
        </el-table-column>

        <el-table-column label="Dataset" width="140" show-overflow-tooltip>
          <template #default="{ row }">{{ row.datasetName || row.datasetId || '—' }}</template>
        </el-table-column>

        <el-table-column label="Output" width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <el-link
              v-if="row.outputModelId"
              type="primary"
              @click.stop="router.push(`/model-square/detail/${row.outputModelId}`)"
            >
              {{ row.outputModelId }}
            </el-link>
            <span v-else>—</span>
          </template>
        </el-table-column>

        <el-table-column label="Evaluation" width="180">
          <template #default="{ row }">{{ evaluationText(row) }}</template>
        </el-table-column>

        <el-table-column label="Created" width="110">
          <template #default="{ row }">
            <el-tooltip :content="formatTimeStr(timeOf(row.createdAt))" placement="top">
              <span>{{ shortTime(timeOf(row.createdAt)) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>

        <el-table-column label="Duration" width="90">
          <template #default="{ row }">{{ durationOf(row) }}</template>
        </el-table-column>

        <el-table-column label="Owner" width="90" show-overflow-tooltip>
          <template #default="{ row }">{{ row.userName || row.userId }}</template>
        </el-table-column>

        <el-table-column label="Actions" width="140" fixed="right">
          <template #default="{ row }"
            ><div class="flex items-center gap-2">
              <el-tooltip
                v-if="isActive(row)"
                :content="actionTip('Stop', isReadonly ? readOnlyTip : '')"
                placement="top"
              >
                <span>
                  <el-button
                    circle
                    class="btn-danger-plain"
                    :icon="Close"
                    :disabled="isReadonly"
                    @click.stop="handleStop(row)"
                  />
                </span>
              </el-tooltip>
              <el-tooltip
                v-if="canCreate"
                :content="actionTip('Clone', createBlocked)"
                placement="top"
              >
                <span>
                  <el-button
                    circle
                    class="btn-success-plain"
                    :icon="DocumentCopy"
                    :disabled="!!createBlocked"
                    @click.stop="
                      router.push({ path: '/post-training/create', query: { fromRun: row.runId } })
                    "
                  />
                </span>
              </el-tooltip>
              <el-tooltip
                :content="actionTip('Delete', isReadonly ? readOnlyTip : '')"
                placement="top"
              >
                <span>
                  <el-button
                    circle
                    class="btn-danger-plain"
                    :icon="Delete"
                    :disabled="isReadonly"
                    @click.stop="openDelete(row)"
                  />
                </span>
              </el-tooltip></div
          ></template>
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
        Its record, curves and samples are removed. The run directory, with its checkpoints, stays
        on the workspace storage.
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

    <router-view />
  </template>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, Delete, DocumentCopy, Plus, Refresh } from '@element-plus/icons-vue'
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
  mine: false,
})
// The All and My Runs switch over filters.mine.
const scope = computed({
  get: () => (filters.mine ? 'My Runs' : 'All'),
  set: (v: string) => (filters.mine = v === 'My Runs'),
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
      filters.mine
    ),
)
const filterSummary = computed(() => {
  const parts: string[] = []
  if (filters.type) parts.push(`Type: ${filters.type.toUpperCase()}`)
  if (filters.method) parts.push(`Method: ${methodLabel(filters.method)}`)
  if (filters.status.length) parts.push(`Status: ${filters.status.join(', ')}`)
  if (filters.q) parts.push(`Search: ${filters.q}`)
  if (filters.baseModel) parts.push(`Base Model: ${filters.baseModel}`)
  if (filters.mine) parts.push('My Runs')
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
  filters.mine = q.mine === '1'
  pagination.page = Number(q.page || 1)
  pagination.pageSize = Number(q.pageSize || pagination.pageSize)
}

/** Writes the filters and page into the URL, so a refresh or a shared link keeps them; the create drawer keeps its own. */
const writeQuery = () => {
  if (router.currentRoute.value.name !== 'PostTraining') return
  const query: Record<string, string> = {}
  if (filters.type) query.type = filters.type
  if (filters.method) query.method = filters.method
  if (filters.status.length) query.status = filters.status.join(',')
  if (filters.q) query.q = filters.q
  if (filters.baseModel) query.baseModel = filters.baseModel
  if (filters.mine) query.mine = '1'
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

/** Loads one page of runs; a quiet refresh keeps the current rows when it fails. */
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
  if (filters.mine) params.owner = userStore.userId
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

/** Reads whether Post Training is on, then lists the runs where it is. */
const loadPage = async () => {
  // The table shows as loading, not as empty, while it waits to learn whether Post Training is on.
  loading.value = !!wsStore.currentWorkspaceId
  await fetchAvailability()
  if (notEnabled.value) loading.value = false
  else fetchData()
}

const onSearch = (resetPage: boolean) => {
  if (resetPage) pagination.page = 1
  fetchData()
}

const resetFilters = () => {
  Object.assign(filters, { type: '', method: '', status: [], q: '', baseModel: '', mine: false })
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

// Parsed once per row; a load replaces the rows.
const summaries = new WeakMap<PostTrainingRun, MetricSummary | undefined>()
const summaryOf = (row: PostTrainingRun) => {
  if (!summaries.has(row)) summaries.set(row, parseJSON<MetricSummary>(row.metricSummary))
  return summaries.get(row)
}

const formatValue = (v: number) => (Math.abs(v) >= 100 ? v.toFixed(1) : v.toPrecision(4))
const methodLabel = (m: string) => (m === 'lora' ? 'LoRA' : 'Full')
const shortTime = (t?: string) => (t ? dayjs(t).format('MM-DD HH:mm') : '—')

/**
 * The second line of the status: checkpoints left a week after the end, why a run waits, failed, was stopped or
 * looks stalled.
 */
const noteOf = (row: PostTrainingRun): { text: string; cls: string } => {
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

// The notes of the current second, each made once however often its row shows it.
const notesNow = computed(() => {
  void now.value
  return new WeakMap<PostTrainingRun, { text: string; cls: string }>()
})
/** The status note of a row as of this second. */
const statusNote = (row: PostTrainingRun) => {
  const notes = notesNow.value
  if (!notes.has(row)) notes.set(row, noteOf(row))
  return notes.get(row)!
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

/** The tooltip of a row action: its name, and why it is unavailable when it is. */
const actionTip = (label: string, reason?: string) => (reason ? `${label} · ${reason}` : label)

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
    loadPage()
  },
)

onMounted(async () => {
  readQuery()
  await loadPage()
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

<style>
/* Fixed columns cover the cells scrolled under them, so unlike the theme's translucent rows they are opaque. */
.pt-run-table .el-table__row > td.el-table-fixed-column--left,
.pt-run-table .el-table__row > td.el-table-fixed-column--right,
.pt-run-table th.el-table-fixed-column--left,
.pt-run-table th.el-table-fixed-column--right {
  background: rgb(255, 255, 255) !important;
}
.dark .pt-run-table .el-table__row > td.el-table-fixed-column--left,
.dark .pt-run-table .el-table__row > td.el-table-fixed-column--right {
  background: rgb(22, 26, 32) !important;
}
.dark .pt-run-table .el-table__row:hover > td.el-table-fixed-column--left,
.dark .pt-run-table .el-table__row:hover > td.el-table-fixed-column--right,
.dark .pt-run-table th.el-table-fixed-column--left,
.dark .pt-run-table th.el-table-fixed-column--right {
  background: rgb(28, 34, 43) !important;
}
</style>

<template>
  <div v-loading="loading && !resp">
    <el-alert
      v-if="resp?.capped"
      type="info"
      :closable="false"
      class="mb-3"
      title="Sample limit reached. Later samples are only in the run folder."
    />
    <div class="flex items-center gap-3 mb-3 flex-wrap">
      <el-segmented v-model="kind" :options="kindOptions">
        <template #default="{ item }">
          <el-tooltip :content="evalOff" :disabled="item.value !== 'eval' || !evalOff">
            <span>{{ item.label }}</span>
          </el-tooltip>
        </template>
      </el-segmented>
      <el-select
        v-model="step"
        :disabled="!stepNumbers.length"
        :placeholder="unit"
        style="width: 150px"
        @change="load()"
      >
        <el-option v-for="s in stepNumbers" :key="s" :label="`${unit} ${s}`" :value="s" />
      </el-select>
      <el-select
        v-if="kind === 'eval' && datasets.length"
        v-model="dataset"
        style="width: 180px"
        @change="load()"
      >
        <el-option v-for="d in datasets" :key="d" :label="d || 'default'" :value="d" />
      </el-select>
      <el-tooltip :content="firstTip" :disabled="!!previous || !stepNumbers.length">
        <span>
          <el-button size="small" :disabled="!previous" @click="go(previous)">Previous</el-button>
        </span>
      </el-tooltip>
      <el-tooltip content="This is the latest one." :disabled="!!next || !stepNumbers.length">
        <span>
          <el-button size="small" :disabled="!next" @click="go(next)">Next</el-button>
        </span>
      </el-tooltip>
      <span v-if="resp?.samples.length" class="text-[13px] text-[var(--safe-muted)]">
        {{ summary }}
      </span>
    </div>

    <el-empty v-if="resp && !resp.samples.length" :image-size="60">
      <template #description>
        <div>No samples yet</div>
        <div class="text-[12px] text-[var(--safe-muted)]">
          Samples appear after the first {{ kind === 'eval' ? 'evaluation' : 'rollout' }} finishes.
        </div>
      </template>
    </el-empty>

    <el-collapse v-else-if="kind === 'rollout'" v-model="open">
      <el-collapse-item v-for="g in groups" :key="g.group" :name="g.group">
        <template #title>
          <span class="font-500 mr-2">Group {{ g.group }}</span>
          <span class="text-[13px] text-[var(--safe-muted)]">
            {{ g.samples.length }} responses · reward mean
            {{ g.rewardMean === null ? '—' : g.rewardMean.toFixed(3) }} · {{ g.correct }} correct ·
            {{ g.wrong }} wrong
          </span>
          <el-tag v-if="g.noSignal" size="small" type="warning" effect="plain" class="ml-2">
            No learning signal
          </el-tag>
        </template>
        <pre class="pt-sample-text">{{ g.prompt }}</pre>
        <div v-if="g.label" class="text-[13px] mb-2">Label: {{ g.label }}</div>
        <el-table :data="shown(g)" size="small" @row-click="openSample">
          <el-table-column label="#" prop="index" width="60" />
          <el-table-column label="Reward" width="90">
            <template #default="{ row }">
              <el-tooltip :content="rewardOf(row).detail" :disabled="!rewardOf(row).detail">
                <span>{{ rewardOf(row).value }}</span>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column label="Status" width="110">
            <template #default="{ row }">{{ statusLabel(row.status) }}</template>
          </el-table-column>
          <el-table-column label="Tokens" width="90">
            <template #default="{ row }">{{ row.tokens.toLocaleString() }}</template>
          </el-table-column>
          <el-table-column label="Response" min-width="300" show-overflow-tooltip>
            <template #default="{ row }">{{ lastLine(row.response) }}</template>
          </el-table-column>
        </el-table>
        <el-button
          v-if="g.samples.length > PER_GROUP && !showAll[g.group]"
          link
          type="primary"
          class="mt-1"
          @click="showAll[g.group] = true"
        >
          Show all {{ g.samples.length }} responses
        </el-button>
      </el-collapse-item>
    </el-collapse>

    <template v-else>
      <el-table :data="resp?.samples ?? []" size="small" @row-click="openSample">
        <el-table-column label="#" prop="index" width="60" />
        <el-table-column label="Prompt" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">{{ lastLine(row.prompt) }}</template>
        </el-table-column>
        <el-table-column label="Label" width="100" show-overflow-tooltip>
          <template #default="{ row }">{{ row.label.Valid ? row.label.String : '—' }}</template>
        </el-table-column>
        <el-table-column label="Reward" width="90">
          <template #default="{ row }">
            <el-tooltip :content="rewardOf(row).detail" :disabled="!rewardOf(row).detail">
              <span>{{ rewardOf(row).value }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="Status" width="110">
          <template #default="{ row }">{{ statusLabel(row.status) }}</template>
        </el-table-column>
        <el-table-column label="Response" min-width="300" show-overflow-tooltip>
          <template #default="{ row }">{{ lastLine(row.response) }}</template>
        </el-table-column>
      </el-table>
      <div v-if="resp?.samples.length" class="text-[12px] text-[var(--safe-muted)] mt-2">
        The same {{ resp.samples.length }} problems at every evaluation, so rows line up across
        rollouts.
      </div>
    </template>

    <el-dialog v-model="dialog" :title="dialogTitle" width="760px" append-to-body>
      <template v-if="current">
        <div class="flex items-center gap-2 mb-2">
          <el-tag v-if="truncated" size="small" type="warning" effect="plain">Truncated</el-tag>
          <el-button link type="primary" @click="copyText(current.response)">Copy</el-button>
        </div>
        <template v-if="truncated">
          <pre class="pt-sample-text">{{ truncated.head }}</pre>
          <div class="pt-sample-omitted">
            {{ truncated.omitted.toLocaleString() }} characters omitted.
            <template v-if="current.file">
              The full response is in {{ relativeFile(current.file) }} in the run folder.
            </template>
          </div>
          <pre class="pt-sample-text">{{ truncated.tail }}</pre>
        </template>
        <pre v-else class="pt-sample-text">{{ current.response }}</pre>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'
import { useDocumentVisibility, useIntervalFn } from '@vueuse/core'
import {
  getPostTrainingSamples,
  groupSamples,
  lastLine,
  rewardOf,
  splitTruncated,
  type RunSample,
  type SampleGroup,
  type SampleKind,
  type SamplesResp,
} from '@/services/postTraining'
import { copyText } from '@/utils/index'

const props = defineProps<{
  runId: string
  active: boolean
  /** Why the run has no evaluation samples, such as no evaluation set; empty when it has them. */
  evalOff?: string
}>()
const emit = defineEmits<{ (e: 'count', n: number): void }>()

const PER_GROUP = 3
const visibility = useDocumentVisibility()

const kind = ref<SampleKind>('rollout')
const step = ref<number>()
const dataset = ref('')
const resp = ref<SamplesResp>()
const loading = ref(false)
const open = ref<number[]>([])
const showAll = reactive<Record<number, boolean>>({})
const dialog = ref(false)
const current = ref<RunSample>()
let controller: AbortController | undefined

const kindOptions = computed(() => [
  { label: 'Rollout', value: 'rollout' },
  { label: 'Evaluation', value: 'eval', disabled: !!props.evalOff },
])
const unit = computed(() => (kind.value === 'eval' ? 'Evaluation' : 'Rollout'))
const firstTip = computed(() =>
  kind.value === 'eval' ? 'This is the first evaluation.' : 'This is the first rollout.',
)

/** The steps that have samples, latest first. */
const stepNumbers = computed(() => [...new Set((resp.value?.steps ?? []).map((s) => s.step))])
const datasets = computed(() =>
  (resp.value?.steps ?? []).filter((s) => s.step === step.value).map((s) => s.dataset),
)
const previous = computed(() => stepNumbers.value[stepNumbers.value.indexOf(step.value!) + 1])
const next = computed(() => {
  const i = stepNumbers.value.indexOf(step.value!)
  return i > 0 ? stepNumbers.value[i - 1] : undefined
})

const groups = computed<SampleGroup[]>(() => groupSamples(resp.value?.samples ?? []))
const summary = computed(() => {
  const samples = resp.value?.samples ?? []
  if (kind.value === 'eval') {
    const correct = samples.filter((s) => s.reward.Valid && s.reward.Float64 > 0).length
    return `${samples.length} samples · ${correct} correct`
  }
  return `${groups.value.length} groups · ${samples.length} responses`
})

const truncated = computed(() => (current.value ? splitTruncated(current.value.response) : null))
const dialogTitle = computed(() =>
  current.value
    ? kind.value === 'eval'
      ? `${current.value.dataset || 'Evaluation'} · sample ${current.value.index}`
      : `Group ${current.value.group} · response ${current.value.index}`
    : '',
)

/** The responses of a group shown before Show all. */
const shown = (g: SampleGroup) => (showAll[g.group] ? g.samples : g.samples.slice(0, PER_GROUP))

/** The sample status as a label, such as Completed. */
const statusLabel = (s: string) => (s ? s[0].toUpperCase() + s.slice(1) : '—')

/** The run folder path of a samples file, from samples/ on. */
const relativeFile = (file: string) => file.slice(Math.max(0, file.lastIndexOf('/samples/') + 1))

/** Opens a sample's whole response. */
const openSample = (row: RunSample) => {
  current.value = row
  dialog.value = true
}

/** Reads the selected step, cancelling a read still under way; a refresh keeps the step and its rows. */
const load = async (refresh = false) => {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  try {
    const params = { kind: kind.value, step: step.value, dataset: dataset.value || undefined }
    const data = (await getPostTrainingSamples(
      props.runId,
      params,
      controller.signal,
      true,
    )) as unknown as SamplesResp
    resp.value = data
    step.value = data.step ?? undefined
    dataset.value = data.dataset
    if (kind.value === 'rollout') emit('count', new Set(data.steps.map((s) => s.step)).size)
    if (!refresh) {
      open.value = groups.value.length ? [groups.value[0].group] : []
      for (const g of Object.keys(showAll)) delete showAll[Number(g)]
    }
  } catch (err) {
    if (axios.isCancel(err)) return
    if (!refresh) ElMessage.error('Failed to load the samples.')
  } finally {
    loading.value = false
  }
}

/** Moves to another step. */
const go = (s?: number) => {
  if (s === undefined) return
  step.value = s
  load()
}

watch(kind, () => {
  step.value = undefined
  dataset.value = ''
  load()
})
watch(
  () => props.runId,
  () => load(),
  { immediate: true },
)

useIntervalFn(() => {
  if (props.active && visibility.value === 'visible' && !loading.value) load(true)
}, 10000)
</script>

<style scoped>
.pt-sample-text {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  margin: 0 0 8px;
  max-height: 480px;
  overflow: auto;
}
.pt-sample-omitted {
  font-size: 12px;
  color: var(--safe-muted);
  border-top: 1px dashed var(--el-border-color);
  border-bottom: 1px dashed var(--el-border-color);
  padding: 4px 0;
  margin: 4px 0 8px;
}
</style>

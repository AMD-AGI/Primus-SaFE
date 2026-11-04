<template>
  <div class="flex items-start justify-between gap-4">
    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-2">
        <el-button link :icon="ArrowLeft" @click="router.push('/post-training')" />
        <span class="text-[20px] font-600 truncate">{{ run.displayName }}</span>
        <el-tag size="small" effect="plain">{{ run.trainType.toUpperCase() }}</el-tag>
        <el-tag size="small" type="info" effect="plain">{{
          run.method === 'lora' ? 'LoRA' : 'Full'
        }}</el-tag>
        <el-tag size="small" :type="statusTagType(run.status)">{{ run.status }}</el-tag>
        <el-tag size="small" type="info" effect="light" round>Imported from Spur</el-tag>
      </div>
      <div class="pt-meta">
        <span>
          Run ID {{ run.runId }}
          <el-icon class="cursor-pointer align-middle" @click="copyText(run.runId)">
            <CopyDocument />
          </el-icon>
        </span>
        <span v-if="info?.slurmJobId">Slurm job {{ info.slurmJobId }}</span>
        <span>Owner {{ run.userName || run.userId }}</span>
        <span>Ran on Spur {{ ranOnSpur }}</span>
        <span>Duration {{ duration }}</span>
        <el-tooltip
          v-if="info"
          :content="`Imported by ${info.importedBy || run.userName}`"
          placement="top"
        >
          <span>Imported {{ importedText }}</span>
        </el-tooltip>
      </div>
    </div>
    <div class="flex max-w-[58%] flex-wrap items-center justify-end gap-2">
      <el-button round @click="copyText(sourcePath)">Copy Spur Path</el-button>
      <el-tooltip v-for="label in RECIPE_ACTIONS" :key="label" :content="NO_RECIPE" placement="top">
        <span
          ><el-button round disabled>{{ label }}</el-button></span
        >
      </el-tooltip>
      <el-tooltip :content="NO_MODEL" placement="top">
        <span><el-button round disabled>Evaluate</el-button></span>
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
            Delete Record
          </el-button>
        </span>
      </el-tooltip>
    </div>
  </div>

  <el-alert v-if="banner" :type="banner.type" :closable="false" show-icon class="mt-3">
    <template #title>{{ banner.text }}</template>
    <div v-if="banner.evidence" class="mt-1 font-mono text-[12px]">{{ banner.evidence }}</div>
    <div v-if="banner.type === 'error'" class="mt-2 flex gap-3">
      <el-button link type="primary" @click="emit('tab', 'gates')">Open Gates</el-button>
      <el-button v-if="banner.logPath" link type="primary" @click="copyText(banner.logPath)">
        Copy log path
      </el-button>
    </div>
  </el-alert>

  <el-card v-if="info" class="safe-card mt-3" shadow="never">
    <div class="mb-3 text-[13px]">
      <span class="font-500">Stages</span>
      <span class="ml-2 text-[var(--safe-muted)]">
        From node-*.log on {{ info.summary.nodes.length }}
        {{ info.summary.nodes.length === 1 ? 'node' : 'nodes' }}
      </span>
    </div>
    <div class="pt-import-stages">
      <el-tooltip
        v-for="st in timeline"
        :key="st.stage"
        :disabled="!st.gates.length"
        placement="top"
      >
        <template #content>
          <div v-for="g in st.gates" :key="g">{{ g }}</div>
        </template>
        <div class="pt-import-stage" :class="`is-${st.state}`">
          <div class="text-[13px] font-500">{{ st.label }}</div>
          <div
            class="text-[12px]"
            :class="st.state === 'failed' ? 'text-[var(--el-color-danger)]' : ''"
          >
            {{ st.text }}
          </div>
          <div v-if="st.sub" class="text-[12px] text-[var(--safe-muted)]">{{ st.sub }}</div>
        </div>
      </el-tooltip>
    </div>
  </el-card>

  <el-dialog
    v-model="deleteDialog.visible"
    :title="`Delete the imported record ${run.displayName}?`"
    width="480px"
  >
    Removes the record and its parsed metrics from SaFE. The run directory on Spur is not touched:
    <div class="my-2 break-all font-mono text-[12px]">{{ sourcePath }}</div>
    You can import it again later.
    <template #footer>
      <el-button @click="deleteDialog.visible = false">Cancel</el-button>
      <el-button type="danger" :loading="deleteDialog.loading" @click="confirmDelete">
        Delete Record
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, CopyDocument } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { useUserStore } from '@/stores/user'
import { copyText } from '@/utils'
import {
  deletePostTrainingRun,
  formatDuration,
  importBanner,
  importTimeline,
  statusTagType,
  timeOf,
  type RunDetail,
  type RunImport,
} from '@/services/postTraining'

const props = defineProps<{
  run: RunDetail
  info?: RunImport
  /** The last step or rollout the main metric logged. */
  step?: number
}>()
const emit = defineEmits<{ tab: [name: string] }>()

const RECIPE_ACTIONS = ['Copy as New Run', 'Save as Template', 'A/B from This Run']
const NO_RECIPE = 'Imported runs are read-only: SaFE has no recipe for them.'
const NO_MODEL = 'No model to evaluate: nothing from an imported run is registered in Model Square.'
const readOnlyTip = 'Read-only access'

const router = useRouter()
const userStore = useUserStore()
const isReadonly = computed(() => userStore.isReadonlyManager)

const sourcePath = computed(() => props.info?.sourcePath || props.run.outputPath || '')
const banner = computed(() => (props.info ? importBanner(props.info.summary) : undefined))
const timeline = computed(() =>
  props.info ? importTimeline(props.info.summary, props.run.trainType, props.step) : [],
)

/** When the run's gates began and ended on Spur; the end shows only its time on the same day. */
const ranOnSpur = computed(() => {
  const start = timeOf(props.run.startTime)
  const end = timeOf(props.run.endTime)
  if (!start) return '—'
  const from = dayjs(start).format('MM-DD HH:mm')
  if (!end) return from
  return `${from} – ${dayjs(end).format(dayjs(end).isSame(dayjs(start), 'day') ? 'HH:mm' : 'MM-DD HH:mm')}`
})
const duration = computed(() => {
  const start = timeOf(props.run.startTime)
  const end = timeOf(props.run.endTime)
  return start && end ? formatDuration((dayjs(end).valueOf() - dayjs(start).valueOf()) / 1000) : '—'
})
const importedText = computed(() => {
  const i = props.info
  if (!i) return ''
  const at = i.importedAt ? dayjs(i.importedAt).format('MM-DD HH:mm') : ''
  return [at, i.via === 'scan' ? 'mounted path' : 'uploaded from Spur'].filter(Boolean).join(' · ')
})

const deleteDialog = reactive({ visible: false, loading: false })

/** Deletes SaFE's record of the run and returns to the list. */
const confirmDelete = async () => {
  deleteDialog.loading = true
  try {
    await deletePostTrainingRun(props.run.runId)
    ElMessage.success('Record deleted')
    deleteDialog.visible = false
    router.push('/post-training')
  } finally {
    deleteDialog.loading = false
  }
}
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
.pt-import-stages {
  display: flex;
  gap: 8px;
}
.pt-import-stage {
  flex: 1;
  min-width: 0;
  padding: 8px 10px 6px;
  border-top: 4px solid var(--safe-border);
  border-radius: 4px;
  background: var(--el-fill-color-lighter);
}
.pt-import-stage.is-done {
  border-top-color: color-mix(in srgb, var(--safe-primary) 62%, transparent);
}
.pt-import-stage.is-failed {
  border-top-color: var(--el-color-danger);
}
.pt-import-stage.is-stopped {
  border-top-color: var(--safe-muted);
}
</style>

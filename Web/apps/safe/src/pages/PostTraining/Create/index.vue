<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <el-drawer
    v-model="visible"
    title="New Post Training Run"
    size="820px"
    direction="rtl"
    append-to-body
    destroy-on-close
    :close-on-click-modal="false"
    :before-close="confirmClose"
    class="pt-create-drawer"
    @closed="leave"
  >
    <div class="drawer-body">
      <div class="mb-3 text-[12px] text-[var(--safe-muted)]">Workspace {{ workspaceName }}</div>

      <el-alert
        v-if="w.models.value?.pausedReason"
        class="mb-3"
        type="warning"
        :closable="false"
        show-icon
        :title="`New runs are paused on this cluster. ${w.models.value.pausedReason}`"
      />
      <el-alert
        v-for="e in loadErrors"
        :key="e"
        class="mb-3"
        type="error"
        :closable="false"
        show-icon
        :title="e"
      />
      <el-alert v-if="prefill" class="mb-3" type="info" :closable="false" show-icon>
        <template #title>
          {{ prefill }}
          <el-link type="primary" class="ml-2" @click="undo">Undo</el-link>
        </template>
      </el-alert>
      <PreflightPanel @goto="scrollTo" />

      <div v-if="ready" :key="formKey">
        <SectionCard
          v-for="(section, i) in sections"
          :id="sectionId(i + 1)"
          :key="section.title"
          :title="section.title"
          :subtitle="section.subtitle"
        >
          <template v-if="errorCount(i + 1)" #actions>
            <el-tag size="small" type="danger">{{ errorText(i + 1) }}</el-tag>
          </template>
          <component
            :is="section.component"
            @changed="onChanged"
            @preset="applyPreset"
            @confirmed="(all: boolean) => (warningsConfirmed = all)"
          />
        </SectionCard>
      </div>
      <div v-else v-loading="true" class="pt-loading" />

      <div v-if="submitError" class="mb-4 text-[13px] text-[var(--el-color-danger)]">
        <div>{{ submitError }}</div>
        <div v-for="(d, i) in submitDiagnostics" :key="i">· {{ d.message }}</div>
      </div>
    </div>

    <template #footer>
      <div class="drawer-footer">
        <el-button @click="close">Cancel</el-button>
        <el-tooltip :content="submitBlocked" :disabled="!submitBlocked" placement="top">
          <span class="ml-3">
            <el-button
              type="primary"
              :disabled="!!submitBlocked"
              :loading="submitting"
              @click="submit"
            >
              Submit
            </el-button>
          </span>
        </el-tooltip>
      </div>
    </template>
  </el-drawer>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import SectionCard from '@/components/Base/SectionCard.vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import {
  getPostTrainingRun,
  submitPostTrainingRun,
  type Diagnostic,
  type RunDetail,
  type SubmitResp,
} from '@/services/postTraining'
import PreflightPanel from './PreflightPanel.vue'
import StepTask from './StepTask.vue'
import StepData from './StepData.vue'
import StepResources from './StepResources.vue'
import StepTraining from './StepTraining.vue'
import StepOutputs from './StepOutputs.vue'
import StepReview from './StepReview.vue'
import { STEPS, createWizard, newRequest, provideWizard, requestFromRun } from './wizard'

defineOptions({ name: 'PostTrainingCreatePage' })

const route = useRoute()
const router = useRouter()
const wsStore = useWorkspaceStore()

const w = createWizard(wsStore.currentWorkspaceId ?? '', wsStore.currentNodeFlavor)
const workspaceName = computed(
  () => wsStore.items.find((i) => i.workspaceId === w.workspace)?.workspaceName || w.workspace,
)
provideWizard(w)

const stepComponents: Component[] = [
  StepTask,
  StepData,
  StepResources,
  StepTraining,
  StepOutputs,
  StepReview,
]
const steps = STEPS.slice(0, stepComponents.length)
const SUBTITLES = [
  'Run name, training type, method and base model',
  'Training data, validation and field mapping',
  'Nodes, GPUs per node and parallelism',
  "Hyperparameters, the spec's defaults unless changed",
  'Export, evaluation, scheduling and image',
  'Summary, recipe and the warnings to confirm',
]
const sections = stepComponents.map((component, i) => ({
  title: steps[i],
  subtitle: SUBTITLES[i],
  component,
}))

const errorCount = (s: number) => w.byStep.value.get(s)?.length ?? 0

/** The error tag of a step's section. */
const errorText = (s: number) => `${errorCount(s)} error${errorCount(s) === 1 ? '' : 's'}`

/** The element id of a step's section. */
const sectionId = (s: number) => `pt-section-${s}`

/** Scrolls the drawer to a step's section. */
const scrollTo = (s: number) =>
  document.getElementById(sectionId(s))?.scrollIntoView({ behavior: 'smooth', block: 'start' })

const visible = ref(true)
// The sections render once the URL's prefill is in the request.
const ready = ref(false)
// What failed while the wizard opened; the form still opens with the rest.
const loadErrors = ref<string[]>([])
// Bumped when the request is replaced as a whole, so that the sections read it again.
const formKey = ref(0)
let pristine = ''

/** Closes the drawer, asking first when the request was edited. */
const confirmClose = (done: () => void) => {
  if (!ready.value || JSON.stringify(w.form) === pristine) {
    done()
    return
  }
  ElMessageBox.confirm('All fields will be cleared.', 'Clear form & close?', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(() => done())
    .catch(() => {})
}

/** Closes the drawer from its Cancel button. */
const close = () => confirmClose(() => (visible.value = false))

/** Returns to the page that opened the drawer, or to the run list. */
const leave = () => {
  if (router.options.history.state.back) router.back()
  else router.replace('/post-training')
}

const modelName = computed(
  () =>
    w.models.value?.items.find((m) => m.modelId === w.form.modelId)?.displayName ?? w.form.modelId,
)

const userStore = useUserStore()
const warningsConfirmed = ref(false)
const submitting = ref(false)
const submitError = ref('')
const submitDiagnostics = ref<Diagnostic[]>([])

/** Why the run cannot be submitted yet, or an empty string. */
const submitBlocked = computed(() => {
  if (userStore.isReadonlyManager) return 'Read-only access'
  if (w.models.value?.pausedReason) return `New runs are paused. ${w.models.value.pausedReason}`
  if (w.previewError.value) return 'Preflight unavailable'
  if (w.errors.value.length) return 'Fix the errors in the preflight first.'
  if (!w.form.displayName.trim()) return 'Name the run.'
  if (!warningsConfirmed.value) return 'Confirm the warnings.'
  return ''
})

type SubmitFailure = {
  response?: { status?: number; data?: { errorMessage?: string; diagnostics?: Diagnostic[] } }
}

/** Submits the run and opens it; a request that no longer compiles shows the compiler's findings. */
const submit = async () => {
  submitting.value = true
  submitError.value = ''
  submitDiagnostics.value = []
  try {
    const res = (await submitPostTrainingRun(
      JSON.parse(JSON.stringify(w.form)),
    )) as unknown as SubmitResp
    ElMessage.success(`Run ${res.runId} submitted`)
    router.push(
      router.hasRoute('PostTrainingDetail')
        ? { path: '/post-training/detail', query: { runId: res.runId } }
        : { path: '/post-training' },
    )
  } catch (err) {
    const resp = (err as SubmitFailure)?.response
    submitError.value =
      resp?.status === 503
        ? `Submission is paused: ${resp.data?.errorMessage ?? ''}`
        : (resp?.data?.errorMessage ?? `Submission failed: ${String(err)}`)
    submitDiagnostics.value = resp?.data?.diagnostics ?? []
  } finally {
    submitting.value = false
  }
}

/** Reloads what depends on a changed field; a new train type drops the choices that belong to the old one. */
const onChanged = async (field: string) => {
  if (field === 'trainType') {
    w.form.datasetId = ''
    w.form.evalDatasetId = undefined
    w.form.choices = {}
    await w.loadDatasets()
    formKey.value++
  }
  await w.loadConfig()
}

/** Fills the empty choices from a preset; the values the user set are kept. */
const applyPreset = (id: string) => {
  const preset = w.config.value?.options?.presets?.find((p) => p.id === id)
  if (!preset) return
  w.form.choices = { ...preset.choices, ...w.form.choices }
  w.form.preset = id
}

const prefill = ref('')
let undoPrefill = () => {}

/** Undoes the URL's prefill; the sections read the request again. */
const undo = () => {
  undoPrefill()
  formKey.value++
}

/** Copies a run's recipe snapshot into the request, as Clone does. */
const prefillFromRun = async (runId: string) => {
  const detail = (await getPostTrainingRun(runId)) as unknown as RunDetail
  const { resources, ...copied } = requestFromRun(detail)
  Object.assign(w.form, copied, {
    workspace: w.form.workspace,
    displayName: `${detail.displayName}-clone`,
  })
  if (resources.nodes) w.form.resources.nodes = resources.nodes
  if (resources.gpusPerNode) w.form.resources.gpusPerNode = resources.gpusPerNode
  const unlisted = !!detail.image && detail.imageListed === false
  if (detail.image && !unlisted) w.form.resources.image = detail.image
  if (detail.priority < 2 || userStore.isManager || wsStore.isCurrentWorkspaceAdmin())
    w.form.resources.priority = detail.priority
  if (detail.timeout > 0) w.form.resources.timeoutSeconds = detail.timeout
  prefill.value = unlisted
    ? `Cloned from run ${detail.displayName}. Its image is no longer in the registry, so the clone uses the default image.`
    : `Cloned from run ${detail.displayName}.`
  undoPrefill = () => {
    Object.assign(w.form, newRequest(w.workspace))
    prefill.value = ''
    router.replace({ query: {} })
    w.loadNodeGpus()
    w.loadConfig()
  }
}

/** Applies the URL's prefill: fromRun wins over modelId and trainType. */
const applyQuery = async () => {
  const q = route.query
  if (q.fromRun) {
    await prefillFromRun(String(q.fromRun))
    return
  }
  if (q.trainType === 'rl' || q.trainType === 'sft') w.form.trainType = q.trainType
  if (q.modelId) {
    w.form.modelId = String(q.modelId)
    prefill.value = `Prefilled from Model Square · ${modelName.value}: base model.`
    undoPrefill = () => {
      w.form.modelId = ''
      prefill.value = ''
      w.loadConfig()
    }
  }
}

/** Runs one step of opening the wizard and shows why it failed, so that the next steps still run. */
const openStep = async (what: string, run: () => Promise<unknown>) => {
  try {
    await run()
  } catch (err) {
    const reason =
      (err as { response?: { data?: { errorMessage?: string } } })?.response?.data?.errorMessage ??
      String(err)
    loadErrors.value.push(`Couldn't ${what}: ${reason}`)
  }
}

onMounted(async () => {
  await Promise.all([
    openStep('load the models', w.loadModels),
    openStep('read the GPUs of a node', w.loadNodeGpus),
  ])
  await openStep('apply the prefill', applyQuery)
  await Promise.all([
    openStep('load the datasets', w.loadDatasets),
    openStep('load the options', w.loadConfig),
  ])
  pristine = JSON.stringify(w.form)
  ready.value = true
  w.runPreview()
})
</script>

<style>
.pt-create-drawer .el-drawer__header {
  padding: 12px 24px 4px;
  margin-bottom: 0;
}
.pt-create-drawer .el-drawer__title {
  font-size: 18px;
  font-weight: 600;
}
.pt-create-drawer .el-drawer__body {
  padding-bottom: 0;
}
</style>
<style scoped>
.drawer-body {
  overflow-y: auto;
}
.pt-loading {
  height: 240px;
}
.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding: 10px 24px;
  border-top: 1px solid var(--el-border-color-lighter);
}
</style>

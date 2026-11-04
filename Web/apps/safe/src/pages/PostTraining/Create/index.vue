<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <div class="flex items-center gap-3">
    <el-text class="block textx-18 font-500" tag="b">New Post Training Run</el-text>
    <span class="text-[12px] text-[var(--safe-muted)]">Workspace {{ w.workspace }}</span>
  </div>

  <el-alert
    v-if="w.models.value?.pausedReason"
    class="mt-3"
    type="warning"
    :closable="false"
    show-icon
    :title="`New runs are paused on this cluster. ${w.models.value.pausedReason}`"
  />
  <el-alert v-if="prefill" class="mt-3" type="info" :closable="false" show-icon>
    <template #title>
      {{ prefill }}
      <el-link type="primary" class="ml-2" @click="undoPrefill">Undo</el-link>
    </template>
  </el-alert>

  <div class="pt-wizard mt-4">
    <nav class="pt-steps">
      <div
        v-for="(title, i) in steps"
        :key="title"
        class="pt-step"
        :class="{ 'is-current': step === i + 1, 'has-error': errorCount(i + 1) > 0 }"
        @click="step = i + 1"
      >
        <span class="pt-step-index">
          <el-icon v-if="visited.has(i + 1) && step !== i + 1 && !errorCount(i + 1)"
            ><Check
          /></el-icon>
          <template v-else>{{ i + 1 }}</template>
        </span>
        <div class="min-w-0">
          <div class="font-500">{{ title }}</div>
          <div v-if="errorCount(i + 1)" class="text-[12px] text-[var(--el-color-danger)]">
            {{ errorCount(i + 1) }} error{{ errorCount(i + 1) === 1 ? '' : 's' }}
          </div>
          <div v-else class="text-[12px] text-[var(--safe-muted)] truncate">
            {{ summary(i + 1) }}
          </div>
        </div>
      </div>
    </nav>

    <section class="pt-step-body">
      <el-card class="safe-card" shadow="never">
        <div class="font-500 mb-3">{{ steps[step - 1] }}</div>
        <component
          :is="stepComponents[step - 1]"
          @changed="onChanged"
          @preset="applyPreset"
          @confirmed="(all: boolean) => (warningsConfirmed = all)"
        />
      </el-card>
      <div v-if="submitError" class="mt-3 text-[13px] text-[var(--el-color-danger)]">
        <div>{{ submitError }}</div>
        <div v-for="(d, i) in submitDiagnostics" :key="i">· {{ d.message }}</div>
      </div>
      <div class="flex items-center justify-end gap-2 mt-3">
        <el-button round @click="router.push('/post-training')">Cancel</el-button>
        <el-button round :disabled="step === 1" @click="step--">Back</el-button>
        <el-button
          v-if="step < steps.length"
          type="primary"
          round
          class="text-black"
          @click="step++"
        >
          Next: {{ steps[step] }}
        </el-button>
        <el-tooltip v-else :content="submitBlocked" :disabled="!submitBlocked" placement="top">
          <span>
            <el-button
              type="primary"
              round
              class="text-black"
              :disabled="!!submitBlocked"
              :loading="submitting"
              @click="submit"
            >
              Submit
            </el-button>
          </span>
        </el-tooltip>
      </div>
    </section>

    <aside class="pt-step-side">
      <PreflightPanel @goto="(s: number) => (step = s)" />
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Check } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import {
  getPostTrainingRun,
  submitPostTrainingRun,
  type Choices,
  type Diagnostic,
  type PostTrainingRun,
  type SubmitRequest,
  type SubmitResp,
} from '@/services/postTraining'
import PreflightPanel from './PreflightPanel.vue'
import StepTask from './StepTask.vue'
import StepData from './StepData.vue'
import StepResources from './StepResources.vue'
import StepTraining from './StepTraining.vue'
import StepOutputs from './StepOutputs.vue'
import StepReview from './StepReview.vue'
import { STEPS, createWizard, newRequest, provideWizard } from './wizard'

defineOptions({ name: 'PostTrainingCreatePage' })

const route = useRoute()
const router = useRouter()
const wsStore = useWorkspaceStore()

const w = createWizard(wsStore.currentWorkspaceId ?? '', wsStore.currentNodeFlavor)
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
const step = ref(1)
const visited = reactive(new Set<number>([1]))
watch(step, (s) => visited.add(s))

const errorCount = (s: number) => w.byStep.value.get(s)?.length ?? 0

const modelName = computed(
  () =>
    w.models.value?.items.find((m) => m.modelId === w.form.modelId)?.displayName ?? w.form.modelId,
)
const datasetName = (id?: string) =>
  w.trainDatasets.value.find((d) => d.datasetId === id)?.displayName ?? id

/** The key values chosen in a step, shown under its title. */
const summary = (s: number) => {
  const f = w.form
  switch (s) {
    case 1: {
      const algorithm =
        f.trainType === 'rl' ? ` · ${(f.choices.advantageEstimator ?? 'grpo').toUpperCase()}` : ''
      return [
        `${f.trainType.toUpperCase()} · ${f.method === 'lora' ? 'LoRA' : 'Full'}${algorithm}`,
        modelName.value,
      ]
        .filter(Boolean)
        .join(' · ')
    }
    case 2:
      return [
        datasetName(f.datasetId),
        f.evalDatasetId ? `eval: ${datasetName(f.evalDatasetId)}` : '',
      ]
        .filter(Boolean)
        .join(' · ')
    case 3: {
      const layout =
        f.trainType === 'rl' && f.choices.layout === 'separate' ? ' · disaggregated' : ''
      return `${f.resources.nodes} node${f.resources.nodes === 1 ? '' : 's'}${layout}`
    }
    case 4:
      return f.method === 'lora'
        ? `LoRA rank ${f.choices.lora?.rank ?? 16}`
        : 'Spec defaults unless changed'
    case 5:
      return [
        f.export.hf === false ? 'No export' : 'HF export',
        f.evaluation.enabled ? 'evaluation' : '',
      ]
        .filter(Boolean)
        .join(' · ')
    default:
      return f.displayName
  }
}

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

/** Copies a run's recipe snapshot into the request, as "Copy as New Run" does. */
const prefillFromRun = async (runId: string) => {
  const detail = (await getPostTrainingRun(runId)) as unknown as PostTrainingRun & {
    recipe?: {
      trainType?: 'sft' | 'rl'
      method?: string
      framework?: string
      model?: { id?: string }
      data?: { trainId?: string }
      choices?: Choices
      layout?: { nodes?: number }
      output?: { exportHf?: boolean; peftAdapter?: boolean }
      evaluation?: { enabled?: boolean; benchmarks?: string[]; keepService?: boolean }
      tracking?: SubmitRequest['tracking']
    }
  }
  const rec = detail.recipe ?? {}
  Object.assign(w.form, {
    displayName: `${detail.displayName}-copy`,
    trainType: rec.trainType ?? detail.trainType,
    method: rec.method ?? detail.method,
    framework: rec.framework ?? detail.framework,
    modelId: rec.model?.id ?? detail.baseModelId,
    datasetId: rec.data?.trainId ?? detail.datasetId,
    evalDatasetId: detail.evalDatasetId || undefined,
    choices: JSON.parse(JSON.stringify(rec.choices ?? {})),
    export: { hf: rec.output?.exportHf ?? true, peftAdapter: rec.output?.peftAdapter },
    evaluation: rec.evaluation ?? {},
    tracking: rec.tracking ? JSON.parse(JSON.stringify(rec.tracking)) : { wandb: {} },
  })
  if (rec.layout?.nodes) w.form.resources.nodes = rec.layout.nodes
  if (detail.image) w.form.resources.image = detail.image
  prefill.value = `Copied from run ${detail.displayName}.`
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

onMounted(async () => {
  await Promise.all([w.loadModels(), w.loadNodeGpus()])
  await applyQuery()
  await Promise.all([w.loadDatasets(), w.loadConfig()])
  w.runPreview()
})
</script>

<style scoped>
.pt-wizard {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr) 320px;
  gap: 16px;
  align-items: start;
}
.pt-step {
  display: flex;
  gap: 10px;
  padding: 10px 8px;
  border-radius: 8px;
  cursor: pointer;
}
.pt-step.is-current {
  background: var(--safe-card-2);
}
.pt-step-index {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  border: 1px solid var(--safe-border);
  font-size: 12px;
}
.pt-step.is-current .pt-step-index {
  border-color: var(--safe-primary);
  color: var(--safe-primary);
}
.pt-step.has-error .pt-step-index {
  border-color: var(--el-color-danger);
  color: var(--el-color-danger);
}
</style>

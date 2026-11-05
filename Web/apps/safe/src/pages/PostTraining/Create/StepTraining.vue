<template>
  <el-form label-position="top">
    <el-collapse v-model="open">
      <el-collapse-item v-if="sft" title="Optimization" name="optimization">
        <div class="pt-grid">
          <NumberField label="Epochs" field="numEpoch" :min="0" />
          <NumberField label="Total steps (overrides epochs)" field="numRollout" :min="0" />
          <NumberField label="Global batch" field="batchSize" :min="1" />
          <NumberField label="Max tokens per GPU" field="maxTokensPerGpu" :min="1" :step="1024" />
          <NumberField label="Learning rate" field="lr" :step="1e-6" :precision="8" :min="0" />
          <NumberField
            label="Min learning rate"
            field="minLr"
            :step="1e-7"
            :precision="9"
            :min="0"
          />
          <NumberField label="Warmup steps" field="warmupIters" :min="0" />
        </div>
      </el-collapse-item>

      <el-collapse-item v-if="sft && hasValidation" title="Validation" name="validation">
        <div class="pt-grid">
          <NumberField label="Validation interval" field="valInterval" :min="1" />
          <NumberField label="Validation samples" field="valMaxSamples" :min="1" />
        </div>
        <div class="pt-help">
          Defaults to the save interval; at least one global batch of samples.
        </div>
      </el-collapse-item>

      <template v-if="!sft">
        <el-collapse-item title="Rollout & reward" name="reward">
          <el-form-item label="Reward" :error="fieldError('choices.rewardType')">
            <el-select v-model="rewardType" style="width: 300px">
              <el-option
                v-for="o in w.fieldValues('rewardType')"
                :key="o.value"
                :label="o.label"
                :value="o.value"
                :disabled="o.disabled"
              />
            </el-select>
          </el-form-item>
          <el-form-item v-if="rewardType === 'custom'" label="Reward function path" required>
            <el-input
              v-model="w.form.choices.rewardHook"
              placeholder="module.path:function on workspace storage"
            />
          </el-form-item>
          <template v-if="rewardType === 'remote' && w.form.choices.rewardRemote">
            <el-form-item label="Reward service URL" required>
              <el-input
                v-model="w.form.choices.rewardRemote.url"
                placeholder="https://reward.example.com/score"
              />
            </el-form-item>
            <el-form-item label="Secret (optional, keys starting with PT_RM_)">
              <el-input v-model="w.form.choices.rewardRemote.secretId" placeholder="secret id" />
            </el-form-item>
            <el-form-item label="Reward key">
              <el-input v-model="w.form.choices.rewardRemote.rewardKey" placeholder="score" />
            </el-form-item>
          </template>
          <div class="pt-grid">
            <NumberField label="Rollouts" field="numRollout" :min="1" />
            <NumberField label="Prompts per rollout (global batch)" field="batchSize" :min="1" />
            <NumberField label="Samples per prompt" field="nSamplesPerPrompt" :min="1" />
            <NumberField label="Max response length" field="maxResponseLen" :min="1" :step="1024" />
            <NumberField
              v-if="w.form.evalDatasetId"
              label="Evaluation interval"
              field="evalInterval"
              :min="1"
            />
          </div>
        </el-collapse-item>

        <el-collapse-item title="Policy optimization" name="policy">
          <div class="pt-grid">
            <NumberField
              label="Actor learning rate"
              field="lr"
              :step="1e-7"
              :precision="9"
              :min="0"
            />
            <div>
              <div class="pt-label">KL loss</div>
              <el-switch v-model="useKlLoss" />
            </div>
            <NumberField
              v-if="useKlLoss"
              label="KL loss coefficient"
              field="klLossCoef"
              :step="0.001"
              :precision="4"
            />
            <div>
              <div class="pt-label">Train-inference correction</div>
              <el-select v-model="correction" style="width: 160px">
                <el-option label="Off" value="" />
                <el-option label="TIS" value="tis" />
              </el-select>
            </div>
            <template v-if="algorithm === 'ppo'">
              <NumberField
                label="Critic learning rate"
                field="criticLr"
                :step="1e-6"
                :precision="9"
                :min="0"
              />
              <NumberField label="Critic-only rollouts" field="criticWarmupRollouts" :min="0" />
            </template>
          </div>
          <div v-if="algorithm === 'ppo'" class="pt-help">
            The critic shares the actor's GPUs and keeps its own checkpoints. {{ criticNote }}
          </div>
        </el-collapse-item>

        <el-collapse-item title="Rollout engine" name="engine">
          <div class="pt-grid">
            <div>
              <div class="pt-label">Rollout precision</div>
              <el-segmented v-model="rolloutPrecision" :options="segments(precisionValues)" />
            </div>
            <div>
              <div class="pt-label">KV cache</div>
              <el-segmented v-model="kvCache" :options="KV_CACHE" />
            </div>
            <div>
              <div class="pt-label">Schedule</div>
              <el-segmented v-model="schedule" :options="segments(scheduleValues)" />
            </div>
            <NumberField
              v-if="schedule === 'async'"
              label="Concurrency"
              field="concurrency"
              :min="1"
            />
            <div>
              <div class="pt-label">Prefill / decode (Mooncake)</div>
              <el-segmented v-model="pd" :options="segments(pdValues)" />
            </div>
            <div v-if="pd === 'on'">
              <div class="pt-label">Prefill GPUs</div>
              <el-input-number v-model="prefillGpus" :min="1" controls-position="right" />
            </div>
          </div>
          <div v-if="schedule === 'async'" class="pt-help">
            Fully async keeps generating across weight updates and skips eval during training.
          </div>
          <div v-if="pd === 'on'" class="pt-help">
            Needs at least 2 rollout engines: one for prefill, one for decode.
          </div>
          <div v-for="o in unavailable" :key="o.label" class="pt-help">
            {{ o.label }} is not available: {{ o.reason }}
          </div>
          <div v-for="msg in engineErrors" :key="msg" class="pt-error">{{ msg }}</div>
        </el-collapse-item>
      </template>

      <el-collapse-item title="Freezing & checkpoints" name="checkpoints">
        <div class="pt-grid">
          <NumberField label="Save interval" field="saveInterval" :min="1" />
        </div>
        <template v-if="w.form.method === 'full' && freezeOptions">
          <div class="flex items-center gap-2">
            <el-switch v-model="frozen" />
            <span class="text-[13px]">Freeze {{ freezeOptions.label || 'parameters' }}</span>
            <el-tag v-if="freeze === 'default'" size="small" effect="plain" round
              >Spec default</el-tag
            >
          </div>
          <div class="pt-help">{{ freezeHelp }}</div>
          <el-alert
            v-if="freeze === 'off' && unfrozenNote"
            type="warning"
            :closable="false"
            show-icon
            class="mt-2"
            :title="unfrozenNote"
          />
          <el-form-item
            v-else-if="freeze !== 'off'"
            label="Patterns to train (one regex per line)"
            class="mt-2"
          >
            <el-input
              :model-value="patternsText"
              type="textarea"
              :rows="3"
              @update:model-value="setTrainOnly"
            />
            <div v-for="msg in patternErrors" :key="msg" class="pt-error">{{ msg }}</div>
          </el-form-item>
          <div v-if="fieldError('choices.freeze')" class="pt-error">
            {{ fieldError('choices.freeze') }}
          </div>
        </template>
      </el-collapse-item>

      <el-collapse-item v-if="w.form.method === 'lora'" title="LoRA" name="lora">
        <div class="pt-grid">
          <div>
            <div class="pt-label">Rank</div>
            <el-input-number
              :model-value="lora.rank ?? 16"
              :min="1"
              :max="256"
              @update:model-value="(v?: number) => setLora({ rank: v })"
            />
          </div>
          <div>
            <div class="pt-label">Alpha</div>
            <el-input-number
              :model-value="lora.alpha ?? 32"
              :min="1"
              @update:model-value="(v?: number) => setLora({ alpha: v })"
            />
          </div>
          <div>
            <div class="pt-label">Adapt expert layers</div>
            <el-switch
              :model-value="!!lora.experts"
              @update:model-value="(v: string | number | boolean) => setLora({ experts: !!v })"
            />
          </div>
          <div>
            <div class="pt-label">Save adapter only</div>
            <el-switch
              :model-value="!!lora.adapterOnlyCheckpoint"
              @update:model-value="
                (v: string | number | boolean) => setLora({ adapterOnlyCheckpoint: !!v })
              "
            />
          </div>
        </div>
      </el-collapse-item>
    </el-collapse>
  </el-form>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref } from 'vue'
import { ElInputNumber } from 'element-plus'
import type { Choices, LoraChoice, OptionValue } from '@/services/postTraining'
import { freezeState, regexErrors, trainableText } from '@/services/postTraining'
import { useWizard } from './wizard'

const w = useWizard()

const sft = computed(() => w.form.trainType === 'sft')
const hasValidation = computed(() => !!w.form.evalDatasetId || !!w.form.choices.valSplit)
const open = ref([
  'optimization',
  'validation',
  'reward',
  'policy',
  'engine',
  'checkpoints',
  'lora',
])
const defaults = computed(() => w.config.value?.options?.defaults ?? ({} as Choices))

type NumericChoice = {
  [K in keyof Choices]-?: NonNullable<Choices[K]> extends number ? K : never
}[keyof Choices]

/** A number input bound to one numeric choice; it shows the spec's default until edited. */
const NumberField = defineComponent({
  props: {
    label: { type: String, required: true },
    field: { type: String as () => NumericChoice, required: true },
    min: { type: Number, default: undefined },
    step: { type: Number, default: 1 },
    precision: { type: Number, default: undefined },
  },
  setup(props) {
    return () =>
      h('div', [
        h('div', { class: 'pt-label' }, props.label),
        h(ElInputNumber, {
          modelValue: w.form.choices[props.field] ?? defaults.value[props.field],
          min: props.min,
          step: props.step,
          precision: props.precision,
          controlsPosition: 'right',
          'onUpdate:modelValue': (v: number | undefined | null) => {
            w.form.choices[props.field] = v ?? undefined
          },
        }),
        fieldError(`choices.${props.field}`)
          ? h('div', { class: 'pt-error' }, fieldError(`choices.${props.field}`))
          : null,
      ])
  },
})

const rewardType = computed({
  get: () =>
    w.form.choices.rewardType ??
    defaults.value.rewardType ??
    w.fieldValues('rewardType')[0]?.value ??
    '',
  set: (v: string) => {
    w.form.choices.rewardType = v
    w.form.choices.rewardRemote = v === 'remote' ? (w.form.choices.rewardRemote ?? {}) : undefined
  },
})

const useKlLoss = computed({
  get: () => w.form.choices.useKlLoss ?? defaults.value.useKlLoss ?? true,
  set: (v: boolean) => (w.form.choices.useKlLoss = v),
})
const algorithm = computed(
  () => w.form.choices.advantageEstimator ?? defaults.value.advantageEstimator ?? 'grpo',
)
/** How the critic-only rollouts start a PPO run. */
const criticNote = computed(() => {
  const n = w.form.choices.criticWarmupRollouts ?? defaults.value.criticWarmupRollouts ?? 0
  if (n === 0) return 'Every rollout trains the actor too.'
  return n === 1
    ? 'The first rollout trains the critic only.'
    : `The first ${n} rollouts train the critic only.`
})

const correction = computed({
  get: () => w.form.choices.correction ?? defaults.value.correction ?? '',
  set: (v: string) => (w.form.choices.correction = v || undefined),
})

const KV_CACHE = [
  { label: 'BF16', value: 'bf16' },
  { label: 'FP8', value: 'fp8' },
]
const layout = computed(() => w.form.choices.layout ?? defaults.value.layout ?? 'colocate')

/** The segments of an option field. */
const segments = (values: OptionValue[]) =>
  values.map((o) => ({ label: o.label, value: o.value, disabled: o.disabled }))
/** Disables a value the layout or the algorithm rules out, unless the config already did. */
const rule = (o: OptionValue, why: string) =>
  o.disabled || !why ? o : { ...o, disabled: true, reason: why }

const precisionValues = computed(() => w.fieldValues('rolloutPrecision'))
const scheduleValues = computed(() =>
  w
    .fieldValues('schedule')
    .map((o) =>
      o.value !== 'async'
        ? o
        : rule(
            o,
            algorithm.value === 'ppo'
              ? 'Fully async supports GRPO only.'
              : layout.value !== 'separate'
                ? 'Fully async needs the disaggregated layout.'
                : '',
          ),
    ),
)
const pdValues = computed(() =>
  w
    .fieldValues('pd')
    .map((o) =>
      o.value === 'on'
        ? rule(o, layout.value !== 'separate' ? 'Mooncake PD needs the disaggregated layout.' : '')
        : o,
    ),
)
/** Why the disabled rollout engine values cannot be picked. */
const unavailable = computed(() => [
  ...precisionValues.value
    .filter((o) => o.disabled)
    .map((o) => ({ label: `${o.label} rollout`, reason: o.reason })),
  ...scheduleValues.value
    .filter((o) => o.disabled)
    .map((o) => ({ label: o.label, reason: o.reason })),
  ...pdValues.value
    .filter((o) => o.disabled)
    .map((o) => ({ label: 'Mooncake PD', reason: o.reason })),
])
const engineErrors = computed(() =>
  [
    'choices.rolloutPrecision',
    'choices.kvCacheDtype',
    'choices.schedule',
    'choices.pd',
    'choices.pd.prefillGpus',
  ]
    .map(fieldError)
    .filter(Boolean),
)

const rolloutPrecision = computed({
  get: () => w.form.choices.rolloutPrecision ?? 'bf16',
  set: (v: string) => (w.form.choices.rolloutPrecision = v === 'bf16' ? undefined : v),
})
const kvCache = computed({
  get: () => (w.form.choices.kvCacheDtype ? 'fp8' : 'bf16'),
  set: (v: string) => (w.form.choices.kvCacheDtype = v === 'fp8' ? 'fp8_e4m3' : undefined),
})
const schedule = computed({
  get: () => w.form.choices.schedule ?? 'sync',
  set: (v: string) => (w.form.choices.schedule = v === 'sync' ? undefined : v),
})
const pd = computed({
  get: () => (w.form.choices.pd?.enabled ? 'on' : 'off'),
  set: (v: string) => (w.form.choices.pd = v === 'on' ? { enabled: true } : undefined),
})
const prefillGpus = computed({
  get: () =>
    w.form.choices.pd?.prefillGpus ??
    w.form.choices.gpusPerEngine ??
    defaults.value.gpusPerEngine ??
    1,
  set: (v: number | undefined) => {
    if (w.form.choices.pd) w.form.choices.pd = { ...w.form.choices.pd, prefillGpus: v ?? undefined }
  },
})

const lora = computed<LoraChoice>(() => w.form.choices.lora ?? defaults.value.lora ?? {})
const setLora = (patch: LoraChoice) => {
  w.form.choices.lora = { ...lora.value, ...patch }
}

const freezeOptions = computed(() => w.config.value?.options?.freeze)
const freeze = computed(() => freezeState(w.form.choices.freeze, freezeOptions.value))
/** On leaves the spec's default patterns to the server; off sends an empty freeze, which trains everything. */
const frozen = computed({
  get: () => freeze.value !== 'off',
  set: (on: boolean) => (w.form.choices.freeze = on ? undefined : { trainOnly: [] }),
})
const patternsText = computed(() =>
  (w.form.choices.freeze?.trainOnly ?? freezeOptions.value?.default ?? []).join('\n'),
)
const patternErrors = computed(() => regexErrors(w.form.choices.freeze?.trainOnly ?? []))

/** The line under the freeze switch: what is trained and how large a checkpoint gets. */
const freezeHelp = computed(() => {
  const f = freezeOptions.value
  if (!f) return ''
  if (freeze.value === 'off') return `${trainableText('off', f)} parameters are trained.`
  const what =
    freeze.value === 'custom'
      ? 'Only the parameters matching these patterns are trained.'
      : `${trainableText('default', f)} parameters are trained.`
  const size =
    freeze.value === 'default' && f.checkpointTB ? `, about ${f.checkpointTB} TB each` : ''
  return `${what} Checkpoints keep only the trainable weights and their optimizer state${size}.`
})

/** The warning when freezing is off: whether that is verified, and the nodes and checkpoint size it needs. */
const unfrozenNote = computed(() => {
  const f = freezeOptions.value
  if (!f) return ''
  const needs = [
    f.fullNodes ? `${f.fullNodes} nodes${w.form.trainType === 'rl' ? ' with rollout' : ''}` : '',
    f.fullCheckpointTB ? `~${f.fullCheckpointTB} TB per checkpoint` : '',
  ].filter(Boolean)
  return [
    f.fullUnverified ? 'Unverified on this platform.' : '',
    needs.length ? `Estimated ${needs.join(' and ')}.` : '',
  ]
    .filter(Boolean)
    .join(' ')
})

/** Stores the patterns of the parameters to train, one per line; the spec's patterns or none mean its default. */
const setTrainOnly = (text: string) => {
  const patterns = text
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
  w.form.choices.freeze =
    !patterns.length || freezeState({ trainOnly: patterns }, freezeOptions.value) === 'default'
      ? undefined
      : { trainOnly: patterns }
}

/** The first error of the preview on a field. */
const fieldError = (field: string) => w.errors.value.find((d) => d.field === field)?.message ?? ''
</script>

<style scoped>
.pt-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 8px;
}
.pt-help {
  font-size: 12px;
  color: var(--safe-muted);
}
:deep(.pt-label) {
  font-size: 12px;
  color: var(--safe-muted);
  margin-bottom: 2px;
}
:deep(.pt-error) {
  font-size: 12px;
  color: var(--el-color-danger);
}
</style>

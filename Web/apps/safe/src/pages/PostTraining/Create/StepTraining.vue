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
          </div>
        </el-collapse-item>
      </template>

      <el-collapse-item title="Freezing & checkpoints" name="checkpoints">
        <div class="pt-grid">
          <NumberField label="Save interval" field="saveInterval" :min="1" />
        </div>
        <el-form-item
          v-if="w.form.method === 'full'"
          label="Train only (one regex per line, optional)"
        >
          <el-input
            :model-value="(w.form.choices.freeze?.trainOnly ?? []).join('\n')"
            type="textarea"
            :rows="3"
            placeholder="e.g. decoder.layers.(4[0-9]).mlp"
            @update:model-value="setTrainOnly"
          />
        </el-form-item>
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
import type { Choices, LoraChoice } from '@/services/postTraining'
import { useWizard } from './wizard'

const w = useWizard()

const sft = computed(() => w.form.trainType === 'sft')
const hasValidation = computed(() => !!w.form.evalDatasetId || !!w.form.choices.valSplit)
const open = ref(['optimization', 'validation', 'reward', 'policy', 'checkpoints', 'lora'])
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
const correction = computed({
  get: () => w.form.choices.correction ?? defaults.value.correction ?? '',
  set: (v: string) => (w.form.choices.correction = v || undefined),
})

const lora = computed<LoraChoice>(() => w.form.choices.lora ?? defaults.value.lora ?? {})
const setLora = (patch: LoraChoice) => {
  w.form.choices.lora = { ...lora.value, ...patch }
}

/** Stores the trainable-parameter regexes, one per line; none means full training. */
const setTrainOnly = (text: string) => {
  const patterns = text
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
  w.form.choices.freeze = patterns.length ? { trainOnly: patterns } : undefined
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

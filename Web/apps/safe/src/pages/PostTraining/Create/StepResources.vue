<template>
  <el-form label-position="top">
    <el-form-item label="Nodes" required :error="fieldError('resources.nodes')">
      <div class="flex items-center gap-2">
        <el-input-number v-model="w.form.resources.nodes" :min="1" :max="64" />
        <span>× {{ w.form.resources.gpusPerNode }} GPUs per node</span>
      </div>
    </el-form-item>

    <template v-if="w.form.trainType === 'rl'">
      <el-form-item label="Layout" required :error="fieldError('choices.layout')">
        <el-segmented v-model="layout" class="form-seg" :options="layoutOptions" />
        <div v-for="o in disabledLayouts" :key="o.value" class="pt-help">
          {{ o.label }} is not available: {{ o.reason }}
        </div>
      </el-form-item>

      <el-form-item v-if="layout === 'separate'" label="Rollout">
        <div class="flex flex-wrap items-center gap-4">
          <div>
            <div class="pt-label">Rollout GPUs</div>
            <el-input-number
              :model-value="
                w.form.choices.rolloutGpus ?? defaults.rolloutGpus ?? w.form.resources.gpusPerNode
              "
              :min="1"
              :max="totalGpus - 1"
              @update:model-value="(v?: number) => (w.form.choices.rolloutGpus = v ?? undefined)"
            />
          </div>
          <div>
            <div class="pt-label">GPUs per engine</div>
            <el-input-number
              :model-value="w.form.choices.gpusPerEngine ?? defaults.gpusPerEngine ?? 1"
              :min="1"
              :max="w.form.resources.gpusPerNode"
              @update:model-value="(v?: number) => (w.form.choices.gpusPerEngine = v ?? undefined)"
            />
          </div>
        </div>
        <div class="pt-help">{{ rolloutSummary }}</div>
      </el-form-item>
    </template>

    <el-form-item>
      <template #label>
        <span>Parallelism</span>
        <span v-if="w.config.value?.spec" class="pt-help-inline"
          >from spec {{ w.config.value.spec }}</span
        >
        <el-link type="primary" class="ml-2" @click="resetParallelism">Reset to spec</el-link>
      </template>
      <div class="flex flex-wrap items-center gap-4">
        <div v-for="key in parallelKeys" :key="key">
          <div class="pt-label">{{ key.toUpperCase() }}</div>
          <el-input-number
            :model-value="w.form.choices[key] ?? defaults[key] ?? 1"
            :min="1"
            :max="64"
            @update:model-value="(v?: number) => (w.form.choices[key] = v ?? undefined)"
          />
        </div>
        <div>
          <div class="pt-label">Sequence parallel</div>
          <el-switch v-model="sequenceParallel" />
        </div>
      </div>
      <div v-if="fieldError('choices.tp')" class="text-[12px] text-[var(--el-color-danger)]">
        {{ fieldError('choices.tp') }}
      </div>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useWizard } from './wizard'

const w = useWizard()

const parallelKeys = ['tp', 'pp', 'cp', 'ep'] as const
const defaults = computed(() => w.config.value?.options?.defaults ?? {})
const totalGpus = computed(() => w.form.resources.nodes * w.form.resources.gpusPerNode)

const layoutOptions = computed(() =>
  w.fieldValues('layout').map((o) => ({ label: o.label, value: o.value, disabled: o.disabled })),
)
const disabledLayouts = computed(() => w.fieldValues('layout').filter((o) => o.disabled))
const layout = computed({
  get: () => w.form.choices.layout ?? defaults.value.layout ?? 'colocate',
  set: (v: string) => (w.form.choices.layout = v),
})

const sequenceParallel = computed({
  get: () => w.form.choices.sequenceParallel ?? defaults.value.sequenceParallel ?? false,
  set: (v: boolean) => (w.form.choices.sequenceParallel = v),
})

const rolloutSummary = computed(() => {
  const rollout =
    w.form.choices.rolloutGpus ?? defaults.value.rolloutGpus ?? w.form.resources.gpusPerNode
  const perEngine = w.form.choices.gpusPerEngine ?? defaults.value.gpusPerEngine ?? 1
  const engines = Math.max(1, Math.floor(rollout / perEngine))
  return `${engines} SGLang engine${engines === 1 ? '' : 's'} at TP ${perEngine}. Training gets the other ${Math.max(0, totalGpus.value - rollout)} GPUs.`
})

/** Drops the edited parallel settings, so the spec's defaults apply again. */
const resetParallelism = () => {
  for (const key of parallelKeys) w.form.choices[key] = undefined
  w.form.choices.sequenceParallel = undefined
}

/** The first error of the preview on a field. */
const fieldError = (field: string) => w.errors.value.find((d) => d.field === field)?.message ?? ''
</script>

<style scoped>
.pt-help {
  width: 100%;
  margin-top: 4px;
  font-size: 12px;
  color: var(--safe-muted);
}
.pt-help-inline {
  margin-left: 8px;
  font-size: 12px;
  color: var(--safe-muted);
}
.pt-label {
  font-size: 12px;
  color: var(--safe-muted);
  margin-bottom: 2px;
}
</style>

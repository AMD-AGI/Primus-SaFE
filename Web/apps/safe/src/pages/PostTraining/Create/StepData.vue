<template>
  <el-form label-position="top">
    <el-form-item label="Training dataset" required :error="fieldError('datasetId')">
      <el-select
        v-model="w.form.datasetId"
        filterable
        placeholder="Select a dataset"
        style="width: 420px"
      >
        <el-option
          v-for="d in w.trainDatasets.value"
          :key="d.datasetId"
          :label="d.displayName"
          :value="d.datasetId"
        >
          <span>{{ d.displayName }}</span>
          <span class="pt-option-note"
            >{{ d.datasetType }} · {{ d.totalSizeStr || `${d.fileCount ?? 0} files` }}</span
          >
        </el-option>
      </el-select>
      <div v-if="!w.trainDatasets.value.length" class="pt-help">
        No ready {{ datasetType }} dataset in this workspace.
      </div>
    </el-form-item>

    <el-form-item
      v-if="w.form.trainType === 'sft'"
      label="Validation"
      :error="fieldError('choices.valSplit')"
    >
      <el-segmented
        v-model="validation"
        class="form-seg"
        :options="[
          { label: 'None', value: 'none' },
          { label: 'Separate dataset', value: 'dataset' },
          { label: 'Split from training', value: 'split' },
        ]"
      />
      <el-select
        v-if="validation === 'dataset'"
        v-model="w.form.evalDatasetId"
        filterable
        clearable
        placeholder="Select a validation dataset"
        class="mt-2"
        style="width: 420px"
      >
        <el-option
          v-for="d in w.trainDatasets.value"
          :key="d.datasetId"
          :label="d.displayName"
          :value="d.datasetId"
        />
      </el-select>
      <div v-if="validation === 'split'" class="flex items-center gap-3 mt-2" style="width: 420px">
        <el-slider
          v-model="splitPercent"
          :min="1"
          :max="20"
          :format-tooltip="(v: number) => `${v}%`"
        />
        <span class="shrink-0">{{ splitPercent }}%</span>
      </div>
      <div class="pt-help">
        Held-out rows are not used for training. Up to 20%. How many are scored each time is set in
        Training.
      </div>
    </el-form-item>

    <el-form-item v-else label="Evaluation dataset" :error="fieldError('evalDatasetId')">
      <el-select
        v-model="w.form.evalDatasetId"
        filterable
        clearable
        placeholder="Optional"
        style="width: 420px"
      >
        <el-option
          v-for="d in w.trainDatasets.value"
          :key="d.datasetId"
          :label="d.displayName"
          :value="d.datasetId"
        />
      </el-select>
      <div class="pt-help">
        Scored during training at the evaluation interval and shown as eval/&lt;dataset&gt;.
      </div>
    </el-form-item>

    <el-form-item label="Field mapping" required>
      <div class="flex flex-col gap-2">
        <div v-for="target in targets" :key="target" class="flex items-center gap-2">
          <span class="pt-target">{{ target }}</span>
          <el-input
            :model-value="fieldMap[target] ?? target"
            style="width: 220px"
            @update:model-value="(v: string) => setField(target, v)"
          />
          <span
            v-if="fieldError(`choices.fieldMap.${target}`)"
            class="text-[12px] text-[var(--el-color-danger)]"
          >
            {{ fieldError(`choices.fieldMap.${target}`) }}
          </span>
        </div>
      </div>
      <div class="pt-help">The dataset column each target field is read from.</div>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { DATASET_TYPE, FIELD_TARGETS, useWizard } from './wizard'

const w = useWizard()

const datasetType = computed(() => DATASET_TYPE[w.form.trainType])
const targets = computed(() => FIELD_TARGETS[w.form.trainType])
const fieldMap = computed(() => w.form.choices.fieldMap ?? {})

/** Sets the column of one target field. */
const setField = (target: string, column: string) => {
  w.form.choices.fieldMap = {
    ...Object.fromEntries(targets.value.map((t) => [t, t])),
    ...fieldMap.value,
    [target]: column,
  }
}

const validation = ref<'none' | 'dataset' | 'split'>(
  w.form.evalDatasetId ? 'dataset' : w.form.choices.valSplit ? 'split' : 'none',
)
const splitPercent = ref(Math.round((w.form.choices.valSplit ?? 0.02) * 100) || 2)

watch([validation, splitPercent], ([mode, percent]) => {
  if (mode !== 'dataset') w.form.evalDatasetId = undefined
  w.form.choices.valSplit = mode === 'split' ? percent / 100 : undefined
})

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
.pt-option-note {
  margin-left: 8px;
  font-size: 12px;
  color: var(--safe-muted);
}
.pt-target {
  width: 90px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>

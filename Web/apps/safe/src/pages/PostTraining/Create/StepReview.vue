<template>
  <el-form label-position="top">
    <el-form-item label="Run name" required>
      <el-input
        v-model="w.form.displayName"
        placeholder="e.g. qwen3-8b-sft-lr2e-5"
        maxlength="64"
        style="width: 420px"
      />
    </el-form-item>

    <el-tabs v-model="tab">
      <el-tab-pane label="Summary" name="summary">
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="Task">{{ taskText }}</el-descriptions-item>
          <el-descriptions-item label="Base model">{{ modelName }}</el-descriptions-item>
          <el-descriptions-item label="Dataset">{{
            datasetName(w.form.datasetId) || '—'
          }}</el-descriptions-item>
          <el-descriptions-item label="Validation / evaluation">{{
            validationText
          }}</el-descriptions-item>
          <el-descriptions-item label="Nodes">
            {{ w.form.resources.nodes }} × {{ w.form.resources.gpusPerNode }} GPUs
          </el-descriptions-item>
          <el-descriptions-item label="Export">{{
            w.form.export.hf === false ? 'None' : 'Hugging Face model'
          }}</el-descriptions-item>
          <el-descriptions-item label="Automatic evaluation">{{
            evaluationText
          }}</el-descriptions-item>
          <el-descriptions-item label="Retries">{{
            w.form.resources.maxRetry ?? 1
          }}</el-descriptions-item>
        </el-descriptions>
      </el-tab-pane>
      <RecipePanes
        :recipe="w.preview.value?.recipe"
        :framework="w.form.framework || w.config.value?.framework"
      />
    </el-tabs>

    <div v-if="w.warnings.value.length" class="mt-4">
      <div class="font-500 mb-1">Confirm the warnings</div>
      <el-checkbox
        v-for="(d, i) in w.warnings.value"
        :key="`${i}-${d.message}`"
        :model-value="confirmed.has(d.message)"
        class="pt-warning"
        @update:model-value="(v: string | number | boolean) => toggle(d.message, !!v)"
      >
        {{ d.message }}
      </el-checkbox>
    </div>
  </el-form>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import RecipePanes from '../Components/RecipePanes.vue'
import { useWizard } from './wizard'

const emit = defineEmits<{ confirmed: [all: boolean] }>()
const w = useWizard()

const tab = ref('summary')
const confirmed = reactive(new Set<string>())
const toggle = (message: string, on: boolean) => {
  if (on) confirmed.add(message)
  else confirmed.delete(message)
}
watch(
  [() => w.warnings.value, () => confirmed.size],
  () =>
    emit(
      'confirmed',
      w.warnings.value.every((d) => confirmed.has(d.message)),
    ),
  { immediate: true },
)

const modelName = computed(
  () =>
    w.models.value?.items.find((m) => m.modelId === w.form.modelId)?.displayName ??
    (w.form.modelId || '—'),
)
const datasetName = (id?: string) =>
  [...w.trainDatasets.value, ...w.evalDatasets.value].find((d) => d.datasetId === id)
    ?.displayName ?? id

const taskText = computed(() => {
  const f = w.form
  const algorithm =
    f.trainType === 'rl' ? ` · ${(f.choices.advantageEstimator ?? 'grpo').toUpperCase()}` : ''
  return `${f.trainType.toUpperCase()} · ${f.method === 'lora' ? 'LoRA' : 'Full'}${algorithm} · ${f.framework || 'slime'}`
})
const validationText = computed(() => {
  if (w.form.evalDatasetId) return datasetName(w.form.evalDatasetId)
  if (w.form.choices.valSplit)
    return `${Math.round(w.form.choices.valSplit * 100)}% of the training data`
  return 'None'
})
const evaluationText = computed(() =>
  w.form.evaluation.enabled
    ? (w.form.evaluation.benchmarks ?? []).map((id) => datasetName(id)).join(', ') ||
      'No benchmark selected'
    : 'Not enabled',
)
</script>

<style scoped>
.pt-warning {
  display: flex;
  height: auto;
  margin: 4px 0;
  white-space: normal;
}
</style>

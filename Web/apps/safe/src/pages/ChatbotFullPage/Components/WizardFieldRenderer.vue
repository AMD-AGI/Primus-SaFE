<template>
  <div class="flex items-start gap-3">
    <label class="field-label">
      {{ field.label }}
      <span v-if="field.required" class="text-red-500 ml-0.5">*</span>
    </label>

    <div class="flex-1 min-w-0">
      <!-- Select -->
      <el-select
        v-if="field.type === 'select'"
        :model-value="(modelValue as string)"
        :placeholder="field.placeholder || `Select ${field.label}`"
        :multiple="field.multiple ?? false"
        :loading="optionsLoading"
        clearable
        filterable
        style="width: 100%"
        @update:model-value="$emit('update:modelValue', $event)"
      >
        <el-option
          v-for="opt in mergedOptions"
          :key="String(opt.value)"
          :label="opt.label"
          :value="opt.value"
        />
      </el-select>

      <!-- Number -->
      <div v-else-if="field.type === 'number'" class="flex items-center gap-1.5">
        <el-input-number
          :model-value="(modelValue as number)"
          :min="field.min ?? 0"
          :max="field.max"
          :placeholder="field.placeholder"
          controls-position="right"
          style="width: 100%"
          @update:model-value="$emit('update:modelValue', $event)"
        />
        <span v-if="field.suffix" class="text-xs flex-shrink-0" style="color: var(--safe-muted)">
          {{ field.suffix }}
        </span>
      </div>

      <!-- Textarea -->
      <el-input
        v-else-if="field.type === 'textarea'"
        :model-value="(modelValue as string)"
        type="textarea"
        :rows="3"
        :placeholder="field.placeholder"
        @update:model-value="$emit('update:modelValue', $event)"
      />

      <!-- Text input -->
      <el-input
        v-else
        :model-value="(modelValue as string)"
        :placeholder="field.placeholder"
        clearable
        @update:model-value="$emit('update:modelValue', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { WizardField, WizardFieldOption } from '../constants/guidedWorkflows'
import { useWizardOptions } from '../composables/useWizardOptions'

interface Props {
  field: WizardField
  modelValue: unknown
}

const props = defineProps<Props>()
defineEmits<{
  (e: 'update:modelValue', value: unknown): void
}>()

const { options: asyncOptions, loading: optionsLoading } = useWizardOptions(
  props.field.optionsLoader,
)

const mergedOptions = computed<WizardFieldOption[]>(() => {
  if (props.field.options?.length) return props.field.options
  return asyncOptions.value
})
</script>

<style scoped>
.field-label {
  flex-shrink: 0;
  width: 130px;
  text-align: right;
  padding-top: 7px;
  font-size: 13px;
  font-weight: 500;
  color: var(--safe-text);
}
</style>

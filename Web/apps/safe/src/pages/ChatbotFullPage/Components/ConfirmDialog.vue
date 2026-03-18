<template>
  <el-dialog
    v-model="dialogVisible"
    :title="data?.title"
    width="600px"
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <!-- Message -->
    <div v-if="data?.message" class="confirm-message" v-html="data.message"></div>

    <!-- Selection Type -->
    <div v-if="data?.confirm_type === 'selection' && data.selections" class="selection-form">
      <div
        v-for="(field, key) in data.selections"
        :key="key"
        class="form-field"
        :class="{ required: field.required }"
      >
        <label class="field-label">
          {{ field.label }}
          <span v-if="field.required" class="required-mark">*</span>
        </label>

        <!-- Input type -->
        <el-input
          v-if="field.type === 'input'"
          v-model="formData[key]"
          :placeholder="field.placeholder || ''"
          clearable
        />

        <!-- Select type -->
        <el-select
          v-else-if="field.type === 'select'"
          v-model="formData[key]"
          :placeholder="'Select ' + field.label"
          clearable
          filterable
          style="width: 100%"
        >
          <el-option
            v-for="option in field.options"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          >
            <div class="option-content">
              <span class="option-label">{{ option.label }}</span>
              <span v-if="option.description" class="option-description">{{
                option.description
              }}</span>
            </div>
          </el-option>
        </el-select>

        <!-- Multi-select type -->
        <el-select
          v-else-if="field.type === 'multi-select'"
          v-model="formData[key]"
          :placeholder="'Select ' + field.label"
          multiple
          clearable
          filterable
          collapse-tags
          collapse-tags-tooltip
          style="width: 100%"
        >
          <el-option
            v-for="option in field.options"
            :key="option.value"
            :label="option.label"
            :value="option.value"
          >
            <div class="option-content">
              <span class="option-label">{{ option.label }}</span>
              <span v-if="option.description" class="option-description">{{
                option.description
              }}</span>
            </div>
          </el-option>
        </el-select>
      </div>
    </div>

    <!-- Execution Type -->
    <div v-if="data?.confirm_type === 'execution' && data.details" class="execution-details">
      <el-descriptions :column="1" border>
        <el-descriptions-item
          v-for="(value, key) in data.details"
          :key="key"
          :label="String(key)"
        >
          <span class="detail-value" v-html="formatDetailValue(String(value))"></span>
        </el-descriptions-item>
      </el-descriptions>
    </div>

    <!-- Actions -->
    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleCancel">Cancel</el-button>
        <el-button
          type="primary"
          @click="handleConfirm"
          :loading="loading"
          :disabled="!isFormValid"
        >
          {{ data?.confirm_type === 'selection' ? 'Submit' : 'Confirm' }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import type { ConfirmMessageData } from '@/services/agent'

interface Props {
  modelValue: boolean
  data: ConfirmMessageData | null
  loading?: boolean
}

interface Emits {
  (e: 'update:modelValue', value: boolean): void
  (e: 'confirm', data: { selections?: Record<string, any>; approved?: boolean }): void
  (e: 'cancel'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const dialogVisible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const formData = ref<Record<string, any>>({})

// Watch data changes to initialize form
watch(
  () => props.data,
  (newData) => {
    if (!newData) {
      formData.value = {}
      return
    }

    // Initialize form data with defaults
    if (newData.confirm_type === 'selection' && newData.selections) {
      const initialData: Record<string, any> = {}
      Object.entries(newData.selections).forEach(([key, field]) => {
        if (field.default !== undefined) {
          initialData[key] = field.default
        } else if (field.type === 'multi-select') {
          initialData[key] = []
        } else {
          initialData[key] = ''
        }
      })
      formData.value = initialData
    }
  },
  { immediate: true },
)

// Validate form
const isFormValid = computed(() => {
  if (!props.data) return false

  if (props.data.confirm_type === 'selection' && props.data.selections) {
    // Check required fields
    for (const [key, field] of Object.entries(props.data.selections)) {
      if (field.required) {
        const value = formData.value[key]
        if (value === undefined || value === null || value === '') {
          return false
        }
        if (Array.isArray(value) && value.length === 0) {
          return false
        }
      }
    }
  }

  return true
})

// Format detail value (convert \n to <br>)
const formatDetailValue = (value: string) => {
  return value.replace(/\n/g, '<br>')
}

// Handle confirm
const handleConfirm = () => {
  if (!props.data) return

  if (props.data.confirm_type === 'selection') {
    emit('confirm', { selections: formData.value })
  } else {
    emit('confirm', { approved: true })
  }
}

// Handle cancel
const handleCancel = () => {
  if (props.data?.confirm_type === 'execution') {
    emit('confirm', { approved: false })
  }
  emit('cancel')
  dialogVisible.value = false
}

// Handle close
const handleClose = () => {
  emit('cancel')
}
</script>

<style scoped lang="scss">
.confirm-message {
  margin-bottom: 20px;
  line-height: 1.6;
  color: #333;
  font-size: 14px;

  :deep(br) {
    display: block;
    margin: 4px 0;
  }
}

.selection-form {
  display: flex;
  flex-direction: column;
  gap: 20px;

  .form-field {
    .field-label {
      display: block;
      margin-bottom: 8px;
      font-size: 14px;
      font-weight: 500;
      color: #333;

      .required-mark {
        color: #f56c6c;
        margin-left: 2px;
      }
    }

    .option-content {
      display: flex;
      flex-direction: column;
      gap: 2px;

      .option-label {
        font-size: 14px;
        color: #333;
      }

      .option-description {
        font-size: 12px;
        color: #999;
      }
    }
  }
}

.execution-details {
  margin-top: 16px;

  .detail-value {
    line-height: 1.6;

    :deep(br) {
      display: block;
      margin: 2px 0;
    }
  }
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}
</style>

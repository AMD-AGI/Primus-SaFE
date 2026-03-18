<template>
  <div class="inline-confirm-form">
    <!-- Title -->
    <div v-if="data.title" class="form-title">{{ data.title }}</div>

    <!-- Message -->
    <div v-if="data.message" class="form-message" v-html="data.message"></div>

    <!-- Selection Form -->
    <div v-if="data.confirm_type === 'selection' && data.selections" class="selection-fields">
      <div
        v-for="(field, key) in data.selections"
        :key="key"
        class="form-field"
        :class="{ required: field.required, readonly: readonly }"
      >
        <label class="field-label">
          {{ field.label }}
          <span v-if="field.required && !readonly" class="required-mark">*</span>
        </label>

        <div class="field-control">
          <!-- Readonly mode: show value only -->
          <div v-if="readonly" class="readonly-value">
            {{ formatReadonlyValue(confirmedSelections?.[key], field) }}
          </div>

          <!-- Editable mode: show form controls -->
          <template v-else>
            <!-- Input type (no options) -->
            <el-input
              v-if="!field.options || field.options.length === 0"
              v-model="formData[key]"
              :placeholder="field.placeholder || ''"
              clearable
            />

            <!-- Multi-select type (has options and multiple: true) -->
            <div v-else-if="field.multiple === true" class="multi-select-wrapper">
              <el-select
                v-model="formData[key]"
                :placeholder="field.placeholder || 'Select ' + field.label"
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
                />
              </el-select>
              <a
                href="javascript:void(0)"
                class="select-all-link"
                @click="toggleSelectAll(key, field)"
              >
                {{ isAllSelected(key, field) ? 'Deselect All' : 'Select All' }}
              </a>
            </div>

            <!-- Single select type (has options, not multiple) -->
            <el-select
              v-else
              v-model="formData[key]"
              :placeholder="field.placeholder || 'Select ' + field.label"
              clearable
              filterable
              style="width: 100%"
            >
              <el-option
                v-for="option in field.options"
                :key="option.value"
                :label="option.label"
                :value="option.value"
              />
            </el-select>
          </template>
        </div>
      </div>
    </div>

    <!-- Execution Details -->
    <div v-if="data.confirm_type === 'execution' && data.details" class="execution-details">
      <el-descriptions :column="1" border size="small">
        <el-descriptions-item v-for="(value, key) in data.details" :key="key" :label="String(key)">
          <span class="detail-value" v-html="formatDetailValue(String(value))"></span>
        </el-descriptions-item>
      </el-descriptions>
    </div>

    <!-- Actions -->
    <div v-if="!readonly" class="form-actions">
      <el-button size="default" @click="handleCancel" :disabled="loading">Cancel</el-button>
      <el-button
        type="primary"
        size="default"
        @click="handleSubmit"
        :loading="loading"
        :disabled="!isFormValid"
      >
        {{ data.confirm_type === 'selection' ? 'Submit' : 'Confirm' }}
      </el-button>
    </div>

    <!-- Confirmed indicator for readonly mode -->
    <div v-else class="confirmed-indicator">
      <el-icon class="confirmed-icon"><CircleCheck /></el-icon>
      <span class="confirmed-text">Confirmed</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { CircleCheck } from '@element-plus/icons-vue'
import type { ConfirmMessageData, SelectionField } from '@/services/agent'

interface Props {
  data: ConfirmMessageData
  loading?: boolean
  readonly?: boolean
  confirmedSelections?: Record<string, unknown>
}

interface Emits {
  (e: 'submit', data: { selections?: Record<string, unknown>; approved?: boolean }): void
  (e: 'cancel', data?: { confirmType?: string }): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const formData = ref<Record<string, unknown>>({})

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
      const initialData: Record<string, unknown> = {}
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

// Format readonly value for display
const formatReadonlyValue = (value: unknown, field: SelectionField): string => {
  // Handle empty/null/undefined
  if (value === null || value === undefined || value === '') {
    return '-'
  }

  if (Array.isArray(value)) {
    if (value.length === 0) return '-'

    // For multi-select, show labels
    if (field.options) {
      return value
        .map((v) => {
          const option = field.options!.find((opt) => opt.value === v)
          return option?.label || v
        })
        .join(', ')
    }
    return value.join(', ')
  }

  // For single select, show label
  if (field.options) {
    const option = field.options.find((opt) => opt.value === value)
    return option?.label || String(value)
  }

  return String(value)
}

// Handle submit
const handleSubmit = () => {
  if (!props.data) return

  if (props.data.confirm_type === 'selection') {
    emit('submit', { selections: formData.value })
  } else {
    emit('submit', { approved: true })
  }
}

// Handle cancel
const handleCancel = () => {
  if (props.data?.confirm_type === 'execution') {
    emit('submit', { approved: false })
  }
  emit('cancel', { confirmType: props.data?.confirm_type })
}

// Check if all options are selected
const isAllSelected = (key: string, field: SelectionField): boolean => {
  if (!field.options || field.options.length === 0) return false
  const currentValue = formData.value[key]
  if (!Array.isArray(currentValue)) return false
  return currentValue.length === field.options.length
}

// Toggle select all / deselect all
const toggleSelectAll = (key: string, field: SelectionField) => {
  if (!field.options) return

  if (isAllSelected(key, field)) {
    // Deselect all
    formData.value[key] = []
  } else {
    // Select all
    formData.value[key] = field.options.map((opt) => opt.value)
  }
}
</script>

<style scoped lang="scss">
.inline-confirm-form {
  background: linear-gradient(135deg, #fafbfc 0%, #fff 100%);
  border: 1.5px solid #e2e8f0;
  border-radius: 16px;
  padding: 20px;
  margin: 12px 0;
  box-shadow:
    0 4px 16px rgba(0, 0, 0, 0.08),
    0 8px 32px rgba(0, 0, 0, 0.04);
  position: relative;

  &::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 50%;
    border-radius: 16px 16px 0 0;
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.2) 0%, transparent 100%);
    pointer-events: none;
  }
}

.form-title {
  font-size: 16px;
  font-weight: 600;
  color: #1e293b;
  margin-bottom: 12px;
  position: relative;
  z-index: 1;
}

.form-message {
  margin-bottom: 16px;
  line-height: 1.7;
  color: #475569;
  font-size: 14px;
  position: relative;
  z-index: 1;

  :deep(br) {
    display: block;
    margin: 4px 0;
  }
}

.selection-fields {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-bottom: 20px;
  position: relative;
  z-index: 1;

  .form-field {
    display: flex;
    align-items: center;
    gap: 16px;

    .field-label {
      flex-shrink: 0;
      width: 150px;
      font-size: 14px;
      font-weight: 500;
      color: #1e293b;
      text-align: right;

      .required-mark {
        color: #ef4444;
        margin-left: 2px;
      }
    }

    .field-control {
      flex: 1;
      min-width: 0;
    }
  }

  .multi-select-wrapper {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 12px;

    .select-all-link {
      flex-shrink: 0;
      font-size: 13px;
      color: #3b82f6;
      text-decoration: none;
      font-weight: 500;
      transition: all 0.2s ease;
      padding: 6px 12px;
      border-radius: 6px;
      white-space: nowrap;

      &:hover {
        color: #2563eb;
        background: rgba(59, 130, 246, 0.08);
        text-decoration: underline;
      }

      &:active {
        transform: scale(0.98);
      }
    }
  }
}

.execution-details {
  margin-bottom: 20px;
  position: relative;
  z-index: 1;

  .detail-value {
    line-height: 1.7;

    :deep(br) {
      display: block;
      margin: 2px 0;
    }
  }
}

.form-field.readonly {
  .field-control {
    .readonly-value {
      padding: 8px 12px;
      background: rgba(241, 245, 249, 0.5);
      border-radius: 8px;
      color: #1e293b;
      font-size: 14px;
    }
  }
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  position: relative;
  z-index: 1;

  :deep(.el-button) {
    border-radius: 10px;
    font-weight: 500;
    transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);

    &:not(.is-disabled):hover {
      transform: translateY(-1px);
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    }
  }

  :deep(.el-button--primary) {
    background: linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%);
    border: none;
    box-shadow: 0 2px 8px rgba(59, 130, 246, 0.25);

    &:not(.is-disabled):hover {
      background: linear-gradient(135deg, #2563eb 0%, #7c3aed 100%);
      box-shadow: 0 4px 16px rgba(59, 130, 246, 0.4);
    }
  }
}

.confirmed-indicator {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  padding: 8px 0;
  color: #10b981;
  font-weight: 500;
  font-size: 14px;
  position: relative;
  z-index: 1;

  .confirmed-icon {
    font-size: 18px;
  }
}

// Dark mode
.dark {
  .inline-confirm-form {
    background: rgba(30, 41, 59, 0.6);
    border-color: #334155;
    backdrop-filter: blur(10px);
    box-shadow:
      0 4px 16px rgba(0, 0, 0, 0.3),
      0 8px 32px rgba(0, 0, 0, 0.2);

    &::before {
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.03) 0%, transparent 100%);
    }
  }

  .form-title {
    color: #e2e8f0;
  }

  .form-message {
    color: #94a3b8;
  }

  .selection-fields {
    .form-field {
      .field-label {
        color: #cbd5e1;

        .required-mark {
          color: #f87171;
        }
      }
    }
  }

  .execution-details {
    :deep(.el-descriptions) {
      background: rgba(15, 23, 42, 0.5);
    }

    :deep(.el-descriptions__label) {
      color: #94a3b8;
    }

    :deep(.el-descriptions__content) {
      color: #e2e8f0;
    }
  }

  .form-field.readonly {
    .field-control {
      .readonly-value {
        background: rgba(30, 41, 59, 0.5);
        color: #e2e8f0;
      }
    }
  }

  .confirmed-indicator {
    color: #34d399;
  }

  .multi-select-wrapper {
    .select-all-link {
      color: #60a5fa;

      &:hover {
        color: #93c5fd;
        background: rgba(59, 130, 246, 0.15);
      }
    }
  }
}
</style>

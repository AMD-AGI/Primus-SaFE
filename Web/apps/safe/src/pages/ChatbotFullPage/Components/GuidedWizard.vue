<template>
  <div class="guided-wizard">
    <!-- Header -->
    <div class="wizard-header">
      <span class="text-lg">{{ workflow.icon }}</span>
      <span class="wizard-title">{{ workflow.name }}</span>
    </div>

    <!-- Step indicator -->
    <el-steps
      :active="readonly ? allSteps.length : currentStep"
      finish-status="success"
      :space="120"
      class="wizard-steps"
    >
      <el-step
        v-for="(step, idx) in allSteps"
        :key="idx"
        :title="step.title"
        :class="{ 'cursor-pointer': !readonly && idx < currentStep }"
        @click="!readonly && idx < currentStep && (currentStep = idx)"
      />
    </el-steps>

    <!-- Readonly: full summary -->
    <template v-if="readonly">
      <el-descriptions :column="1" border size="small">
        <el-descriptions-item
          v-for="item in summaryItems"
          :key="item.key"
          :label="item.label"
        >
          {{ item.displayValue }}
        </el-descriptions-item>
      </el-descriptions>
      <div class="wizard-submitted">
        <el-icon :size="18"><CircleCheck /></el-icon>
        <span>Submitted</span>
      </div>
    </template>

    <!-- Active wizard steps -->
    <template v-else>
      <!-- Summary step -->
      <template v-if="isSummaryStep">
        <div class="wizard-step-desc">Review the information below and submit.</div>
        <el-descriptions :column="1" border size="small" class="mb-4">
          <el-descriptions-item
            v-for="item in summaryItems"
            :key="item.key"
            :label="item.label"
          >
            {{ item.displayValue }}
          </el-descriptions-item>
        </el-descriptions>
      </template>

      <!-- Normal field step -->
      <template v-else>
        <div v-if="currentStepDef.description" class="wizard-step-desc">
          {{ currentStepDef.description }}
        </div>
        <div class="wizard-fields">
          <WizardFieldRenderer
            v-for="field in currentStepDef.fields"
            :key="field.key"
            :field="field"
            :model-value="stepData[field.key]"
            @update:model-value="(v: unknown) => (stepData[field.key] = v)"
          />
        </div>
      </template>

      <!-- Navigation buttons -->
      <div class="wizard-actions">
        <el-button v-if="currentStep > 0" size="default" @click="currentStep--">Back</el-button>
        <el-button v-else size="default" @click="$emit('cancel')">Cancel</el-button>
        <el-button
          v-if="!isSummaryStep"
          type="primary"
          size="default"
          :disabled="!isCurrentStepValid"
          @click="currentStep++"
        >
          Next
        </el-button>
        <el-button
          v-else
          type="primary"
          size="default"
          @click="$emit('submit', { ...stepData })"
        >
          Submit
        </el-button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { CircleCheck } from '@element-plus/icons-vue'
import type { GuidedWorkflow, WizardStep, WizardField } from '../constants/guidedWorkflows'
import WizardFieldRenderer from './WizardFieldRenderer.vue'

interface Props {
  workflow: GuidedWorkflow
  readonly?: boolean
  submittedData?: Record<string, unknown>
  prefilled?: Record<string, unknown>
}

const props = withDefaults(defineProps<Props>(), {
  readonly: false,
  submittedData: undefined,
  prefilled: undefined,
})

defineEmits<{
  (e: 'submit', data: Record<string, unknown>): void
  (e: 'cancel'): void
}>()

const currentStep = ref(0)
const stepData = ref<Record<string, unknown>>({})

const allSteps = computed<WizardStep[]>(() => [
  ...props.workflow.steps,
  { title: 'Confirm', fields: [] },
])

const isSummaryStep = computed(() => currentStep.value === props.workflow.steps.length)
const currentStepDef = computed(() => props.workflow.steps[currentStep.value])

watch(
  () => props.workflow,
  (wf) => {
    const defaults: Record<string, unknown> = {}
    for (const step of wf.steps) {
      for (const field of step.fields) {
        defaults[field.key] = field.default ?? (field.type === 'number' ? undefined : '')
      }
    }
    if (props.prefilled) {
      for (const [key, value] of Object.entries(props.prefilled)) {
        if (value !== undefined && value !== null) {
          defaults[key] = value
        }
      }
    }
    stepData.value = defaults
    currentStep.value = 0
  },
  { immediate: true },
)

const isCurrentStepValid = computed(() => {
  if (isSummaryStep.value) return true
  const step = currentStepDef.value
  if (!step) return false
  return step.fields.every((f) => {
    if (!f.required) return true
    const val = stepData.value[f.key]
    return val !== undefined && val !== null && val !== ''
  })
})

function allFields(): WizardField[] {
  return props.workflow.steps.flatMap((s) => s.fields)
}

const summaryItems = computed(() => {
  const data = props.readonly && props.submittedData ? props.submittedData : stepData.value
  return allFields().map((f) => {
    const raw = data[f.key]
    let displayValue = String(raw ?? '-')
    if (f.options) {
      const opt = f.options.find((o) => o.value === raw)
      if (opt) displayValue = opt.label
    }
    if (f.suffix && raw != null && raw !== '') {
      displayValue = `${raw} ${f.suffix}`
    }
    return { key: f.key, label: f.label, displayValue }
  })
})
</script>

<style scoped lang="scss">
.guided-wizard {
  background: linear-gradient(135deg, #fafbfc 0%, #fff 100%);
  border: 1.5px solid #e2e8f0;
  border-radius: 16px;
  padding: 20px;
  margin: 12px 0;
  max-width: 560px;
  box-shadow:
    0 4px 16px rgba(0, 0, 0, 0.08),
    0 8px 32px rgba(0, 0, 0, 0.04);
  position: relative;

  &::before {
    content: '';
    position: absolute;
    inset: 0 0 auto 0;
    height: 50%;
    border-radius: 16px 16px 0 0;
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.2) 0%, transparent 100%);
    pointer-events: none;
  }
}

.wizard-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
  position: relative;
  z-index: 1;

  .wizard-title {
    font-size: 16px;
    font-weight: 600;
    color: #1e293b;
  }
}

.wizard-steps {
  margin-bottom: 20px;
  position: relative;
  z-index: 1;
}

.wizard-step-desc {
  font-size: 13px;
  color: #64748b;
  margin-bottom: 16px;
  line-height: 1.5;
  position: relative;
  z-index: 1;
}

.wizard-fields {
  display: flex;
  flex-direction: column;
  gap: 14px;
  position: relative;
  z-index: 1;
}

.wizard-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
  position: relative;
  z-index: 1;

  :deep(.el-button) {
    border-radius: 10px;
    font-weight: 500;
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

.wizard-submitted {
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
}

// Dark mode
.dark .guided-wizard {
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

.dark .wizard-header .wizard-title {
  color: #e2e8f0;
}

.dark .wizard-step-desc {
  color: #94a3b8;
}

.dark .wizard-submitted {
  color: #34d399;
}
</style>

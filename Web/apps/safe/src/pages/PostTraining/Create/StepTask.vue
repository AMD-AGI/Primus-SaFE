<template>
  <el-form label-position="top">
    <el-form-item label="Framework">
      <el-select
        v-if="frameworks.length > 1"
        v-model="w.form.framework"
        style="width: 240px"
        @change="emit('changed', 'framework')"
      >
        <el-option v-for="f in frameworks" :key="f.name" :label="f.name" :value="f.name" />
      </el-select>
      <el-tooltip
        v-else
        content="More frameworks can be added to the registry later."
        placement="top"
      >
        <span>{{ w.form.framework || frameworks[0]?.name || 'slime' }}</span>
      </el-tooltip>
    </el-form-item>

    <el-form-item label="Training type" required>
      <el-segmented
        v-model="w.form.trainType"
        class="form-seg"
        :options="[
          { label: 'SFT', value: 'sft' },
          { label: 'RL', value: 'rl' },
        ]"
        @change="emit('changed', 'trainType')"
      />
    </el-form-item>

    <el-form-item label="Method" required>
      <el-segmented
        v-model="w.form.method"
        class="form-seg"
        :options="[
          { label: 'Full', value: 'full' },
          { label: 'LoRA', value: 'lora' },
        ]"
        @change="emit('changed', 'method')"
      />
      <div class="pt-help">
        Full trains every weight that is not frozen. LoRA trains small adapters on a frozen base.
      </div>
    </el-form-item>

    <el-form-item v-if="w.form.trainType === 'rl'" label="Algorithm" required>
      <el-segmented v-model="algorithm" class="form-seg" :options="algorithmOptions" />
      <div v-for="o in disabledAlgorithms" :key="o.value" class="pt-help">
        {{ o.label }}: {{ o.reason }}
      </div>
    </el-form-item>

    <el-form-item label="Base model" required :error="fieldError('modelId')">
      <el-select
        v-model="w.form.modelId"
        filterable
        placeholder="Select a base model"
        style="width: 420px"
        @change="emit('changed', 'modelId')"
      >
        <el-option-group label="Supported">
          <el-option
            v-for="m in supported"
            :key="m.modelId"
            :label="m.displayName"
            :value="m.modelId"
          >
            <span>{{ m.displayName }}</span>
            <span class="pt-option-note">{{ m.spec }}</span>
          </el-option>
        </el-option-group>
        <el-option-group v-if="unsupported.length" label="Not supported">
          <el-option
            v-for="m in unsupported"
            :key="m.modelId"
            :label="m.displayName"
            :value="m.modelId"
            disabled
          >
            <span>{{ m.displayName }}</span>
            <span class="pt-option-note">{{ m.reason }}</span>
          </el-option>
        </el-option-group>
      </el-select>
    </el-form-item>

    <el-form-item v-if="templates.length" label="Start from template">
      <el-select
        :model-value="w.form.templateId"
        placeholder="Pick a template of this workspace"
        clearable
        style="width: 420px"
        @change="(id: string) => id && emit('template', id)"
      >
        <el-option
          v-for="t in templates"
          :key="t.templateId"
          :label="`${t.name} · v${t.version}`"
          :value="t.templateId"
        />
      </el-select>
      <div class="pt-help">Applying a template counts as one change you can undo.</div>
    </el-form-item>

    <el-form-item v-if="presets.length" label="Preset">
      <div class="flex flex-wrap gap-2">
        <el-card
          v-for="p in presets"
          :key="p.id"
          shadow="never"
          class="pt-preset"
          :class="{ 'is-active': w.form.preset === p.id }"
          @click="emit('preset', p.id)"
        >
          <div class="font-500">{{ p.label }}</div>
        </el-card>
      </div>
      <div class="pt-help">
        A preset fills in starting values. Fields you have edited are kept, and you can undo it.
      </div>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  listPostTrainingTemplates,
  type TemplateItem,
  type TemplateListResp,
} from '@/services/postTraining'
import { useWizard } from './wizard'

const emit = defineEmits<{
  changed: [field: string]
  preset: [id: string]
  template: [id: string]
}>()
const w = useWizard()

const templates = ref<TemplateItem[]>([])

/** Loads the workspace's templates of the request's train type. */
const loadTemplates = async () => {
  try {
    const res = (await listPostTrainingTemplates({
      workspace: w.workspace,
      trainType: w.form.trainType,
    })) as unknown as TemplateListResp
    templates.value = res.items ?? []
  } catch {
    templates.value = []
  }
}
watch(() => w.form.trainType, loadTemplates, { immediate: true })

const frameworks = computed(() => w.config.value?.frameworks ?? [])
const supported = computed(() => (w.models.value?.items ?? []).filter((m) => m.supported))
const unsupported = computed(() => (w.models.value?.items ?? []).filter((m) => !m.supported))
const presets = computed(() =>
  (w.config.value?.options?.presets ?? []).filter((p) => p.trainType === w.form.trainType),
)

const algorithmOptions = computed(() =>
  w
    .fieldValues('advantageEstimator')
    .map((o) => ({ label: o.label, value: o.value, disabled: o.disabled })),
)
const disabledAlgorithms = computed(() =>
  w.fieldValues('advantageEstimator').filter((o) => o.disabled),
)
const algorithm = computed({
  get: () =>
    w.form.choices.advantageEstimator ??
    w.config.value?.options?.defaults.advantageEstimator ??
    'grpo',
  set: (v: string) => (w.form.choices.advantageEstimator = v),
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
.pt-preset {
  width: 220px;
  cursor: pointer;
}
.pt-preset.is-active {
  border-color: var(--safe-primary);
}
</style>

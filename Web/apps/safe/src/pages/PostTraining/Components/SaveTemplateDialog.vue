<template>
  <el-dialog
    :model-value="modelValue"
    :title="editing ? `Save template ${editing.name}` : 'Save as Template'"
    width="520px"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
    @open="onOpen"
  >
    <el-alert
      v-if="editing"
      type="info"
      :closable="false"
      class="mb-3"
      :title="`Saving creates v${editing.version + 1}. Runs created from v${editing.version} keep their recipe.`"
    />
    <el-form label-position="top">
      <el-form-item label="Name" required :error="nameError">
        <el-input v-model="form.name" maxlength="128" />
      </el-form-item>
      <el-form-item label="Description">
        <el-input v-model="form.description" placeholder="Shown in the template list" />
      </el-form-item>
      <el-form-item label="Applies to">
        <el-radio-group
          v-model="form.specOnly"
          :disabled="!!editing"
          class="flex flex-col items-start"
        >
          <el-radio v-if="spec" :value="true">Spec {{ spec }} only</el-radio>
          <el-radio :value="false">
            Any supported model
            <span class="text-[12px] text-[var(--safe-muted)]">
              parallel settings follow each model's spec
            </span>
          </el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item v-if="!editing" label="Also save">
        <div class="flex flex-col items-start">
          <el-checkbox v-model="form.model" :disabled="!modelLabel">
            Base model{{ modelLabel ? ` · ${modelLabel}` : '' }}
          </el-checkbox>
          <el-checkbox v-model="form.data" :disabled="!dataLabel">
            Datasets{{ dataLabel ? ` · ${dataLabel}` : '' }}
          </el-checkbox>
        </div>
      </el-form-item>
    </el-form>
    <div class="text-[12px] text-[var(--safe-muted)]">
      Saved to workspace {{ request.workspace }}. W&B settings keep only the secret reference. Every
      edit creates a new version.
    </div>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">Cancel</el-button>
      <el-button
        type="primary"
        :disabled="!form.name.trim() || !!nameError"
        :loading="saving"
        @click="save"
      >
        Save Template
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  createPostTrainingTemplate,
  listPostTrainingTemplates,
  updatePostTrainingTemplate,
  type SubmitRequest,
  type TemplateItem,
  type TemplateListResp,
} from '@/services/postTraining'
import { templateContent } from '../Create/wizard'

const props = defineProps<{
  modelValue: boolean
  /** The request the template keeps, from the wizard or a run's recipe snapshot. */
  request: SubmitRequest
  /** The spec the request's model matches. */
  spec?: string
  modelLabel?: string
  dataLabel?: string
  sourceRunId?: string
  /** The template being edited; without it a new template is created. */
  editing?: TemplateItem
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; saved: [version: number] }>()

const form = reactive({ name: '', description: '', specOnly: false, model: false, data: false })
const saving = ref(false)
const takenNames = ref<string[]>([])

const nameError = computed(() =>
  takenNames.value.includes(form.name.trim()) ? 'A template with this name already exists.' : '',
)

/** Resets the form and loads the names already taken in the workspace. */
const onOpen = async () => {
  const e = props.editing
  Object.assign(form, {
    name: e?.name ?? props.request.displayName ?? '',
    description: e?.description ?? '',
    specOnly: e ? !!e.spec : !!props.spec,
    model: !!e?.content.modelId,
    data: !!e?.content.datasetId,
  })
  try {
    const res = (await listPostTrainingTemplates({
      workspace: props.request.workspace,
    })) as unknown as TemplateListResp
    takenNames.value = (res.items ?? [])
      .filter((t) => t.templateId !== e?.templateId)
      .map((t) => t.name)
  } catch {
    takenNames.value = []
  }
}

const save = async () => {
  const r = props.request
  const e = props.editing
  const content = templateContent(
    r,
    e ? { model: !!e.content.modelId, data: !!e.content.datasetId } : form,
  )
  saving.value = true
  try {
    if (e) {
      const res = (await updatePostTrainingTemplate(e.templateId, {
        name: form.name.trim(),
        description: form.description,
        content,
      })) as unknown as { version: number }
      ElMessage.success(`Template saved · v${res.version}`)
      emit('saved', res.version)
    } else {
      await createPostTrainingTemplate({
        workspace: r.workspace,
        name: form.name.trim(),
        description: form.description,
        trainType: r.trainType,
        method: r.method,
        framework: r.framework,
        spec: form.specOnly ? props.spec : '',
        content,
        sourceRunId: props.sourceRunId,
      })
      ElMessage.success('Template saved · v1')
      emit('saved', 1)
    }
    emit('update:modelValue', false)
  } finally {
    saving.value = false
  }
}
</script>

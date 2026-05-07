<template>
  <el-dialog
    v-model="visible"
    title="Edit Model"
    :close-on-click-modal="false"
    width="600"
    destroy-on-close
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="auto"
      class="p-x-4"
    >
      <el-form-item label="Display Name" prop="displayName">
        <el-input v-model="form.displayName" />
      </el-form-item>

      <el-form-item label="Description">
        <el-input v-model="form.description" type="textarea" :rows="2" />
      </el-form-item>

      <el-form-item label="Model Name">
        <el-input v-model="form.modelName" placeholder="e.g. org/model-name" />
      </el-form-item>

      <el-form-item label="Icon URL">
        <el-input v-model="form.icon" placeholder="https://..." />
      </el-form-item>

      <el-form-item label="Label">
        <el-input v-model="form.label" placeholder="e.g. team-name" />
      </el-form-item>

      <el-form-item label="Tags">
        <div class="flex gap-2 flex-wrap">
          <el-tag
            v-for="tag in form.tags"
            :key="tag"
            closable
            type="primary"
            effect="plain"
            @close="handleTagRemove(tag)"
          >{{ tag }}</el-tag>
          <el-input
            v-if="tagInputVisible"
            ref="tagInputRef"
            v-model="tagInputValue"
            style="width: 80px"
            size="small"
            @keyup.enter="handleTagConfirm"
            @blur="handleTagConfirm"
          />
          <el-button v-else size="small" @click="showTagInput">+ New Tag</el-button>
        </div>
      </el-form-item>

      <el-form-item label="Max Tokens">
        <el-input-number v-model="form.maxTokens" :min="1" :max="1048576" style="width: 200px" />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">Save</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules, InputInstance } from 'element-plus'
import { patchModel } from '@/services/playground'
import type { PlaygroundModel, PatchModelPayload } from '@/services/playground'

const props = defineProps<{
  visible: boolean
  model: PlaygroundModel | null
}>()

const emit = defineEmits(['update:visible', 'success'])

const formRef = ref<FormInstance>()
const submitting = ref(false)
const tagInputVisible = ref(false)
const tagInputValue = ref('')
const tagInputRef = ref<InputInstance>()

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val),
})

const form = reactive({
  displayName: '',
  description: '',
  modelName: '',
  icon: '',
  label: '',
  tags: [] as string[],
  maxTokens: undefined as number | undefined,
})

const rules: FormRules = {
  displayName: [{ required: true, message: 'Display name is required', trigger: 'blur' }],
}

watch(() => props.visible, (v) => {
  if (v && props.model) {
    form.displayName = props.model.displayName || ''
    form.description = props.model.description || ''
    form.modelName = ''
    form.icon = props.model.icon || ''
    form.label = props.model.label || ''
    form.tags = props.model.tags ? props.model.tags.split(',').map((t) => t.trim()).filter(Boolean) : []
    form.maxTokens = (props.model as any).maxTokens || undefined
  }
})

const showTagInput = () => {
  tagInputVisible.value = true
  nextTick(() => tagInputRef.value?.input?.focus())
}

const handleTagConfirm = () => {
  const val = tagInputValue.value.trim()
  if (val && !form.tags.includes(val)) {
    form.tags.push(val)
  }
  tagInputVisible.value = false
  tagInputValue.value = ''
}

const handleTagRemove = (tag: string) => {
  form.tags = form.tags.filter((t) => t !== tag)
}

const handleSubmit = async () => {
  try {
    await formRef.value?.validate()
  } catch {
    return
  }

  if (!props.model) return

  submitting.value = true
  try {
    const payload: PatchModelPayload = {}
    if (form.displayName !== props.model.displayName) payload.displayName = form.displayName
    if (form.description !== (props.model.description || '')) payload.description = form.description
    if (form.modelName) payload.modelName = form.modelName
    if (form.icon !== (props.model.icon || '')) payload.icon = form.icon
    if (form.label !== (props.model.label || '')) payload.label = form.label
    payload.tags = form.tags
    if (form.maxTokens) payload.maxTokens = form.maxTokens

    await patchModel(props.model.id, payload)
    ElMessage.success('Model updated')
    emit('success')
    handleClose()
  } catch (e: any) {
    ElMessage.error(e?.message || 'Failed to update model')
  } finally {
    submitting.value = false
  }
}

const handleClose = () => {
  visible.value = false
}
</script>

<style scoped>
.flex { display: flex; }
.gap-2 { gap: 8px; }
.flex-wrap { flex-wrap: wrap; }
.p-x-4 { padding: 0 16px; }
</style>

<template>
  <el-dialog
    :model-value="visible"
    title="Create SSH Key "
    width="600"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      label-width="auto"
      style="max-width: 600px"
      class="p-5"
      :rules="rules"
    >
      <el-form-item label="Label" prop="description">
        <el-input v-model="form.description" maxlength="50" show-word-limit />
      </el-form-item>

      <el-form-item label="Public Key" prop="publicKey">
        <el-input v-model="form.publicKey" :rows="8" type="textarea" />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" @click="onSubmit(ruleFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, reactive, ref, computed, nextTick } from 'vue'
import { addPublickey } from '@/services'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'

defineProps<{
  visible: boolean
}>()
const emit = defineEmits(['update:visible', 'success'])

const initialForm = () => ({
  description: '',
  publicKey: '',
})
const form = reactive({ ...initialForm() })

const ruleFormRef = ref<FormInstance>()
const rules = computed<FormRules>(() => ({
  description: [{ required: true, message: 'Please input description', trigger: 'change' }],
  publicKey: [{ required: true, message: 'Please input publicKey', trigger: 'change' }],
}))

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()
    await addPublickey(form)
    ElMessage({ message: 'Create successful', type: 'success' })

    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey as any)
      ElMessage.error(firstMsg)
    }
  }
}

const onOpen = async () => {
  ruleFormRef.value?.resetFields()
  Object.assign(form, initialForm())
  await nextTick()
}
</script>

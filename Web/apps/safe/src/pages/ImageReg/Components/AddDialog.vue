<template>
  <el-dialog
    :model-value="visible"
    :title="`${props.action} Registry`"
    width="500"
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
      <el-form-item label="Name" prop="name">
        <el-input v-model="form.name" />
      </el-form-item>

      <el-form-item label="URL" prop="url">
        <el-input v-model="form.url" />
      </el-form-item>

      <el-form-item label="User Name" prop="username">
        <el-input v-model="form.username" />
      </el-form-item>

      <el-form-item label="Password" prop="password">
        <el-input v-model="form.password" />
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
import type { RegisterReq, UserSelfData } from '@/services'
import { addImageReg, editImageReg } from '@/services'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'

interface RegData {
  id: number
  name: string
  url: string
  username: string
}
const props = defineProps<{
  visible: boolean
  action: string
  regData?: RegData
}>()
const emit = defineEmits(['update:visible', 'success'])
const isEdit = computed(() => props.action === 'Edit')

const initialForm = () => ({
  name: '',
  username: '',
  url: '',
  password: '',
})
const form = reactive({ ...initialForm() })

const ruleFormRef = ref<FormInstance>()
const rules = computed<FormRules<RegisterReq>>(() => ({
  name: [{ required: true, message: 'Please input registry name', trigger: 'change' }],
  username: [{ required: true, message: 'Please input username', trigger: 'change' }],
  url: [{ required: true, message: 'Please input url', trigger: 'change' }],
  password: [{ required: !isEdit.value, message: 'Please input password', trigger: 'blur' }],
}))

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()

    if (!isEdit.value) {
      await addImageReg({
        ...form,
      })
      ElMessage({ message: 'Create successful', type: 'success' })
    } else {
      if (!props.regData) return
      const { password, ...editPayload } = form
      await editImageReg(props.regData.id, {
        ...editPayload,
        ...(form.password ? { password: form.password } : {}),
      })
      ElMessage({ message: 'Edit successful', type: 'success' })
    }

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

// Populate form for editing
const setInitialFormValues = async () => {
  if (!props.regData) return

  form.name = props.regData.name
  form.username = props.regData.username
  form.url = props.regData.url
  form.password = ''
}

const onOpen = async () => {
  if (isEdit.value) {
    setInitialFormValues()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, initialForm())
  }
  await nextTick()
}
</script>

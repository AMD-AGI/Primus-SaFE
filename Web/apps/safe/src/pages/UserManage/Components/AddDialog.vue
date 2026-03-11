<template>
  <el-dialog
    :model-value="visible"
    :title="`${props.action} User`"
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
      <el-form-item label="ID" props="id" v-if="isEdit">
        <el-input v-model="form.id" :disabled="isEdit" />
      </el-form-item>

      <el-form-item label="Name" prop="name">
        <el-input v-model="form.name" :disabled="isEdit" />
      </el-form-item>

      <el-form-item label="Password" prop="password">
        <el-input v-model="form.password" />
      </el-form-item>

      <el-form-item label="Email" prop="email" v-if="isEdit">
        <el-input v-model="form.email" />
      </el-form-item>
      <el-form-item label="Roles" prop="roles" v-if="isEdit">
        <el-select v-model="form.roles" multiple clearable>
          <el-option label="default" value="default" />
          <el-option label="system-admin" value="system-admin" />
          <el-option label="system-admin-readonly" value="system-admin-readonly" />
        </el-select>
      </el-form-item>
      <el-form-item label="Restricted Type" prop="restrictedType" v-if="isEdit">
        <el-switch
          v-model="form.restrictedType"
          :active-value="1"
          :inactive-value="0"
          class="mr-2"
        />
        {{ form.restrictedType ? 'frozen' : 'normal' }}
      </el-form-item>

      <el-form-item label="Workspaces" prop="workspaces" v-if="isEdit">
        <el-select v-model="form.workspaces" multiple>
          <el-option
            v-for="ws in wsStore.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
      </el-form-item>
      <!--
      <el-form-item label="Managed Workspaces" prop="managedWorkspaces">
        <el-select v-model="form.managedWorkspaces" multiple>
          <el-option
            v-for="ws in wsStore.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
      </el-form-item> -->
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
import { register, editUser } from '@/services'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{
  visible: boolean
  action: string
  user?: UserSelfData
}>()
const emit = defineEmits(['update:visible', 'success'])
const isEdit = computed(() => props.action === 'Edit')

const wsStore = useWorkspaceStore()

const initialForm = () => ({
  id: '',
  name: '',
  type: 'default',
  workspaces: [] as string[],
  password: '',

  managedWorkspaces: [] as string[],
  roles: [''],
  email: '',
  restrictedType: 0,
  creationTime: '',
})
const form = reactive({ ...initialForm() })

const ruleFormRef = ref<FormInstance>()
const rules = computed<FormRules<RegisterReq>>(() => ({
  id: [{ required: true, message: 'Please input id', trigger: 'change' }],
  name: [{ required: !isEdit.value, message: 'Please input user name', trigger: 'change' }],
  password: [{ required: !isEdit.value, message: 'Please input password', trigger: 'blur' }],
}))

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()

    if (!isEdit.value) {
      const {
        managedWorkspaces,
        roles,
        email,
        restrictedType,
        creationTime,
        id,
        workspaces,
        ...baseForm
      } = form
      await register({
        ...baseForm,
      })
      ElMessage({ message: 'Create successful', type: 'success' })
    } else {
      const { id, name, type, creationTime, managedWorkspaces, password, ...baseEditForm } = form
      if (!props.user) return
      await editUser(form.id, {
        ...baseEditForm,
        ...(password ? { password } : {}),
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
  if (!props.user) return

  form.id = props.user.id
  form.name = props.user.name
  form.roles = props.user.roles
  form.workspaces = props.user.workspaces?.map((item) => item.id) || []
  form.email = props.user.email
  form.restrictedType = props.user.restrictedType
  form.type = props.user.type
  form.creationTime = props.user.creationTime

  form.password = ''
}
// Pre-select the default workspace when creating
const fillDefaults = () => {
  form.workspaces = wsStore.items.filter((w) => w.isDefault).map((w) => w.workspaceId)
}

const onOpen = async () => {
  if (isEdit.value) {
    setInitialFormValues()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, initialForm())
    fillDefaults()
  }
  await nextTick()
}
</script>

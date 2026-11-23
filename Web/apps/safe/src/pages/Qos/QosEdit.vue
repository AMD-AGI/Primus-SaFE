<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">{{ title }}</el-text>
    <el-text class="block mt-2" type="info">
      {{
        cloneFrom
          ? `Copied from ${cloneFrom}. Select the workspace this ceiling applies to.`
          : 'Resource totals and timeouts for one workspace. Memory is in Gi. Personal values are set on a user.'
      }}
    </el-text>
  </div>
  <el-card class="mt-6 safe-card" shadow="never" v-loading="loading">
    <el-form ref="formRef" :model="form" :rules="rules" label-width="140px">
      <el-form-item label="Workspace" prop="workspace" :required="isCreate">
        <el-select
          v-if="isCreate"
          v-model="form.workspace"
          filterable
          placeholder="Select a workspace"
          style="width: 280px"
        >
          <el-option
            v-for="item in availableWorkspaces"
            :key="item.workspaceId"
            :label="item.workspaceName || item.workspaceId"
            :value="item.workspaceId"
          />
        </el-select>
        <el-text v-else>{{ form.workspace }}</el-text>
      </el-form-item>
    </el-form>
    <QosScopeForm v-model="draft" />
    <div class="mt-4">
      <el-button type="primary" :loading="saving" @click="onSubmit">
        {{ isCreate ? 'Create' : 'Save' }}
      </el-button>
      <el-button @click="router.push('/qos')">Cancel</el-button>
    </div>
  </el-card>
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import { createQos, getQos, listQos, updateQos } from '@/services/qos'
import { draftToScope, emptyDraft, scopeToDraft, type ScopeDraft } from '@/services/qos/type'
import QosScopeForm from './QosScopeForm.vue'

const route = useRoute()
const router = useRouter()
const workspaceStore = useWorkspaceStore()

const isCreate = computed(() => route.path.endsWith('/create'))
const cloneFrom = computed(() => (isCreate.value ? String(route.query.from ?? '') : ''))
const title = computed(() => {
  if (cloneFrom.value) return 'Clone QoS'
  return isCreate.value ? 'Create QoS' : 'Edit QoS'
})
const loading = ref(false)
const saving = ref(false)
const formRef = ref<FormInstance>()
const form = reactive({ workspace: '' })
const rules: FormRules = {
  workspace: [{ required: true, message: 'Select a workspace', trigger: 'change' }],
}
const taken = ref<string[]>([])
const draft = ref<ScopeDraft>(emptyDraft())

const availableWorkspaces = computed(() =>
  workspaceStore.items.filter((item) => item.workspaceId && !taken.value.includes(item.workspaceId)),
)

/** load prepares an empty create form, a clone with no workspace, or the ceiling being edited. */
const load = async () => {
  loading.value = true
  try {
    if (!workspaceStore.isFetched) await workspaceStore.fetchWorkspace()
    if (isCreate.value) {
      const res = await listQos()
      taken.value = (res.items ?? []).map((item) => item.name)
      form.workspace = ''
      draft.value = cloneFrom.value ? scopeToDraft(await getQos(cloneFrom.value)) : emptyDraft()
      if (!cloneFrom.value) form.workspace = availableWorkspaces.value[0]?.workspaceId ?? ''
      return
    }
    const name = String(route.query.name ?? '')
    if (!name) {
      router.replace('/qos')
      return
    }
    form.workspace = name
    draft.value = scopeToDraft(await getQos(name))
  } finally {
    loading.value = false
  }
}

/** onSubmit creates a ceiling or replaces the one being edited. */
const onSubmit = async () => {
  if (isCreate.value) {
    const valid = await formRef.value?.validate().catch(() => false)
    if (!valid) return
  }
  if (!form.workspace) return
  saving.value = true
  try {
    const body = draftToScope(draft.value) ?? {}
    if (isCreate.value) {
      await createQos(form.workspace, body)
      ElMessage.success('Created')
    } else {
      await updateQos(form.workspace, body)
      ElMessage.success('Saved')
    }
    router.push('/qos')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

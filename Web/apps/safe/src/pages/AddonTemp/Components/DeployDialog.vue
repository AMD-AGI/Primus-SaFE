<template>
  <el-dialog
    :model-value="visible"
    :title="`${props.action} Addon`"
    width="800"
    :close-on-click-modal="false"
    destroy-on-close
    @close="emit('update:visible', false)"
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      :rules="rules"
      label-width="auto"
      class="p-5"
      style="max-width: 800px"
    >
      <!-- Section: Basic Information -->
      <div class="flex items-center m-b-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Basic Information</span>
      </div>

      <el-form-item label="Addon Template" v-if="props.id">
        <div class="flex items-center gap-2">
          <el-tag type="info">{{ form.template || '-' }}</el-tag>
          <el-tag v-if="detail?.version" type="success">v{{ detail?.version }}</el-tag>
        </div>
      </el-form-item>

      <el-form-item label="Cluster">
        <el-text type="primary">
          {{ isEdit ? curCluster || '-' : clusterStore.currentClusterId || '-' }}
        </el-text>
      </el-form-item>

      <el-form-item label="Template" prop="template" v-if="props.name">
        <el-select v-model="form.template">
          <el-option v-for="v in tempOptions" :key="v" :label="v" :value="v" />
        </el-select>
      </el-form-item>

      <el-form-item label="Release Name" prop="releaseName">
        <el-input
          v-model="form.releaseName"
          :disabled="isEdit"
          placeholder="e.g. gpu-driver"
          clearable
        />
      </el-form-item>

      <el-form-item label="Namespace">
        <el-input
          v-model="form.namespace"
          :disabled="isEdit"
          placeholder="Input a namespace"
          clearable
        />
      </el-form-item>

      <el-form-item label="Description">
        <el-input v-model="form.description" :rows="2" type="textarea" clearable />
      </el-form-item>

      <!-- Section: values.yaml -->
      <div class="flex items-center m-b-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">YAML</span>
      </div>

      <el-form-item label="values">
        <div class="w-full">
          <div class="flex items-center justify-between mb-2">
            <el-text type="info">Helm values (YAML). Leave empty to use template defaults.</el-text>
            <div class="flex gap-2">
              <el-button link type="primary" @click="resetValues">Reset</el-button>
            </div>
          </div>
          <el-input v-model="form.values" type="textarea" :rows="20" placeholder="# key: value" />
        </div>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :loading="submitting" @click="onSubmit(ruleFormRef)">
          Confirm
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, reactive, ref, nextTick, computed } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import { useClusterStore } from '@/stores/cluster'
import {
  createAddon,
  getAddontempDetail,
  type AddonTemplateDetail,
  getAddonDetail,
  type AddonDetailData,
  getAddontemps,
  type AddonTemp,
  editAddon,
} from '@/services'

const clusterStore = useClusterStore()

const props = defineProps<{
  visible: boolean
  id?: string
  name?: string
  action: string
}>()

const isEdit = computed(() => props.action === 'Edit')
const detail = ref<any>(null)
const tempOptions = ref([] as string[])
const curCluster = ref('')

const emit = defineEmits(['update:visible', 'success'])

/** Form */
const form = reactive({
  releaseName: '',
  template: '',
  namespace: 'default',
  values: '',
  description: '',
})

/** Validation */
// const nameRegex = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/
const rules: FormRules = {
  releaseName: [{ required: !isEdit.value, message: 'Please input release name', trigger: 'blur' }],
  template: [
    {
      required: true,
      message: 'Please select template',
      trigger: 'change',
    },
  ],
}

const ruleFormRef = ref<FormInstance>()
const submitting = ref(false)

function resetValues() {
  form.values = detail.value?.helmStatus?.valuesYaml ?? detail.value?.helmDefaultValues ?? ''
  ElMessage.success('Reset to default values')
}

const fetchTemps = async () => {
  const res = await getAddontemps({}).catch(() => ({ items: [] }))
  tempOptions.value = (res?.items ?? []).map((n: AddonTemp) => n.addonTemplateId)
}

const onOpen = async () => {
  // Reset form
  ruleFormRef.value?.resetFields?.()
  form.releaseName = ''
  form.template = ''
  form.namespace = 'default'
  form.values = ''
  form.description = ''

  if (props.name && isEdit.value) {
    // Edit
    fetchTemps()
    try {
      const res = (await getAddonDetail(
        clusterStore.currentClusterId ?? '',
        props.name,
      )) as AddonDetailData
      detail.value = res

      form.template = res.template
      form.namespace = res.namespace ?? 'default'
      form.values = res.values ?? ''
      form.releaseName = res.releaseName ?? ''
      form.description = res.description ?? ''
      curCluster.value = res.cluster
    } catch (e) {
      ElMessage.error((e as Error).message || 'Failed to load addon detail')
    }
  } else if (props.id) {
    // Create from template
    try {
      const res = (await getAddontempDetail(props.id)) as AddonTemplateDetail
      detail.value = res

      form.template = res.addonTemplateId || ''
      form.namespace = res.helmDefaultNamespace || 'default'
      form.values = res.helmStatus?.valuesYaml ?? res.helmDefaultValues ?? ''
    } catch (e) {
      ElMessage.error((e as Error).message || 'Failed to load template detail')
    }
  }

  await nextTick()
}

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return

  try {
    await formEl.validate()

    submitting.value = true

    const BasePayload = {
      template: form.template,
      values: form.values || undefined,
      description: form.description || undefined,
    }
    if (!isEdit.value) {
      await createAddon(clusterStore.currentClusterId ?? '', {
        ...BasePayload,
        releaseName: form.releaseName,
        namespace: form.namespace || undefined,
      })
      ElMessage.success('Addon created')
    } else {
      await editAddon(curCluster.value, props.name!, BasePayload)
      ElMessage.success('Addon edited')
    }

    emit('update:visible', false)
    emit('success')
  } catch (err: any) {
    const msg = err?.response?.data?.message || err?.message || 'Failed to deploy'
    ElMessage.error(msg)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>

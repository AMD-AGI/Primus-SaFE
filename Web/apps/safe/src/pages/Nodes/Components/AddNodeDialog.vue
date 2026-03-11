<template>
  <el-dialog
    :model-value="visible"
    :title="`${props.action} Node`"
    width="600"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      label-width="auto"
      style="max-width: 600px"
      class="p-5"
      :rules="rules"
    >
      <el-form-item label="Node Flavor" prop="flavorId" v-if="isManager">
        <el-select v-model="form.flavorId" placeholder="please select flavor name">
          <el-option v-for="item in state.flavorOptions" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>
      <el-form-item label="SSH Secret" prop="sshSecretId" v-if="!isEdit && isManager">
        <el-select v-model="form.sshSecretId" placeholder="please select ssh secret">
          <el-option v-for="item in state.secretOptions" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>
      <el-form-item label="Template" v-if="isManager">
        <el-select v-model="form.templateId" placeholder="please select template">
          <el-option v-for="item in state.tempOptions" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>

      <el-form-item label="Hostname" prop="hostname" v-if="!isEdit && isManager">
        <el-input v-model="form.hostname" />
      </el-form-item>
      <el-form-item label="Private IP" prop="privateIP" v-if="isManager">
        <el-input v-model="form.privateIP" />
      </el-form-item>
      <el-form-item label="Port" v-if="isManager">
        <el-input v-model="form.port" />
      </el-form-item>

      <el-form-item label="Labels">
        <KeyValueList
          v-model="form.labelList"
          keyMode="input"
          :max="20"
          info="Add up to 20 labels"
          :validate="true"
        />
      </el-form-item>

      <el-form-item label="Taints" v-if="isEdit">
        <KeyValueList
          v-model="taintsList"
          :KeyOptions="taintOptions"
          keyMode="select"
          :max="10"
          info="Add up to 10 taints"
          :valuePlaceholderFromKey="true"
          :validate="true"
        />
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
import { defineProps, defineEmits, reactive, onMounted, ref, computed, watch } from 'vue'
import {
  getNodeTemps,
  getNodeFlavors,
  getSecrets,
  addNode,
  getNodeDetail,
  editNode,
} from '@/services/nodes/index'
import {
  type TemplateOptionsType,
  type SecretOptionsType,
  type FlavorOptionsType,
  type TaintEffects,
  type Taint,
  type TaintListItem,
  type KeyValueOption,
  taintOptions,
} from '@/services'
import { type FormInstance, type FormRules, ElMessage } from 'element-plus'
import KeyValueList from '@/components/Base/KeyValueList.vue'
import { toKVList } from '@/utils'
import { useUserStore } from '@/stores/user'

const userStore = useUserStore()
const isManager = computed(() => userStore.isManager) // Manager role

interface NodeForm {
  flavorId: string
  sshSecretId: string
  templateId: string
  hostname: string
  privateIP: string
  port: number
  labelList: KeyValueOption[]
}

const props = defineProps<{
  visible: boolean
  action: string
  nodeid: string
}>()
const emit = defineEmits(['update:visible', 'success'])
const isEdit = computed(() => props.action === 'Edit')

const state = reactive({
  tempOptions: [],
  flavorOptions: [] as string[],
  secretOptions: [],
})
const form = reactive({
  flavorId: '',
  sshSecretId: '',
  templateId: '',
  hostname: '',
  privateIP: '',
  port: 22,
  labelList: [
    {
      key: '',
      value: '',
    },
  ],
})
const taintsList = ref([
  {
    key: 'NoSchedule' as TaintEffects,
    value: '',
  },
])

// Stores original taints' timeAdded info, keyed by _uid
const taintsTimeMap = ref<Record<string, string>>({})

const ipv4Regex = /^(25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d{2}|[1-9]?\d)){3}$/

const ruleFormRef = ref<FormInstance>()
const rules = reactive<FormRules<NodeForm>>({
  flavorId: [{ required: true, message: 'Please select flavor name', trigger: 'change' }],
  sshSecretId: [{ required: true, message: 'Please select ssh secret', trigger: 'change' }],
  hostname: [
    { required: true, message: 'Please input activity name', trigger: 'blur' },
    { max: 64, message: 'Must be less than 64 characters', trigger: 'blur' },
  ],
  privateIP: [
    { required: true, message: 'Please input private ip', trigger: 'blur' },
    {
      pattern: ipv4Regex,
      message: 'Please enter a valid IPv4 address',
      trigger: 'blur',
    },
  ],
})

const getOptions = async () => {
  try {
    const [temps, flavors, secrets] = await Promise.all([
      getNodeTemps(),
      getNodeFlavors(),
      getSecrets({ type: 'ssh' }),
    ])
    return {
      temps,
      flavors,
      secrets,
    }
  } catch (err) {
    console.error(err)
  }
}

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()
    const labels = Object.fromEntries(
      form.labelList.filter((tag) => tag.key && tag.value).map((tag) => [tag.key, tag.value]),
    )
    // Edit
    if (isEdit.value) {
      const taintsPayload = (taintsList.value || [])
        .filter((t) => t.key && t.value)
        .map((t: TaintListItem) => {
          const payload: Taint = { effect: t.key, key: t.value }
          // Look up original timeAdded via _uid
          const timeAdded = t._uid ? taintsTimeMap.value[t._uid] : undefined
          if (timeAdded) {
            payload.timeAdded = timeAdded
          }
          return payload
        })

      await editNode(props.nodeid, {
        flavorId: form.flavorId,
        templateId: form.templateId,
        port: form.port,
        labels,
        taints: taintsPayload,
        privateIP: form.privateIP,
      })
      ElMessage.success('Edit successful')
    } else {
      // Add new node
      await addNode({
        flavorId: form.flavorId,
        sshSecretId: form.sshSecretId,
        templateId: form.templateId,
        hostname: form.hostname,
        privateIP: form.privateIP,
        port: form.port,
        labels,
      })
      ElMessage.success('Create successful')
    }

    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey as keyof NodeForm)
      ElMessage.error(firstMsg)
    }
  }
}

const setInitialFormValues = async () => {
  if (!props.nodeid) return

  const res = await getNodeDetail(props.nodeid)

  form.labelList = toKVList(res?.customerLabels)
  form.port = res.port
  form.templateId = res.templateId
  form.privateIP = res.internalIP
  form.flavorId = res.flavorId

  // Clear previous mapping
  taintsTimeMap.value = {}

  taintsList.value = res.taints?.map((v: Taint) => {
    const uid = `${Date.now()}-${Math.random().toString(36).slice(2)}`
    // If timeAdded exists, save it to the mapping
    if (v.timeAdded) {
      taintsTimeMap.value[uid] = v.timeAdded
    }
    return {
      key: v.effect as TaintEffects,
      value: v.key,
      _uid: uid,
    }
  }) || [
    {
      key: 'NoSchedule' as TaintEffects,
      value: '',
    },
  ]
}

onMounted(async () => {
  const { temps, flavors, secrets } = (await getOptions()) ?? {}
  state.tempOptions = temps?.items?.map((item: TemplateOptionsType) => item.templateId)
  state.flavorOptions = flavors?.items?.map((item: FlavorOptionsType) => item.flavorId) || []
  state.secretOptions = secrets?.items?.map((item: SecretOptionsType) => item.secretId)

  form.templateId = state.tempOptions?.[0] ?? ''
  form.flavorId = state.flavorOptions?.[0] ?? ''
  form.sshSecretId = state.secretOptions?.[0] ?? ''
})

watch(
  () => props.visible,
  () => {
    taintsList.value = [
      {
        key: 'NoSchedule' as TaintEffects,
        value: '',
      },
    ]
    taintsTimeMap.value = {} // Clear mapping
    if (props.visible && isEdit) {
      setInitialFormValues()
      return
    }
  },
)
</script>

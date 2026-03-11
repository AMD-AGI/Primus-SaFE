<template>
  <el-dialog
    :model-value="visible"
    :title="`${props.action} Serect`"
    width="600"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      :rules="rules"
      label-width="auto"
      style="max-width: 600px"
      class="p-5"
    >
      <el-form-item label="Name" prop="name">
        <el-input v-model="form.name" :disabled="isEdit" />
      </el-form-item>

      <el-form-item label="Type" prop="type">
        <el-segmented
          v-model="form.type"
          :options="availableTypes"
          :disabled="isEdit"
          @change="onTypeChange"
          class="secret-type-segmented"
        />
      </el-form-item>

      <el-form-item label="Workspaces" prop="workspaceIds">
        <el-select
          v-model="form.workspaceIds"
          multiple
          placeholder="Select workspaces"
          :disabled="!userStore.isManager"
        >
          <el-option
            v-for="ws in wsStore.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
      </el-form-item>

      <!-- Labels - common field, available for all types -->
      <el-form-item label="Labels">
        <KeyValueList
          v-model="form.labelList"
          keyMode="input"
          :max="20"
          info="Add up to 20 labels"
          :validate="true"
        />
      </el-form-item>

      <!-- ssh: only 1 param entry, cannot add or remove -->
      <template v-if="form.type === 'ssh'">
        <el-alert
          type="info"
          :closable="false"
          class="mb-2"
          show-icon
          title="SSH supports only one entry"
        />
        <el-form-item :label="'User Name'" :prop="`params.0.username`">
          <el-input v-model="form.params[0].username" />
        </el-form-item>

        <el-form-item label="Private Key" :prop="`params.0.privateKey`">
          <el-input v-model="form.params[0].privateKey" type="textarea" :rows="8" />
        </el-form-item>

        <el-form-item label="Public Key" :prop="`params.0.publicKey`">
          <el-input v-model="form.params[0].publicKey" type="textarea" :rows="8" />
        </el-form-item>
      </template>

      <!-- image: params can be added/removed -->
      <template v-else-if="form.type === 'image'">
        <div class="flex items-center justify-between mb-2">
          <span class="text-sm opacity-70">Image params (multiple)</span>
          <el-button type="primary" size="small" @click="addImageParam">Add</el-button>
        </div>

        <el-card
          v-for="(p, idx) in form.params"
          :key="idx"
          class="mb-3"
          shadow="never"
          body-class="pt-3"
        >
          <div class="flex items-start justify-between mb-2">
            <div class="text-sm font-medium">Item #{{ idx + 1 }}</div>
            <el-button
              v-if="form.params.length > 1"
              type="danger"
              size="small"
              text
              @click="removeImageParam(idx)"
            >
              Remove
            </el-button>
          </div>

          <el-form-item :label="'User Name'" :prop="`params.${idx}.username`">
            <el-input v-model="p.username" />
          </el-form-item>

          <el-form-item :label="'Server'" :prop="`params.${idx}.server`">
            <el-input v-model="p.server" placeholder="e.g. registry.example.com" />
          </el-form-item>

          <el-form-item :label="'Password'" :prop="`params.${idx}.password`">
            <el-input v-model="p.password" show-password />
          </el-form-item>
        </el-card>
      </template>

      <!-- general: params are objects, converted to key:value format -->
      <template v-else-if="form.type === 'general'">
        <div class="flex items-center justify-between mb-2">
          <div class="flex items-center gap-1">
            <span class="text-sm opacity-70">General params (key-value pairs)</span>
            <el-tooltip content="If creating PAT, the key should be github_token" placement="top">
              <el-icon size="14" color="#909399">
                <QuestionFilled />
              </el-icon>
            </el-tooltip>
          </div>
          <el-button type="primary" size="small" @click="addGeneralParam">Add</el-button>
        </div>

        <el-card
          v-for="(item, idx) in generalParamsList"
          :key="item._uid"
          class="mb-3"
          shadow="never"
          body-class="pt-3"
        >
          <div class="flex items-start justify-between mb-2">
            <div class="text-sm font-medium">Item #{{ idx + 1 }}</div>
            <el-button
              v-if="generalParamsList.length > 1"
              type="danger"
              size="small"
              text
              @click="removeGeneralParam(idx)"
            >
              Remove
            </el-button>
          </div>

          <el-form-item :label="'Key'">
            <el-input v-model="item.key" placeholder="e.g. API_KEY" />
          </el-form-item>

          <el-form-item :label="'Value'">
            <el-input v-model="item.value" type="textarea" :rows="3" placeholder="Enter value" />
          </el-form-item>
        </el-card>
      </template>

      <!--
      <el-form-item label="User Name" prop="params.username">
        <el-input v-model="form.params.username" />
      </el-form-item>

      <template v-if="form.type === 'ssh'">
        <el-form-item label="Private Key" prop="params.privateKey">
          <el-input v-model="form.params.privateKey" :rows="8" type="textarea" />
        </el-form-item>

        <el-form-item label="Public Key" prop="params.publicKey">
          <el-input v-model="form.params.publicKey" :rows="8" type="textarea" />
        </el-form-item>
      </template>
      <template v-else>
        <el-form-item label="Server" prop="params.server">
          <el-input v-model="form.params.server" />
        </el-form-item>

        <el-form-item label="Password" prop="params.password">
          <el-input v-model="form.params.password" />
        </el-form-item>
      </template> -->
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
import { defineProps, defineEmits, reactive, ref, computed, nextTick, toRaw } from 'vue'
import { useRouter } from 'vue-router'
import { addSecret, getSecretDetail, editSecret } from '@/services'
import type { SSHParam, ImageParam } from '@/services'
import { type FormInstance, ElMessage, ElMessageBox, type FormRules } from 'element-plus'
import { QuestionFilled } from '@element-plus/icons-vue'
import {
  encodeToBase64String,
  decodeFromBase64String,
  convertListToKeyValueMap,
  convertKeyValueMapToList,
} from '@/utils'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import KeyValueList from '@/components/Base/KeyValueList.vue'

const props = defineProps<{
  visible: boolean
  action: string
  id: string
}>()
const emit = defineEmits(['update:visible', 'success'])
const isEdit = computed(() => props.action === 'Edit')

const router = useRouter()
const wsStore = useWorkspaceStore()
const userStore = useUserStore()

// Determine available secret types based on user permissions
const availableTypes = computed(() => {
  // In edit mode, only show current type (cannot change)
  if (isEdit.value) {
    const labelMap: Record<string, string> = { ssh: 'SSH', image: 'Image', general: 'General' }
    return [{ label: labelMap[form.type] || form.type, value: form.type }]
  }
  // Create mode: SSH Key entry visible to all; ssh type only for managers
  return [
    { label: 'SSH Key', value: 'sshkey' },
    ...(userStore.isManager ? [{ label: 'SSH', value: 'ssh' }] : []),
    { label: 'Image', value: 'image' },
    { label: 'General', value: 'general' },
  ]
})

type FormModel =
  | {
      name: string
      type: 'ssh'
      workspaceIds: string[]
      params: SSHParam[]
      labelList: Array<{ key: string; value: string }>
    }
  | {
      name: string
      type: 'image'
      workspaceIds: string[]
      params: ImageParam[]
      labelList: Array<{ key: string; value: string }>
    }
  | {
      name: string
      type: 'general'
      workspaceIds: string[]
      params: Array<Record<string, string>>
      labelList: Array<{ key: string; value: string }>
    }
const initialForm = () => {
  // Non-managers default to general type
  const defaultType = userStore.isManager ? 'ssh' : 'general'
  let defaultParams: SSHParam[] | ImageParam[] | Array<Record<string, string>>

  if (defaultType === 'ssh') {
    defaultParams = [initSSHParam()]
  } else {
    // General type initialized as empty array
    defaultParams = []
  }

  // Regular users default to current workspace, managers can select multiple
  const defaultWorkspaceIds = userStore.isManager
    ? []
    : wsStore.currentWorkspaceId
      ? [wsStore.currentWorkspaceId]
      : []

  return {
    name: '',
    type: defaultType,
    workspaceIds: defaultWorkspaceIds,
    params: defaultParams,
    labelList: [
      {
        key: '',
        value: '',
      },
    ],
  } as FormModel
}
const form = reactive(initialForm())

function initSSHParam(): SSHParam {
  return { username: '', privateKey: '', publicKey: '' }
}
function initImageParam(): ImageParam {
  return { username: '', server: '', password: '' }
}

// Temporary array used by General type for UI display
const generalParamsList = ref<Array<{ key: string; value: string; _uid: string }>>([])

function ensureUid() {
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

const ruleFormRef = ref<FormInstance>()
const rules = computed<FormRules>(() => ({
  name: [{ required: true, message: 'Please input serect name', trigger: 'blur' }],
  type: [{ required: true, trigger: 'change' }],

  // 'params.username': [{ required: true, message: 'Please input username', trigger: 'blur' }],
  // 'params.privateKey': [{ required: true, message: 'Please input privateKey', trigger: 'blur' }],
  // 'params.publicKey': [{ required: true, message: 'Please input publicKey', trigger: 'blur' }],
  // 'params.server': [{ required: true, message: 'Please input server', trigger: 'blur' }],
  // 'params.password': [{ required: true, message: 'Please input password', trigger: 'blur' }],
}))

function validatePrivateKey(key: string): boolean {
  // Check if ends with \n
  return key.endsWith('\n')
}

function onTypeChange(val: 'ssh' | 'image' | 'general' | 'sshkey') {
  if (val === 'sshkey') {
    // SSH Key entry: close dialog, navigate to public key management and auto-open create dialog
    emit('update:visible', false)
    router.push({ path: '/publickeys', query: { create: '1' } })
    return
  }
  if (val === 'ssh') {
    form.params = [initSSHParam()]
  } else if (val === 'image') {
    form.params = [initImageParam()]
  } else {
    form.params = []
    generalParamsList.value = [{ key: '', value: '', _uid: ensureUid() }]
  }
  // Keep labels when switching types, do not clear
}

function addImageParam() {
  if (form.type !== 'image') return
  ;(form.params as ImageParam[]).push(initImageParam())
}
function removeImageParam(idx: number) {
  if (form.type !== 'image') return
  ;(form.params as ImageParam[]).splice(idx, 1)
}

function addGeneralParam() {
  if (form.type !== 'general') return
  generalParamsList.value.push({ key: '', value: '', _uid: ensureUid() })
}
function removeGeneralParam(idx: number) {
  generalParamsList.value.splice(idx, 1)
}

// Sync generalParamsList to form.params
function syncGeneralParamsToForm() {
  if (form.type !== 'general') return
  const obj: Record<string, string> = {}
  generalParamsList.value.forEach((item) => {
    if (item.key) {
      obj[item.key] = item.value
    }
  })
  form.params = [obj]
}

const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()

    // Sync general type data
    if (form.type === 'general') {
      syncGeneralParamsToForm()
    }

    const raw = structuredClone(toRaw(form))

    if (raw.type === 'ssh') {
      const p0 = ((raw.params as SSHParam[]) || [])[0] ?? {}
      if (!validatePrivateKey(p0.privateKey)) {
        try {
          await ElMessageBox.confirm(
            'Missing newline character at the end of private key. Continue?',
            'Warning',
            {
              confirmButtonText: 'Continue',
              cancelButtonText: 'Cancel',
              type: 'warning',
            },
          )
        } catch {
          return
        }
      }
    }

    // Exclude labelList to avoid sending to backend
    const { labelList, ...rawWithoutLabelList } = raw

    // Convert labels
    const convertedLabels = convertListToKeyValueMap(labelList)

    const payload = {
      ...rawWithoutLabelList,
      bindAllWorkspaces: !raw.workspaceIds || raw.workspaceIds.length === 0,
      params:
        raw.type === 'ssh'
          ? [
              {
                username: raw.params?.[0]?.username ?? '',
                privateKey: raw.params?.[0]?.privateKey
                  ? encodeToBase64String(raw.params[0].privateKey)
                  : '',
                publicKey: raw.params?.[0]?.publicKey
                  ? encodeToBase64String(raw.params[0].publicKey)
                  : '',
              },
            ]
          : raw.type === 'image'
            ? Array.isArray(raw.params)
              ? raw.params
                  .map((it: ImageParam) => ({
                    username: it?.username,
                    server: it?.server ?? '',
                    password: it?.password ? encodeToBase64String(it.password) : '',
                  }))
                  .filter(
                    (it: { username?: string; server: string; password: string }) =>
                      it.server || it.password,
                  )
              : []
            : raw.type === 'general'
              ? Array.isArray(raw.params)
                ? raw.params
                    .map((item: Record<string, string>) => {
                      const result: Record<string, string> = {}
                      Object.keys(item).forEach((key) => {
                        result[key] = item[key] ? encodeToBase64String(item[key]) : ''
                      })
                      return result
                    })
                    .filter((item: Record<string, string>) => Object.keys(item).length > 0)
                : []
              : [],
      // Only add labels when they have content
      ...(convertedLabels && Object.keys(convertedLabels).length > 0
        ? { labels: convertedLabels }
        : {}),
    }

    // ===== Submit =====
    if (!isEdit.value) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      await addSecret(payload as any)
      ElMessage({ message: 'Create successful', type: 'success' })
    } else {
      if (!props.id) return
      const { name, type, ...editPayload } = payload
      await editSecret(props.id, editPayload)
      ElMessage({ message: 'Edit successful', type: 'success' })
    }

    emit('update:visible', false)
    emit('success')
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey)
      ElMessage.error(firstMsg)
    }
  }
}

const setInitialFormValues = async () => {
  if (!props.id) return

  const res = await getSecretDetail(props.id)

  form.name = res.secretName ?? ''
  form.type = res.type as 'ssh' | 'image' | 'general'
  form.workspaceIds = res.workspaceIds ?? []

  // Load labels
  form.labelList = convertKeyValueMapToList(res.labels ?? {})

  const paramsAny = res.params

  if (form.type === 'ssh') {
    const p0 = Array.isArray(paramsAny) ? (paramsAny[0] ?? {}) : (paramsAny ?? {})
    const normalized: SSHParam = {
      username: p0.username ?? '',
      privateKey: decodeFromBase64String(p0.privateKey),
      publicKey: decodeFromBase64String(p0.publicKey),
    }
    form.params = [normalized] as SSHParam[]
  } else if (form.type === 'image') {
    const arr = Array.isArray(paramsAny) ? paramsAny : [paramsAny ?? {}]
    const normalized = arr.map<ImageParam>((it) => ({
      username: it?.username,
      server: it?.server ?? '',
      password: decodeFromBase64String(it?.password),
    }))

    form.params = (normalized.length ? normalized : [{ server: '', password: '' }]) as ImageParam[]
  } else if (form.type === 'general') {
    // paramsAny should be an array of objects [{ key1: value1 }, { key2: value2 }]
    const arr = Array.isArray(paramsAny) ? paramsAny : [paramsAny ?? {}]

    // Decode each object in array and convert to UI display format
    generalParamsList.value = []
    const decodedParams: Array<Record<string, string>> = []

    arr.forEach((obj) => {
      if (obj && typeof obj === 'object') {
        const decodedObj: Record<string, string> = {}
        Object.keys(obj).forEach((key) => {
          const value = decodeFromBase64String(obj[key])
          decodedObj[key] = value
          generalParamsList.value.push({
            key,
            value,
            _uid: ensureUid(),
          })
        })
        decodedParams.push(decodedObj)
      }
    })

    form.params = decodedParams as Array<Record<string, string>>

    // If no data, initialize with an empty item
    if (generalParamsList.value.length === 0) {
      generalParamsList.value = [{ key: '', value: '', _uid: ensureUid() }]
    }
  }
}

const onOpen = async () => {
  if (isEdit.value) {
    setInitialFormValues()
  } else {
    ruleFormRef.value?.resetFields()
    Object.assign(form, initialForm())
    // For new general type creation, initialize with an empty item
    if (form.type === 'general') {
      form.params = []
      generalParamsList.value = [{ key: '', value: '', _uid: ensureUid() }]
    }
  }
  await nextTick()
}
</script>

<style scoped>
/* Enhance visual effect of selected item in disabled segmented component */
.secret-type-segmented :deep(.el-segmented__item.is-disabled.is-selected) {
  background-color: var(--el-color-primary) !important;
  color: #fff !important;
  opacity: 0.8;
}
</style>

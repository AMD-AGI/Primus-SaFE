<template>
  <el-dialog
    :model-value="visible"
    title="Create Cluster"
    width="720"
    :close-on-click-modal="false"
    @close="emit('update:visible', false)"
    @open="
      () => {
        fetchSecretsOnce()
        fetchNodes()
      }
    "
  >
    <el-form
      ref="formRef"
      :model="form"
      :rules="rules"
      label-width="auto"
      class="p-y-3 p-x-5"
      style="max-width: 720px"
    >
      <!-- Basic Information -->
      <div class="flex items-center m-b-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Basic Information</span>
      </div>

      <el-form-item label="Name" prop="name">
        <el-input v-model="form.name" />
      </el-form-item>

      <el-form-item label="Description">
        <el-input v-model="form.description" :rows="2" type="textarea" />
      </el-form-item>

      <el-form-item label="SSH Secret" prop="sshSecretId">
        <el-select v-model="form.sshSecretId" placeholder="please select ssh secret">
          <el-option v-for="item in state.secretOptions" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>

      <el-form-item label="Image Secret" prop="imageSecretId">
        <el-select v-model="form.imageSecretId" placeholder="please select image secret">
          <el-option
            v-for="item in state.imageSecretOptions"
            :key="item"
            :label="item"
            :value="item"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Protected">
        <el-switch v-model="form.isProtected" />
      </el-form-item>

      <el-form-item label="Managed Cluster">
        <el-switch v-model="form.isManagedCluster" />
      </el-form-item>

      <el-form-item label="Kube Network Plugin" prop="kubeNetworkPlugin">
        <el-select v-model="form.kubeNetworkPlugin">
          <el-option v-for="p in state.pluginOptions" :key="p" :label="p" :value="p" />
        </el-select>
      </el-form-item>

      <el-form-item label="Nodes" prop="nodes">
        <el-select
          v-model="form.nodes"
          multiple
          filterable
          collapse-tags
          collapse-tags-tooltip
          :max-collapse-tags="5"
          placeholder="Select one or more nodes (required)"
        >
          <el-option
            v-for="n in state.nodeOptions"
            :key="n.value"
            :label="n.label"
            :value="n.value"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Kube Spray Image" prop="kubeSprayImage">
        <el-select v-model="form.kubeSprayImage">
          <el-option v-for="img in state.imageOptions" :key="img" :label="img" :value="img" />
        </el-select>
      </el-form-item>

      <el-form-item label="Kubernetes Version" prop="kubernetesVersion">
        <el-segmented v-model="form.kubernetesVersion" :options="segVersionOptions" size="small" />
      </el-form-item>

      <el-form-item label="Kube Apiserver Args">
        <KeyValueList
          v-model="form.kubeApiServerArgsList"
          keyMode="input"
          :max="50"
          info="Add API server args"
        />
      </el-form-item>

      <!-- Network Settings -->
      <div class="flex items-center m-b-4 mt-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="textx-15 font-medium">Network Settings</span>
      </div>

      <el-form-item label="Kube Pods Subnet" prop="kubePodsSubnet">
        <CidrPicker v-model="form.kubePodsSubnet" />
      </el-form-item>
      <el-form-item label="Kube Service Address" prop="kubeServiceAddress">
        <CidrPicker v-model="form.kubeServiceAddress" />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button :disabled="loading" @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :loading="loading" @click="onSubmit(formRef)">Create</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, computed } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import KeyValueList from '@/components/Base/KeyValueList.vue'
import { getSecrets, getNodesList, addCluster } from '@/services/nodes'
import CidrPicker from './CidrPicker.vue'

// ====== props & emits ======
defineProps<{ visible: boolean }>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'success'): void
}>()

// ====== state ======
const state = reactive({
  secretOptions: [] as string[],
  imageSecretOptions: [] as string[],
  pluginOptions: ['cilium', 'flannel'],
  imageOptions: ['primussafe/kubespray:20200530'],
  versionOptions: ['1.32.5'],
  nodeOptions: [] as Array<{ label: string; value: string }>,
})
const segVersionOptions = computed(() =>
  (state.versionOptions ?? []).map((v) => ({ label: v, value: v })),
)

interface KV {
  key: string
  value: string
}
const form = reactive({
  // required
  name: '',
  sshSecretId: '',
  imageSecretId: '',
  kubeNetworkPlugin: 'cilium',
  nodes: [] as string[],
  kubeSprayImage: 'primussafe/kubespray:20200530',
  kubePodsSubnet: '',
  kubeServiceAddress: '',
  kubernetesVersion: '1.32.5',

  // optional
  description: '',
  isProtected: true,
  isManagedCluster: false,
  kubeApiServerArgsList: [
    { key: 'max-mutating-requests-inflight', value: '5000' },
    { key: 'max-requests-inflight', value: '10000' },
  ] as KV[],
})

const formRef = ref<FormInstance>()
const loading = ref(false)

// ====== validators ======
const isValidCIDR = (cidr: string) => {
  const m = /^(\d{1,3}(?:\.\d{1,3}){3})\/(\d{1,2})$/.exec(cidr || '')
  if (!m) return false
  const [ip, prefixStr] = [m[1], m[2]]
  const prefix = Number(prefixStr)
  if (prefix < 0 || prefix > 32) return false
  return ip.split('.').every((n) => {
    const v = Number(n)
    return v >= 0 && v <= 255
  })
}

const rules: FormRules = {
  name: [{ required: true, message: 'Please input name', trigger: 'blur' }],
  sshSecretId: [{ required: true, message: 'Please select SSH secret', trigger: 'change' }],
  imageSecretId: [{ required: true, message: 'Please select Image secret', trigger: 'change' }],
  kubeNetworkPlugin: [
    { required: true, message: 'Please select network plugin', trigger: 'change' },
  ],
  nodes: [
    {
      type: 'array',
      required: true,
      message: 'Please select at least one node',
      trigger: 'change',
    },
  ],
  kubeSprayImage: [{ required: true, message: 'Please select image', trigger: 'change' }],
  kubernetesVersion: [{ required: true, message: 'Please select version', trigger: 'change' }],
  kubePodsSubnet: [
    { required: true, message: 'Please input Pod CIDR', trigger: 'blur' },
    {
      validator: (_r, v, cb) => cb(isValidCIDR(v) ? undefined : new Error('Invalid CIDR')),
      trigger: 'blur',
    },
  ],
  kubeServiceAddress: [
    { required: true, message: 'Please input Service CIDR', trigger: 'blur' },
    {
      validator: (_r, v, cb) => cb(isValidCIDR(v) ? undefined : new Error('Invalid CIDR')),
      trigger: 'blur',
    },
  ],
}

// ====== lifecycle ======
const fetchNodes = async () => {
  const nodes = await getNodesList({ clusterId: '', limit: -1, brief: true }).catch(() => ({
    items: [],
  }))
  state.nodeOptions = (nodes?.items ?? []).map((n: any) => ({
    label: n.hostname ?? n.nodeName ?? n.nodeId ?? n.name,
    value: n.nodeId ?? n.name ?? n.hostname,
  }))
}
const fetchSecretsOnce = async () => {
  const secrets = await getSecrets({ type: 'ssh' }).catch(() => ({ items: [] }))
  state.secretOptions = (secrets?.items ?? []).map((s: any) => s.secretId ?? s.name ?? s.id)
  form.sshSecretId ||= state.secretOptions[0] || ''

  const imageSecrets = await getSecrets({ type: 'image' }).catch(() => ({ items: [] }))
  state.imageSecretOptions = (imageSecrets?.items ?? []).map(
    (s: any) => s.secretId ?? s.name ?? s.id,
  )
  form.imageSecretId ||= state.imageSecretOptions[0] || ''
}

// ====== submit ======
const onSubmit = async (el?: FormInstance) => {
  if (!el) return
  try {
    await el.validate()
    loading.value = true

    // list → object
    const kubeApiServerArgs = Object.fromEntries(
      (form.kubeApiServerArgsList || []).filter((kv) => kv.key).map((kv) => [kv.key, kv.value]),
    )

    // Assemble payload
    const payload = {
      name: form.name,
      description: form.description || undefined,
      sshSecretId: form.sshSecretId,
      imageSecretId: form.imageSecretId,
      isProtected: form.isProtected,
      kubeNetworkPlugin: form.kubeNetworkPlugin,
      nodes: form.nodes,
      kubeSprayImage: form.kubeSprayImage,
      kubePodsSubnet: form.kubePodsSubnet,
      kubeServiceAddress: form.kubeServiceAddress,
      kubernetesVersion: form.kubernetesVersion,
      kubeApiServerArgs: Object.keys(kubeApiServerArgs).length ? kubeApiServerArgs : undefined,
      ...(form.isManagedCluster ? { labels: { 'primus-safe.cluster.control-plane': '' } } : {}),
    }

    await addCluster(payload)
    ElMessage.success('Cluster created')
    emit('update:visible', false)
    emit('success')
  } catch (e: any) {
    ElMessage.error(e?.message ?? 'Create failed')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="`${action} Cluster`"
    width="720"
    :close-on-click-modal="false"
    @close="emit('update:visible', false)"
    @open="onOpen"
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
        <span class="fs-subtitle font-medium">Basic Information</span>
      </div>

      <el-form-item label="Name" prop="name">
        <el-input v-model="form.name" :disabled="isUpgrade" />
      </el-form-item>

      <el-form-item label="Description">
        <el-input v-model="form.description" :rows="2" type="textarea" :disabled="isUpgrade" />
      </el-form-item>

      <el-form-item label="SSH Secret" prop="sshSecretId">
        <el-select
          v-model="form.sshSecretId"
          placeholder="please select ssh secret"
          :disabled="isUpgrade"
        >
          <el-option v-for="item in state.secretOptions" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>

      <el-form-item label="Image Secret" prop="imageSecretId">
        <el-select
          v-model="form.imageSecretId"
          placeholder="please select image secret"
          :disabled="isUpgrade"
        >
          <el-option
            v-for="item in state.imageSecretOptions"
            :key="item"
            :label="item"
            :value="item"
          />
        </el-select>
      </el-form-item>

      <el-form-item label="Protected">
        <el-switch v-model="form.isProtected" :disabled="isUpgrade" />
      </el-form-item>

      <el-form-item label="Managed Cluster">
        <el-switch v-model="form.isManagedCluster" :disabled="isUpgrade" />
      </el-form-item>

      <el-form-item label="Kube Network Plugin" prop="kubeNetworkPlugin">
        <el-select v-model="form.kubeNetworkPlugin" :disabled="isUpgrade">
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
          :disabled="isUpgrade"
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
        <el-select v-model="form.kubeSprayImage" @change="onKubeSprayImageChange">
          <el-option v-for="img in imageOptions" :key="img" :label="img" :value="img" />
        </el-select>
      </el-form-item>

      <el-form-item label="Kubernetes Version" prop="kubernetesVersion">
        <el-input v-model="form.kubernetesVersion" :disabled="isVersionDerived" />
        <el-text v-if="isVersionDerived" size="small" type="info">
          Installed by the selected KubeSpray image.
        </el-text>
      </el-form-item>

      <el-form-item label="Kube Apiserver Args">
        <KeyValueList
          v-model="form.kubeApiServerArgsList"
          keyMode="input"
          :max="50"
          info="Add API server args"
          :disabled="isUpgrade"
        />
      </el-form-item>

      <!-- Network Settings -->
      <div class="flex items-center m-b-4 mt-4">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="fs-subtitle font-medium">Network Settings</span>
      </div>

      <!-- CidrPicker has no disabled state, and a subnet cannot be changed after the
           cluster is installed anyway, so an upgrade shows the stored value as text. -->
      <el-form-item label="Kube Pods Subnet" prop="kubePodsSubnet">
        <el-input v-if="isUpgrade" :model-value="form.kubePodsSubnet" disabled />
        <CidrPicker v-else v-model="form.kubePodsSubnet" />
      </el-form-item>
      <el-form-item label="Kube Service Address" prop="kubeServiceAddress">
        <el-input v-if="isUpgrade" :model-value="form.kubeServiceAddress" disabled />
        <CidrPicker v-else v-model="form.kubeServiceAddress" />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button :disabled="loading" @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :loading="loading" @click="onSubmit(formRef)">
          {{ action }}
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { reactive, ref, computed } from 'vue'
import { ElMessage, type FormInstance, type FormRules } from 'element-plus'
import KeyValueList from '@/components/Base/KeyValueList.vue'
import {
  getSecrets,
  getNodesList,
  addCluster,
  getClusterDetail,
  patchCluster,
} from '@/services/nodes'
import { useUserStore } from '@/stores/user'
import CidrPicker from './CidrPicker.vue'

// ====== props & emits ======
const props = withDefaults(
  defineProps<{ visible: boolean; action?: 'Create' | 'Upgrade'; clusterId?: string }>(),
  { action: 'Create', clusterId: '' },
)
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'success'): void
}>()

const userStore = useUserStore()
const isUpgrade = computed(() => props.action === 'Upgrade')

// ====== state ======
const state = reactive({
  secretOptions: [] as string[],
  imageSecretOptions: [] as string[],
  pluginOptions: ['cilium', 'flannel'],
  nodeOptions: [] as Array<{ label: string; value: string }>,
})

// The API owns which KubeSpray images exist and which Kubernetes version each one
// installs, so the pair is read from /envs rather than pinned in the frontend.
const kubeSprayVersions = computed(() => userStore.envs?.kubeSprayK8sVersions ?? {})
const imageOptions = computed(() => Object.keys(kubeSprayVersions.value))
// An image the API does not know about -- an older cluster, or an empty mapping --
// leaves the version for the operator to fill in rather than blanking it.
const isVersionDerived = computed(() => !!kubeSprayVersions.value[form.kubeSprayImage])

interface KV {
  key: string
  value: string
}
const initialForm = () => ({
  // required
  name: '',
  sshSecretId: '',
  imageSecretId: '',
  kubeNetworkPlugin: 'cilium',
  nodes: [] as string[],
  kubeSprayImage: '',
  kubePodsSubnet: '',
  kubeServiceAddress: '',
  kubernetesVersion: '',

  // optional
  description: '',
  isProtected: true,
  isManagedCluster: false,
  kubeApiServerArgsList: [
    { key: 'max-mutating-requests-inflight', value: '5000' },
    { key: 'max-requests-inflight', value: '10000' },
  ] as KV[],
})
const form = reactive(initialForm())

const onKubeSprayImageChange = (image: string) => {
  const version = kubeSprayVersions.value[image]
  if (version) form.kubernetesVersion = version
}

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

const setInitialFormValues = async () => {
  const detail = await getClusterDetail(props.clusterId)
  form.name = detail.clusterId ?? ''
  form.description = detail.description ?? ''
  form.sshSecretId = detail.sshSecretId ?? ''
  form.imageSecretId = detail.imageSecretId ?? ''
  form.isProtected = detail.isProtected ?? true
  form.isManagedCluster = detail.isControlPlane ?? false
  form.kubeNetworkPlugin = detail.kubeNetworkPlugin ?? ''
  form.nodes = detail.nodes ?? []
  form.kubeSprayImage = detail.kubeSprayImage ?? ''
  form.kubernetesVersion = detail.kubernetesVersion ?? ''
  form.kubePodsSubnet = detail.kubePodsSubnet ?? ''
  form.kubeServiceAddress = detail.kubeServiceAddress ?? ''
  form.kubeApiServerArgsList = Object.entries(detail.kubeApiServerArgs ?? {}).map(
    ([key, value]) => ({ key, value: String(value) }),
  )
}

// The dialog instance is shared by Create and Upgrade, so each open starts from a clean
// form rather than whatever the previous cluster left behind.
const onOpen = async () => {
  Object.assign(form, initialForm())
  formRef.value?.clearValidate()
  fetchSecretsOnce()
  fetchNodes()
  // The image/version mapping is part of the envs payload, which older caches may not
  // have yet.
  if (!userStore.envs) await userStore.fetchEnvs().catch(() => {})

  if (isUpgrade.value) {
    await setInitialFormValues()
    return
  }
  form.kubeSprayImage = imageOptions.value[0] ?? ''
  onKubeSprayImageChange(form.kubeSprayImage)
}

// ====== submit ======
const onSubmit = async (el?: FormInstance) => {
  if (!el) return
  try {
    await el.validate()
    loading.value = true

    // An upgrade swaps the KubeSpray image and the Kubernetes version it installs;
    // every other field on this form is disabled and left untouched.
    if (isUpgrade.value) {
      await patchCluster(props.clusterId, {
        kubeSprayImage: form.kubeSprayImage,
        kubernetesVersion: form.kubernetesVersion,
      })
      ElMessage.success('Cluster upgrade started')
      emit('update:visible', false)
      emit('success')
      return
    }

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
    ElMessage.error(e?.message ?? `${props.action} failed`)
  } finally {
    loading.value = false
  }
}
</script>

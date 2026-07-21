<template>
  <el-drawer
    v-model="visible"
    :title="isEdit ? 'Edit Slurm Cluster' : isClone ? 'Clone Slurm Cluster' : 'Create Slurm Cluster'"
    size="560px"
    :close-on-click-modal="false"
    @closed="onClosed"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
      <el-divider content-position="left">Basic information</el-divider>
      <el-form-item label="Name" prop="name">
        <el-input
          v-model="form.name"
          placeholder="Unique cluster name"
          :disabled="isEdit"
          data-testid="slurm-name"
        />
        <div v-if="!isEdit" class="name-hint" data-testid="slurm-name-hint">
          Up to {{ maxNameLength }} characters in this workspace (Slurm limits the internal cluster
          name to 40).
        </div>
      </el-form-item>
      <el-form-item label="Workspace">
        <el-input :model-value="workspaceId" disabled data-testid="slurm-workspace" />
      </el-form-item>
      <el-form-item>
        <el-checkbox v-model="form.accountingEnabled" data-testid="slurm-accounting">
          Enable accounting (slurmdbd)
        </el-checkbox>
      </el-form-item>

      <el-divider content-position="left">Node pools</el-divider>
      <div
        v-for="(pool, idx) in form.pools"
        :key="idx"
        class="border rounded-lg p-3 mb-3"
        data-testid="slurm-pool"
      >
        <div class="flex items-center justify-between mb-2">
          <el-text class="font-500">Pool {{ idx + 1 }}</el-text>
          <el-button
            v-if="form.pools.length > 1"
            circle
            size="small"
            class="btn-danger-plain"
            :icon="Delete"
            data-testid="slurm-pool-remove"
            @click="removePool(idx)"
          />
        </div>
        <el-form-item
          label="Partition name"
          :prop="`pools.${idx}.name`"
          :rules="poolNameRule"
        >
          <el-input
            v-model="pool.name"
            placeholder="e.g. main"
            data-testid="slurm-pool-name"
          />
        </el-form-item>
        <div class="grid grid-cols-2 gap-3">
          <el-form-item label="Node count">
            <el-input-number
              v-model="pool.nodes"
              :min="1"
              class="w-full"
              data-testid="slurm-pool-nodes"
            />
          </el-form-item>
          <el-form-item label="GPU per node">
            <el-input-number
              v-model="pool.gpu"
              :min="0"
              class="w-full"
              data-testid="slurm-pool-gpu"
            />
          </el-form-item>
          <el-form-item label="CPU per node">
            <el-input v-model="pool.cpu" placeholder="e.g. 128" data-testid="slurm-pool-cpu" />
          </el-form-item>
          <el-form-item label="Memory per node">
            <el-input
              v-model="pool.memory"
              placeholder="e.g. 1024Gi"
              data-testid="slurm-pool-memory"
            />
          </el-form-item>
        </div>
      </div>
      <el-button
        plain
        :icon="Plus"
        class="w-full"
        data-testid="slurm-pool-add"
        @click="addPool"
      >
        Add node pool
      </el-button>

      <el-divider content-position="left">Advanced</el-divider>
      <el-form-item label="Image tag override">
        <el-input
          v-model="form.imageTag"
          placeholder="Chart default (optional), e.g. 26.05-ubuntu26.04"
          data-testid="slurm-image-tag"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="visible = false">Cancel</el-button>
      <el-button type="primary" :loading="submitting" data-testid="slurm-submit" @click="onSubmit">
        Submit
      </el-button>
    </template>
  </el-drawer>
</template>

<script lang="ts" setup>
import { computed, reactive, ref, watch } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { Plus, Delete } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { createSlurmCluster, editSlurmCluster } from '@/services'
import type { NodePool, SlurmClusterItem } from '@/services/slurm/type'
import { useClusterStore } from '@/stores/cluster'
import { useWorkspaceStore } from '@/stores/workspace'

const props = defineProps<{
  visible: boolean
  editItem?: SlurmClusterItem | null
  cloneItem?: SlurmClusterItem | null
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'success'): void
}>()

const clusterStore = useClusterStore()
const workspaceStore = useWorkspaceStore()
const formRef = ref<FormInstance>()
const submitting = ref(false)

const isEdit = computed(() => !!props.editItem)
const isClone = computed(() => !!props.cloneItem)
const workspaceId = computed(() => workspaceStore.currentWorkspaceId ?? '')

// The Slinky operator rejects an internal ClusterName longer than 40 chars.
// That name is derived as "<namespace>_slurm-<name>", so the room left for the
// user-chosen name shrinks with the workspace (namespace) length.
const SLURM_CLUSTER_NAME_MAX = 40
const maxNameLength = computed(() =>
  Math.max(0, SLURM_CLUSTER_NAME_MAX - workspaceId.value.length - '_slurm-'.length),
)

const visible = ref(props.visible)
watch(
  () => props.visible,
  (v) => (visible.value = v),
)
watch(visible, (v) => emit('update:visible', v))

const emptyPool = (): NodePool => ({ name: '', nodes: 1, gpu: 0, cpu: '', memory: '' })

const initialForm = () => ({
  name: '',
  accountingEnabled: false,
  imageTag: '',
  pools: [emptyPool()],
})

const form = reactive(initialForm())

// Prefill the form from a source cluster when opening in edit or clone mode.
const prefillFrom = (item: SlurmClusterItem, clone: boolean) => {
  // On clone the name must be unique, so suggest "<name>-copy" (editable);
  // on edit the name is fixed and disabled.
  form.name = clone ? `${item.name}-copy` : item.name
  form.accountingEnabled = !!item.accountingEnabled
  form.imageTag = clone ? item.imageTag ?? '' : ''
  form.pools =
    item.pools && item.pools.length > 0
      ? item.pools.map((p) => ({
          name: p.name,
          nodes: p.nodes ?? 1,
          gpu: p.gpu ?? 0,
          cpu: p.cpu ?? '',
          memory: p.memory ?? '',
        }))
      : [emptyPool()]
}

watch(
  () => props.editItem,
  (item) => {
    if (item) prefillFrom(item, false)
  },
  { immediate: true },
)
watch(
  () => props.cloneItem,
  (item) => {
    if (item) prefillFrom(item, true)
  },
  { immediate: true },
)

const validateName = (_rule: unknown, value: string, cb: (err?: Error) => void) => {
  if (!value) return cb(new Error('Name is required'))
  if (value.length > maxNameLength.value) {
    return cb(
      new Error(
        `Name is too long: with workspace "${workspaceId.value}" the maximum is ${maxNameLength.value} characters (Slurm limits the internal cluster name to 40).`,
      ),
    )
  }
  cb()
}

const rules: FormRules = {
  name: [{ validator: validateName, trigger: 'blur' }],
}
const poolNameRule = [{ required: true, message: 'Partition name is required', trigger: 'blur' }]

const addPool = () => form.pools.push(emptyPool())
const removePool = (idx: number) => form.pools.splice(idx, 1)

const onClosed = () => {
  Object.assign(form, initialForm())
  formRef.value?.clearValidate()
}

const buildPools = (): NodePool[] =>
  form.pools.map((p) => ({
    name: p.name,
    nodes: p.nodes,
    gpu: p.gpu || undefined,
    cpu: p.cpu || undefined,
    memory: p.memory || undefined,
  }))

const onSubmit = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (!valid) return
    if (!workspaceId.value) {
      ElMessage.error('Select a workspace with the Slurm scope first')
      return
    }
    submitting.value = true
    try {
      const clusterId = clusterStore.currentClusterId ?? ''
      if (isEdit.value) {
        await editSlurmCluster(clusterId, form.name, workspaceId.value, {
          accountingEnabled: form.accountingEnabled,
          imageTag: form.imageTag || undefined,
          pools: buildPools(),
        })
        ElMessage.success('Slurm cluster updated')
      } else {
        await createSlurmCluster(clusterId, {
          workspaceId: workspaceId.value,
          name: form.name,
          accountingEnabled: form.accountingEnabled,
          imageTag: form.imageTag || undefined,
          pools: buildPools(),
        })
        ElMessage.success('Slurm cluster created')
      }
      visible.value = false
      emit('success')
    } finally {
      submitting.value = false
    }
  })
}

defineOptions({ name: 'SlurmClusterAddDialog' })
</script>

<style scoped>
.name-hint {
  font-size: 12px;
  line-height: 1.4;
  color: var(--el-text-color-secondary);
  margin-top: 2px;
}
</style>

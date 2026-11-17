<template>
  <el-dialog
    :model-value="visible"
    :title="`QoS · ${user?.name || user?.id || ''}`"
    width="640px"
    @open="onOpen"
    @close="emit('update:visible', false)"
  >
    <el-radio-group v-model="target" class="mb-6">
      <el-radio-button value="all">All clusters</el-radio-button>
      <el-radio-button v-for="cluster in clusters" :key="cluster.clusterId" :value="cluster.clusterId">
        {{ cluster.clusterId }}
      </el-radio-button>
    </el-radio-group>
    <QosScopeForm v-if="drafts[target]" v-model="drafts[target]" />
    <template #footer>
      <el-button @click="emit('update:visible', false)">Cancel</el-button>
      <el-button type="primary" :loading="saving" @click="onSave">Save</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useClusterStore } from '@/stores/cluster'
import { editUser, type UserSelfData } from '@/services'
import {
  draftToScope,
  emptyDraft,
  scopeToDraft,
  type ScopeDraft,
  type UserQos,
} from '@/services/qos/type'
import QosScopeForm from '@/pages/Qos/QosScopeForm.vue'

const props = defineProps<{ visible: boolean; user: UserSelfData | null }>()
const emit = defineEmits(['update:visible', 'success'])

const clusterStore = useClusterStore()
const clusters = computed(() =>
  clusterStore.items.filter((item) => item.clusterId && item.clusterId !== 'default'),
)

const saving = ref(false)
const target = ref('all')
const drafts = reactive<Record<string, ScopeDraft>>({ all: emptyDraft() })

/** resetDrafts loads the user's stored ceilings into the form. */
const resetDrafts = () => {
  for (const key of Object.keys(drafts)) delete drafts[key]
  const qos = props.user?.qos
  drafts.all = scopeToDraft(qos)
  for (const [clusterId, scope] of Object.entries(qos?.clusters ?? {})) {
    drafts[clusterId] = scopeToDraft(scope)
  }
  for (const cluster of clusters.value) {
    if (!drafts[cluster.clusterId]) drafts[cluster.clusterId] = emptyDraft()
  }
  target.value = 'all'
}

/** onOpen prepares the form when the dialog is shown. */
const onOpen = async () => {
  if (!clusterStore.isFetched) await clusterStore.fetchClusters()
  resetDrafts()
}

/** buildQos keeps only the scopes that set a value. */
const buildQos = (): UserQos => {
  const qos: UserQos = {}
  const all = draftToScope(drafts.all)
  if (all?.maxResources) qos.maxResources = all.maxResources
  if (all?.maxRuntime) qos.maxRuntime = all.maxRuntime
  const clusters: UserQos['clusters'] = {}
  for (const [name, draft] of Object.entries(drafts)) {
    if (name === 'all') continue
    const scope = draftToScope(draft)
    if (scope) clusters[name] = scope
  }
  if (Object.keys(clusters).length) qos.clusters = clusters
  return qos
}

/** onSave writes this user's ceilings. Other user fields stay unchanged. */
const onSave = async () => {
  if (!props.user?.id) return
  saving.value = true
  try {
    await editUser(props.user.id, { qos: buildQos() })
    ElMessage.success('Saved')
    emit('update:visible', false)
    emit('success')
  } finally {
    saving.value = false
  }
}
</script>

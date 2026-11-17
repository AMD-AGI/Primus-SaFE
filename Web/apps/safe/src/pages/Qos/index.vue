<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">QoS</el-text>
    <el-text class="block mt-2" type="info">
      Resource totals and timeouts for a cluster. Timeouts are set per workspace scope: Train, Infer,
      Authoring, CICD, Ray, Sandbox. Personal values are set on a user.
    </el-text>
  </div>
  <el-card class="mt-6 safe-card" shadow="never" v-loading="loading">
    <div v-if="clusters.length" class="mb-6">
      <el-text class="mb-2 block" type="info">Cluster</el-text>
      <el-radio-group v-model="target">
        <el-radio-button v-for="cluster in clusters" :key="cluster.clusterId" :value="cluster.clusterId">
          {{ cluster.clusterId }}
        </el-radio-button>
      </el-radio-group>
    </div>
    <QosScopeForm v-if="drafts[target]" v-model="drafts[target]" />
    <div v-if="target" class="mt-4">
      <el-button type="primary" :loading="saving" @click="onSave">Save</el-button>
      <el-button v-if="stored.includes(target)" :loading="saving" @click="onClear">Clear</el-button>
    </div>
  </el-card>
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useClusterStore } from '@/stores/cluster'
import { deleteQos, listQos, putQos } from '@/services/qos'
import { draftToScope, emptyDraft, scopeToDraft, type ScopeDraft } from '@/services/qos/type'
import QosScopeForm from './QosScopeForm.vue'

const clusterStore = useClusterStore()
const clusters = computed(() =>
  clusterStore.items.filter((item) => item.clusterId && item.clusterId !== 'default'),
)

const loading = ref(false)
const saving = ref(false)
// Selected cluster. The page edits that cluster's ceiling only.
const target = ref('')
const stored = ref<string[]>([])
const drafts = reactive<Record<string, ScopeDraft>>({})

/** ensureDraft creates an empty form for a cluster that has no ceiling yet. */
const ensureDraft = (name: string) => {
  if (!drafts[name]) drafts[name] = emptyDraft()
}

/** selectCluster keeps the chosen cluster, or selects the first one. */
const selectCluster = () => {
  const ids = clusters.value.map((cluster) => cluster.clusterId)
  if (target.value && ids.includes(target.value)) return
  target.value = ids[0] ?? ''
}

/** load copies stored cluster ceilings into the forms. */
const load = async () => {
  loading.value = true
  try {
    const res = await listQos()
    for (const key of Object.keys(drafts)) delete drafts[key]
    const ids = new Set(clusters.value.map((cluster) => cluster.clusterId))
    stored.value = (res.items ?? []).map((item) => item.name).filter((name) => ids.has(name))
    for (const item of res.items ?? []) {
      if (ids.has(item.name)) drafts[item.name] = scopeToDraft(item)
    }
    for (const cluster of clusters.value) ensureDraft(cluster.clusterId)
    selectCluster()
  } finally {
    loading.value = false
  }
}

/** onSave writes the selected cluster ceiling. Empty fields clear that dimension. */
const onSave = async () => {
  if (!target.value) return
  saving.value = true
  try {
    await putQos(target.value, draftToScope(drafts[target.value]) ?? {})
    ElMessage.success('Saved')
    await load()
  } finally {
    saving.value = false
  }
}

/** onClear removes the selected cluster ceiling. */
const onClear = async () => {
  if (!target.value) return
  saving.value = true
  try {
    await deleteQos(target.value)
    ElMessage.success('Cleared')
    await load()
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  if (!clusterStore.isFetched) await clusterStore.fetchClusters()
  await load()
})
</script>

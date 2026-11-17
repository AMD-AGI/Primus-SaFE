<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">QoS</el-text>
    <el-text class="block mt-2" type="info">
      Default resource totals and timeouts. A cluster value overrides the global default for that
      cluster. Personal values are set on a user.
    </el-text>
  </div>
  <el-card class="mt-6 safe-card" shadow="never" v-loading="loading">
    <el-radio-group v-model="target" class="mb-6">
      <el-radio-button value="default">Global</el-radio-button>
      <el-radio-button v-for="cluster in clusters" :key="cluster.clusterId" :value="cluster.clusterId">
        {{ cluster.clusterId }}
      </el-radio-button>
    </el-radio-group>
    <QosScopeForm v-if="drafts[target]" v-model="drafts[target]" />
    <div class="mt-4">
      <el-button type="primary" :loading="saving" @click="onSave">Save</el-button>
      <el-button v-if="target !== 'default' && stored.includes(target)" :loading="saving" @click="onClear">
        Clear
      </el-button>
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
// Selected scope: 'default' for the global ceiling, or a cluster id for its override.
const target = ref('default')
const stored = ref<string[]>([])
const drafts = reactive<Record<string, ScopeDraft>>({ default: emptyDraft() })

/** ensureDraft creates an empty form for a target that has no ceiling yet. */
const ensureDraft = (name: string) => {
  if (!drafts[name]) drafts[name] = emptyDraft()
}

/** load copies stored ceilings into the forms. */
const load = async () => {
  loading.value = true
  try {
    const res = await listQos()
    for (const key of Object.keys(drafts)) delete drafts[key]
    drafts.default = emptyDraft()
    stored.value = (res.items ?? []).map((item) => item.name)
    for (const item of res.items ?? []) {
      drafts[item.name] = scopeToDraft(item)
    }
    for (const cluster of clusters.value) ensureDraft(cluster.clusterId)
    if (!drafts[target.value]) target.value = 'default'
  } finally {
    loading.value = false
  }
}

/** onSave writes the selected ceiling. Empty fields clear that dimension. */
const onSave = async () => {
  saving.value = true
  try {
    await putQos(target.value, draftToScope(drafts[target.value]) ?? {})
    ElMessage.success('Saved')
    await load()
  } finally {
    saving.value = false
  }
}

/** onClear removes a cluster ceiling so that cluster uses the global default. */
const onClear = async () => {
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

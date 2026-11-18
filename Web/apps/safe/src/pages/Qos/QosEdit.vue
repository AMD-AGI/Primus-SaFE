<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">{{ isCreate ? 'Create QoS' : 'Edit QoS' }}</el-text>
    <el-text class="block mt-2" type="info">
      Resource totals and timeouts for one cluster. Memory is in Gi. Personal values are set on a user.
    </el-text>
  </div>
  <el-card class="mt-6 safe-card" shadow="never" v-loading="loading">
    <el-form label-width="140px">
      <el-form-item label="Cluster">
        <el-select v-if="isCreate" v-model="cluster" filterable placeholder="Select a cluster" style="width: 280px">
          <el-option v-for="id in availableClusters" :key="id" :label="id" :value="id" />
        </el-select>
        <el-text v-else>{{ cluster }}</el-text>
      </el-form-item>
    </el-form>
    <QosScopeForm v-model="draft" />
    <div class="mt-4">
      <el-button type="primary" :loading="saving" :disabled="!cluster" @click="onSubmit">
        {{ isCreate ? 'Create' : 'Save' }}
      </el-button>
      <el-button @click="router.push('/qos')">Cancel</el-button>
    </div>
  </el-card>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useClusterStore } from '@/stores/cluster'
import { createQos, getQos, listQos, updateQos } from '@/services/qos'
import { draftToScope, emptyDraft, scopeToDraft, type ScopeDraft } from '@/services/qos/type'
import QosScopeForm from './QosScopeForm.vue'

const route = useRoute()
const router = useRouter()
const clusterStore = useClusterStore()

const isCreate = computed(() => route.path.endsWith('/create'))
const loading = ref(false)
const saving = ref(false)
const cluster = ref('')
const taken = ref<string[]>([])
const draft = ref<ScopeDraft>(emptyDraft())

const availableClusters = computed(() =>
  clusterStore.items
    .map((item) => item.clusterId)
    .filter((id) => id && id !== 'default' && !taken.value.includes(id)),
)

/** load prepares an empty create form or the ceiling being edited. */
const load = async () => {
  loading.value = true
  try {
    if (!clusterStore.isFetched) await clusterStore.fetchClusters()
    if (isCreate.value) {
      const res = await listQos()
      taken.value = (res.items ?? []).map((item) => item.name)
      cluster.value = availableClusters.value[0] ?? ''
      draft.value = emptyDraft()
      return
    }
    const name = String(route.query.name ?? '')
    if (!name) {
      router.replace('/qos')
      return
    }
    cluster.value = name
    draft.value = scopeToDraft(await getQos(name))
  } finally {
    loading.value = false
  }
}

/** onSubmit creates a ceiling or replaces the one being edited. */
const onSubmit = async () => {
  if (!cluster.value) return
  saving.value = true
  try {
    const body = draftToScope(draft.value) ?? {}
    if (isCreate.value) {
      await createQos(cluster.value, body)
      ElMessage.success('Created')
    } else {
      await updateQos(cluster.value, body)
      ElMessage.success('Saved')
    }
    router.push('/qos')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="usage-page__header">
    <el-breadcrumb separator="/" class="usage-page__crumbs">
      <el-breadcrumb-item :to="cluster ? { path: '/usage' } : undefined">Usage</el-breadcrumb-item>
      <el-breadcrumb-item
        v-if="cluster"
        :to="workspace ? { path: '/usage', query: { cluster } } : undefined"
      >
        {{ cluster }}
      </el-breadcrumb-item>
      <el-breadcrumb-item v-if="workspace">{{ workspace }}</el-breadcrumb-item>
    </el-breadcrumb>
    <UsageRangePicker v-model="range" @change="reload" />
  </div>

  <div class="usage-page__body">
    <ClusterUsageCards
      v-if="!cluster"
      :items="clustersData?.items ?? []"
      :loading="clustersLoading"
      @select="goCluster"
    />

    <ClusterUsageDetail
      v-else-if="!workspace"
      v-model:sort="sort"
      :summary="workspacesData?.total ?? null"
      :current="clusterItem?.current ?? null"
      :series="clusterItem?.series ?? []"
      :items="workspacesData?.items ?? []"
      :loading="workspacesLoading || clustersLoading"
      @select="goWorkspace"
    />

    <WorkspaceUsagePanel v-else :data="workspaceData" :loading="workspaceLoading" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getUsageWorkspace, listUsageClusters, listUsageWorkspaces } from '@/services/usage'
import type {
  GetUsageWorkspaceResp,
  ListUsageClustersResp,
  ListUsageWorkspacesResp,
  UsageQuery,
} from '@/services/usage/type'
import ClusterUsageCards from './Components/ClusterUsageCards.vue'
import ClusterUsageDetail from './Components/ClusterUsageDetail.vue'
import UsageRangePicker from './Components/UsageRangePicker.vue'
import WorkspaceUsagePanel from './Components/WorkspaceUsagePanel.vue'
import { buildUsageQuery, type UsageRangeValue, type WorkspaceSort } from './usageUtils'

// Workload list length of the workspace detail.
const WORKLOAD_LIMIT = 5

const route = useRoute()
const router = useRouter()

const cluster = computed(() => (route.query.cluster as string) || '')
const workspace = computed(() => (route.query.workspace as string) || '')

const range = ref<UsageRangeValue>({ key: '7d', custom: null })
const sort = ref<WorkspaceSort>('occupancy')

const clustersData = ref<ListUsageClustersResp | null>(null)
const workspacesData = ref<ListUsageWorkspacesResp | null>(null)
const workspaceData = ref<GetUsageWorkspaceResp | null>(null)
const clustersLoading = ref(false)
const workspacesLoading = ref(false)
const workspaceLoading = ref(false)

// The cluster list and the workspace list must share one start / end / step, so the query is
// built when the range changes and reused while navigating between levels.
let query: UsageQuery = {}

// Responses of an older request must not overwrite a newer one.
const seq = { clusters: 0, workspaces: 0, workspace: 0 }

const clusterItem = computed(
  () => clustersData.value?.items.find((i) => i.cluster === cluster.value) ?? null,
)

async function loadClusters() {
  const id = ++seq.clusters
  clustersLoading.value = true
  try {
    const res = await listUsageClusters(query)
    if (id === seq.clusters) clustersData.value = res
  } finally {
    if (id === seq.clusters) clustersLoading.value = false
  }
}

async function loadWorkspaces() {
  const id = ++seq.workspaces
  workspacesLoading.value = true
  try {
    const res = await listUsageWorkspaces(cluster.value, {
      ...query,
      sortBy: sort.value === 'name' ? undefined : sort.value,
      order: sort.value === 'name' ? undefined : 'desc',
    })
    if (id === seq.workspaces) workspacesData.value = res
  } finally {
    if (id === seq.workspaces) workspacesLoading.value = false
  }
}

async function loadWorkspace() {
  const id = ++seq.workspace
  workspaceLoading.value = true
  try {
    const res = await getUsageWorkspace(workspace.value, { ...query, limit: WORKLOAD_LIMIT })
    if (id === seq.workspace) workspaceData.value = res
  } finally {
    if (id === seq.workspace) workspaceLoading.value = false
  }
}

// Loads what the current level needs. The cluster list is the source of the cluster series,
// so it is also loaded on the cluster level, but only when it is not loaded yet.
function load() {
  if (workspace.value) return loadWorkspace()
  if (!clustersData.value) loadClusters()
  if (cluster.value) return loadWorkspaces()
}

function reload() {
  query = buildUsageQuery(range.value)
  clustersData.value = null
  workspacesData.value = null
  workspaceData.value = null
  load()
}

const goCluster = (name: string) => router.push({ path: '/usage', query: { cluster: name } })
const goWorkspace = (name: string) =>
  router.push({ path: '/usage', query: { cluster: cluster.value, workspace: name } })

watch([cluster, workspace], load)
watch(sort, () => {
  if (cluster.value && !workspace.value) loadWorkspaces()
})
onMounted(reload)
</script>

<style scoped>
.usage-page__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.usage-page__crumbs {
  font-size: var(--fs-title);
  line-height: 1.4;
}
.usage-page__crumbs :deep(.el-breadcrumb__item .el-breadcrumb__inner.is-link) {
  color: var(--safe-muted);
  font-weight: 500;
}
.usage-page__crumbs :deep(.el-breadcrumb__item .el-breadcrumb__inner.is-link:hover) {
  color: var(--safe-primary);
}
.usage-page__crumbs :deep(.el-breadcrumb__item:last-child .el-breadcrumb__inner) {
  color: var(--safe-text);
  font-weight: 500;
}
.usage-page__body {
  margin-top: 16px;
}
</style>

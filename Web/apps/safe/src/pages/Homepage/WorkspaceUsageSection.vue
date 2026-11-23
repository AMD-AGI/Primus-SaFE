<template>
  <section class="workspace-usage-section">
    <div class="header-row">
      <h3 class="chart-title">
        Workspace Usage
        <span v-if="data" class="workspace-usage-section__sub">
          {{ data.workspace }}<template v-if="data.cluster"> · {{ data.cluster }}</template>
        </span>
      </h3>
      <div class="workspace-usage-section__filters">
        <UsageRangePicker v-model="range" @change="load" />
        <el-link
          v-if="data && userStore.hasManagerAccess"
          type="primary"
          :underline="false"
          @click="openInUsage"
        >
          Open in System › Usage<el-icon class="ml-1"><Right /></el-icon>
        </el-link>
      </div>
    </div>
    <WorkspaceUsagePanel :data="data" :loading="loading" :chart-height="240" />
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Right } from '@element-plus/icons-vue'
import { getUsageWorkspace } from '@/services/usage'
import type { GetUsageWorkspaceResp } from '@/services/usage/type'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import UsageRangePicker from '../Usage/Components/UsageRangePicker.vue'
import WorkspaceUsagePanel from '../Usage/Components/WorkspaceUsagePanel.vue'
import { buildUsageQuery, type UsageRangeValue } from '../Usage/usageUtils'

// Workload list length of the panel.
const WORKLOAD_LIMIT = 5

const router = useRouter()
const store = useWorkspaceStore()
const userStore = useUserStore()

const range = ref<UsageRangeValue>({ key: '7d', custom: null })
const data = ref<GetUsageWorkspaceResp | null>(null)
const loading = ref(false)

// Responses of an older request must not overwrite a newer one.
let seq = 0

async function load() {
  const id = store.currentWorkspaceId
  if (!id) {
    data.value = null
    return
  }
  const mySeq = ++seq
  loading.value = true
  try {
    const res = await getUsageWorkspace(id, {
      ...buildUsageQuery(range.value),
      limit: WORKLOAD_LIMIT,
    })
    if (mySeq === seq) data.value = res
  } catch {
    if (mySeq === seq) data.value = null
  } finally {
    if (mySeq === seq) loading.value = false
  }
}

const openInUsage = () => {
  if (!data.value) return
  router.push({
    path: '/usage',
    query: { cluster: data.value.cluster, workspace: data.value.workspace },
  })
}

watch(() => store.currentWorkspaceId, load, { immediate: true })
</script>

<style scoped>
.workspace-usage-section {
  margin-top: 20px;
}
/* Same title row as the other Homepage sections */
.header-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: clamp(10px, 1vw, 16px) 0;
  flex-wrap: wrap;
  gap: 12px;
}
.chart-title {
  font-weight: 600;
  color: var(--el-text-color-primary);
  font-size: calc(clamp(16px, 0.9vw + 12px, 24px) * var(--scale));
  margin: 0;
}
.workspace-usage-section__sub {
  margin-left: 8px;
  color: var(--safe-muted);
  font-size: var(--fs-caption);
  font-weight: 400;
}
.workspace-usage-section__filters {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}
</style>

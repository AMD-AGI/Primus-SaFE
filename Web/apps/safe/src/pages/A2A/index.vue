<template>
  <el-text class="block textx-18 font-500" tag="b">A2A Protocol</el-text>
  <div class="flex flex-wrap items-center gap-2 mt-4">
    <el-segmented
      v-model="activeTab"
      :options="tabSegOptions"
      class="myself-seg"
      style="background: none"
    />

    <el-button
      v-if="activeTab === 'registry'"
      type="primary"
      round
      :icon="Plus"
      @click="registerVisible = true"
      class="text-black"
    >
      Register Agent
    </el-button>
    <el-button round :icon="Refresh" @click="refreshCurrentTab">Refresh</el-button>

    <div v-if="activeTab === 'invocations'" class="flex flex-wrap items-center gap-2 ml-auto">
      <el-input
        v-model="callerFilter"
        placeholder="Filter by Caller"
        clearable
        style="width: 180px"
        size="default"
        :prefix-icon="Search"
        @input="handleInvocationFilterDebounced"
      />
      <el-input
        v-model="targetFilter"
        placeholder="Filter by Target"
        clearable
        style="width: 180px"
        size="default"
        :prefix-icon="Search"
        @input="handleInvocationFilterDebounced"
      />
    </div>
  </div>

  <!-- Dashboard Tab -->
  <DashboardTab v-if="activeTab === 'dashboard'" ref="dashboardRef" :services="services" :call-logs="allCallLogs" :loading="dashboardLoading" />

  <!-- Agent Registry Tab -->
  <AgentRegistryTab v-if="activeTab === 'registry'" :services="services" :loading="servicesLoading" @delete="onDeleteAgent" />

  <!-- Invocations Tab -->
  <InvocationsTab
    v-if="activeTab === 'invocations'"
    :data="callLogs"
    :loading="callLogsLoading"
    :total="callLogsTotal"
    :page="callLogsPage"
    :page-size="callLogsPageSize"
    @page-change="handleCallLogsPageChange"
    @size-change="handleCallLogsSizeChange"
  />

  <!-- Register Agent Dialog -->
  <RegisterDialog v-model:visible="registerVisible" @success="fetchServices" />
</template>

<script lang="ts" setup>
import { ref, computed, watch, onMounted, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useDebounceFn } from '@vueuse/core'
import {
  getA2AServices,
  getA2ACallLogs,
  deleteA2AService,
} from '@/services'
import type { A2AService, A2ACallLog } from '@/services'
import DashboardTab from './components/DashboardTab.vue'
import AgentRegistryTab from './components/AgentRegistryTab.vue'
import InvocationsTab from './components/InvocationsTab.vue'
import RegisterDialog from './components/RegisterDialog.vue'

defineOptions({ name: 'A2AProtocolPage' })

const route = useRoute()
const router = useRouter()

const activeTab = ref((route.query.tab as string) || 'dashboard')
watch(activeTab, (newTab) => {
  router.replace({ query: { ...route.query, tab: newTab } })
})

const tabSegOptions = [
  { label: 'Dashboard', value: 'dashboard' },
  { label: 'Agent Registry', value: 'registry' },
  { label: 'Invocations', value: 'invocations' },
] as const

// ── Services ──
const services = ref<A2AService[]>([])
const servicesLoading = ref(false)

const fetchServices = async () => {
  servicesLoading.value = true
  try {
    const res = await getA2AServices({ status: 'active' })
    services.value = res.data || []
  } catch {
    services.value = []
  } finally {
    servicesLoading.value = false
  }
}

// ── Call Logs (for Invocations tab) ──
const callLogs = ref<A2ACallLog[]>([])
const callLogsLoading = ref(false)
const callLogsTotal = ref(0)
const callLogsPage = ref(1)
const callLogsPageSize = ref(20)
const callerFilter = ref('')
const targetFilter = ref('')

const fetchCallLogs = async () => {
  callLogsLoading.value = true
  try {
    const params: Record<string, any> = {
      limit: callLogsPageSize.value,
      offset: (callLogsPage.value - 1) * callLogsPageSize.value,
    }
    if (callerFilter.value) params.caller = callerFilter.value
    if (targetFilter.value) params.target = targetFilter.value
    const res = await getA2ACallLogs(params)
    callLogs.value = res.data || []
    callLogsTotal.value = res.total || 0
  } catch {
    callLogs.value = []
    callLogsTotal.value = 0
  } finally {
    callLogsLoading.value = false
  }
}

// ── All Call Logs (for Dashboard stats, fetches up to 500) ──
const allCallLogs = ref<A2ACallLog[]>([])
const allCallLogsLoading = ref(false)
const dashboardLoading = computed(() => servicesLoading.value || allCallLogsLoading.value)

const fetchAllCallLogs = async () => {
  allCallLogsLoading.value = true
  try {
    const res = await getA2ACallLogs({ limit: 500, offset: 0 })
    allCallLogs.value = res.data || []
  } catch {
    allCallLogs.value = []
  } finally {
    allCallLogsLoading.value = false
  }
}

const handleCallLogsPageChange = (page: number) => {
  callLogsPage.value = page
  fetchCallLogs()
}

const handleCallLogsSizeChange = (size: number) => {
  callLogsPageSize.value = size
  callLogsPage.value = 1
  fetchCallLogs()
}

const handleInvocationFilterDebounced = useDebounceFn(() => {
  callLogsPage.value = 1
  fetchCallLogs()
}, 500)

// ── Delete Agent ──
const onDeleteAgent = (serviceName: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete agent: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, serviceName),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete Agent', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteA2AService(serviceName)
      ElMessage.success('Agent deleted successfully')
      fetchServices()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') return
      ElMessage.error((err as Error).message || 'Failed to delete agent')
    })
}

// ── Register Dialog ──
const registerVisible = ref(false)

// ── Dashboard ref ──
const dashboardRef = ref<InstanceType<typeof DashboardTab> | null>(null)

const refreshCurrentTab = () => {
  if (activeTab.value === 'dashboard') {
    fetchServices()
    fetchAllCallLogs()
  } else if (activeTab.value === 'registry') {
    fetchServices()
  } else if (activeTab.value === 'invocations') {
    fetchCallLogs()
  }
}

const loadTabData = (tab: string) => {
  if (tab === 'dashboard') {
    fetchServices()
    fetchAllCallLogs()
  } else if (tab === 'registry') {
    fetchServices()
  } else if (tab === 'invocations') {
    fetchCallLogs()
  }
}

watch(activeTab, (tab) => loadTabData(tab))

onMounted(() => {
  loadTabData(activeTab.value)
})
</script>

<style>
.myself-seg .el-segmented__item-selected {
  background: none;
}
.myself-seg .el-segmented__item.is-selected {
  color: var(--safe-primary) !important;
}
</style>

<template>
  <el-empty v-if="!nodeName" :image-size="200" />
  <template v-else>
    <!-- title -->
    <p class="large-title">Node: {{ nodeName || '-' }}</p>
  
    <!-- info -->
    <p class="default-title">Node Basic Information</p>
    <InfoPanel :items="items" />

    <p class="default-title">Node Stat</p>
    <GrafanaIframe
      :path="getGrafanaPath('/grafana/d/node-stat/nodestat')"
      :orgId="1"
      datasource=""
      varKey="var-node"
      :varValue="nodeName"
      theme="dark"
      kiosk
      refresh="30s"
      height="650px"
    />

    <p class="default-title mt-8">Node History</p>
    <GrafanaIframe
      :path="getGrafanaPath('/grafana/d/node-history/nodehistory')"
      :orgId="1"
      datasource=""
      varKey="var-node"
      :varValue="nodeName"
      theme="dark"
      kiosk
      refresh="30s"
      height="650px"
    />

    <!-- scheduling info table -->
    <p class="default-title mt-8">Scheduling Infomation</p>
    <el-tabs v-model="activeAlloName">
      <el-tab-pane label="Current GPU Allocation" name="current" />
      <el-tab-pane label="Historical GPU Allocation" name="history" />
    </el-tabs>
    <div class="table-box">
    <el-table
      :data="wfTableData"
      size="large"
      style="width: 100%"
      height="400"
      cell-class-name="resource-table-header"
      v-loading="wfLoading"
    >
      <el-table-column
        v-for="col in columns"
        :key="col.prop"
        :fixed="col.fixed"
        v-bind="col"
      />
    </el-table>
    <el-pagination
      v-if="paginatedWfTableData"
      v-model:current-page="paginatedWfTableData.pagination.pageNum"
      page-size="10"
      :total="paginatedWfTableData.pagination.total"
      @current-change="paginatedWfTableData.fetchData"
      class="p2.5"
    />
  </div>

  </template>
</template>

<script setup lang="ts">
import { onMounted, ref, watch, computed, shallowRef } from 'vue';
import type { VNode } from 'vue'
import dayjs from 'dayjs'
import {
  getNodeByName,
  getWorkloads,
  getWorkloadsHistory,
} from '@/services/dashboard/index'
import InfoPanel from '@/components/base/InfoPanel.vue';
import {usePaginatedTable} from '@/pages/useTable'
import { useRoute } from 'vue-router'
import {
    formatNodeInfo,
} from '@/utils/index'
import GrafanaIframe from '@/components/base/GrafanaIframe.vue'
import { useClusterSync } from '@/composables/useClusterSync'

interface TableColumn {
  prop: string
  label: string
  fixed?: boolean
  width?: number
  minWidth?: number
  formatter?: (row: any, column: TableColumn) => string | VNode
}

const route = useRoute()
const { syncFromUrl, updateUrlWithCluster } = useClusterSync()
const nodeName = computed(() => route.params.name as string | undefined)

// Get Grafana path based on base URL
const getGrafanaPath = (path: string) => {
  const baseUrl = import.meta.env.BASE_URL || '/'
  // Remove leading / from path, then concatenate with baseUrl
  const cleanPath = path.startsWith('/') ? path.slice(1) : path
  return `${baseUrl}${cleanPath}`.replace('//', '/')
}

// detail list val
const items = ref([] as any[])

// wf table initial val
const activeAlloName = ref<'current' | 'history'>('current')
const paginatedWfTableData = shallowRef<ReturnType<typeof usePaginatedTable>>()
const wfTableData = computed(() => paginatedWfTableData.value?.tableData?.value ?? [])
const wfLoading = computed(() => paginatedWfTableData.value?.loading?.value ?? false)
// wf table columns
const commonColumns: TableColumn[] = [
  { prop: 'name', label: 'Name', width: 160, fixed: true },
  { prop: 'namespace', label: 'Namespace', width: 180 },
  { prop: 'kind', label: 'Kind', width: 180 },
  { prop: 'gpuAllocated', label: 'Gpu Allocated', width: 160 },
  { prop: 'uid', label: 'Uid', minWidth: 300 },
]
const currentOnlyColumns: TableColumn[] = [
  { prop: 'nodeName', label: 'Node Name', minWidth: 220 },
  { prop: 'gpuAllocatedNode', label: 'Gpu Allocated Node', width: 180 },
]
const historyOnlyColumns: TableColumn[] = [
  { prop: 'podName', label: 'Pod Name', minWidth: 220 },
  { prop: 'podNamespace', label: 'Pod Namespace', width: 180 },
  { prop: 'startTime', label: 'Start Time', width: 180,
    formatter: (row: any) => dayjs(row.startTime * 1000).format('YYYY-MM-DD HH:mm:ss')
   },
  { prop: 'endTime', label: 'End Time', width: 180,
    formatter: (row: any) => dayjs(row.endTime * 1000).format('YYYY-MM-DD HH:mm:ss')
  }
]
const columns = computed(() => {
  if (activeAlloName.value === 'current') {
    return [...commonColumns, ...currentOnlyColumns]
  } else {
    return [...commonColumns, ...historyOnlyColumns]
  }
})

const getAlloTableData = async () => {
  if (!nodeName.value) return
  paginatedWfTableData.value = undefined

  const api = activeAlloName.value === 'current' ? getWorkloads : getWorkloadsHistory
  paginatedWfTableData.value = usePaginatedTable(api, [nodeName.value])
  paginatedWfTableData.value.fetchData()
}

const getDetails = async () => {
  if (!nodeName.value) return
  
  const res = await getNodeByName(nodeName.value)
  items.value = formatNodeInfo(res)
  getAlloTableData()
}

onMounted(() => {
  // Sync cluster from URL to global state
  syncFromUrl()
  // Ensure URL contains cluster parameter
  updateUrlWithCluster()
  getDetails()
})

watch(() => activeAlloName.value, () => {
  getAlloTableData()
})

</script>
<template>
  <div v-if="detailData" class="w-header">
    <div class="w-row">
      <div class="w-left">
        <el-button @click="router.back()" :icon="ArrowLeft" text type="primary" class="mr-2 mt-1">
          Back
        </el-button>
        <h1 class="w-name">{{ detailData.nodeName }}</h1>
        <StatusDot v-if="detailData.available" class="ml-2" type="success" :isLabel="false" />
        <el-tooltip :content="detailData.message ?? '-'" v-else>
          <StatusDot class="ml-2 cursor-pointer" type="error" :isLabel="false" />
        </el-tooltip>
      </div>
    </div>

    <div class="w-meta">
      <span class="item">
        <span class="label">ID</span>
        <code class="code">{{ detailData.nodeId }}</code>
        <el-icon
          class="copy"
          size="12"
          style="color: var(--safe-primary)"
          @click="copyText(detailData.nodeId)"
        >
          <CopyDocument />
        </el-icon>
      </span>
    </div>
  </div>

  <el-tabs v-model="activeName" class="mt-4" @tab-click="handleClick">
    <el-tab-pane label="Detail" name="detail">
      <el-card class="mt-4 safe-card" shadow="never">
        <el-descriptions
          v-if="detailData"
          v-loading="detailLoading"
          :element-loading-text="$loadingText"
          border
        >
          <el-descriptions-item label="nodeFlavor" :span="2">{{
            detailData.flavorId
          }}</el-descriptions-item>
          <el-descriptions-item label="nodeTemplate">{{
            detailData.templateId
          }}</el-descriptions-item>
          <el-descriptions-item label="workspace">{{
            detailData.workspace?.name || '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="phase">
            <el-tag :type="detailData.phase === 'Ready' ? 'success' : 'danger'">{{
              detailData.phase
            }}</el-tag>
          </el-descriptions-item>

          <el-descriptions-item label="resources (avail / total)" :span="3">
            <div class="space-y-1">
              <div v-for="r in resourcesRows" :key="r.key">
                <span class="mono">{{ r.label }}:</span>
                <span>{{ r.avail }} / {{ r.total }}</span>
              </div>
            </div>
          </el-descriptions-item>

          <el-descriptions-item label="cluster">{{ detailData.clusterId }}</el-descriptions-item>
          <el-descriptions-item label="internalIP">{{
            detailData.internalIP
          }}</el-descriptions-item>
          <el-descriptions-item label="isControlPlane">{{
            detailData.isControlPlane
          }}</el-descriptions-item>
          <el-descriptions-item label="lastStartupTime">
            {{
              detailData.lastStartupTime
                ? dayjs(detailData.lastStartupTime).format('YYYY-MM-DD HH:mm:ss')
                : '-'
            }}
          </el-descriptions-item>
          <el-descriptions-item label="labels">
            <span v-if="Object.keys(detailData.labels).length === 0">-</span>
            <span v-else>
              <span v-for="(item, index) in customerLabelsList" :key="item[0]">
                {{ item[0] }}: {{ item[1]
                }}<span v-if="index < customerLabelsList.length - 1">, </span>
              </span>
            </span>
          </el-descriptions-item>
          <el-descriptions-item label="taints" :span="3">
            <el-table v-if="detailData.taints" :data="detailData.taints">
              <el-table-column prop="key" label="key" width="180" />
              <el-table-column prop="effect" label="effect" width="180" />
              <el-table-column prop="timeAdded" label="timeAdded">
                <template #default="{ row }">
                  {{ row.timeAdded ? dayjs(row.timeAdded).format('YYYY-MM-DD HH:mm:ss') : '-' }}
                </template>
              </el-table-column>
            </el-table>
            <span v-else>-</span>
          </el-descriptions-item>
          <el-descriptions-item label="workloads">
            <el-table v-if="detailData.workloads" :data="detailData.workloads">
              <el-table-column prop="id" label="id" width="180">
                <template #default="{ row }">
                  <el-link type="primary" v-route="getDetailRoute(row.kind, row.id)">{{
                    row.id
                  }}</el-link>
                </template>
              </el-table-column>
              <el-table-column prop="userId" label="user" width="180" />
              <el-table-column prop="workspaceId" label="workspace" />
              <el-table-column label="action" width="120" fixed="right">
                <template #default="{ row }">
                  <el-link type="primary" @click="onStop(row.id)">stop</el-link>
                </template>
              </el-table-column>
            </el-table>
            <span v-else>-</span>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>
    </el-tab-pane>
    <el-tab-pane
      label="Manage/UnManage Log"
      name="log"
      v-if="['Managing', 'Unmanaging'].includes(detailData?.phase)"
    >
      <el-button :icon="Refresh" size="default" @click="getLogs">Refresh</el-button>
      <el-skeleton v-if="logLoading" class="m-t-4" :rows="9" animated />
      <div ref="logContainer" v-else-if="logResponse.logs?.length" class="log-box">
        <p v-for="(line, index) in logResponse.logs" :key="index">
          {{ line }}
        </p>
      </div>
      <div v-else>No Data</div>
    </el-tab-pane>
    <el-tab-pane label="Reboot Log" name="rebootLog" lazy>
      <el-date-picker
        v-model="searchParams.dateRange"
        size="default"
        type="datetimerange"
        range-separator="To"
        start-placeholder="Start date"
        end-placeholder="End date"
        clearable
        @change="onSearch({ resetPage: true })"
      />
      <el-card class="mt-4 safe-card" shadow="never">
        <el-table
          :height="'calc(100vh - 340px)'"
          :data="tableData"
          size="large"
          class="m-t-2"
          v-loading="loading"
          ref="tblRef"
          :element-loading-text="$loadingText"
        >
          <el-table-column prop="userName" label="User Name" min-width="140" />
          <el-table-column prop="creationTime" label="Operation Time">
            <template #default="{ row }">
              {{ formatTimeStr(row.creationTime) }}
            </template>
          </el-table-column>
        </el-table>

        <el-pagination
          class="m-t-2"
          :current-page="pagination.page"
          :page-size="pagination.pageSize"
          :total="pagination.total"
          @current-change="handlePageChange"
          @size-change="handlePageSizeChange"
          layout="total, sizes, prev, pager, next"
          :page-sizes="[10, 20, 50, 100]"
        />
      </el-card>
    </el-tab-pane>
  </el-tabs>
</template>
<script lang="ts" setup>
import { Refresh, CopyDocument, ArrowLeft } from '@element-plus/icons-vue'
import {
  getNodeDetail,
  getNodeDetailLogs,
  getRebootLogs,
  stopWorkload,
  KindPathMap,
} from '@/services'
import type {
  GetNodePodLogResponse,
  NodeRebootData,
  RebootLogsItem,
  WorkloadKind,
} from '@/services'
import type { TabsPaneContext } from 'element-plus'
import { onMounted, ref, computed, watch, nextTick, reactive, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { fmtVal, copyText, formatTimeStr } from '@/utils/index'
import dayjs from 'dayjs'
import { RESOURCE_KEYS } from '../Workspace/index'
import StatusDot from '@/components/UI/StatusDot.vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()
const nodeId = computed(() => route.query.id as string | undefined)

const activeName = ref('detail')

const detailData = ref()
const detailLoading = ref(false)
const logLoading = ref(false)
const logContainer = ref<HTMLElement | null>(null)
const logResponse = ref<GetNodePodLogResponse>({
  clusterId: '',
  nodeId: '',
  podId: '',
  logs: [],
})

const loading = ref(false)
const tableData = ref([] as RebootLogsItem[])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const searchParams = ref({
  dateRange: [
    // Default: last one month
    dayjs().subtract(1, 'month').toDate(),
    dayjs().toDate(),
  ],
})

const customerLabelsList = computed(() => {
  const labels = detailData.value.labels || {}
  return Object.entries(labels)
})

const onStop = (id: string) => {
  const msg = h('span', null, [
    'Are you sure you want to stop workload: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, id),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Stop workload', {
    confirmButtonText: 'Stop',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await stopWorkload(id)
    ElMessage({
      type: 'success',
      message: 'Stop completed',
    })
    getDetail()
  })
}

const getLogs = async () => {
  if (!nodeId.value) return
  logLoading.value = true
  logResponse.value = await getNodeDetailLogs(nodeId.value)
  logLoading.value = false
}

const handleClick = async (tab: TabsPaneContext) => {
  if (tab.paneName === 'log') getLogs()
  if (tab.paneName === 'rebootLog') onSearch({ resetPage: true })
}

const basePathOf = (kind: WorkloadKind): `/${string}` => KindPathMap[kind] ?? '/training'
const pathFor = (kind: WorkloadKind, sub: string = ''): `/${string}` => {
  const base = basePathOf(kind)
  return sub ? (`${base}/${sub.replace(/^\/+/, '')}` as `/${string}`) : base
}
const getDetailRoute = (kind: WorkloadKind, id: string) => ({
  path: `${basePathOf(kind)}/detail`,
  query: { id },
})
const jumpToDetail = (kind: WorkloadKind, id: string) => {
  router.push({ path: pathFor(kind, 'detail'), query: { id } })
}

const getDetail = async () => {
  if (!nodeId.value) return
  detailLoading.value = true
  detailData.value = await getNodeDetail(nodeId.value)
  detailLoading.value = false
}

const resourcesRows = computed(() => {
  return RESOURCE_KEYS.map(({ label, key }) => ({
    key,
    label,
    avail: fmtVal(detailData.value.availResources?.[key], key),
    total: fmtVal(detailData.value.totalResources?.[key], key),
  }))
})

const fetchRebootLogs = async (params?: NodeRebootData) => {
  try {
    loading.value = true

    const res = await getRebootLogs(detailData.value.nodeId, {
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      order: 'desc',
      sortBy: 'creationTime',
      ...params,
    })

    tableData.value = res?.items || []
    pagination.total = res?.totalCount || 0
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
  }
}
const onSearch = (options?: { resetPage?: boolean }) => {
  const reset = options?.resetPage ?? true
  if (reset) pagination.page = 1

  const [start, end] = searchParams.value.dateRange
  const sinceTime = start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''
  const untilTime = end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : ''

  fetchRebootLogs({
    sinceTime,
    untilTime,
  })
}

const handlePageChange = (newPage: number) => {
  pagination.page = newPage
  onSearch({ resetPage: false })
}

const handlePageSizeChange = (newSize: number) => {
  pagination.pageSize = newSize
  pagination.page = 1
  onSearch({ resetPage: false })
}

onMounted(() => {
  getDetail()
})

watch(
  () => logResponse.value.logs,
  async () => {
    await nextTick()
    if (logContainer.value) {
      logContainer.value.scrollTop = logContainer.value.scrollHeight
    }
  },
  { deep: true },
)
</script>
<style scoped>
.log-box {
  margin-top: 20px;
  max-height: calc(100vh - 260px);
  overflow-y: auto;
  font-family: monospace;
  white-space: pre-wrap;
  background: #111;
  color: #0f0;
  padding: 10px;
  border-radius: 6px;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  margin-right: 10px;
}
</style>

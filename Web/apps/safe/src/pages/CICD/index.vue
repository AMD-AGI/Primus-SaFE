<template>
  <el-text class="block textx-18 font-500" tag="b">CICD</el-text>

  <div class="flex flex-wrap items-center mt-4">
    <!-- Left side actions -->
    <div class="flex flex-wrap items-center gap-2">
      <el-button
        type="primary"
        round
        :icon="Plus"
        :disabled="!canWrite"
        @click="
          () => {
            addVisible = true
            curWlId = ''
            curAction = 'Create'
          }
        "
        class="mb-2 text-black"
      >
        Create CICD
      </el-button>
      <el-segmented
        v-model="searchParams.onlyMyself"
        :options="['All', 'My Workloads']"
        @change="filterByMyself"
        class="myself-seg ml-2 mt-2 sm:mt-0 mb-2"
        style="background: none"
      />
    </div>

    <!-- Right side search, aligned right -->
    <div class="flex flex-wrap items-center mt-2 mb-2 sm:mt-0 ml-auto">
      <el-date-picker
        v-model="searchParams.dateRange"
        size="default"
        type="datetimerange"
        range-separator="To"
        start-placeholder="Start date"
        end-placeholder="End date"
        class="mr-3"
        @change="onSearch({ resetPage: true })"
      />
      <el-input
        v-model="searchParams.workloadId"
        size="default"
        placeholder="Name/ID"
        style="width: 200px"
        class="mr-3"
        clearable
        @keyup.enter="onSearch({ resetPage: true })"
        @clear="onSearch({ resetPage: true })"
      />
      <el-button
        :icon="Search"
        size="default"
        type="primary"
        @click="onSearch({ resetPage: true })"
      ></el-button>
      <el-tooltip content="Reset filters" placement="top">
        <el-button
          :icon="ResetIcon"
          size="default"
          @click="
          () => {
            const { onlyMyself, userId } = searchParams
            Object.assign(searchParams, initialSearchParams, { onlyMyself, userId })
            pagination.page = 1
            onSearch({ resetPage: true })
          }
          "
        ></el-button>
      </el-tooltip>
      <el-tooltip content="Refresh" placement="top">
        <el-button
          :icon="Refresh"
          size="default"
          @click="onSearch({ resetPage: false })"
        ></el-button>
      </el-tooltip>
    </div>
  </div>

  <el-card class="mt-4 safe-card" shadow="never">
    <div class="table-wrap">
      <el-table
        :height="tableHeight"
        :data="tableData"
        @selection-change="onSelectionChange"
        @expand-change="handleExpandChange"
        ref="tableRef"
        size="large"
        class="m-t-2"
        v-loading="loading"
        :element-loading-text="$loadingText"
      >
        <el-table-column type="selection" width="56" />

        <el-table-column type="expand">
          <template #default="{ row }">
            <div v-if="subLoadingMap[row.workloadId]" class="flex items-center justify-center py-8">
              <el-icon class="is-loading mr-2" color="var(--safe-primary)">
                <Loading />
              </el-icon>
              <span>Loading...</span>
            </div>
            <el-card v-else class="safe-card m-4" shadow="never">
              <el-table :data="subTableDataMap[row.workloadId] || []" size="small">
                <el-table-column prop="workloadId" label="Name/ID" min-width="180">
                  <template #default="{ row: subRow }">
                    <el-link type="primary" v-route="{ path: '/cicd/detail', query: { id: subRow.workloadId } }">
                      {{ subRow.displayName }}
                    </el-link>
                    <div class="text-[12px] text-gray-400">
                      {{ subRow.workloadId }}
                      <el-icon
                        class="cursor-pointer hover:text-blue-500 transition ml-1"
                        size="11"
                        @click="copyText(subRow.workloadId)"
                      >
                        <CopyDocument />
                      </el-icon>
                    </div>
                  </template>
                </el-table-column>
                <el-table-column prop="kind" label="Kind" width="150">
                  <template #default="{ row: subRow }">
                    {{ subRow.groupVersionKind?.kind || '-' }}
                  </template>
                </el-table-column>
                <el-table-column
                  v-if="parentMultiNodeMap[row.workloadId]"
                  prop="relatedTask"
                  label="Association"
                  width="150"
                >
                  <template #default="{ row: subRow }">
                    <el-link
                      v-if="subRow.scaleRunnerId"
                      type="primary"
                      @click="handleQueryRelatedTask(subRow)"
                    >
                      {{
                        subRow.groupVersionKind?.kind === 'UnifiedJob'
                          ? 'Related Runner'
                          : 'Related Job'
                      }}
                    </el-link>
                    <span v-else>-</span>
                  </template>
                </el-table-column>
                <el-table-column prop="phase" label="Phase" width="120">
                  <template #default="{ row: subRow }">
                    <el-tag
                      :type="WorkloadPhaseButtonType[subRow.phase]?.type || 'info'"
                      :effect="isDark ? 'plain' : 'light'"
                      size="small"
                    >
                      {{ subRow.phase }}
                    </el-tag>
                  </template>
                </el-table-column>
                <!-- <el-table-column prop="userName" label="User" min-width="120">
                  <template #default="{ row: subRow }">
                    {{ subRow.userName || '-' }}
                  </template>
                </el-table-column> -->
                <el-table-column
                  prop="description"
                  label="Description"
                  min-width="150"
                  show-overflow-tooltip
                >
                  <template #default="{ row: subRow }">
                    {{ subRow.description || '-' }}
                  </template>
                </el-table-column>
                <el-table-column prop="creationTime" label="Creation Time" width="180">
                  <template #default="{ row: subRow }">
                    {{ formatTimeStr(subRow.creationTime) }}
                  </template>
                </el-table-column>
                <el-table-column prop="endTime" label="End Time" width="180">
                  <template #default="{ row: subRow }">
                    {{ formatTimeStr(subRow.endTime) }}
                  </template>
                </el-table-column>
                <el-table-column prop="resource" label="Resource" min-width="200">
                  <template #default="{ row: subRow }">
                    <span class="res-line">
                      <span class="t">{{ subRow.resources?.[0]?.gpu ?? 0 }} card</span>
                      <span class="sep">*</span>
                      <span class="t">{{ subRow.resources?.[0]?.cpu }} core</span>
                      <span class="sep">*</span>
                      <span class="t">{{ subRow.resources?.[0]?.memory }}</span>
                    </span>
                  </template>
                </el-table-column>
                <el-table-column label="Actions" width="120">
                  <template #default="{ row: subRow }">
                    <el-tooltip content="Stop" placement="top">
                      <el-button
                        circle
                        size="small"
                        class="btn-warning-plain"
                        :icon="Close"
                        @click="handleSubWorkloadStop(subRow.workloadId)"
                      />
                    </el-tooltip>
                    <el-tooltip content="Delete" placement="top">
                      <el-button
                        circle
                        size="small"
                        class="btn-danger-plain"
                        :icon="Delete"
                        @click="handleSubWorkloadDelete(subRow.workloadId, row.workloadId)"
                      />
                    </el-tooltip>
                  </template>
                </el-table-column>
              </el-table>
              <div v-if="subTableTotalMap[row.workloadId] > 4" class="flex justify-center mt-4">
                <el-button
                  type="primary"
                  size="small"
                  plain
                  @click="showMoreSubWorkloads(row.workloadId)"
                >
                  Show More ({{ subTableTotalMap[row.workloadId] }} items)
                </el-button>
              </div>
            </el-card>
          </template>
        </el-table-column>
        <el-table-column prop="workloadId" label="Name/ID" min-width="200" :fixed="true">
          <template #default="{ row }">
            <div class="flex flex-col items-start">
              <el-link type="primary" v-route="{ path: '/cicd/detail', query: { id: row.workloadId } }">{{
                row.displayName
              }}</el-link>
              <div class="text-[13px] text-gray-400">
                {{ row.workloadId }}
                <el-icon
                  class="cursor-pointer hover:text-blue-500 transition"
                  size="11"
                  @click="copyText(row.workloadId)"
                >
                  <CopyDocument />
                </el-icon>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="phase" label="Phase" width="160" header-align="center">
          <template #default="{ row }">
            <el-tooltip
              effect="dark"
              :content="row.message || '-'"
              :disabled="row.phase !== 'Pending'"
              placement="top"
            >
              <div class="flex flex-col items-center gap-1">
                <el-tag
                  :type="WorkloadPhaseButtonType[row.phase]?.type || 'info'"
                  :effect="isDark ? 'plain' : 'light'"
                  >{{ row.phase }}</el-tag
                >
                <el-text
                  class="mx-1"
                  size="small"
                  v-if="row.phase === 'Pending' && !!row.queuePosition"
                >
                  position in queue:{{ row.queuePosition }}
                </el-text>
              </div>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="priority" label="Priority" min-width="100">
          <template #default="{ row }">
            {{ PRIORITY_LABEL_MAP[row.priority as PriorityValue] }}
          </template>
        </el-table-column>
        <el-table-column prop="userName" label="User" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.userName || '-' }}
          </template>
        </el-table-column>
        <el-table-column
          prop="description"
          label="Description"
          min-width="180"
          show-overflow-tooltip
        >
          <template #default="{ row }">
            {{ row.description || '-' }}
          </template>
        </el-table-column>

        <el-table-column prop="creationTime" label="Creation Time" width="180">
          <template #default="{ row }">
            {{ formatTimeStr(row.creationTime) }}
          </template>
        </el-table-column>
        <el-table-column prop="endTime" label="End Time" width="180">
          <template #default="{ row }">
            {{ formatTimeStr(row.endTime) }}
          </template>
        </el-table-column>

        <el-table-column label="Actions" width="180" fixed="right">
          <template #default="{ row }">
            <!-- First 3 inline -->
            <template v-for="act in getActions(row).slice(0, 2)" :key="act.key">
              <el-tooltip :content="act.label" placement="top">
                <el-button
                  circle
                  size="default"
                  :class="act.btnClass"
                  :icon="act.icon"
                  :disabled="act.disabled?.(row) ?? false"
                  @click="act.onClick(row)"
                />
              </el-tooltip>
            </template>

            <el-popover
              v-if="getActions(row).length > 2"
              placement="bottom-start"
              trigger="click"
              :width="240"
              :teleported="true"
              :enterable="true"
              popper-class="actions-menu"
              :visible="moreOpenId === row.workloadId"
              @hide="moreOpenId === row.workloadId && (moreOpenId = null)"
            >
              <template #reference>
                <el-button
                  circle
                  class="btn-primary-plain"
                  :icon="MoreFilled"
                  size="default"
                  @click.stop="toggleMore(row.workloadId)"
                />
              </template>

              <ul class="menu-col">
                <li
                  v-for="act in getActions(row).slice(2)"
                  :key="act.key"
                  :class="['menu-item', { disabled: act.disabled?.(row) }]"
                  @click.stop="handleMenuClick(act, row)"
                >
                  <component :is="act.icon" class="menu-ico" />
                  <span class="menu-label">{{ act.label }}</span>
                </li>
              </ul>
            </el-popover>
          </template>
        </el-table-column>
      </el-table>

      <!-- Floating bottom action bar -->
      <transition name="slide-up" @after-leave="onBarAfterLeave">
        <div v-if="selectedRows.length" class="selection-bar">
          <div class="left">
            <span class="ml-2"
              >Selected {{ selectedRows.length }} item{{
                selectedRows.length === 1 ? '' : 's'
              }}</span
            >
          </div>

          <div class="right">
            <el-button type="danger" plain :disabled="!canWrite" @click="onBatchDelete">Delete</el-button>
          </div>
        </div>
      </transition>

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
    </div>
  </el-card>

  <AddDialog
    v-model:visible="addVisible"
    :wlid="curWlId"
    :action="curAction"
    @success="onSearch({ resetPage: false })"
  />

  <SshConfigDialog
    v-model:visible="sshVisible"
    :wlid="curWlId"
    :podid="curPodId"
    :ssh-command="curSshCommand"
  />

  <!-- Sub-workload detail dialog -->
  <el-dialog
    v-model="subWorkloadsDialogVisible"
    title="All Sub Workloads"
    width="90%"
    :close-on-click-modal="false"
    destroy-on-close
    @closed="resetSubWorkloadsDialog"
  >
    <!-- Search bar -->
    <div class="mb-4 flex gap-3">
      <el-input
        v-model="subWorkloadsSearchParams.workloadId"
        placeholder="Search by Name/ID"
        clearable
        style="width: 300px"
        @input="handleSubWorkloadsSearch"
        @clear="handleSubWorkloadsSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
      <el-input
        v-model="subWorkloadsSearchParams.description"
        placeholder="Search by description"
        clearable
        style="width: 300px"
        @input="handleSubWorkloadsSearch"
        @clear="handleSubWorkloadsSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
    </div>

    <el-table
      :data="allSubWorkloadsData"
      v-loading="allSubWorkloadsLoading"
      :element-loading-text="$loadingText"
      height="520"
      size="default"
      @filter-change="handleSubWorkloadsFilterChange"
    >
      <el-table-column prop="workloadId" label="Name/ID" min-width="180">
        <template #default="{ row: subRow }">
          <el-link type="primary" v-route="{ path: '/cicd/detail', query: { id: subRow.workloadId } }">
            {{ subRow.displayName }}
          </el-link>
          <div class="text-[12px] text-gray-400">
            {{ subRow.workloadId }}
            <el-icon
              class="cursor-pointer hover:text-blue-500 transition ml-1"
              size="11"
              @click="copyText(subRow.workloadId)"
            >
              <CopyDocument />
            </el-icon>
          </div>
        </template>
      </el-table-column>
      <el-table-column
        prop="kind"
        label="Kind"
        width="150"
        column-key="kindFilter"
        :filters="kindFilters"
        :filter-multiple="false"
        filter-placement="bottom-start"
      >
        <template #default="{ row: subRow }">
          {{ subRow.groupVersionKind?.kind || '-' }}
        </template>
      </el-table-column>
      <el-table-column
        v-if="currentParentIsMultiNode"
        prop="relatedTask"
        label="Related Task"
        width="150"
      >
        <template #default="{ row: subRow }">
          <el-link
            v-if="subRow.scaleRunnerId"
            type="primary"
            @click="handleQueryRelatedTask(subRow)"
          >
            {{ subRow.groupVersionKind?.kind === 'UnifiedJob' ? 'Related Runner' : 'Related Job' }}
          </el-link>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column
        prop="phase"
        label="Phase"
        width="120"
        column-key="phaseFilter"
        :filters="phaseFilters"
        :filter-multiple="true"
        filter-placement="bottom-start"
      >
        <template #default="{ row: subRow }">
          <el-tag
            :type="WorkloadPhaseButtonType[subRow.phase]?.type || 'info'"
            :effect="isDark ? 'plain' : 'light'"
          >
            {{ subRow.phase }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="userName" label="User" min-width="120">
        <template #default="{ row: subRow }">
          {{ subRow.userName || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="description" label="Description" min-width="150" show-overflow-tooltip>
        <template #default="{ row: subRow }">
          {{ subRow.description || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time" width="180">
        <template #default="{ row: subRow }">
          {{ formatTimeStr(subRow.creationTime) }}
        </template>
      </el-table-column>
      <el-table-column prop="endTime" label="End Time" width="180">
        <template #default="{ row: subRow }">
          {{ formatTimeStr(subRow.endTime) }}
        </template>
      </el-table-column>
      <el-table-column prop="resource" label="Resource" min-width="200">
        <template #default="{ row: subRow }">
          <span class="res-line">
            <span class="t">{{ subRow.resources?.[0]?.gpu ?? 0 }} card</span>
            <span class="sep">*</span>
            <span class="t">{{ subRow.resources?.[0]?.cpu }} core</span>
            <span class="sep">*</span>
            <span class="t">{{ subRow.resources?.[0]?.memory }}</span>
          </span>
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="120">
        <template #default="{ row: subRow }">
          <el-tooltip content="Stop" placement="top">
            <el-button
              circle
              size="small"
              class="btn-warning-plain"
              :icon="Close"
              @click="handleSubWorkloadStop(subRow.workloadId)"
            />
          </el-tooltip>
          <el-tooltip content="Delete" placement="top">
            <el-button
              circle
              size="small"
              class="btn-danger-plain"
              :icon="Delete"
              @click="handleSubWorkloadDelete(subRow.workloadId)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>

    <!-- Pagination -->
    <el-pagination
      class="mt-4"
      :current-page="subWorkloadsPagination.page"
      :page-size="subWorkloadsPagination.pageSize"
      :total="subWorkloadsPagination.total"
      @current-change="handleSubWorkloadsPageChange"
      @size-change="handleSubWorkloadsPageSizeChange"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[10, 20, 50, 100]"
    />
  </el-dialog>
</template>
<script lang="ts" setup>
import { ref, reactive, watch, nextTick, onMounted, onBeforeUnmount, h, computed } from 'vue'
import { useWorkloadWriteGuard } from '@/composables/useWorkloadWriteGuard'
import {
  getWorkloadsList,
  getWorkloadDetail,
  editWorkload,
  deleteWorkload,
  stopWorkload,
  batchDelWorkload,
  batchStopWorkload,
  phaseFilters,
} from '@/services'
import { useWorkspaceStore } from '@/stores/workspace'
import { WorkloadKind, WorkloadPhase, WorkloadPhaseButtonType } from '@/services/workload/type'
import { Search, Refresh, CopyDocument, Plus, Loading } from '@element-plus/icons-vue'
import ResetIcon from '@/components/icons/ResetIcon.vue'
import { copyText, formatTimeStr } from '@/utils/index'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { useRoute, useRouter } from 'vue-router'
import { useRouteAction, ROUTE_ACTIONS } from '@/composables/useRouteAction'
import AddDialog from './Components/AddDialog.vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, DocumentCopy, MoreFilled, Close, Edit, Key, VideoPlay } from '@element-plus/icons-vue'
import { type WorkloadParams, PRIORITY_LABEL_MAP, type PriorityValue } from '@/services'
import { encodeToBase64String } from '@/utils'
import { useUserStore } from '@/stores/user'
import { useDark, useDebounceFn } from '@vueuse/core'
import { useAutoRefreshUserInfo } from '@/composables/useAutoRefreshUserInfo'
// import SshConfigDialog from './Components/SshConfigDialog.vue'

dayjs.extend(utc)

const tableRef = ref()
const isDark = useDark()
const route = useRoute()
const router = useRouter()
const store = useWorkspaceStore()
const userStore = useUserStore()

// Auto refresh user info on page entry (permission-sensitive page)
useAutoRefreshUserInfo({ immediate: true })

const { canWrite } = useWorkloadWriteGuard()

const addVisible = ref(false)
const initialSearchParams = {
  userName: '',
  description: '',
  phase: [] as WorkloadPhase[],
  dateRange: '',
  workloadId: '',
  onlyMyself: 'All',
  userId: '',
}
const searchParams = reactive({ ...initialSearchParams })

const loading = ref(false)
const tableData = ref([])
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const curWlId = ref()
const curAction = ref<'Create' | 'Edit' | 'Clone' | 'Resume'>('Create')

// Sub-table data management
const subTableDataMap = reactive<Record<string, any[]>>({})
const subLoadingMap = reactive<Record<string, boolean>>({})
const subTableTotalMap = reactive<Record<string, number>>({})
const parentMultiNodeMap = reactive<Record<string, boolean>>({})

// Sub-workload dialog
const subWorkloadsDialogVisible = ref(false)
const allSubWorkloadsData = ref<any[]>([])
const allSubWorkloadsLoading = ref(false)
const currentParentWorkloadId = ref('')
const currentParentIsMultiNode = ref(false)

// Sub-workload dialog search and pagination
const subWorkloadsSearchParams = reactive({
  workloadId: '',
  description: '',
  kind: '',
  phase: [] as string[],
})
const subWorkloadsPagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})

// Kind and Phase filter options - using fixed values
const kindFilters = computed(() => [
  { text: 'EphemeralRunner', value: 'EphemeralRunner' },
  { text: 'UnifiedJob', value: 'UnifiedJob' },
])

// ssh
const curPodId = ref()
const curSshCommand = ref()
const sshVisible = ref(false)

const SELECTION_BAR_H = 56
const BASE_OFFSET = 245

// Multi-select
const selectedRows = ref<Array<any>>([])
function onSelectionChange(rows: Array<any>) {
  selectedRows.value = rows
}

// Batch action bar placeholder related
const hasSelection = computed(() => selectedRows.value.length > 0)
const hasBarSpace = ref(false)
watch(hasSelection, (v) => {
  if (v) hasBarSpace.value = true // Space reserved for selection bar
})
function onBarAfterLeave() {
  hasBarSpace.value = false
}
const tableHeight = computed(() => {
  const extra = hasBarSpace.value ? SELECTION_BAR_H : 0
  return `calc(100vh - ${BASE_OFFSET + extra}px)`
})

const jumpToDetail = (id: string) => {
  router.push({ path: '/cicd/detail', query: { id } })
}

const fetchData = async (params?: WorkloadParams) => {
  try {
    loading.value = true

    if (!params?.phase) {
      tableRef.value?.clearFilter(['phase'])
    }

    const res = await getWorkloadsList({
      workspaceId: store.currentWorkspaceId,
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      kind: WorkloadKind.AutoscalingRunnerSet,
      ...params,
    })
    tableData.value = res?.items || []
    pagination.total = res?.totalCount || 0

    // Clear sub-table cache
    Object.keys(subTableDataMap).forEach((key) => delete subTableDataMap[key])
    Object.keys(subLoadingMap).forEach((key) => delete subLoadingMap[key])
    Object.keys(subTableTotalMap).forEach((key) => delete subTableTotalMap[key])
    Object.keys(parentMultiNodeMap).forEach((key) => delete parentMultiNodeMap[key])
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    loading.value = false
  }
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

const filterByMyself = () => {
  searchParams.userId = searchParams.onlyMyself !== 'All' ? userStore.userId : ''
  onSearch({ resetPage: true })
}

const onSearch = (options?: { resetPage?: boolean }) => {
  const reset = options?.resetPage ?? true
  if (reset) pagination.page = 1

  const [start, end] = searchParams.dateRange
  fetchData({
    userName: searchParams.userName,
    description: searchParams.description,
    phase: searchParams.phase && searchParams.phase.length ? searchParams.phase.join(',') : '',
    since: start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : '',
    until: end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : '',
    workloadId: searchParams.workloadId,
    userId: searchParams.userId,
  })
}

type Row = { workloadId: string; phase: string; pods: { podId?: string }[]; displayName: string }

const handleUpdatePAT = async (row: Row) => {
  try {
    // First get the current workload details
    const workloadDetail = await getWorkloadDetail(row.workloadId)

    // Show dialog to input new PAT
    const messageBox = ElMessageBox.prompt('', 'Update PAT', {
      confirmButtonText: 'Update',
      cancelButtonText: 'Cancel',
      inputPattern: /.+/,
      inputErrorMessage: 'PAT cannot be empty',
      inputPlaceholder: 'Enter new GitHub PAT',
      distinguishCancelAndClose: true,
      beforeClose: (action, instance, done) => {
        if (action === 'confirm') {
          if (!instance.inputValue) {
            ElMessage.error('PAT cannot be empty')
            return
          }
        }
        done()
      },
    })

    const { value } = (await messageBox) as { value: string }

    // Get all existing env vars and update GITHUB_PAT
    const updatedEnv = {
      ...workloadDetail.env,
      GITHUB_PAT: value,
    }

    // Call workload edit API
    await editWorkload(row.workloadId, {
      env: updatedEnv,
    })

    ElMessage.success('GitHub PAT updated successfully')
  } catch (error: any) {
    if (error === 'cancel' || error === 'close') {
      // User cancelled the operation
      return
    }
    console.error('Failed to update GitHub PAT:', error)
    ElMessage.error('Failed to update GitHub PAT')
  }
}

type Action = {
  key: string
  label: string
  icon: any
  btnClass?: string
  disabled?: (row: Row) => boolean
  onClick: (row: Row) => void | Promise<void>
}
const getActions = (_row: Row): Action[] => [
  {
    key: 'clone',
    label: 'Clone',
    icon: DocumentCopy,
    btnClass: 'btn-success-plain',
    disabled: () => !canWrite.value,
    onClick: (r: Row) => {
      curAction.value = 'Clone'
      curWlId.value = r.workloadId
      addVisible.value = true
    },
  },
  {
    key: 'resume',
    label: 'Resume',
    icon: VideoPlay,
    btnClass: 'btn-primary-plain',
    disabled: (r: Row) => !canWrite.value || !['Stopped', 'Failed', 'Succeeded'].includes(r.phase),
    onClick: (r: Row) => {
      const endTime = (r as any).endTime
      if (endTime && dayjs().diff(dayjs.utc(endTime), 'second') < 15) {
        ElMessage.warning('Please wait 15 seconds after stopping before resuming the workload.')
        return
      }
      curAction.value = 'Resume'
      curWlId.value = r.workloadId
      addVisible.value = true
    },
  },
  {
    key: 'edit',
    label: 'Edit',
    icon: Edit,
    btnClass: 'btn-primary-plain',
    disabled: (r: Row) => !canWrite.value || !['Running', 'Pending'].includes(r.phase),
    onClick: (r: Row) => {
      curAction.value = 'Edit'
      curWlId.value = r.workloadId
      addVisible.value = true
    },
  },
  {
    key: 'updatePat',
    label: 'Update PAT',
    icon: Key,
    btnClass: 'btn-warning-plain',
    disabled: () => !canWrite.value,
    onClick: (r: Row) => {
      handleUpdatePAT(r)
    },
  },
  {
    key: 'delete',
    label: 'Delete',
    icon: Delete,
    btnClass: 'btn-danger-plain',
    disabled: () => !canWrite.value,
    onClick: async (r: Row) => {
      const msg = h('span', null, [
        'Are you sure you want to delete workload: ',
        h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, r.workloadId),
        ' ?',
      ])

      await ElMessageBox.confirm(msg, 'Delete workload', {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning',
      })
      await deleteWorkload(r.workloadId)
      ElMessage.success('Deleted')
      onSearch({ resetPage: false })
    },
  },
  {
    key: 'stop',
    label: 'Stop',
    icon: Close,
    btnClass: 'btn-danger-plain',
    disabled: () => !canWrite.value,
    onClick: async (r: Row) => {
      const msg = h('span', null, [
        'Are you sure you want to stop workload: ',
        h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, r.workloadId),
        ' ?',
      ])

      await ElMessageBox.confirm(msg, 'Stop workload', {
        confirmButtonText: 'Stop',
        cancelButtonText: 'Cancel',
        type: 'warning',
      })
      await stopWorkload(r.workloadId)
      ElMessage.success('Stop complete')
      onSearch({ resetPage: false })
    },
  },
]

type BatchAction = 'delete' | 'stop'

const apiMap: Record<BatchAction, (body: any) => Promise<any>> = {
  delete: batchDelWorkload,
  stop: batchStopWorkload,
}

const batchLoading = ref(false)

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

function previewIds(ids: string[], max = 5) {
  return ids.length <= max
    ? ids.join(', ')
    : `${ids.slice(0, max).join(', ')} +${ids.length - max} more`
}

async function onBatch(action: BatchAction) {
  const ids = selectedRows.value.map((r) => r.workloadId).filter(Boolean)
  if (!ids.length) return ElMessage.warning('Please select at least one workload')

  const Title = `${cap(action)} workloads`
  const Confirm = cap(action)
  const OkMsg = `${cap(action)} completed`
  const Cancel = `${cap(action)} canceled`

  const msg = h('span', null, [
    `Are you sure you want to ${action} workloads: `,
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, previewIds(ids)),
    ' ?',
  ])

  try {
    await ElMessageBox.confirm(msg, Title, {
      confirmButtonText: Confirm,
      cancelButtonText: 'Cancel',
      type: action === 'delete' ? 'warning' : 'info',
    })
    batchLoading.value = true

    await apiMap[action]({ workloadIds: ids })

    ElMessage.success(OkMsg)
    await onSearch({ resetPage: false })
    selectedRows.value = []
  } catch (err: any) {
    if (err === 'cancel' || err === 'close') {
      ElMessage.info(Cancel)
    } else if (err?.message) {
      ElMessage.error(err.message)
    }
  } finally {
    batchLoading.value = false
  }
}

// Reuse buttons directly
const onBatchDelete = () => onBatch('delete')

const moreOpenId = ref<string | null>(null) // Currently open popover row ID

const toggleMore = async (id: string) => {
  if (moreOpenId.value === id) {
    moreOpenId.value = null
    return
  }
  moreOpenId.value = null
  await nextTick()
  moreOpenId.value = id
}

const closeMore = () => {
  moreOpenId.value = null
}

const handleMenuClick = async (act: Action, row: Row) => {
  if (act.disabled?.(row)) return
  await act.onClick(row)
  closeMore()
}

// Handle row expansion, fetch sub-table data
const handleExpandChange = async (row: any, expandedRows: any[]) => {
  // If this is an expand operation
  const isExpanding = expandedRows.some((r) => r.workloadId === row.workloadId)

  if (isExpanding) {
    // Skip if data already exists
    if (subTableDataMap[row.workloadId] && subTableDataMap[row.workloadId].length > 0) {
      return
    }

    try {
      // Set loading state
      subLoadingMap[row.workloadId] = true

      // Check if parent is multi-node
      const parentDetail = await getWorkloadDetail(row.workloadId)
      let isMultiNode = false
      if (parentDetail.env && parentDetail.env.UNIFIED_JOB_ENABLE) {
        isMultiNode = parentDetail.env.UNIFIED_JOB_ENABLE === 'true'
      }
      parentMultiNodeMap[row.workloadId] = isMultiNode

      const res = await getWorkloadsList({
        workspaceId: store.currentWorkspaceId,
        scaleRunnerSet: row.workloadId,
        limit: 4,
      })

      // Take only first 4 items and store in Map
      subTableDataMap[row.workloadId] = (res?.items || []).slice(0, 4)
      // Record total count
      subTableTotalMap[row.workloadId] = res?.totalCount || 0
    } catch (err) {
      ElMessage.error('Failed to load sub workloads')
      console.error(err)
      subTableDataMap[row.workloadId] = []
      subTableTotalMap[row.workloadId] = 0
      parentMultiNodeMap[row.workloadId] = false
    } finally {
      subLoadingMap[row.workloadId] = false
    }
  }
}

// Show more sub-workloads
const showMoreSubWorkloads = async (workloadId: string) => {
  currentParentWorkloadId.value = workloadId
  // Use existing multi-node check result
  currentParentIsMultiNode.value = parentMultiNodeMap[workloadId] || false
  // Reset search params and pagination
  subWorkloadsSearchParams.workloadId = ''
  subWorkloadsSearchParams.description = ''
  subWorkloadsSearchParams.kind = ''
  subWorkloadsSearchParams.phase = []
  subWorkloadsPagination.page = 1
  subWorkloadsPagination.pageSize = 20

  subWorkloadsDialogVisible.value = true
  await fetchSubWorkloadsForDialog()
}

// Fetch sub-workload data (for dialog)
const fetchSubWorkloadsForDialog = async () => {
  try {
    allSubWorkloadsLoading.value = true

    const params: any = {
      workspaceId: store.currentWorkspaceId,
      scaleRunnerSet: currentParentWorkloadId.value,
      limit: subWorkloadsPagination.pageSize,
      offset: (subWorkloadsPagination.page - 1) * subWorkloadsPagination.pageSize,
    }

    // Add search and filter params
    if (subWorkloadsSearchParams.workloadId) {
      params.workloadId = subWorkloadsSearchParams.workloadId
    }
    if (subWorkloadsSearchParams.description) {
      params.description = subWorkloadsSearchParams.description
    }
    if (subWorkloadsSearchParams.kind) {
      params.kind = subWorkloadsSearchParams.kind
    }
    if (subWorkloadsSearchParams.phase && subWorkloadsSearchParams.phase.length > 0) {
      // Pass array directly, format: phase=['Running','Pending']
      params.phase = subWorkloadsSearchParams.phase
    }

    const res = await getWorkloadsList(params)

    allSubWorkloadsData.value = res?.items || []
    subWorkloadsPagination.total = res?.totalCount || 0
  } catch (err) {
    ElMessage.error('Failed to load sub workloads')
    console.error(err)
    allSubWorkloadsData.value = []
    subWorkloadsPagination.total = 0
  } finally {
    allSubWorkloadsLoading.value = false
  }
}

// Handle sub-workload search
const handleSubWorkloadsSearch = useDebounceFn(() => {
  subWorkloadsPagination.page = 1
  fetchSubWorkloadsForDialog()
}, 300)

// Handle sub-workload filter change
const handleSubWorkloadsFilterChange = (filters: Record<string, string[]>) => {
  if ('kindFilter' in filters) {
    subWorkloadsSearchParams.kind = filters.kindFilter?.[0] || ''
  }
  if ('phaseFilter' in filters) {
    subWorkloadsSearchParams.phase = filters.phaseFilter || []
  }
  subWorkloadsPagination.page = 1
  fetchSubWorkloadsForDialog()
}

// Handle sub-workload page change
const handleSubWorkloadsPageChange = (page: number) => {
  subWorkloadsPagination.page = page
  fetchSubWorkloadsForDialog()
}

// Handle sub-workload page size change
const handleSubWorkloadsPageSizeChange = (pageSize: number) => {
  subWorkloadsPagination.pageSize = pageSize
  subWorkloadsPagination.page = 1
  fetchSubWorkloadsForDialog()
}

// Reset sub-workload dialog
const resetSubWorkloadsDialog = () => {
  subWorkloadsSearchParams.workloadId = ''
  subWorkloadsSearchParams.description = ''
  subWorkloadsSearchParams.kind = ''
  subWorkloadsSearchParams.phase = []
  subWorkloadsPagination.page = 1
  subWorkloadsPagination.pageSize = 20
  allSubWorkloadsData.value = []
  currentParentWorkloadId.value = ''
}

// Handle sub-workload stop operation
const handleSubWorkloadStop = async (workloadId: string) => {
  try {
    const msg = h('span', null, [
      'Are you sure you want to stop workload: ',
      h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, workloadId),
      ' ?',
    ])

    await ElMessageBox.confirm(msg, 'Stop workload', {
      confirmButtonText: 'Stop',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })

    await stopWorkload(workloadId)
    ElMessage.success('Stopped')

    // Refresh sub-table data
    await refreshSubWorkloadData()
  } catch (err: any) {
    if (err !== 'cancel' && err !== 'close') {
      ElMessage.error(err?.message || 'Stop failed')
    }
  }
}

// Handle sub-workload delete operation
const handleSubWorkloadDelete = async (workloadId: string, parentWorkloadId?: string) => {
  try {
    const msg = h('span', null, [
      'Are you sure you want to delete workload: ',
      h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, workloadId),
      ' ?',
    ])

    await ElMessageBox.confirm(msg, 'Delete workload', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })

    await deleteWorkload(workloadId)
    ElMessage.success('Deleted')

    // Refresh sub-table data
    await refreshSubWorkloadData(parentWorkloadId)
  } catch (err: any) {
    if (err !== 'cancel' && err !== 'close') {
      ElMessage.error(err?.message || 'Delete failed')
    }
  }
}

// Refresh sub-table data
const refreshSubWorkloadData = async (parentWorkloadId?: string) => {
  // If dialog is open, refresh dialog data
  if (subWorkloadsDialogVisible.value && currentParentWorkloadId.value) {
    await fetchSubWorkloadsForDialog()
  }

  // If parentWorkloadId exists, refresh expanded row data
  if (parentWorkloadId && subTableDataMap[parentWorkloadId]) {
    const res = await getWorkloadsList({
      workspaceId: store.currentWorkspaceId,
      scaleRunnerSet: parentWorkloadId,
      limit: 4,
    })
    subTableDataMap[parentWorkloadId] = (res?.items || []).slice(0, 4)
    subTableTotalMap[parentWorkloadId] = res?.totalCount || 0
  }
}

// Query related task
const handleQueryRelatedTask = async (subRow: any) => {
  if (!subRow.scaleRunnerId) {
    ElMessage.warning('scaleRunnerId does not exist')
    return
  }

  const loading = ElMessage({
    message: 'Querying related task...',
    type: 'info',
    duration: 0,
  })

  try {
    // Determine the target kind to query
    const currentKind = subRow.groupVersionKind?.kind
    const targetKind =
      currentKind === 'EphemeralRunner'
        ? 'UnifiedJob'
        : currentKind === 'UnifiedJob'
          ? 'EphemeralRunner'
          : null

    if (!targetKind) {
      loading.close()
      ElMessage.warning('Unknown task type')
      return
    }

    // Query related task
    const res = await getWorkloadsList({
      workspaceId: store.currentWorkspaceId,
      scaleRunnerId: subRow.scaleRunnerId,
      kind: targetKind,
      limit: 1,
    })

    loading.close()

    // Jump to related task detail
    if (res?.items && res.items.length > 0) {
      const relatedWorkload = res.items[0]
      jumpToDetail(relatedWorkload.workloadId)
    } else {
      ElMessage.warning('Related task not found')
    }
  } catch (err: any) {
    loading.close()
    ElMessage.error(err?.message || 'Failed to query related task')
  }
}

defineOptions({
  name: 'cicdPage',
})

// Close popover on any scroll/click elsewhere
const onAnyScroll = () => closeMore()
const onAnyPointerDown = (e: Event) => {
  const el = e.target as HTMLElement
  const inMenu = el.closest('.actions-menu') !== null
  const inRefBtn = el.closest('.btn-primary-plain') !== null
  if (!inMenu && !inRefBtn) closeMore()
}
useRouteAction({
  [ROUTE_ACTIONS.CREATE]: () => { addVisible.value = true },
})

onMounted(() => {
  window.addEventListener('scroll', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('wheel', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('touchmove', onAnyScroll, { passive: true, capture: true })
  window.addEventListener('pointerdown', onAnyPointerDown, { capture: true })
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', onAnyScroll, { capture: true } as any)
  window.removeEventListener('wheel', onAnyScroll, { capture: true } as any)
  window.removeEventListener('touchmove', onAnyScroll, { capture: true } as any)
  window.removeEventListener('pointerdown', onAnyPointerDown, { capture: true } as any)
})

watch(
  // Refresh on workspace dropdown change - update list data immediately
  () => store.currentWorkspaceId,
  (id) => {
    if (id) fetchData()
  },
  { immediate: true },
)
</script>
<style scoped>
.res-line {
  display: inline-flex;
  align-items: baseline;
  gap: 0;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.t {
  /* font-weight: 500; */
  letter-spacing: 0.2px;
}
.sep {
  opacity: 0.55;
  margin: 0 0.6ch;
  font-weight: 700;
}

/* Bottom action bar */
.selection-bar {
  position: sticky;
  bottom: 0;
  z-index: 1;
  height: 56px;
  padding: 0 16px;
  background: var(--el-bg-color);
  border-top: 1px solid var(--el-border-color);
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 -6px 12px rgba(0, 0, 0, 0.06);
  /* Optional: tighten left-right content spacing */
  gap: 12px;
}

/* Enter animation */
.slide-up-enter-active,
.slide-up-leave-active {
  transition:
    transform 0.18s ease,
    opacity 0.18s ease;
}
.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateY(100%);
  opacity: 0;
}
</style>
<style>
/* Override segmented styles */
.myself-seg .el-segmented__item-selected {
  background: none;
}
.myself-seg .el-segmented__item.is-selected {
  color: var(--safe-primary) !important;
}
</style>

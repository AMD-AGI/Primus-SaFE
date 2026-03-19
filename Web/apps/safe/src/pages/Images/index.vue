<template>
  <el-text class="block textx-18 font-500" tag="b">Images</el-text>
  <div class="flex flex-wrap items-center gap-2 mt-4">
    <el-button
      v-if="activeTab === 'list'"
      type="primary"
      round
      :icon="Upload"
      data-tour="images-import-btn"
      @click="visible = true"
      class="text-black"
    >
      Import Image
    </el-button>
    <el-button
      v-if="activeTab === 'prewarm'"
      type="primary"
      round
      :icon="SetUp"
      data-tour="images-preheat-btn"
      @click="prewarmVisible = true"
      class="text-black"
    >
      Preheat Image
    </el-button>
    <el-segmented
      v-model="activeTab"
      :options="tabSegOptions"
      class="myself-seg"
      style="background: none"
      data-tour="images-tabs"
    />

    <!-- Right side search, Import tab -->
    <div v-if="activeTab === 'list'" class="flex flex-wrap items-center ml-auto" data-tour="images-search">
      <el-input
        v-model="imageFilter"
        placeholder="Search by image name"
        clearable
        style="width: 300px"
        size="default"
        :prefix-icon="Search"
        @input="handleImageSearchDebounced"
      />
    </div>

    <!-- Right side filter, Preheat tab -->
    <div v-if="activeTab === 'prewarm'" class="flex flex-wrap items-center ml-auto">
      <el-input
        v-model="prewarmState.imageNameFilter"
        placeholder="Search by image name"
        clearable
        style="width: 300px"
        size="default"
        :prefix-icon="Search"
        @input="handlePrewarmSearchDebounced"
      />
    </div>
  </div>

  <!-- Image list -->
  <el-card v-show="activeTab === 'list'" class="mt-4 safe-card" shadow="never" data-tour="images-table">
    <el-table
      :height="'calc(100vh - 240px)'"
      :data="state.rowData"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column type="expand">
        <template #default="props">
          <el-card class="safe-card m-4" shadow="never">
            <el-table :data="props.row.artifacts">
              <el-table-column label="ID" prop="id" />
              <el-table-column label="Version" prop="imageTag">
                <template #default="{ row }">
                  <el-tooltip
                    v-if="row.includeType === 'import'"
                    :content="`View details for ${row.imageTag}`"
                  >
                    <el-link type="primary" @click="onOpenDetail(row.id, row.imageTag)">{{
                      row.imageTag
                    }}</el-link>
                  </el-tooltip>
                  <span v-else>{{ row.imageTag }}</span>
                </template>
              </el-table-column>
              <el-table-column label="Include Type" prop="includeType" />
              <el-table-column label="Description" prop="description" show-overflow-tooltip />
              <el-table-column label="Arch" prop="arch" />
              <el-table-column label="OS" prop="os" />
              <el-table-column label="Size" prop="size">
                <template #default="{ row }"> {{ formatBytes(row.size) }}</template>
              </el-table-column>
              <el-table-column label="Status" prop="status">
                <template #default="{ row }">
                  <el-tag
                    :effect="isDark ? 'plain' : 'light'"
                    :type="ImagePhaseButtonType[row.status]?.type"
                    >{{ row.status }}</el-tag
                  >
                </template>
              </el-table-column>
              <el-table-column label="Created Time" prop="createdTime">
                <template #default="{ row }">
                  {{ formatTimeStr(row.createdTime) }}
                </template>
              </el-table-column>
              <el-table-column label="Actions" width="120" fixed="right">
                <template #default="{ row }">
                  <el-tooltip content="Delete" placement="top">
                    <el-button
                      circle
                      size="default"
                      class="btn-danger-plain"
                      :icon="Delete"
                      @click="onDelete(row.id, row.imageTag)"
                    />
                  </el-tooltip>
                  <el-tooltip content="Retry" placement="top">
                    <el-button
                      circle
                      size="default"
                      class="btn-primary-plain"
                      :icon="Refresh"
                      :disabled="row.status !== 'Pending'"
                      @click="onRetry(row.id)"
                    />
                  </el-tooltip>
                </template>
              </el-table-column>
            </el-table>
          </el-card>
        </template>
      </el-table-column>
      <el-table-column prop="registryHost" label="Registry Host" min-width="220" />
      <el-table-column prop="repo" label="Repo" min-width="120" />
    </el-table>

    <!-- Pagination -->
    <el-pagination
      class="mt-4"
      :current-page="imagePagination.page"
      :page-size="imagePagination.pageSize"
      :total="imagePagination.total"
      @current-change="handleImagePageChange"
      @size-change="handleImagePageSizeChange"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[10, 20, 50, 100]"
    />
  </el-card>

  <!-- Preheat records list -->
  <el-card v-show="activeTab === 'prewarm'" class="mt-4 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 245px)'"
      :data="prewarmState.rowData"
      size="large"
      v-loading="prewarmLoading"
      :element-loading-text="$loadingText"
      @filter-change="handleFilterChange"
    >
      <el-table-column type="expand">
        <template #default="{ row }">
          <PrewarmNodeDetail :job-name="row.jobName" />
        </template>
      </el-table-column>
      <el-table-column
        label="Image Name"
        prop="imageName"
        min-width="240"
        show-overflow-tooltip
      />
      <el-table-column
        label="Workspace"
        prop="workspaceName"
        min-width="150"
        column-key="workspace"
        :filters="workspaceFilterOptions"
        :filter-method="() => true"
      >
        <template #default="{ row }">
          {{ row.workspaceName }}
        </template>
      </el-table-column>
      <el-table-column
        label="User"
        prop="userName"
        min-width="180"
        show-overflow-tooltip
        column-key="userName"
        :filters="userNameFilterOptions"
        :filter-method="() => true"
      >
        <template #default="{ row }">
          {{ row.userName || '-' }}
        </template>
      </el-table-column>
      <el-table-column label="Progress" prop="prewarmProgress" min-width="220">
        <template #default="{ row }">
          <div class="progress-cell">
            <el-progress
              :percentage="parseFloat(row.prewarmProgress) || 0"
              :status="
                row.status === 'Completed'
                  ? 'success'
                  : row.status === 'Failed'
                    ? 'exception'
                    : undefined
              "
              :stroke-width="8"
              class="w-200px"
            >
              <span class="text-xs">
                {{ row.nodesReady || '0' }}/{{ row.nodesTotal || '0' }}
              </span>
            </el-progress>
            <span class="progress-pct">{{ row.prewarmProgress || '0%' }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column
        label="Status"
        prop="status"
        width="180"
        column-key="status"
        :filters="statusFilterOptions"
        :filter-method="() => true"
      >
        <template #default="{ row }">
          <el-tag :type="getStatusType(row.status)">{{ row.status || 'Unknown' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Created Time" prop="createdTime" width="200">
        <template #default="{ row }">
          {{ formatTimeStr(row.createdTime) }}
        </template>
      </el-table-column>
      <el-table-column label="End Time" prop="endTime" width="200">
        <template #default="{ row }">
          {{ formatTimeStr(row.endTime) }}
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="100">
        <template #default="{ row }">
          <el-tooltip content="Retry" placement="top">
            <el-button
              circle
              size="default"
              class="btn-warning-plain"
              :icon="Refresh"
              :disabled="row.status !== 'Failed'"
              @click="onRetryPrewarm(row)"
            />
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>

    <!-- Pagination -->
    <el-pagination
      v-model:current-page="prewarmState.page"
      v-model:page-size="prewarmState.pageSize"
      :page-sizes="[10, 20, 50, 100]"
      layout="total, sizes, prev, pager, next, jumper"
      :total="prewarmState.total"
      @size-change="getPrewarmList"
      @current-change="getPrewarmList"
      class="mt-4"
    />
  </el-card>

  <!-- Import Image Dialog -->
  <el-dialog
    :model-value="visible"
    title="Import Image"
    width="600"
    @close="visible = false"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form
      ref="ruleFormRef"
      :model="form"
      label-width="auto"
      style="max-width: 600px"
      class="p-5"
      :rules="rules"
    >
      <el-form-item label="Source" prop="source">
        <el-input
          v-model="form.source"
          clearable
          placeholder="Enter full image address, e.g. docker.io/library/nginx:latest"
        />
      </el-form-item>

      <el-form-item label="Secret">
        <el-select v-model="form.secretId" placeholder="Please select secret">
          <el-option
            v-for="item in secretOptions"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-select>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="visible = false" :disabled="importLoading">Cancel</el-button>
        <el-button type="primary" :loading="importLoading" @click="onSubmit(ruleFormRef)">
          {{ importLoading ? 'Checking if image already exists...' : 'Confirm' }}
        </el-button>
      </div>
    </template>
  </el-dialog>

  <!-- Prewarm Image Dialog -->
  <el-dialog
    :model-value="prewarmVisible"
    title="Preheat Image"
    width="600"
    @close="prewarmVisible = false"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onPrewarmOpen"
  >
    <el-form
      ref="prewarmFormRef"
      :model="prewarmForm"
      label-width="auto"
      style="max-width: 600px"
      class="p-5"
      :rules="prewarmRules"
    >
      <el-form-item label="Image" prop="image">
        <ImageInput v-model="prewarmForm.image" />
      </el-form-item>

      <el-form-item label="Workspace" prop="workspace">
        <el-select v-model="prewarmForm.workspace" placeholder="Select workspace" class="w-full">
          <el-option
            v-for="ws in wsStore.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="prewarmVisible = false">Cancel</el-button>
        <el-button type="primary" @click="onPrewarmSubmit(prewarmFormRef)"> Confirm </el-button>
      </div>
    </template>
  </el-dialog>

  <ImportDetailDialog :id="state.curId" :tag="state.curTag" v-model:visible="state.detailVisible" />
</template>

<script lang="ts" setup>
import { ref, onMounted, onUnmounted, h, reactive, computed, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  deleteImage,
  getImagesList,
  ImagePhaseButtonType,
  importImage,
  getImageRegList,
  retryImage,
  getImagePrewarmList,
  rebootNodes,
} from '@/services'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Delete, Upload, Refresh, SetUp, Search, CopyDocument } from '@element-plus/icons-vue'
import { useDark, useDebounceFn } from '@vueuse/core'
import ImportDetailDialog from './Components/ImportDetailDialog.vue'
import PrewarmNodeDetail from './Components/PrewarmNodeDetail.vue'
import { formatTimeStr, formatBytes, copyText } from '@/utils'
import { useWorkspaceStore } from '@/stores/workspace'
import { useUserStore } from '@/stores/user'
import { useSecrets } from '@/composables'
import { usePageTour } from '@/composables/usePageTour'
import type { DriveStep } from 'driver.js'
import ImageInput from '@/components/Base/ImageInput.vue'

const isDark = useDark()
const wsStore = useWorkspaceStore()
const userStore = useUserStore()
const route = useRoute()
const router = useRouter()

// Tab switch - initialize from URL
const activeTab = ref((route.query.tab as string) || 'list')

// Watch tab changes, update URL
watch(activeTab, (newTab) => {
  router.replace({ query: { ...route.query, tab: newTab } })
})

const loading = ref(false)
const tabSegOptions = [
  { label: 'Import', value: 'list' },
  { label: 'Preheat', value: 'prewarm' },
] as const
const state = reactive({
  rowData: [] as any[],
  curId: 0,
  curTag: '',
  detailData: [],
  detailVisible: false,
})

// Image list pagination and search
const imagePagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const imageFilter = ref('')

const visible = ref(false)

/* ── Page Tours (tourId-driven from Quick Reference ?tour=<id>) ── */
usePageTour((tourId) => {
  switch (tourId) {
    /* ─ "Import image" — import button → search → table ─ */
    case 'import':
    default:
      return [
        {
          element: '[data-tour="images-import-btn"]',
          popover: {
            title: 'Import Image',
            description:
              'Import a container image from a supported registry. It appears under the /sync prefix once Ready.',
            side: 'bottom' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="images-search"]',
          popover: {
            title: 'Search Images',
            description: 'Search your imported images by name.',
            side: 'bottom' as const,
            align: 'end' as const,
          },
        },
        {
          element: '[data-tour="images-table"]',
          popover: {
            title: 'Image List',
            description:
              'All imported images appear here. Expand a row to see versions and tags. You can also use a Harbor proxy URL for public images.',
            side: 'top' as const,
          },
        },
      ]

    /* ─ "Prewarm image" — switch to Preheat tab → Preheat button ─ */
    case 'prewarm':
      // Side-effect: switch tab so the Preheat button becomes visible
      activeTab.value = 'prewarm'
      return [
        {
          element: '[data-tour="images-tabs"]',
          popover: {
            title: 'Preheat Tab',
            description:
              'Switch to the "Preheat" tab to manage image pre-warming across workspaces.',
            side: 'bottom' as const,
            align: 'start' as const,
          },
        },
        {
          element: '[data-tour="images-preheat-btn"]',
          popover: {
            title: 'Preheat Image',
            description:
              'Click here to select an image and target workspace. Preheating pulls the image to cluster nodes in advance, reducing startup time.',
            side: 'bottom' as const,
            align: 'start' as const,
          },
        },
      ]
  }
})

const regOptions = ref([])
// Use composable to fetch secrets
const { secretOptions, fetchSecrets } = useSecrets('image')
const initialForm = () => ({
  source: '',
  secretId: '',
})
const form = reactive({ ...initialForm() })
const ruleFormRef = ref<FormInstance>()
const rules = computed<FormRules>(() => ({
  source: [{ required: true, message: 'Please input source', trigger: 'blur' }],
}))

const onRetry = async (id: number) => {
  try {
    await retryImage(id)
    ElMessage({
      type: 'success',
      message: `Image ${id} retry completed`,
    })
    getImages()
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to retry image')
  }
}

const onDelete = (id: number, v: string) => {
  const msg = h('span', null, [
    'Are you sure you want to delete version: ',
    h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, v),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete Image Version', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteImage(id)
      ElMessage({
        type: 'success',
        message: 'Delete completed',
      })
      getImages()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Delete canceled')
      }
    })
}

const onOpenDetail = async (id: number, tag: string) => {
  state.curId = id
  state.detailVisible = true
  state.curTag = tag
}

const importLoading = ref(false)
const onSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()
    importLoading.value = true
    const res = await importImage(form)
    importLoading.value = false

    if (res && res.alreadyImageId > 0) {
      ElMessage({
        type: 'warning',
        message: 'Image already existed. We don\'t need to import it again.',
      })
      visible.value = false
      getImages()
      return
    }

    ElMessage({ message: 'Import successful', type: 'success' })
    visible.value = false
    getImages()
  } catch (err) {
    importLoading.value = false
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey as any)
      ElMessage.error(firstMsg)
    }
  }
}

const getImages = async () => {
  loading.value = true
  try {
    const params: any = {
      page_num: imagePagination.page,
      page_size: imagePagination.pageSize,
      orderBy: 'created_at',
      order: 'desc',
    }

    // Add search criteria to params if present
    if (imageFilter.value) {
      params.image = imageFilter.value
    }

    const res = await getImagesList(params)
    state.rowData = res.images || []
    imagePagination.total = res.totalCount || 0
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to fetch images')
  } finally {
    loading.value = false
  }
}

const getImgRegs = async () => {
  const res = await getImageRegList({})
  regOptions.value = res?.map((v: any) => v.url) || []
}

const onOpen = async () => {
  ruleFormRef.value?.resetFields()
  Object.assign(form, initialForm())
  getImgRegs()
  fetchSecrets()
  await nextTick()
}

// Image list pagination handling
const handleImagePageChange = (newPage: number) => {
  imagePagination.page = newPage
  getImages()
}

const handleImagePageSizeChange = (newSize: number) => {
  imagePagination.pageSize = newSize
  imagePagination.page = 1
  getImages()
}

// Image list search handling
const handleImageSearch = () => {
  imagePagination.page = 1
  getImages()
}

const handleImageSearchDebounced = useDebounceFn(() => {
  handleImageSearch()
}, 500)

// ========== Preheat related ==========
const prewarmVisible = ref(false)
const prewarmLoading = ref(false)
const prewarmState = reactive({
  rowData: [] as any[],
  page: 1,
  pageSize: 20,
  total: 0,
  statusFilter: '' as string, // Status filter
  workspaceFilter: '' as string, // Workspace filter (workspaceId)
  userNameFilter: '' as string, // Username filter
  imageNameFilter: '' as string, // Image name filter
})

// Preheat form
const prewarmFormRef = ref<FormInstance>()
const initialPrewarmForm = () => ({
  image: '',
  workspace: '',
})
const prewarmForm = reactive({ ...initialPrewarmForm() })
const prewarmRules = computed<FormRules>(() => ({
  image: [{ required: true, message: 'Please select image', trigger: 'change' }],
  workspace: [{ required: true, message: 'Please select workspace', trigger: 'change' }],
}))

const copyPrewarmImage = async () => {
  if (!prewarmForm.image) return
  await copyText(prewarmForm.image)
}

const imageOptions = ref([] as Array<{ id: number; tag: string }>)

const getStatusType = (status: string) => {
  const map: Record<string, any> = {
    completed: 'success',
    failed: 'danger',
    running: 'warning',
    pending: 'info',
  }
  return map[status?.toLowerCase()] || 'info'
}

// StateFilter options
const statusFilterOptions = [
  { text: 'Completed', value: 'Completed' },
  { text: 'Failed', value: 'Failed' },
  { text: 'Running', value: 'Running' },
]

// Workspace Filter options
const workspaceFilterOptions = computed(() => {
  return wsStore.items.map((ws) => ({
    text: ws.workspaceName,
    value: ws.workspaceId, // Use workspaceId as value
  }))
})

// UserName filter options - extract unique usernames from current data
const userNameFilterOptions = computed(() => {
  const uniqueUserNames = [
    ...new Set(prewarmState.rowData.map((item) => item.userName).filter(Boolean)),
  ]
  return uniqueUserNames.map((userName) => ({
    text: userName,
    value: userName,
  }))
})

// Unified header filter handling
const handleFilterChange = (filters: Record<string, string[]>) => {
  // Handle Status filter
  if ('status' in filters) {
    const selectedStatuses = filters.status || []
    prewarmState.statusFilter = selectedStatuses.length === 1 ? selectedStatuses[0] : ''
  }

  // Handle Workspace filter (pass workspaceId)
  if ('workspace' in filters) {
    const selectedWorkspaces = filters.workspace || []
    prewarmState.workspaceFilter = selectedWorkspaces.length === 1 ? selectedWorkspaces[0] : ''
  }

  // Handle UserName filter
  if ('userName' in filters) {
    const selectedUserNames = filters.userName || []
    prewarmState.userNameFilter = selectedUserNames.length === 1 ? selectedUserNames[0] : ''
  }

  prewarmState.page = 1
  getPrewarmList()
}

// Preheat list search handling
const handlePrewarmSearch = () => {
  prewarmState.page = 1
  getPrewarmList()
}

const handlePrewarmSearchDebounced = useDebounceFn(() => {
  handlePrewarmSearch()
}, 500)

// Get preheat list
const getPrewarmList = async () => {
  prewarmLoading.value = true
  try {
    const params: any = {
      // offset: (prewarmState.page - 1) * prewarmState.pageSize,
      // limit: prewarmState.pageSize,
    }

    // Add status filter to params if present
    if (prewarmState.statusFilter) {
      params.status = prewarmState.statusFilter
    }

    // Add workspace filter to params if present (value is workspaceId, but param name is workspace)
    if (prewarmState.workspaceFilter) {
      params.workspace = prewarmState.workspaceFilter
    }

    // Add username filter to params if present
    if (prewarmState.userNameFilter) {
      params.userName = prewarmState.userNameFilter
    }

    // Add image name filter to params if present
    if (prewarmState.imageNameFilter) {
      params.image = prewarmState.imageNameFilter
    }

    const res = await getImagePrewarmList(params)
    prewarmState.rowData = res.items || []
    prewarmState.total = res.totalCount || 0
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to fetch preheat list')
  } finally {
    prewarmLoading.value = false
  }
}

// Load image list when opening preheat dialog
const onPrewarmOpen = async () => {
  prewarmFormRef.value?.resetFields()
  Object.assign(prewarmForm, initialPrewarmForm())

  // Set default workspace
  if (wsStore.currentWorkspaceId) {
    prewarmForm.workspace = wsStore.currentWorkspaceId
  }

  const res = await getImagesList({ flat: true })
  imageOptions.value = res ?? []

  await nextTick()
}

// Submit preheat
const onPrewarmSubmit = async (formEl: FormInstance | undefined) => {
  if (!formEl) return
  try {
    await formEl.validate()

    // Call opsjobs API
    await rebootNodes({
      name: `prewarm-${Date.now()}`,
      type: 'prewarm',
      inputs: [
        { name: 'image', value: prewarmForm.image },
        { name: 'workspace', value: prewarmForm.workspace },
      ],
      timeoutSecond: 1800,
    })

    ElMessage({ message: 'Preheat task created successfully', type: 'success' })
    prewarmVisible.value = false

    // Switch to preheat records tab and refresh
    activeTab.value = 'prewarm'
    getPrewarmList()
  } catch (err) {
    if (err && typeof err === 'object' && !(err instanceof Error)) {
      const fields = err as Record<string, Array<{ message?: string }>>
      const firstKey = Object.keys(fields)[0]
      const firstMsg = fields[firstKey]?.[0]?.message || 'Invalid form'
      formEl.scrollToField?.(firstKey as any)
      ElMessage.error(firstMsg)
    }
  }
}

// Retry preheat
const onRetryPrewarm = async (row: any) => {
  try {
    const msg = h('span', null, [
      'Are you sure you want to retry preheating image: ',
      h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, row.imageName),
      ' ?',
    ])

    ElMessageBox.confirm(msg, 'Retry Preheat', {
      confirmButtonText: 'Retry',
      cancelButtonText: 'Cancel',
      type: 'warning',
    }).then(async () => {
      // Call opsjobs API to retry preheat
      await rebootNodes({
        name: `prewarm-retry-${Date.now()}`,
        type: 'prewarm',
        inputs: [
          { name: 'image', value: row.imageName },
          { name: 'workspace', value: row.workspaceId || row.workspaceName },
        ],
        timeoutSecond: 1800,
      })

      ElMessage({ message: 'Retry preheat task created successfully', type: 'success' })
      getPrewarmList()
    })
  } catch (err) {
    ElMessage.error((err as Error).message || 'Failed to retry preheat')
  }
}

// Auto-poll when any prewarm task is Running
const pollTimer = ref<ReturnType<typeof setInterval>>()

watch(() => prewarmState.rowData, (rows) => {
  clearInterval(pollTimer.value)
  if (rows.some((r: any) => r.status === 'Running')) {
    pollTimer.value = setInterval(getPrewarmList, 15000)
  }
}, { immediate: true })

onUnmounted(() => clearInterval(pollTimer.value))

// Watch tab switch, auto load corresponding data
watch(activeTab, (newTab) => {
  if (newTab === 'prewarm' && prewarmState.rowData.length === 0) {
    getPrewarmList()
  }
})

onMounted(() => {
  getImages()
  // If initial tab is prewarm, load preheat data
  if (activeTab.value === 'prewarm') {
    getPrewarmList()
  }
})

defineOptions({
  name: 'ImagesPage',
})
</script>

<style scoped>
.header-section {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.page-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.actions-wrapper {
  display: flex;
  gap: 12px;
}

.progress-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.progress-pct {
  font-size: 12px;
  color: transparent;
  transition: color 0.2s ease;
  white-space: nowrap;
}

.progress-cell:hover .progress-pct {
  color: var(--el-text-color-secondary);
}
</style>
<style>
/* Reuse project-wide segmented unified styles */
.myself-seg .el-segmented__item-selected {
  background: none;
}
.myself-seg .el-segmented__item.is-selected {
  color: var(--safe-primary) !important;
}
</style>

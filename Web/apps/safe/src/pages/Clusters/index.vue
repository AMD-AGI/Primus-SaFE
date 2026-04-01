<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">Clusters</el-text>
    <el-button
      type="primary"
      round
      :icon="Plus"
      class="m-t-4 text-black"
      @click="
        () => {
          addVisible = true
        }
      "
    >
      Create Cluster
    </el-button>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 205px)'"
      :data="clusterStore.items"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column prop="clusterId" label="Cluster ID">
        <template #default="{ row }">
          <el-link type="primary" v-route="{ path: '/cluster/detail', query: { id: row.clusterId } }">{{ row.clusterId }}</el-link>
        </template>
      </el-table-column>
      <el-table-column prop="phase" label="Phase">
        <template #default="{ row }">
          <el-tag :type="row.phase === 'Ready' ? 'success' : 'danger'">{{ row.phase }}</el-tag>
        </template>
      </el-table-column>

      <el-table-column prop="isProtected" label="Is Protected">
        <template #default="{ row }">
          <el-switch
            v-model="row.isProtected"
            size="large"
            class="m-r-2"
            inline-prompt
            :active-icon="Check"
            :inactive-icon="Close"
            @change="(val: boolean) => changeProtected(val, row.clusterId)"
          />
        </template>
      </el-table-column>
      <el-table-column prop="creationTime" label="Creation Time">
        <template #default="{ row }">
          {{ row.creationTime ? dayjs(row.creationTime).format('YYYY-MM-DD HH:mm:ss') : '-' }}
        </template>
      </el-table-column>
      <el-table-column label="Actions" width="180" fixed="right">
        <template #default="{ row }">
          <!-- First 2 actions inline -->
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
            :visible="moreOpenId === row.clusterId"
            @hide="moreOpenId === row.clusterId && (moreOpenId = null)"
          >
            <template #reference>
              <el-button
                circle
                class="btn-primary-plain"
                :icon="MoreFilled"
                size="default"
                @click.stop="toggleMore(row.clusterId)"
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
  </el-card>

  <ManageDialog
    v-model:visible="state.showDialog"
    :action="state.curAction"
    :rowdata="state.rowData"
    :id="state.id"
  />

  <AddDialog v-model:visible="addVisible" @success="clusterStore.fetchClusters()" />

  <el-dialog
    v-model="editState.editVisible"
    title="Bind Image Secret"
    width="520px"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div class="space-y-3">
      <div class="textx-12 mt-2 flex" style="font-weight: 500; align-items: center">
        Image Secret:
        <el-select
          v-model="selectedSecretId"
          size="default"
          class="mt-3 mb-3 ml-2"
          style="width: 300px"
        >
          <el-option
            v-for="item in editState.imageSecretOptions"
            :key="item"
            :label="item"
            :value="item"
          />
        </el-select>
      </div>
    </div>

    <template #footer>
      <el-button :disabled="editState.bindLoading" @click="editState.editVisible = false"
        >Cancel</el-button
      >
      <el-button
        type="primary"
        :loading="editState.bindLoading"
        :disabled="!selectedSecretId"
        @click="onBindConfirm"
        >Confirm</el-button
      >
    </template>
  </el-dialog>
</template>
<script lang="ts" setup>
import { ref, onMounted, reactive, nextTick, h } from 'vue'
import { useClusterStore } from '@/stores/cluster'
import { useUserStore } from '@/stores/user'
import { getNodesList, editClusterProtected, deleteCluster, getClusterDetail } from '@/services'
import { getSecrets } from '@/services'
import ManageDialog from './Components/ManageDialog.vue'
import { Check, Close, Plus, Minus, Edit, Delete, MoreFilled } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import AddDialog from './Components/AddDialog.vue'
import { useRouter } from 'vue-router'
import dayjs from 'dayjs'

interface ClusterRowItem {
  clusterId: string
  phase: string
  endpoint: string
  storage: unknown
  isProtected: boolean
}

const router = useRouter()
const userStore = useUserStore()

const state = reactive({
  showDialog: false,
  curAction: '',
  id: '',
  rowData: [],
})

const editState = reactive({
  curId: '',
  curSerect: '',
  imageSecretOptions: [] as string[],
  bindLoading: false,
  editVisible: false,
})
const selectedSecretId = ref('')

const addVisible = ref(false)
const clusterStore = useClusterStore()
const loading = ref(false)

const getNodeList = async (id?: string) => {
  const res = await getNodesList({ clusterId: id, limit: -1, brief: true })
  state.rowData = res.items || []
}

type Action = {
  key: string
  label: string
  icon: any
  btnClass?: string
  disabled?: (row: ClusterRowItem) => boolean
  onClick: (row: ClusterRowItem) => void | Promise<void>
}
const getActions = (row: ClusterRowItem): Action[] => [
  {
    key: 'manage',
    label: 'Manage',
    icon: Plus,
    btnClass: 'btn-success-plain',
    onClick: () => openDialog('Manage', row),
  },
  {
    key: 'unmanage',
    label: 'Unmanage',
    icon: Minus,
    btnClass: 'btn-danger-plain',
    onClick: () => openDialog('Unmanage', row),
  },
  {
    key: 'bind',
    label: 'Bind Image Secret',
    icon: Edit,
    btnClass: 'btn-primary-plain',
    onClick: () => handleBind(row.clusterId),
  },
  {
    key: 'delete',
    label: 'Delete',
    icon: Delete,
    btnClass: 'btn-danger-plain',
    onClick: async (r: ClusterRowItem) => {
      const msg = h('span', null, [
        'Are you sure you want to delete cluster: ',
        h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, r.clusterId),
        ' ?',
      ])

      await ElMessageBox.confirm(msg, 'Delete cluster', {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning',
      })
      await deleteCluster(r.clusterId)
      ElMessage.success('Deleted')
      clusterStore.fetchClusters()
    },
  },
]

const moreOpenId = ref<string | null>(null) // Currently open popover row ID (only one at a time)
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
const handleMenuClick = async (act: Action, row: ClusterRowItem) => {
  if (act.disabled?.(row)) return
  await act.onClick(row)
  closeMore()
}

const jumpToDetail = (id: string) => {
  router.push({ path: '/cluster/detail', query: { id } })
}

const handleBind = async (id: string) => {
  const res = await getClusterDetail(id)
  selectedSecretId.value = res?.imageSecretId || ''
  editState.curId = id
  fetchSecrets()
  editState.editVisible = true
}

const openDialog = (action: 'Manage' | 'Unmanage', row?: ClusterRowItem) => {
  state.curAction = action
  state.showDialog = true
  state.id = row?.clusterId || ''
  // Manage: fetch nodes with no cluster assigned
  // Unmanage: fetch nodes assigned to current cluster
  getNodeList(action === 'Manage' ? '' : row?.clusterId)
}

const changeProtected = async (val: boolean, id: string) => {
  try {
    await editClusterProtected(id, { isProtected: val })
    ElMessage({
      type: 'success',
      message: 'Edit completed',
    })
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    clusterStore.fetchClusters()
  }
}
const onBindConfirm = async () => {
  try {
    await editClusterProtected(editState.curId, { imageSecretId: selectedSecretId.value })
    ElMessage({
      type: 'success',
      message: 'Bind completed',
    })
    editState.editVisible = false
    editState.curId = ''
    selectedSecretId.value = ''
  } catch (err) {
    if (err instanceof Error) throw err
  } finally {
    clusterStore.fetchClusters()
  }
}

const fetchSecrets = async () => {
  const imageSecrets = await getSecrets({ type: 'image' }).catch(() => ({ items: [] }))
  editState.imageSecretOptions = (imageSecrets?.items ?? []).map(
    (s: any) => s.secretId ?? s.name ?? s.id,
  )
}

onMounted(() => {
  loading.value = true
  clusterStore.fetchClusters()
  loading.value = false
})
defineOptions({
  name: 'ClustersPage',
})
</script>

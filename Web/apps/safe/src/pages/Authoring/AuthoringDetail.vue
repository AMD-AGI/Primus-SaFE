<template>
  <WorkloadHeader
    v-if="detailData"
    :detail-data="detailData"
    :edit-disabled="editDisabled"
    @edit="onEdit"
    @clone="onClone"
    @delete="onDelete"
    @stop="onStop"
    @resume="onResume"
  />

  <el-tabs v-model="activeTab" class="mt-4" @tab-click="handleClick">
    <el-tab-pane label="Overview" name="overview">
      <el-card class="mt-2 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">configuration</span>
        </div>

        <el-descriptions v-if="detailData" class="m-t-4" border :column="5" direction="vertical">
          <el-descriptions-item label="image">{{
            (Array.isArray(detailData.images) ? detailData.images[0] : undefined) ??
            detailData.image ??
            '-'
          }}</el-descriptions-item>

          <el-descriptions-item label="kind">{{
            detailData.groupVersionKind?.kind ?? '-'
          }}</el-descriptions-item>

          <el-descriptions-item label="specifiedNode" v-if="detailData?.specifiedNodes">{{
            detailData?.specifiedNodes?.join(',') || '-'
          }}</el-descriptions-item>

          <el-descriptions-item label="excludedNodes" v-if="detailData?.excludedNodes">{{
            detailData?.excludedNodes?.join(',') || '-'
          }}</el-descriptions-item>

          <el-descriptions-item label="env" :span="2" v-if="detailData.env">
            <div>
              <template v-if="Object.keys(detailData.env).length === 1">
                <div>{{ firstEnvKey }}: {{ firstEnvValue }}</div>
              </template>
              <template v-else>
                <template v-if="!envExpanded">
                  <div>{{ firstEnvKey }}: {{ firstEnvValue }}</div>
                  <el-button type="text" @click="envExpanded = true">View More</el-button>
                </template>
                <template v-else>
                  <div v-for="(value, key) in detailData.env" :key="key">
                    {{ key }}: {{ value }}
                  </div>
                  <el-button type="text" @click="envExpanded = false">Collapse</el-button>
                </template>
              </template>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="forceHostNetwork">{{
            detailData.forceHostNetwork ? 'Yes' : 'No'
          }}</el-descriptions-item>
        </el-descriptions>
      </el-card>

      <el-card class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">resource</span>
        </div>
        <div class="grid gap-3 mt-4 sm:grid-cols-2 lg:grid-cols-3">
          <StatCard
            label="Replicas"
            :value="detailData?.resources?.[0]?.replica ?? '-'"
            :icon="DataLine"
          />
          <StatCard label="CPU" :value="detailData?.resources?.[0]?.cpu ?? '-'" :icon="Cpu" />
          <StatCard label="GPU" :value="detailData?.resources?.[0]?.gpu ?? '-'" :icon="Monitor" />
          <StatCard label="Memory" :value="detailData?.resources?.[0]?.memory ?? '-'" :icon="Box" />
          <StatCard
            label="Ephemeral Storage"
            :value="detailData?.resources?.[0]?.ephemeralStorage ?? '-'"
            :icon="Collection"
          />
          <StatCard
            label="RDMA"
            :value="detailData?.resources?.[0]?.rdmaResource ?? '-'"
            :icon="Connection"
          />
        </div>
      </el-card>
    </el-tab-pane>
    <el-tab-pane label="Pods" name="pods">
      <WorkloadPodsTable
        :pods="detailData?.pods"
        :workload-phase="detailData?.phase"
        :refresh-loading="detailLoading"
        :show-ssh="true"
        @open-log="openLog"
        @open-ssh="openSsh"
        @refresh="refreshPods"
      />
    </el-tab-pane>
    <el-tab-pane label="Timeline" name="timeline">
      <WorkloadTimeline :conditions="detailData?.conditions" />
    </el-tab-pane>
    <el-tab-pane label="Logs" name="logs" lazy v-if="userStore.envs?.enableLog">
      <LogTable
        :wlid="workloadId"
        :dispatchCount="detailData?.dispatchCount"
        :nodes="detailData?.nodes"
        :ranks="detailData?.ranks"
        :failedNodes="detailData?.failedNodes"
        :isDownload="userStore.envs?.enableLogDownload"
      />
    </el-tab-pane>
    <el-tab-pane label="Grafana" name="Grafana" lazy>
      <GrafanaIframe
        path="/lens/grafana/d/training-workload/training-workload"
        :orgId="1"
        datasource=""
        varKey="var-workload_uid"
        :varValue="detailData?.workloadUid"
        :time="defaultTime"
        theme="dark"
        kiosk
        refresh="30s"
        height="1050px"
        :cluster="detailData?.clusterId"
      />
    </el-tab-pane>
    <el-tab-pane label="Images" name="Custom Images" lazy>
      <div class="mb-4">
        <el-button type="primary" class="save-image-btn" @click="saveImages">
          <img
            :src="saveIcon"
            alt="Save"
            class="w-4 h-4 pointer-events-none object-contain mr-2 save-icon-black"
          />
          Save as Image
        </el-button>
      </div>
      <el-card class="mt-2 safe-card" shadow="never">
        <el-table v-if="imageLogData" :data="imageLogData" v-loading="imageLogLoading">
          <el-table-column prop="imageName" label="Image Name" min-width="260">
            <template #default="{ row }">
              <span v-if="row.imageName">
                {{ row.imageName }}
              </span>

              <span v-else-if="row.status === 'Failed'" class="text-red-500 text-xs">
                Save failed – no image generated
              </span>

              <span v-else class="text-gray-400 text-xs"> - </span>
              <el-icon
                class="cursor-pointer hover:text-blue-500 transition ml-2"
                size="11"
                v-if="row.imageName"
                @click="copyText(row.imageName)"
              >
                <CopyDocument />
              </el-icon>
            </template>
          </el-table-column>
          <el-table-column prop="label" label="Label" min-width="260" show-overflow-tooltip>
            <template #default="{ row }">
              {{ row.label || '-' }}
            </template>
          </el-table-column>
          <el-table-column prop="status" label="Status" min-width="180">
            <template #default="{ row }">
              <el-tag :type="WorkloadPhaseButtonType[row.status]?.type || 'info'">{{
                row.status
              }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="createdTime" label="Created Time" min-width="160">
            <template #default="{ row }">
              {{ formatTimeStr(row.createdTime) }}
            </template>
          </el-table-column>
          <el-table-column label="Actions" width="100" fixed="right" align="center">
            <template #default="{ row }">
              <el-button type="text" size="small" @click="handleDeleteCustomImage(row)">
                Delete
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </el-tab-pane>
  </el-tabs>
  <LogsDialog v-model:visible="logVisible" :wlid="workloadId" :podid="curPodId" />
  <AddDialog
    v-model:visible="addVisible"
    :wlid="workloadId"
    :action="addAction"
    @success="addAction === 'Edit' ? getDetail() : router.push('/authoring')"
  />
  <SshConfigDialog
    v-model:visible="sshVisible"
    :wlid="workloadId"
    :podid="curPodId"
    :ssh-command="curSshCommand"
    :enable-image-saving-check="true"
  />
</template>
<script lang="ts" setup>
import { onMounted, ref, computed, h } from 'vue'
import { useRouter } from 'vue-router'
import {
  getImageCustom,
  deleteImageCustom,
  downloadWlLogs,
  WorkloadPhaseButtonType,
} from '@/services'

interface ImageCustomRow {
  jobId: string
  imageName?: string
  label?: string
  status: string
  createdTime?: string
}
import {
  CopyDocument,
  Cpu,
  Monitor,
  Collection,
  Connection,
  Box,
  DataLine,
} from '@element-plus/icons-vue'
import LogsDialog from '@/components/Workload/LogsDialog.vue'
import SshConfigDialog from '@/components/Workload/SshConfigDialog.vue'
import AddDialog from './Components/AddDialog.vue'
import LogTable from './Components/LogTable.vue'
import StatCard from '@/components/Base/StatCard.vue'
import GrafanaIframe from '@/components/Base/GrafanaIframe.vue'
import WorkloadHeader from '@/components/Workload/WorkloadHeader.vue'
import WorkloadPodsTable from '@/components/Workload/WorkloadPodsTable.vue'
import WorkloadTimeline from '@/components/Workload/WorkloadTimeline.vue'
import { copyText, formatTimeStr, calculateDefaultTime } from '@/utils/index'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '@/stores/user'
import { useWorkloadDetail } from '@/composables/useWorkloadDetail'
import { usePodActions } from '@/composables/usePodActions'
import type { TabsPaneContext } from 'element-plus'
import saveIcon from '@/assets/icons/save.png'

const router = useRouter()
const userStore = useUserStore()

const { workloadId, detailData, detailLoading, getDetail, onDelete, onStop, onResume } =
  useWorkloadDetail({
    redirectPath: '/authoring',
    extractFailedNodes: true,
  })

const { curPodId, curSshCommand, logVisible, sshVisible, openLog, openSsh } = usePodActions()

const activeTab = ref('overview')
const addVisible = ref(false)
const addAction = ref<'Clone' | 'Edit'>('Clone')

const editDisabled = computed(() => {
  const d = detailData.value
  if (!d) return true
  return !['Running', 'Pending'].includes(d.phase)
})

const onEdit = () => {
  addAction.value = 'Edit'
  addVisible.value = true
}

const onClone = () => {
  addAction.value = 'Clone'
  addVisible.value = true
}

const refreshPods = async () => {
  await getDetail()
  activeTab.value = 'pods'
}
const imageLogData = ref()
const imageLogLoading = ref(false)
const envExpanded = ref(false)

const firstEnvKey = computed(() => {
  if (!detailData?.value.env) return ''
  return Object.keys(detailData.value.env)[0] || ''
})
const firstEnvValue = computed(() => {
  if (!detailData?.value.env) return ''
  const key = Object.keys(detailData.value.env)[0]
  return key ? detailData.value.env[key] : ''
})

const saveImages = async () => {
  ElMessageBox.prompt('Please enter the reason for saving this image.', 'Save Image', {
    confirmButtonText: 'Save',
    cancelButtonText: 'Cancel',
    type: 'warning',
    inputPlaceholder: 'max 64 characters',
    inputValidator: (value) => {
      const v = (value || '').trim()

      if (!v) {
        return 'Reason is required'
      }

      if (v.length > 64) {
        return 'Reason must be at most 64 characters'
      }

      return true
    },
  })
    .then(async (result) => {
      const { value } = result as { value: string }
      const label = value.trim()

      await downloadWlLogs({
        name: detailData.value?.workloadId,
        inputs: [
          {
            name: 'workload',
            value: detailData.value?.workloadId,
          },
          {
            name: 'label',
            value: label,
          },
        ],
        type: 'exportimage',
        timeoutSecond: 1800,
      })

      ElMessage({
        type: 'success',
        message: 'Save completed',
      })

      activeTab.value = 'Custom Images'
      getImageLogs()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Save canceled')
      }
    })
}

const getImageLogs = async () => {
  try {
    imageLogLoading.value = true
    const res = await getImageCustom({ workload: detailData?.value.workloadId })
    imageLogData.value = res?.items ?? []
  } catch {
  } finally {
    imageLogLoading.value = false
  }
}

const handleClick = async (tab: TabsPaneContext) => {
  if (tab.paneName === 'Custom Images') getImageLogs()
}

const handleDeleteCustomImage = async (row: ImageCustomRow) => {
  const msg = h('span', null, [
    'Are you sure you want to delete ',
    h(
      'span',
      { style: 'color: var(--el-color-primary); font-weight: 600' },
      row.imageName || 'this image',
    ),
    ' ?',
  ])

  ElMessageBox.confirm(msg, 'Delete custom image', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      await deleteImageCustom(row.jobId)
      ElMessage({
        type: 'success',
        message: 'Delete completed',
      })
      getImageLogs()
    })
    .catch((err) => {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Delete canceled')
      }
    })
}

const defaultTime = computed<[Date, Date] | null>(() => {
  return calculateDefaultTime(
    detailData.value?.startTime,
    detailData.value?.endTime,
    detailData.value?.creationTime,
  )
})

onMounted(() => {
  userStore.fetchEnvs()
})
</script>
<style scoped>
.glass-btn {
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);
  background: var(--button-bg-color);
  border: 1px solid rgba(255, 255, 255, 0.15);
  color: var(--el-text-color-primary);
  transition:
    transform 0.2s ease,
    border-color 0.2s ease;
}

.glass-btn:hover {
  transform: scale(1.05);
  border-color: rgba(255, 255, 255, 0.35);
}

.glass-btn--export {
  color: var(--el-color-primary);
}

.save-icon-white {
  filter: brightness(0) invert(1);
}

.save-icon-black {
  filter: brightness(0);
}

.save-image-btn.el-button--primary {
  color: #000 !important;
}

.save-image-btn.el-button--primary :deep(span) {
  color: #000 !important;
}
</style>

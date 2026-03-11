<template>
  <WorkloadHeader
    v-if="downloadDetail"
    :detail-data="downloadDetail"
    @delete="onDelete"
    @stop="onStop"
  />

  <el-tabs v-model="activeTab" class="mt-4">
    <el-tab-pane label="Overview" name="overview">
      <el-card class="mt-2 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">configuration</span>
        </div>

        <el-descriptions
          v-if="downloadDetail"
          class="m-t-4"
          border
          :column="5"
          direction="vertical"
        >
          <el-descriptions-item label="Secret" :span="3">
            {{ getInputValue('secret') }}
          </el-descriptions-item>
          <el-descriptions-item label="Url" :span="5">
            <div style="word-break: break-all">
              {{ getInputValue('endpoint') }}
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="Destination Path" :span="5">
            {{ getInputValue('dest.path') }}
          </el-descriptions-item>
          <el-descriptions-item label="Outputs" :span="5">
            <div v-if="outputsText" style="white-space: pre-wrap; word-break: break-word">
              {{ outputsText }}
            </div>
            <span v-else>-</span>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <el-card class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">resource</span>
        </div>
        <div class="grid gap-3 mt-4 sm:grid-cols-2 lg:grid-cols-3">
          <StatCard label="CPU" :value="workloadDetail?.resource?.cpu ?? '-'" :icon="Cpu" />
          <StatCard label="GPU" :value="workloadDetail?.resource?.gpu ?? '-'" :icon="Monitor" />
          <StatCard label="Memory" :value="workloadDetail?.resource?.memory ?? '-'" :icon="Box" />
          <StatCard
            label="Ephemeral Storage"
            :value="workloadDetail?.resource?.ephemeralStorage ?? '-'"
            :icon="Collection"
          />
        </div>
      </el-card>
    </el-tab-pane>
    <el-tab-pane label="Pods" name="pods">
      <WorkloadPodsTable
        :pods="workloadDetail?.pods"
        :workload-phase="workloadDetail?.phase"
        :refresh-loading="workloadLoading"
        :show-ssh="false"
        @open-log="openLog"
        @refresh="refreshPods"
      />
    </el-tab-pane>
    <el-tab-pane label="Logs" name="logs" lazy v-if="userStore.envs?.enableLog">
      <LogTable
        :wlid="workloadId"
        :dispatchCount="workloadDetail?.dispatchCount"
        :nodes="workloadDetail?.nodes"
        :ranks="workloadDetail?.ranks"
        :failedNodes="workloadDetail?.failedNodes"
        :isDownload="userStore.envs?.enableLogDownload"
      />
    </el-tab-pane>
    <el-tab-pane label="Grafana" name="Grafana" lazy>
      <GrafanaIframe
        path="/lens/grafana/d/training-workload/training-workload"
        :orgId="1"
        datasource=""
        varKey="var-workload_uid"
        :varValue="workloadDetail?.workloadUid"
        :time="defaultTime"
        theme="dark"
        kiosk
        refresh="30s"
        height="1050px"
        :cluster="workloadDetail?.clusterId"
      />
    </el-tab-pane>
  </el-tabs>
  <LogsDialog v-model:visible="logVisible" :wlid="workloadId" :podid="curPodId || ''" />
  <SshConfigDialog
    v-model:visible="sshVisible"
    :wlid="workloadId"
    :podid="curPodId || ''"
    :ssh-command="curSshCommand"
  />
</template>

<script lang="ts" setup>
import { onMounted, ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { getOpsjobsDetail } from '@/services'
import { Cpu, Monitor, Collection, Box } from '@element-plus/icons-vue'
import LogsDialog from '@/components/Workload/LogsDialog.vue'
import SshConfigDialog from '@/components/Workload/SshConfigDialog.vue'
import LogTable from './Components/LogTable.vue'
import StatCard from '@/components/Base/StatCard.vue'
import WorkloadHeader from '@/components/Workload/WorkloadHeader.vue'
import WorkloadPodsTable from '@/components/Workload/WorkloadPodsTable.vue'
import GrafanaIframe from '@/components/Base/GrafanaIframe.vue'
import { usePodActions } from '@/composables/usePodActions'
import { useWorkloadDetail } from '@/composables/useWorkloadDetail'
import { useUserStore } from '@/stores/user'
import { calculateDefaultTime } from '@/utils'

interface InputItem {
  name?: string
  value?: string
}

interface OutputItem {
  name?: string
  value?: string
}

interface DownloadDetailData {
  workspaceId?: string
  inputs?: InputItem[]
  outputs?: OutputItem[]
  resource?: {
    cpu?: string
    gpu?: string
    memory?: string
    ephemeralStorage?: string
  }
  // Properties required by WorkloadHeader
  displayName?: string
  jobName?: string
  phase: string
  message?: string
  workloadId?: string
  jobId?: string
  creationTime?: string
  endTime?: string
  userName?: string
  description?: string
}

const route = useRoute()
const userStore = useUserStore()

const workloadId = computed(() => route.query.id as string)
const downloadDetail = ref<DownloadDetailData | null>(null)
const detailLoading = ref(false)

const {
  detailData: workloadDetail,
  detailLoading: workloadLoading,
  getDetail: getWorkloadDetailData,
  onDelete,
  onStop,
} = useWorkloadDetail({
  redirectPath: '/download',
  overrideWorkspaceId: () => downloadDetail.value?.workspaceId,
})

const { curPodId, curSshCommand, logVisible, sshVisible, openLog } = usePodActions()

const activeTab = ref('overview')

const refreshPods = async () => {
  await getWorkloadDetailData()
  activeTab.value = 'pods'
}

const defaultTime = computed<[Date, Date] | null>(() => {
  return calculateDefaultTime(
    workloadDetail.value?.startTime,
    workloadDetail.value?.endTime,
    workloadDetail.value?.creationTime,
  )
})

// Get value by name from inputs
const getInputValue = (name: string) => {
  const inputs = downloadDetail.value?.inputs || []
  const input = inputs.find((i) => i?.name === name)
  return input?.value || '-'
}

// Extract outputs text
const outputsText = computed(() => {
  const outputs = downloadDetail.value?.outputs || []
  if (!outputs.length) return ''

  // Find result output
  const resultOutput = outputs.find((o) => o?.name === 'result')
  if (resultOutput?.value) {
    return resultOutput.value
  }

  // If no result, return all outputs
  return (
    outputs
      .filter((o) => o?.value)
      .map((o) => `${o.name}: ${o.value}`)
      .join('\n') || ''
  )
})

const getDetail = async () => {
  if (!workloadId.value) return
  detailLoading.value = true
  try {
    downloadDetail.value = await getOpsjobsDetail(workloadId.value)
  } catch (err) {
    console.error('Failed to fetch download detail:', err)
  } finally {
    detailLoading.value = false
  }
}

onMounted(() => {
  getDetail()
  userStore.fetchEnvs()
})
</script>

<style scoped>
.pulse-scale {
  animation: pulse-scale 1.4s ease-in-out infinite;
}
@keyframes pulse-scale {
  0%,
  100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.2);
  }
}
.el-descriptions__body {
  background-color: none !important;
}
</style>

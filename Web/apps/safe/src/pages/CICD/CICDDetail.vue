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

  <el-tabs v-model="activeTab" class="mt-4">
    <el-tab-pane label="Overview" name="overview">
      <el-card class="mt-2 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">configuration</span>
        </div>

        <el-descriptions v-if="detailData" class="m-t-4" border :column="5" direction="vertical">
          <el-descriptions-item label="image">{{ displayImage || '-' }}</el-descriptions-item>
          <el-descriptions-item label="kind">{{
            detailData.groupVersionKind?.kind ?? '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="priority">{{
            PRIORITY_LABEL_MAP[detailData.priority as PriorityValue] ?? '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="timeout" v-if="detailData.timeout"
            >{{ detailData.timeout ?? 0 }}s</el-descriptions-item
          >
          <el-descriptions-item label="secondsUntilTimeout" v-if="detailData.secondsUntilTimeout">{{
            detailData.secondsUntilTimeout
          }}</el-descriptions-item>
        </el-descriptions>
        <el-descriptions v-if="detailData" border :column="5" direction="vertical" class="no-margin-top">
          <el-descriptions-item label="entryPoint" :span="3" v-if="displayEntryPoint">
            <div>
              <span v-if="!entryPointExpanded">
                {{ truncatedEntryPoint }}...
                <el-button type="text" @click="entryPointExpanded = true">View More</el-button>
              </span>
              <span v-else>
                <pre style="white-space: pre-wrap; word-break: break-word">{{
                  displayEntryPoint
                }}</pre>
                <el-button type="text" @click="entryPointExpanded = false">Collapse</el-button>
              </span>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="specifiedNode" v-if="detailData?.specifiedNodes?.length">{{
            detailData?.specifiedNodes?.join(',') || '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="nodesAffinity" v-if="detailData.nodesAffinity">{{
            detailData.nodesAffinity
          }}</el-descriptions-item>
          <el-descriptions-item label="GitHubConfigURL" v-if="envData.githubConfigUrl">{{
            envData.githubConfigUrl
          }}</el-descriptions-item>
          <el-descriptions-item label="multiNodes">{{
            envData.unifiedJobEnable ? 'Enabled' : 'Disabled'
          }}</el-descriptions-item>
          <el-descriptions-item label="actionRunner" v-if="detailData.scaleRunnerId">{{
            detailData.scaleRunnerId || '-'
          }}</el-descriptions-item>
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
        <div class="grid gap-3 mt-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard
            label="CPU"
            :value="envData.resources?.cpu ?? detailData?.resources?.[0]?.cpu ?? '-'"
            :icon="Cpu"
          />
          <StatCard
            label="GPU"
            :value="envData.resources?.gpu ?? detailData?.resources?.[0]?.gpu ?? '-'"
            :icon="Monitor"
          />
          <StatCard
            label="Memory"
            :value="envData.resources?.memory ?? detailData?.resources?.[0]?.memory ?? '-'"
            :icon="Box"
          />
          <StatCard
            label="Ephemeral Storage"
            :value="
              envData.resources?.ephemeralStorage ??
              detailData?.resources?.[0]?.ephemeralStorage ??
              '-'
            "
            :icon="Collection"
          />
        </div>
      </el-card>

      <el-card class="mt-6 safe-card" shadow="never" v-if="detailData?.scaleRunnerId">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">association</span>
        </div>
        <div class="mt-4">
          <el-text v-if="relatedTaskLoading">Loading...</el-text>
          <el-link
            v-else-if="relatedTaskId"
            type="primary"
            @click="jumpToRelatedTask"
            class="text-base"
          >
            {{ relatedTaskId }}
          </el-link>
          <el-text v-else>-</el-text>
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
      <LogTerminal
        :wlid="workloadId"
        :dispatchCount="detailData?.dispatchCount"
        :nodes="detailData?.nodes"
        :ranks="detailData?.ranks"
        :failedNodes="detailData?.failedNodes"
        :isDownload="userStore.envs?.enableLogDownload"
      />
    </el-tab-pane>
    <el-tab-pane
      label="Grafana"
      name="Grafana"
      lazy
      v-if="detailData?.groupVersionKind?.kind === 'UnifiedJob'"
    >
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
  </el-tabs>
  <LogsDialog v-model:visible="logVisible" :wlid="workloadId" :podid="curPodId || ''" />
  <AddDialog
    v-model:visible="addVisible"
    :wlid="workloadId"
    :action="addAction"
    @success="addAction === 'Edit' ? getDetail() : router.push('/cicd')"
  />
  <SshConfigDialog
    v-model:visible="sshVisible"
    :wlid="workloadId"
    :podid="curPodId || ''"
    :ssh-command="curSshCommand"
  />
</template>
<script lang="ts" setup>
import { onMounted, ref, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  PRIORITY_LABEL_MAP,
  type PriorityValue,
  getWorkloadsList,
  getWorkloadDetail,
} from '@/services'
import { Cpu, Monitor, Collection, Box } from '@element-plus/icons-vue'
import LogsDialog from '@/components/Workload/LogsDialog.vue'
import SshConfigDialog from '@/components/Workload/SshConfigDialog.vue'
import AddDialog from './Components/AddDialog.vue'
import LogTerminal from '@/components/Workload/LogTerminal.vue'
import StatCard from '@/components/Base/StatCard.vue'
import GrafanaIframe from '@/components/Base/GrafanaIframe.vue'
import WorkloadHeader from '@/components/Workload/WorkloadHeader.vue'
import WorkloadPodsTable from '@/components/Workload/WorkloadPodsTable.vue'
import WorkloadTimeline from '@/components/Workload/WorkloadTimeline.vue'
import { decodeFromBase64String, calculateDefaultTime } from '@/utils/index'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import { useWorkloadDetail } from '@/composables/useWorkloadDetail'
import { usePodActions } from '@/composables/usePodActions'

const router = useRouter()
const userStore = useUserStore()
const wsStore = useWorkspaceStore()

const { workloadId, detailData, detailLoading, getDetail, onDelete, onStop, onResume } =
  useWorkloadDetail({
    redirectPath: '/cicd',
    extractFailedNodes: true,
    processData: extractEnvData,
  })

const { curPodId, curSshCommand, logVisible, sshVisible, openLog, openSsh } = usePodActions()

const activeTab = ref('overview')
const addVisible = ref(false)
const addAction = ref<'Clone' | 'Edit'>('Clone')
const entryPointExpanded = ref(false)

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
const relatedTaskId = ref<string>('')
const relatedTaskLoading = ref(false)

// Data extracted from env
interface EnvData {
  image?: string
  entryPoint?: string
  githubConfigUrl?: string
  unifiedJobEnable?: boolean
  resources?: {
    replica?: number
    cpu?: string
    gpu?: string
    memory?: string
    ephemeralStorage?: string
  }
}

const envData = ref<EnvData>({
  image: '',
  entryPoint: '',
  githubConfigUrl: '',
  unifiedJobEnable: false,
  resources: undefined,
})

function extractEnvData(res: { env?: Record<string, string> }) {
  if (res.env) {
    if (res.env.IMAGE) {
      envData.value.image = res.env.IMAGE
    }

    if (res.env.ENTRYPOINT) {
      envData.value.entryPoint = decodeFromBase64String(res.env.ENTRYPOINT)
    }

    if (res.env.GITHUB_CONFIG_URL) {
      envData.value.githubConfigUrl = res.env.GITHUB_CONFIG_URL
    }

    if (res.env.UNIFIED_JOB_ENABLE) {
      envData.value.unifiedJobEnable = res.env.UNIFIED_JOB_ENABLE === 'true'
    }

    if (res.env.RESOURCES) {
      try {
        const resources = JSON.parse(res.env.RESOURCES)
        envData.value.resources = {
          replica: resources.replica,
          cpu: resources.cpu,
          gpu: resources.gpu,
          memory: resources.memory,
          ephemeralStorage: resources.ephemeralStorage,
        }
      } catch (err) {
        console.warn('Failed to parse RESOURCES from env:', err)
      }
    }
  }
}

const truncatedEntryPoint = computed(() => {
  if (!displayEntryPoint.value) return ''
  return displayEntryPoint.value.substring(0, 100)
})

const currentKind = computed(() => detailData.value?.groupVersionKind?.kind ?? '')

const displayImage = computed(() => {
  if (currentKind.value === 'UnifiedJob' || currentKind.value === 'EphemeralRunner') {
    return detailData.value?.images?.[0] || ''
  }
  return envData.value.image || ''
})

const displayEntryPoint = computed(() => {
  if (currentKind.value === 'UnifiedJob' || currentKind.value === 'EphemeralRunner') {
    const entryPoint = detailData.value?.entryPoints?.[0]
    return entryPoint ? decodeFromBase64String(entryPoint) : ''
  }
  return envData.value.entryPoint || ''
})

const defaultTime = computed<[Date, Date | 'now'] | null>(() => {
  return calculateDefaultTime(
    detailData.value?.startTime,
    detailData.value?.endTime,
    detailData.value?.creationTime,
  )
})

// Query related task
const queryRelatedTask = async () => {
  if (!detailData.value?.scaleRunnerId || !detailData.value?.scaleRunnerSet) {
    return
  }

  try {
    relatedTaskLoading.value = true
    relatedTaskId.value = ''

    // 1. Query parent workload detail to check if it's multi-node
    const parentDetail = await getWorkloadDetail(detailData.value.scaleRunnerSet)

    // 2. Check parent's unifiedJobEnable
    let isMultiNode = false
    if (parentDetail.env && parentDetail.env.UNIFIED_JOB_ENABLE) {
      isMultiNode = parentDetail.env.UNIFIED_JOB_ENABLE === 'true'
    }

    if (!isMultiNode) {
      return
    }

    // 3. Determine the target kind to query
    const currentKind = detailData.value.groupVersionKind?.kind
    const targetKind =
      currentKind === 'EphemeralRunner'
        ? 'UnifiedJob'
        : currentKind === 'UnifiedJob'
          ? 'EphemeralRunner'
          : null

    if (!targetKind) {
      return
    }

    // 4. Query related task
    const res = await getWorkloadsList({
      workspaceId: wsStore.currentWorkspaceId,
      scaleRunnerId: detailData.value.scaleRunnerId,
      kind: targetKind,
      limit: 1,
    })

    // 5. Set related task ID
    if (res?.items && res.items.length > 0) {
      relatedTaskId.value = res.items[0].workloadId
    }
  } catch (err) {
    console.error('Failed to query related task:', err)
  } finally {
    relatedTaskLoading.value = false
  }
}

// Jump to related task detail
const jumpToRelatedTask = () => {
  if (relatedTaskId.value) {
    router.push({ path: '/cicd/detail', query: { id: relatedTaskId.value } })
  }
}

// Watch detailData to query related task
watch(
  () => detailData.value,
  (newVal) => {
    if (newVal?.scaleRunnerId) {
      queryRelatedTask()
    }
  },
  { immediate: true },
)

onMounted(() => {
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
.no-margin-top {
  margin-top: -1px;
}
</style>

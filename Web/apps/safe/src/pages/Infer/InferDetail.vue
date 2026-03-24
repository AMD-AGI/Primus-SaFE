<template>
  <WorkloadHeader
    v-if="detailData"
    :detail-data="detailData"
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
          <el-descriptions-item label="image">{{
            (Array.isArray(detailData.images) ? detailData.images[0] : undefined) ??
            detailData.image ??
            '-'
          }}</el-descriptions-item>
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
          <el-descriptions-item label="entryPoint" :span="3" v-if="decodedEntryPoint">
            <div>
              <span v-if="!entryPointExpanded">
                {{ truncatedEntryPoint }}...
                <el-button type="text" @click="entryPointExpanded = true">View More</el-button>
              </span>
              <span v-else>
                <pre style="white-space: pre-wrap; word-break: break-word">{{
                  decodedEntryPoint
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
          <el-descriptions-item
            v-if="detailData.env && Object.keys(detailData.env).length > 0"
            label="env"
            :span="2"
          >
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

      <!-- Service info -->
      <el-card v-if="serviceData" class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">service</span>
        </div>

        <el-descriptions class="m-t-4" border :column="4" direction="vertical">
          <el-descriptions-item label="Type">
            <el-tag :type="serviceData.type === 'NodePort' ? 'success' : 'info'" size="small">
              {{ serviceData.type || '-' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="Cluster IP">{{
            serviceData.clusterIp || '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="Protocol">{{
            serviceData.port?.protocol || '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="Service Port">{{
            serviceData.port?.port || '-'
          }}</el-descriptions-item>

          <el-descriptions-item label="Target Port">{{
            serviceData.port?.targetPort || '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="Node Port" v-if="serviceData.port?.nodePort">
            {{ serviceData.port.nodePort }}
          </el-descriptions-item>
          <el-descriptions-item label="Internal Domain" :span="serviceData.port?.nodePort ? 2 : 3">
            <el-tooltip
              :content="serviceData.internalDomain"
              placement="top"
              v-if="serviceData.internalDomain && serviceData.internalDomain.length > 50"
            >
              <span class="truncate block max-w-full">{{ serviceData.internalDomain }}</span>
            </el-tooltip>
            <span v-else>{{ serviceData.internalDomain || '-' }}</span>
          </el-descriptions-item>

          <el-descriptions-item label="External Domain" :span="4" v-if="serviceData.externalDomain">
            <div class="flex items-center gap-2">
              <el-link :href="serviceData.externalDomain" target="_blank" type="primary">
                {{ serviceData.externalDomain }}
              </el-link>
              <el-button
                size="small"
                text
                @click="copyText(serviceData.externalDomain)"
                title="Copy URL"
              >
                <el-icon><CopyDocument /></el-icon>
              </el-button>
            </div>
          </el-descriptions-item>
        </el-descriptions>
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
  </el-tabs>
  <LogsDialog v-model:visible="logVisible" :wlid="workloadId" :podid="curPodId || ''" />
  <AddDialog
    v-model:visible="addVisible"
    :wlid="workloadId"
    :action="addAction"
    @success="addAction === 'Edit' ? getDetail() : router.push('/infer')"
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
import { getWorkloadService, PRIORITY_LABEL_MAP, type PriorityValue } from '@/services'
import {
  Cpu,
  Monitor,
  Collection,
  Connection,
  Box,
  DataLine,
  CopyDocument,
} from '@element-plus/icons-vue'
import LogsDialog from '@/components/Workload/LogsDialog.vue'
import SshConfigDialog from '@/components/Workload/SshConfigDialog.vue'
import AddDialog from './Components/AddDialog.vue'
import LogTerminal from '@/components/Workload/LogTerminal.vue'
import StatCard from '@/components/Base/StatCard.vue'
import GrafanaIframe from '@/components/Base/GrafanaIframe.vue'
import WorkloadHeader from '@/components/Workload/WorkloadHeader.vue'
import WorkloadPodsTable from '@/components/Workload/WorkloadPodsTable.vue'
import WorkloadTimeline from '@/components/Workload/WorkloadTimeline.vue'
import { decodeFromBase64String, copyText, calculateDefaultTime } from '@/utils/index'
import { useUserStore } from '@/stores/user'
import { useWorkloadDetail } from '@/composables/useWorkloadDetail'
import { usePodActions } from '@/composables/usePodActions'

const router = useRouter()
const userStore = useUserStore()

// Use composable
const { workloadId, detailData, detailLoading, getDetail, onDelete, onStop, onResume } =
  useWorkloadDetail({
    redirectPath: '/infer',
    extractFailedNodes: true,
  })

const { curPodId, curSshCommand, logVisible, sshVisible, openLog, openSsh } = usePodActions()

const activeTab = ref('overview')
const addVisible = ref(false)
const addAction = ref<'Clone' | 'Edit'>('Clone')

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

interface ServiceData {
  type?: string
  clusterIp?: string
  port?: {
    protocol?: string
    port?: number
    targetPort?: number
    nodePort?: number
  }
  internalDomain?: string
  externalDomain?: string
}

const serviceData = ref<ServiceData | null>(null)
const envExpanded = ref(false)
const entryPointExpanded = ref(false)

const decodedEntryPoint = computed(() => {
  const d = detailData?.value as { entryPoints?: string[]; entryPoint?: string } | undefined
  const raw =
    (Array.isArray(d?.entryPoints) ? d?.entryPoints?.[0] : undefined) ?? d?.entryPoint ?? ''
  if (!raw) return ''
  return decodeFromBase64String(raw)
})

const firstEnvKey = computed(() => {
  if (!detailData?.value?.env) return ''
  return Object.keys(detailData.value.env)[0] || ''
})

const firstEnvValue = computed(() => {
  if (!detailData?.value?.env) return ''
  const key = Object.keys(detailData.value.env)[0]
  return key ? detailData.value.env[key] : ''
})

const truncatedEntryPoint = computed(() => {
  if (!decodedEntryPoint.value) return ''
  return decodedEntryPoint.value.substring(0, 100)
})

// Fetch service info
const getServiceInfo = async () => {
  if (!workloadId.value || !detailData.value) return

  // Skip service info fetch if task has ended (has endTime)
  if (!detailData.value?.endTime) {
    try {
      const serviceRes = await getWorkloadService(workloadId.value)
      serviceData.value = serviceRes
    } catch (error) {
      console.error(error)
      serviceData.value = null
    }
  } else {
    serviceData.value = null
  }
}

// Watch detailData changes and fetch service info when data is loaded
watch(
  () => detailData.value,
  (newData) => {
    if (newData) {
      getServiceInfo()
    }
  },
  { immediate: true },
)

const defaultTime = computed<[Date, Date | 'now'] | null>(() => {
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
.log-box {
  margin-top: 20px;
  max-height: 400px;
  overflow-y: auto;
  font-family: monospace;
  white-space: pre-wrap;
  background: #111;
  color: #0f0;
  padding: 10px;
  border-radius: 6px;
}

.glass-btn {
  backdrop-filter: blur(6px);
  -webkit-backdrop-filter: blur(6px);
  background: var(--button-bg-color); /* Slightly transparent */
  border: 1px solid rgba(255, 255, 255, 0.15);
  color: var(--el-text-color-primary);
  transition:
    transform 0.2s ease,
    border-color 0.2s ease;
}

/* Hover: no background change, subtle scale and border highlight only */
.glass-btn:hover {
  transform: scale(1.05);
  border-color: rgba(255, 255, 255, 0.35);
}

.glass-btn--export {
  color: var(--el-color-primary);
}

.glass-btn--clone {
  color: var(--el-color-success);
}
.glass-btn--danger {
  color: var(--el-color-danger);
}
.glass-btn--warning {
  color: var(--el-color-warning);
}
.glass-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
  transform: none;
}
</style>

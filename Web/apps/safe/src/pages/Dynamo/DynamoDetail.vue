<template>
  <WorkloadHeader
    v-if="detailData"
    :detail-data="detailData"
    hide-edit
    @clone="onClone"
    @delete="onDelete"
    @stop="onStop"
  />

  <el-tabs v-model="activeTab" class="mt-4">
    <el-tab-pane label="Overview" name="overview">
      <el-card class="mt-2 safe-card" shadow="never">
        <div class="section-heading">
          <div class="section-bar"></div>
          <span class="textx-15 font-medium">Dynamo Configuration</span>
        </div>
        <el-descriptions v-if="detailData" class="m-t-4" border :column="4" direction="vertical">
          <el-descriptions-item label="kind">
            {{ detailData.groupVersionKind?.kind ?? '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="mode">
            <el-tag :type="mode === 'PD' ? 'warning' : 'info'" size="small">{{ mode }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="priority">
            {{ PRIORITY_LABEL_MAP[detailData.priority as PriorityValue] ?? '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="kvTransferBackend">
            {{ detailData.dynamoOptions?.kvTransferBackend ?? '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="serviceRoles" :span="2">
            <div class="flex flex-wrap gap-1">
              <el-tag v-for="role in serviceRoles" :key="role" size="small" effect="plain">
                {{ role }}
              </el-tag>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="multinodeRoles" :span="2">
            {{ multinodeRoles.length ? multinodeRoles.join(', ') : '-' }}
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <el-card class="mt-6 safe-card" shadow="never">
        <div class="section-heading">
          <div class="section-bar"></div>
          <span class="textx-15 font-medium">Roles</span>
        </div>

        <template v-for="role in roleRows" :key="role.index">
          <div class="role-section mt-4">
            <div class="role-title">
              {{ role.label }}
              <span class="role-replica">Replicas: {{ role.resource?.replica ?? '-' }}</span>
            </div>
            <el-descriptions border :column="4" direction="vertical" class="mt-2">
              <el-descriptions-item label="image" :span="4">
                {{ role.image || '-' }}
              </el-descriptions-item>
              <el-descriptions-item label="entryPoint" :span="4">
                <template v-if="role.entryPoint">
                  <pre class="entry-pre">{{ role.entryPoint }}</pre>
                </template>
                <span v-else class="text-gray-400">-</span>
              </el-descriptions-item>
              <el-descriptions-item label="CPU">{{ role.resource?.cpu ?? '-' }}</el-descriptions-item>
              <el-descriptions-item label="GPU">{{ role.resource?.gpu ?? '-' }}</el-descriptions-item>
              <el-descriptions-item label="Memory">{{ role.resource?.memory ?? '-' }}</el-descriptions-item>
              <el-descriptions-item label="Shared Memory">
                {{ role.resource?.sharedMemory ?? '-' }}
              </el-descriptions-item>
              <el-descriptions-item label="RDMA">
                {{ role.resource?.rdmaResource ?? '-' }}
              </el-descriptions-item>
            </el-descriptions>
          </div>
        </template>
      </el-card>

      <el-card v-if="serviceData" class="mt-6 safe-card" shadow="never">
        <div class="section-heading">
          <div class="section-bar"></div>
          <span class="textx-15 font-medium">Service</span>
        </div>
        <el-descriptions class="m-t-4" border :column="4" direction="vertical">
          <el-descriptions-item label="Type">{{ serviceData.type || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Cluster IP">{{ serviceData.clusterIp || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Protocol">{{ serviceData.port?.protocol || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Service Port">{{ serviceData.port?.port || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Target Port">{{ serviceData.port?.targetPort || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Node Port" v-if="serviceData.port?.nodePort">
            {{ serviceData.port.nodePort }}
          </el-descriptions-item>
          <el-descriptions-item label="Internal Domain" :span="4">
            {{ serviceData.internalDomain || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="External Domain" :span="4" v-if="serviceData.externalDomain">
            <el-link :href="serviceData.externalDomain" target="_blank" type="primary">
              {{ serviceData.externalDomain }}
            </el-link>
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
        :disable-ssh="!canWrite"
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
  </el-tabs>

  <LogsDialog v-model:visible="logVisible" :wlid="workloadId" :podid="curPodId || ''" />
  <AddDialog
    v-model:visible="addVisible"
    :wlid="workloadId"
    :action="addAction"
    @success="router.push('/dynamo')"
  />
  <SshConfigDialog
    v-model:visible="sshVisible"
    :wlid="workloadId"
    :podid="curPodId || ''"
    :ssh-command="curSshCommand"
  />
</template>

<script lang="ts" setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { getWorkloadService, PRIORITY_LABEL_MAP, type PriorityValue } from '@/services'
import WorkloadHeader from '@/components/Workload/WorkloadHeader.vue'
import WorkloadPodsTable from '@/components/Workload/WorkloadPodsTable.vue'
import WorkloadTimeline from '@/components/Workload/WorkloadTimeline.vue'
import LogsDialog from '@/components/Workload/LogsDialog.vue'
import SshConfigDialog from '@/components/Workload/SshConfigDialog.vue'
import LogTerminal from '@/components/Workload/LogTerminal.vue'
import AddDialog from './Components/AddDialog.vue'
import { useWorkloadDetail } from '@/composables/useWorkloadDetail'
import { usePodActions } from '@/composables/usePodActions'
import { useWorkloadWriteGuard } from '@/composables/useWorkloadWriteGuard'
import { useUserStore } from '@/stores/user'
import { decodeFromBase64String } from '@/utils'

const router = useRouter()
const userStore = useUserStore()
const { workloadId, detailData, detailLoading, getDetail, onDelete, onStop } = useWorkloadDetail({
  redirectPath: '/dynamo',
  extractFailedNodes: true,
})
const { curPodId, curSshCommand, logVisible, sshVisible, openLog, openSsh } = usePodActions()
const { canWrite } = useWorkloadWriteGuard()

const activeTab = ref('overview')
const addVisible = ref(false)
const addAction = ref<'Clone'>('Clone')

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

interface DynamoDetailShape {
  image?: string
  images?: string[]
  entryPoint?: string
  entryPoints?: string[]
  resources?: Array<Record<string, unknown>>
  dynamoOptions?: {
    serviceRoles?: string[]
    multinodeRoles?: string[]
    kvTransferBackend?: string
  }
}

const serviceData = ref<ServiceData | null>(null)
const imagesList = computed<string[]>(() => {
  const d = detailData.value as DynamoDetailShape | undefined
  if (!d) return []
  if (Array.isArray(d.images) && d.images.length) return d.images.filter(Boolean)
  if (d.image) return [d.image]
  return []
})

const entryPointsList = computed<string[]>(() => {
  const d = detailData.value as DynamoDetailShape | undefined
  if (!d) return []
  if (Array.isArray(d.entryPoints) && d.entryPoints.length) return d.entryPoints.filter(Boolean)
  if (d.entryPoint) return [d.entryPoint]
  return []
})

const decodedEntryPoints = computed(() =>
  entryPointsList.value.map((item) => decodeFromBase64String(item)),
)

const serviceRoles = computed(() => {
  const roles = (detailData.value as DynamoDetailShape | undefined)?.dynamoOptions?.serviceRoles
  if (Array.isArray(roles) && roles.length) return roles
  const count = detailData.value?.resources?.length ?? 0
  return count === 3 ? ['frontend', 'prefill', 'decode'] : ['frontend', 'worker']
})

const multinodeRoles = computed(() => {
  const roles = (detailData.value as DynamoDetailShape | undefined)?.dynamoOptions?.multinodeRoles
  return Array.isArray(roles) ? roles : []
})

const mode = computed(() => {
  if (serviceRoles.value.includes('prefill')) return 'PD'
  if (multinodeRoles.value.includes('worker')) return 'Aggregation'
  return 'Standard'
})

const roleRows = computed(() => {
  const resources = detailData.value?.resources ?? []
  return serviceRoles.value.map((role, index) => ({
    index,
    label: role,
    image: imagesList.value[index],
    entryPoint: decodedEntryPoints.value[index],
    resource: resources[index],
  }))
})

const onClone = () => {
  addAction.value = 'Clone'
  addVisible.value = true
}

const refreshPods = async () => {
  await getDetail()
  activeTab.value = 'pods'
}

const getServiceInfo = async () => {
  if (!workloadId.value || !detailData.value || detailData.value.endTime) {
    serviceData.value = null
    return
  }
  try {
    serviceData.value = await getWorkloadService(workloadId.value)
  } catch {
    serviceData.value = null
  }
}

watch(
  () => detailData.value,
  (next) => {
    if (next) getServiceInfo()
  },
  { immediate: true },
)

onMounted(() => {
  userStore.fetchEnvs()
})
</script>

<style scoped>
.section-heading {
  display: flex;
  align-items: center;
  gap: 8px;
}

.section-bar {
  width: 4px;
  height: 16px;
  border-radius: 999px;
  background: var(--safe-primary);
}

.role-section {
  padding: 12px 16px;
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--el-border-color-lighter);
}

html.dark .role-section {
  background: rgba(255, 255, 255, 0.03);
}

.role-title {
  font-size: calc(14px * var(--scale, 1));
  font-weight: 600;
  color: var(--el-text-color-primary);
  padding-left: 8px;
  border-left: 3px solid var(--safe-primary);
  display: flex;
  align-items: center;
  gap: 20px;
}

.role-replica {
  font-size: calc(13px * var(--scale, 1));
  font-weight: 400;
  color: var(--el-text-color-regular);
}

.entry-pre {
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
}
</style>

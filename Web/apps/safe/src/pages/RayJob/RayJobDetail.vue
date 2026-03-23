<template>
  <WorkloadHeader
    v-if="detailData"
    :detail-data="detailData"
    :edit-disabled="editDisabled"
    @edit="onEdit"
    @clone="onClone"
    @delete="onDelete"
    @stop="onStop"
  />

  <el-tabs v-model="activeTab" class="mt-4">
    <el-tab-pane label="Overview" name="overview">
      <!-- ===== Ray Job ===== -->
      <el-card class="mt-2 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Ray Job</span>
        </div>

        <el-descriptions v-if="detailData" class="m-t-4" border :column="5" direction="vertical">
          <el-descriptions-item v-if="jobEntrypoint" label="entryPoint" :span="2">
            {{ jobEntrypoint }}
          </el-descriptions-item>
          <el-descriptions-item label="priority">{{
            PRIORITY_LABEL_MAP[detailData.priority as PriorityValue]
          }}</el-descriptions-item>
          <el-descriptions-item label="maxRetry">{{ detailData.maxRetry }}</el-descriptions-item>
          <el-descriptions-item label="dispatchCount">{{
            detailData.dispatchCount
          }}</el-descriptions-item>
          <el-descriptions-item label="secondsUntilTimeout" v-if="detailData.secondsUntilTimeout">{{
            detailData.secondsUntilTimeout
          }}</el-descriptions-item>
          <el-descriptions-item label="timeout" v-if="detailData.timeout"
            >{{ detailData.timeout ?? 0 }}s</el-descriptions-item
          >
          <el-descriptions-item label="specifiedNode" v-if="detailData?.specifiedNodes?.length">{{
            detailData?.specifiedNodes?.join(',') || '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="nodesAffinity" v-if="detailData.nodesAffinity">{{
            detailData.nodesAffinity
          }}</el-descriptions-item>
          <el-descriptions-item v-if="detailData.dependencies?.length" label="dependencies">
            <div class="flex flex-wrap gap-2">
              <el-link
                v-for="dep in detailData.dependencies"
                :key="dep"
                type="primary"
                :underline="false"
                @click="goDetail(dep)"
              >
                {{ dep }}
              </el-link>
            </div>
          </el-descriptions-item>
          <el-descriptions-item
            v-if="filteredEnvKeys.length > 0"
            label="env"
            :span="2"
          >
            <div>
              <template v-if="filteredEnvKeys.length === 1">
                <div>{{ filteredEnvKeys[0] }}: {{ filteredEnv[filteredEnvKeys[0]] }}</div>
              </template>
              <template v-else>
                <template v-if="!envExpanded">
                  <div>{{ filteredEnvKeys[0] }}: {{ filteredEnv[filteredEnvKeys[0]] }}</div>
                  <el-button type="text" @click="envExpanded = true">View More</el-button>
                </template>
                <template v-else>
                  <div v-for="key in filteredEnvKeys" :key="key">
                    {{ key }}: {{ filteredEnv[key] }}
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

      <!-- ===== Ray Cluster ===== -->
      <el-card class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Ray Cluster</span>
        </div>

        <!-- Header -->
        <div class="cluster-role-section mt-4">
          <div class="cluster-role-title">Header</div>
          <el-descriptions border :column="4" direction="vertical" class="mt-2">
            <el-descriptions-item label="image" :span="4">
              {{ imagesList[0] ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="entryPoint" :span="4">
              <template v-if="decodedHeaderEntryPoint">
                <span v-if="!headerEpExpanded">
                  {{ decodedHeaderEntryPoint.slice(0, 80) }}...
                  <el-button type="text" @click="headerEpExpanded = true">View More</el-button>
                </span>
                <span v-else>
                  <pre class="ep-pre">{{ decodedHeaderEntryPoint }}</pre>
                  <el-button type="text" @click="headerEpExpanded = false">Collapse</el-button>
                </span>
              </template>
              <span v-else class="text-gray-400">-</span>
            </el-descriptions-item>
            <el-descriptions-item>
              <template #label><el-icon class="res-icon"><Cpu /></el-icon> CPU</template>
              {{ detailData?.resources?.[0]?.cpu ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item>
              <template #label><el-icon class="res-icon"><Monitor /></el-icon> GPU</template>
              {{ detailData?.resources?.[0]?.gpu ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item>
              <template #label><el-icon class="res-icon"><Box /></el-icon> Memory</template>
              {{ detailData?.resources?.[0]?.memory ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item>
              <template #label><el-icon class="res-icon"><Collection /></el-icon> Ephemeral</template>
              {{ detailData?.resources?.[0]?.ephemeralStorage ?? '-' }}
            </el-descriptions-item>
          </el-descriptions>
        </div>

        <!-- Workers -->
        <template v-for="wi in workerIndexes" :key="wi">
          <div class="cluster-role-section mt-4">
            <div class="cluster-role-title">
              Worker {{ wi }}
              <span class="worker-replica-badge">
                <el-icon class="res-icon"><DataLine /></el-icon>
                Replicas: {{ detailData?.resources?.[wi]?.replica ?? '-' }}
              </span>
            </div>
            <el-descriptions border :column="4" direction="vertical" class="mt-2">
              <el-descriptions-item label="image" :span="4">
                {{ imagesList[wi] ?? '-' }}
              </el-descriptions-item>
              <el-descriptions-item label="entryPoint" :span="4">
                <template v-if="decodedWorkerEntryPoints[wi]">
                  <span v-if="!workerEpExpanded[wi]">
                    {{ decodedWorkerEntryPoints[wi].slice(0, 80) }}...
                    <el-button type="text" @click="workerEpExpanded[wi] = true">View More</el-button>
                  </span>
                  <span v-else>
                    <pre class="ep-pre">{{ decodedWorkerEntryPoints[wi] }}</pre>
                    <el-button type="text" @click="workerEpExpanded[wi] = false">Collapse</el-button>
                  </span>
                </template>
                <span v-else class="text-gray-400">-</span>
              </el-descriptions-item>
              <el-descriptions-item>
                <template #label><el-icon class="res-icon"><Cpu /></el-icon> CPU</template>
                {{ detailData?.resources?.[wi]?.cpu ?? '-' }}
              </el-descriptions-item>
              <el-descriptions-item>
                <template #label><el-icon class="res-icon"><Monitor /></el-icon> GPU</template>
                {{ detailData?.resources?.[wi]?.gpu ?? '-' }}
              </el-descriptions-item>
              <el-descriptions-item>
                <template #label><el-icon class="res-icon"><Box /></el-icon> Memory</template>
                {{ detailData?.resources?.[wi]?.memory ?? '-' }}
              </el-descriptions-item>
              <el-descriptions-item>
                <template #label><el-icon class="res-icon"><Collection /></el-icon> Ephemeral</template>
                {{ detailData?.resources?.[wi]?.ephemeralStorage ?? '-' }}
              </el-descriptions-item>
            </el-descriptions>
          </div>
        </template>
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
        :select-first-n="1"
        :dispatchCount="detailData?.dispatchCount"
        :nodes="detailData?.nodes"
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
    @success="addAction === 'Edit' ? getDetail() : router.push('/rayjob')"
  />
  <SshConfigDialog
    v-model:visible="sshVisible"
    :wlid="workloadId"
    :podid="curPodId || ''"
    :ssh-command="curSshCommand"
  />
</template>
<script lang="ts" setup>
import { onMounted, ref, computed, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { type PriorityValue, PRIORITY_LABEL_MAP } from '@/services'
import { Cpu, Monitor, Collection, Box, DataLine } from '@element-plus/icons-vue'
import LogsDialog from '@/components/Workload/LogsDialog.vue'
import SshConfigDialog from '@/components/Workload/SshConfigDialog.vue'
import AddDialog from './Components/AddDialog.vue'
import LogTerminal from '@/components/Workload/LogTerminal.vue'
import GrafanaIframe from '@/components/Base/GrafanaIframe.vue'
import WorkloadHeader from '@/components/Workload/WorkloadHeader.vue'
import WorkloadPodsTable from '@/components/Workload/WorkloadPodsTable.vue'
import WorkloadTimeline from '@/components/Workload/WorkloadTimeline.vue'
import { decodeFromBase64String, calculateDefaultTime } from '@/utils'
import { useUserStore } from '@/stores/user'
import { useWorkloadDetail } from '@/composables/useWorkloadDetail'
import { usePodActions } from '@/composables/usePodActions'

const router = useRouter()
const userStore = useUserStore()

const { workloadId, detailData, detailLoading, getDetail, onDelete, onStop } = useWorkloadDetail({
  redirectPath: '/rayjob',
  extractFailedNodes: true,
})

const { curPodId, curSshCommand, logVisible, sshVisible, openLog, openSsh } = usePodActions()

const activeTab = ref('overview')
const addVisible = ref(false)
const addAction = ref<'Clone' | 'Edit'>('Clone')
const envExpanded = ref(false)

const editDisabled = computed(() => {
  const d = detailData.value
  if (!d) return true
  const maxRetry = d.maxRetry ?? 0
  const phase = d.phase
  const queuePosition = d.queuePosition ?? 0

  if (maxRetry > 0) {
    return !['Running', 'Pending'].includes(phase)
  } else {
    return !(phase === 'Pending' && queuePosition > 0)
  }
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
const headerEpExpanded = ref(false)
const workerEpExpanded = reactive<Record<number, boolean>>({})

const imagesList = computed<string[]>(() => {
  const d: any = detailData?.value
  if (!d) return []
  if (Array.isArray(d.images) && d.images.length) return d.images.filter(Boolean)
  if (d.image) return [d.image]
  return []
})

const entryPointsList = computed<string[]>(() => {
  const d: any = detailData?.value
  if (!d) return []
  if (Array.isArray(d.entryPoints) && d.entryPoints.length) return d.entryPoints.filter(Boolean)
  if (d.entryPoint) return [d.entryPoint]
  return []
})

// Header entryPoint (index 0)
const decodedHeaderEntryPoint = computed(() => {
  const raw = entryPointsList.value[0]
  return raw ? decodeFromBase64String(raw) : ''
})

// Worker entryPoints (index 1, 2)
const decodedWorkerEntryPoints = computed<Record<number, string>>(() => {
  const result: Record<number, string> = {}
  for (let i = 1; i < entryPointsList.value.length && i <= 2; i++) {
    const raw = entryPointsList.value[i]
    result[i] = raw ? decodeFromBase64String(raw) : ''
  }
  return result
})

const jobEntrypoint = computed(() => {
  const raw = detailData?.value?.env?.RAY_JOB_ENTRYPOINT ?? ''
  return raw ? decodeFromBase64String(raw) : ''
})

const filteredEnv = computed<Record<string, string>>(() => {
  if (!detailData?.value?.env) return {}
  const { RAY_JOB_ENTRYPOINT: _, ...rest } = detailData.value.env
  return rest
})

const filteredEnvKeys = computed(() => Object.keys(filteredEnv.value))

const workerIndexes = computed(() => {
  const n = detailData?.value?.resources?.length ?? 0
  if (n <= 1) return []
  return Array.from({ length: n - 1 }, (_, k) => k + 1).slice(0, 2)
})

const goDetail = (id: string) => {
  router.push({ path: '/rayjob/detail', query: { id } })
}

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
.el-descriptions__body {
  background-color: none !important;
}

.cluster-role-section {
  padding: 12px 16px;
  border-radius: 8px;
  background: var(--el-fill-color-lighter);
  border: 1px solid var(--el-border-color-lighter);
}

html.dark .cluster-role-section {
  background: rgba(255, 255, 255, 0.03);
}

.cluster-role-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  padding-left: 8px;
  border-left: 3px solid var(--safe-primary);
  display: flex;
  align-items: center;
  gap: 20px;
}

.worker-replica-badge {
  font-size: 13px;
  font-weight: 400;
  color: var(--el-text-color-regular);
}

.ep-pre {
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
}

.res-icon {
  font-size: 13px;
  vertical-align: -1px;
  margin-right: 2px;
  color: var(--safe-primary);
}
</style>

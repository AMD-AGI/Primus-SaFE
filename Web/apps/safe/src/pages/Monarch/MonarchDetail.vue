<template>
  <WorkloadHeader
    v-if="detailData"
    :detail-data="detailData"
    :edit-disabled="true"
    @clone="onClone"
    @delete="onDelete"
    @stop="onStop"
  />

  <el-tabs v-model="activeTab" class="mt-4">
    <el-tab-pane label="Overview" name="overview">
      <!-- ===== Configuration ===== -->
      <el-card class="mt-2 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Configuration</span>
        </div>

        <el-descriptions v-if="detailData" class="m-t-4" border :column="5" direction="vertical">
          <el-descriptions-item label="kind">{{
            detailData.groupVersionKind?.kind ?? '-'
          }}</el-descriptions-item>
          <el-descriptions-item label="priority">{{
            PRIORITY_LABEL_MAP[detailData.priority as PriorityValue]
          }}</el-descriptions-item>
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

      <!-- ===== Client Resource ===== -->
      <el-card class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Client</span>
        </div>

        <div class="cluster-role-section mt-4">
          <el-descriptions border :column="4" direction="vertical" class="mt-2">
            <el-descriptions-item label="image" :span="4">
              {{ imagesList[0] ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="entryPoint" :span="4">
              <template v-if="decodedClientEntryPoint">
                <span v-if="!clientEpExpanded">
                  {{ decodedClientEntryPoint.slice(0, 80) }}...
                  <el-button type="text" @click="clientEpExpanded = true">View More</el-button>
                </span>
                <span v-else>
                  <pre class="ep-pre">{{ decodedClientEntryPoint }}</pre>
                  <el-button type="text" @click="clientEpExpanded = false">Collapse</el-button>
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
      </el-card>

      <!-- ===== Mesh Group Resource ===== -->
      <el-card class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Mesh Group</span>
        </div>

        <div class="cluster-role-section mt-4">
          <div class="cluster-role-title mb-2">
            <span class="worker-replica-badge">
              <el-icon class="res-icon"><DataLine /></el-icon>
              Replicas: {{ detailData?.resources?.[1]?.replica ?? '-' }}
            </span>
          </div>
          <el-descriptions border :column="4" direction="vertical" class="mt-2">
            <el-descriptions-item label="image" :span="4">
              {{ imagesList[1] ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="entryPoint" :span="4">
              <template v-if="decodedMeshEntryPoint">
                <span v-if="!meshEpExpanded">
                  {{ decodedMeshEntryPoint.slice(0, 80) }}...
                  <el-button type="text" @click="meshEpExpanded = true">View More</el-button>
                </span>
                <span v-else>
                  <pre class="ep-pre">{{ decodedMeshEntryPoint }}</pre>
                  <el-button type="text" @click="meshEpExpanded = false">Collapse</el-button>
                </span>
              </template>
              <span v-else class="text-gray-400">-</span>
            </el-descriptions-item>
            <el-descriptions-item>
              <template #label><el-icon class="res-icon"><Cpu /></el-icon> CPU</template>
              {{ detailData?.resources?.[1]?.cpu ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item>
              <template #label><el-icon class="res-icon"><Monitor /></el-icon> GPU</template>
              {{ detailData?.resources?.[1]?.gpu ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item>
              <template #label><el-icon class="res-icon"><Box /></el-icon> Memory</template>
              {{ detailData?.resources?.[1]?.memory ?? '-' }}
            </el-descriptions-item>
            <el-descriptions-item>
              <template #label><el-icon class="res-icon"><Collection /></el-icon> Ephemeral</template>
              {{ detailData?.resources?.[1]?.ephemeralStorage ?? '-' }}
            </el-descriptions-item>
          </el-descriptions>
        </div>
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
    :action="'Clone'"
    @success="router.push('/monarch')"
  />
  <SshConfigDialog
    v-model:visible="sshVisible"
    :wlid="workloadId"
    :podid="curPodId || ''"
    :ssh-command="curSshCommand"
  />
</template>
<script lang="ts" setup>
import { onMounted, ref, computed } from 'vue'
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
import { useWorkloadWriteGuard } from '@/composables/useWorkloadWriteGuard'

const router = useRouter()
const userStore = useUserStore()

const { workloadId, detailData, detailLoading, getDetail, onDelete, onStop } = useWorkloadDetail({
  redirectPath: '/monarch',
  extractFailedNodes: true,
})

const { curPodId, curSshCommand, logVisible, sshVisible, openLog, openSsh } = usePodActions()
const { canWrite } = useWorkloadWriteGuard()

const activeTab = ref('overview')
const addVisible = ref(false)
const envExpanded = ref(false)
const clientEpExpanded = ref(false)
const meshEpExpanded = ref(false)

const onClone = () => {
  addVisible.value = true
}

const refreshPods = async () => {
  await getDetail()
  activeTab.value = 'pods'
}

const imagesList = computed<string[]>(() => {
  const d = detailData?.value as { images?: string[]; image?: string } | undefined
  if (!d) return []
  if (Array.isArray(d.images) && d.images.length) return d.images.filter(Boolean)
  if (d.image) return [d.image]
  return []
})

const entryPointsList = computed<string[]>(() => {
  const d = detailData?.value as { entryPoints?: string[]; entryPoint?: string } | undefined
  if (!d) return []
  if (Array.isArray(d.entryPoints) && d.entryPoints.length) return d.entryPoints.filter(Boolean)
  if (d.entryPoint) return [d.entryPoint]
  return []
})

// Client entryPoint (index 0)
const decodedClientEntryPoint = computed(() => {
  const raw = entryPointsList.value[0]
  return raw ? decodeFromBase64String(raw) : ''
})

// Mesh Group entryPoint (index 1)
const decodedMeshEntryPoint = computed(() => {
  const raw = entryPointsList.value[1]
  return raw ? decodeFromBase64String(raw) : ''
})

// Filter out REPLICA_COUNT from env display
const filteredEnv = computed<Record<string, string>>(() => {
  if (!detailData?.value?.env) return {}
  const { REPLICA_COUNT: _, ...rest } = detailData.value.env
  return rest
})

const filteredEnvKeys = computed(() => Object.keys(filteredEnv.value))

const goDetail = (id: string) => {
  router.push({ path: '/monarch/detail', query: { id } })
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

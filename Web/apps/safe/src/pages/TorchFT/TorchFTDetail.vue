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
          <el-descriptions-item label="maxRetry">{{ detailData.maxRetry }}</el-descriptions-item>
          <el-descriptions-item label="priority">{{
            PRIORITY_LABEL_MAP[detailData.priority as PriorityValue]
          }}</el-descriptions-item>
          <el-descriptions-item label="dispatchCount">{{
            detailData.dispatchCount
          }}</el-descriptions-item>

          <el-descriptions-item label="entryPoint" :span="3">
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

      <!-- Lighthouse Resource -->
      <el-card class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Lighthouse Resource</span>
        </div>
        <div class="grid gap-3 mt-4 sm:grid-cols-2 lg:grid-cols-3">
          <StatCard
            label="Replicas"
            :value="detailData?.resources?.[0]?.replica ?? '-'"
            :icon="DataLine"
          />
          <StatCard label="CPU" :value="detailData?.resources?.[0]?.cpu ?? '-'" :icon="Cpu" />
          <StatCard label="Memory" :value="detailData?.resources?.[0]?.memory ?? '-'" :icon="Box" />
          <StatCard
            label="Ephemeral Storage"
            :value="detailData?.resources?.[0]?.ephemeralStorage ?? '-'"
            :icon="Collection"
          />
        </div>
      </el-card>

      <!-- Worker Group Resource -->
      <el-card class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Worker Group Resource</span>
        </div>
        <div class="grid gap-3 mt-4 sm:grid-cols-2 lg:grid-cols-3">
          <StatCard
            label="Replicas"
            :value="detailData?.resources?.[1]?.replica ?? '-'"
            :icon="DataLine"
          />
          <StatCard label="CPU" :value="detailData?.resources?.[1]?.cpu ?? '-'" :icon="Cpu" />
          <StatCard label="GPU" :value="detailData?.resources?.[1]?.gpu ?? '-'" :icon="Monitor" />
          <StatCard label="Memory" :value="detailData?.resources?.[1]?.memory ?? '-'" :icon="Box" />
          <StatCard
            label="Ephemeral Storage"
            :value="detailData?.resources?.[1]?.ephemeralStorage ?? '-'"
            :icon="Collection"
          />
          <StatCard
            label="RDMA"
            :value="detailData?.resources?.[1]?.rdmaResource ?? '-'"
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
    :action="addAction"
    @success="addAction === 'Edit' ? getDetail() : router.push('/torchft')"
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
import { Cpu, Monitor, Collection, Connection, Box, DataLine } from '@element-plus/icons-vue'
import LogsDialog from '@/components/Workload/LogsDialog.vue'
import SshConfigDialog from '@/components/Workload/SshConfigDialog.vue'
import AddDialog from './Components/AddDialog.vue'
import LogTerminal from '@/components/Workload/LogTerminal.vue'
import StatCard from '@/components/Base/StatCard.vue'
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
  redirectPath: '/torchft',
  extractFailedNodes: true,
})

const { curPodId, curSshCommand, logVisible, sshVisible, openLog, openSsh } = usePodActions()

const activeTab = ref('overview')
const addVisible = ref(false)
const addAction = ref<'Clone' | 'Edit'>('Clone')
const entryPointExpanded = ref(false)
const envExpanded = ref(false)

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

const decodedEntryPoint = computed(() => {
  const d = detailData?.value as { entryPoints?: string[]; entryPoint?: string } | undefined
  const raw =
    (Array.isArray(d?.entryPoints) ? d?.entryPoints?.[0] : undefined) ?? d?.entryPoint ?? ''
  if (!raw) return ''
  return decodeFromBase64String(raw)
})

const truncatedEntryPoint = computed(() => {
  if (!decodedEntryPoint.value) return ''
  return decodedEntryPoint.value.slice(0, 50)
})

const firstEnvKey = computed(() => {
  if (!detailData?.value.env) return ''
  return Object.keys(detailData.value.env)[0] || ''
})
const firstEnvValue = computed(() => {
  if (!detailData?.value.env) return ''
  const key = Object.keys(detailData.value.env)[0]
  return key ? detailData.value.env[key] : ''
})

const goDetail = (id: string) => {
  router.push({ path: '/torchft/detail', query: { id } })
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

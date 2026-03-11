<template>
  <WorkloadHeader
    v-if="preflightDetail"
    :detail-data="preflightDetail"
    :hide-edit="true"
    @clone="addVisible = true"
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
          v-if="preflightDetail"
          class="m-t-4"
          border
          :column="5"
          direction="vertical"
        >
          <el-descriptions-item label="image">{{ preflightDetail.image }}</el-descriptions-item>
        </el-descriptions>
        <el-descriptions
          v-if="preflightDetail"
          border
          :column="5"
          direction="vertical"
          class="no-margin-top"
        >
          <el-descriptions-item label="entryPoint" :span="3">
            <div>
              <span v-if="!entryPointExpanded">
                {{ truncatedEntryPoint }}...
                <el-button type="text" @click="entryPointExpanded = true">View More</el-button>
              </span>
              <span v-else>
                <pre style="white-space: pre-wrap; word-break: break-word">{{
                  decodeFromBase64String(preflightDetail.entryPoint)
                }}</pre>
                <el-button type="text" @click="entryPointExpanded = false">Collapse</el-button>
              </span>
            </div>
          </el-descriptions-item>
          <el-descriptions-item
            v-if="preflightDetail?.env && Object.keys(preflightDetail.env).length > 0"
            label="env"
            :span="2"
          >
            <div>
              <template v-if="Object.keys(preflightDetail.env).length === 1">
                <div>{{ firstEnvKey }}: {{ firstEnvValue }}</div>
              </template>
              <template v-else>
                <template v-if="!envExpanded">
                  <div>{{ firstEnvKey }}: {{ firstEnvValue }}</div>
                  <el-button type="text" @click="envExpanded = true">View More</el-button>
                </template>
                <template v-else>
                  <div v-for="(value, key) in preflightDetail.env" :key="key">
                    {{ key }}: {{ value }}
                  </div>
                  <el-button type="text" @click="envExpanded = false">Collapse</el-button>
                </template>
              </template>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="inputs" :span="5">
            {{ inputsText }}
          </el-descriptions-item>
          <el-descriptions-item label="outputs" :span="5">{{
            (preflightDetail?.outputs || []).find((o: any) => o?.name === 'result')?.value ?? '-'
          }}</el-descriptions-item>
        </el-descriptions>
      </el-card>

      <el-card class="mt-6 safe-card" shadow="never">
        <div class="flex items-center">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">resource</span>
        </div>
        <div class="grid gap-3 mt-4 sm:grid-cols-2 lg:grid-cols-3">
          <StatCard label="CPU" :value="preflightDetail?.resource?.cpu ?? '-'" :icon="Cpu" />
          <StatCard label="GPU" :value="preflightDetail?.resource?.gpu ?? '-'" :icon="Monitor" />
          <StatCard label="Memory" :value="preflightDetail?.resource?.memory ?? '-'" :icon="Box" />
          <StatCard
            label="Ephemeral Storage"
            :value="preflightDetail?.resource?.ephemeralStorage ?? '-'"
            :icon="Collection"
          />
        </div>
      </el-card>
    </el-tab-pane>
    <el-tab-pane label="Pods" name="pods">
      <WorkloadPodsTable
        :pods="workloadDetail?.pods"
        :workload-phase="workloadDetail?.phase"
        :refresh-loading="detailLoading"
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
  <AddDialog
    v-model:visible="addVisible"
    :jobid="workloadId"
    action="Clone"
    @success="() => router.push('/preflight')"
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
import { useRoute, useRouter } from 'vue-router'
import { getWorkloadDetail, getOpsjobsDetail } from '@/services'
import { Cpu, Monitor, Collection, Box } from '@element-plus/icons-vue'
import LogsDialog from '@/components/Workload/LogsDialog.vue'
import SshConfigDialog from '@/components/Workload/SshConfigDialog.vue'
import AddDialog from './Components/AddDialog.vue'
import LogTable from './Components/LogTable.vue'
import StatCard from '@/components/Base/StatCard.vue'
import WorkloadHeader from '@/components/Workload/WorkloadHeader.vue'
import WorkloadPodsTable from '@/components/Workload/WorkloadPodsTable.vue'
import GrafanaIframe from '@/components/Base/GrafanaIframe.vue'
import { useUserStore } from '@/stores/user'
import { decodeFromBase64String, calculateDefaultTime } from '@/utils'
import { usePodActions } from '@/composables/usePodActions'
import { useWorkloadDetail } from '@/composables/useWorkloadDetail'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

// Special handling: preflight uses a different API
const workloadId = computed(() => route.query.id as string)
const preflightDetail = ref()
const workloadDetail = ref()
const detailLoading = ref(false)

const { onDelete, onStop } = useWorkloadDetail({
  redirectPath: '/preflight',
})

const { curPodId, curSshCommand, logVisible, sshVisible, openLog } = usePodActions()

const activeTab = ref('overview')
const addVisible = ref(false)

const refreshPods = async () => {
  await getDetail()
  activeTab.value = 'pods'
}
const entryPointExpanded = ref(false)
const envExpanded = ref(false)

const defaultTime = computed<[Date, Date] | null>(() => {
  return calculateDefaultTime(
    workloadDetail.value?.startTime,
    workloadDetail.value?.endTime,
    workloadDetail.value?.creationTime,
  )
})

const truncatedEntryPoint = computed(() => {
  if (!preflightDetail?.value.entryPoint) return ''
  return decodeFromBase64String(preflightDetail.value.entryPoint).slice(0, 50)
})

const firstEnvKey = computed(() => {
  if (!preflightDetail?.value.env) return ''
  return Object.keys(preflightDetail.value.env)[0] || ''
})
const firstEnvValue = computed(() => {
  if (!preflightDetail?.value.env) return ''
  const key = Object.keys(preflightDetail.value.env)[0]
  return key ? preflightDetail.value.env[key] : ''
})

type InputNV = { name?: string; value?: string }

const inputsText = computed(() => {
  const arr: InputNV[] = (preflightDetail.value?.inputs as InputNV[] | undefined) ?? []
  if (!arr.length) return '-'

  const nodeVals = arr
    .filter((i) => i?.name === 'node' && i?.value?.trim())
    .map((i) => i!.value!.trim())
  if (nodeVals.length) return `node: ${Array.from(new Set(nodeVals)).join(', ')}`

  const first = arr.find((i) => i?.name && i?.value?.trim())
  return first ? `${first.name}: ${first.value!.trim()}` : '-'
})

const getDetail = async () => {
  if (!workloadId.value) return
  detailLoading.value = true
  preflightDetail.value = await getOpsjobsDetail(workloadId.value)
  if (preflightDetail.value?.startTime) {
    workloadDetail.value = await getWorkloadDetail(workloadId.value)
  }
  detailLoading.value = false
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
.no-margin-top {
  margin-top: -1px;
}
</style>

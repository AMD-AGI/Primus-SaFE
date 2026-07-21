<template>
  <div v-loading="loading" :element-loading-text="$loadingText">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-3">
        <el-button :icon="ArrowLeft" circle data-testid="slurm-detail-back" @click="goBack" />
        <el-text class="textx-18 font-500" tag="b" data-testid="slurm-detail-name">
          {{ detail?.name || name }}
        </el-text>
        <el-tag :type="phaseTagType(detail?.phase)" data-testid="slurm-detail-phase">
          {{ detail?.phase || '-' }}
        </el-tag>
      </div>
      <div class="flex items-center gap-2">
        <el-button :icon="Refresh" data-testid="slurm-detail-refresh" @click="getDetail">
          Refresh
        </el-button>
        <el-button
          :icon="Connection"
          :disabled="detail?.phase !== 'Running'"
          data-testid="slurm-detail-ssh"
          @click="onSsh"
        >
          SSH
        </el-button>
        <el-button
          v-if="isStopped"
          type="success"
          :icon="VideoPlay"
          data-testid="slurm-detail-resume"
          @click="onResume"
        >
          Resume
        </el-button>
        <el-button
          v-else
          type="warning"
          :icon="VideoPause"
          data-testid="slurm-detail-stop"
          @click="onStop"
        >
          Stop
        </el-button>
      </div>
    </div>

    <!-- Aggregate compute -->
    <div class="grid gap-3 mt-6 sm:grid-cols-2 lg:grid-cols-4">
      <StatCard label="Nodes (ready / desired)" :icon="DataLine">
        {{ detail?.nodesReady ?? 0 }} / {{ detail?.nodesDesired ?? 0 }}
      </StatCard>
      <StatCard label="Total GPUs" :value="totalGpu" :icon="Monitor" />
      <StatCard label="Partitions" :value="(detail?.pools || []).length" :icon="Collection" />
      <StatCard label="Accounting" :value="detail?.accountingEnabled ? 'On' : 'Off'" :icon="Cpu" />
    </div>

    <!-- Node pools / partitions -->
    <el-card class="mt-6 safe-card" shadow="never">
      <div class="flex items-center mb-2">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="fs-subtitle font-medium">Node pools</span>
      </div>
      <div
        v-for="(pool, idx) in detail?.pools || []"
        :key="pool.name"
        class="mt-4"
        data-testid="slurm-detail-pool"
      >
        <div class="font-500 mb-2 flex items-center gap-2">
          <span>{{ pool.name }}</span>
          <el-tag v-if="idx === 0" size="small" type="success">Default partition</el-tag>
        </div>
        <el-descriptions border :column="4" direction="vertical">
          <el-descriptions-item>
            <template #label><el-icon class="res-icon"><DataLine /></el-icon> Nodes</template>
            {{ pool.nodes ?? '-' }}
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label><el-icon class="res-icon"><Monitor /></el-icon> GPU / node</template>
            {{ pool.gpu ?? 0 }}
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label><el-icon class="res-icon"><Cpu /></el-icon> CPU / node</template>
            {{ pool.cpu || '-' }}
          </el-descriptions-item>
          <el-descriptions-item>
            <template #label><el-icon class="res-icon"><Box /></el-icon> Memory / node</template>
            {{ pool.memory || '-' }}
          </el-descriptions-item>
        </el-descriptions>
      </div>
      <el-empty v-if="!(detail?.pools || []).length" description="No node pools" />
    </el-card>

    <!-- Live pods -->
    <el-card class="mt-6 safe-card" shadow="never">
      <div class="flex items-center mb-2">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="fs-subtitle font-medium">Pods</span>
      </div>
      <el-table :data="detail?.pods || []" size="default" data-testid="slurm-detail-pods">
        <el-table-column prop="role" label="Role" width="140">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.role }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="Pod" min-width="240" show-overflow-tooltip />
        <el-table-column prop="node" label="Node" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">{{ row.node || '-' }}</template>
        </el-table-column>
        <el-table-column prop="phase" label="Phase" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="row.phase === 'Running' ? 'success' : 'info'">
              {{ row.phase }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="podIP" label="Pod IP" width="150">
          <template #default="{ row }">{{ row.podIP || '-' }}</template>
        </el-table-column>
        <template #empty>
          <span>No pods running. Stopped clusters release all worker/login pods.</span>
        </template>
      </el-table>
    </el-card>

    <SshDialog
      v-model:visible="showSshDialog"
      :info="sshInfo"
      :cluster-name="detail?.name"
      :loading="sshLoading"
    />
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowLeft,
  Refresh,
  Connection,
  VideoPlay,
  VideoPause,
  Cpu,
  Monitor,
  Box,
  DataLine,
  Collection,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getSlurmClusterDetail,
  getSlurmClusterLogin,
  stopSlurmCluster,
  resumeSlurmCluster,
} from '@/services'
import type { SlurmClusterItem, SlurmLoginInfo } from '@/services/slurm/type'
import StatCard from '@/components/Base/StatCard.vue'
import SshDialog from './Components/SshDialog.vue'

const route = useRoute()
const router = useRouter()

const name = computed(() => (route.query.name as string) || '')
const workspaceId = computed(() => (route.query.workspaceId as string) || '')
const clusterId = computed(() => (route.query.clusterId as string) || '')

const loading = ref(false)
const detail = ref<SlurmClusterItem | null>(null)
const showSshDialog = ref(false)
const sshInfo = ref<SlurmLoginInfo | null>(null)
const sshLoading = ref(false)

const isStopped = computed(
  () => detail.value?.stopped === true || detail.value?.phase === 'Stopped',
)
const totalGpu = computed(() =>
  (detail.value?.pools || []).reduce((sum, p) => sum + (p.nodes || 0) * (p.gpu || 0), 0),
)

const phaseTagType = (phase?: string) => {
  if (phase === 'Running') return 'success'
  if (phase === 'Failed') return 'danger'
  return 'info'
}

const getDetail = async () => {
  if (!name.value || !workspaceId.value) return
  loading.value = true
  try {
    detail.value = await getSlurmClusterDetail(clusterId.value, name.value, workspaceId.value)
  } finally {
    loading.value = false
  }
}

const goBack = () => router.push({ path: '/slurm' })

const onSsh = async () => {
  sshInfo.value = null
  showSshDialog.value = true
  sshLoading.value = true
  try {
    sshInfo.value = await getSlurmClusterLogin(clusterId.value, name.value, workspaceId.value)
    if (!sshInfo.value?.ready && sshInfo.value?.message) {
      ElMessage.warning(sshInfo.value.message)
    }
  } catch {
    showSshDialog.value = false
    ElMessage.error('Failed to fetch the login SSH command')
  } finally {
    sshLoading.value = false
  }
}

const onStop = () => {
  ElMessageBox.confirm(
    'Stop this Slurm cluster? Its components scale to zero (freeing compute); it stays in the list and can be resumed. Running jobs are lost.',
    'Stop Slurm cluster',
    { confirmButtonText: 'Stop', cancelButtonText: 'Cancel', type: 'warning' },
  ).then(async () => {
    await stopSlurmCluster(clusterId.value, name.value, workspaceId.value)
    ElMessage.success('Stopping cluster')
    getDetail()
  })
}

const onResume = async () => {
  await resumeSlurmCluster(clusterId.value, name.value, workspaceId.value)
  ElMessage.success('Resuming cluster')
  getDetail()
}

onMounted(() => getDetail())

defineOptions({ name: 'SlurmClusterDetailPage' })
</script>

<style scoped>
.res-icon {
  vertical-align: middle;
  margin-right: 4px;
}
</style>

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
        <el-table :data="roleRows">
          <el-table-column prop="role" label="Role" width="120" />
          <el-table-column prop="image" label="Image" min-width="280" show-overflow-tooltip />
          <el-table-column prop="entryPoint" label="Entrypoint" min-width="280" show-overflow-tooltip />
          <el-table-column prop="replica" label="Replica" width="100" />
          <el-table-column prop="cpu" label="CPU" width="90" />
          <el-table-column prop="gpu" label="GPU" width="90" />
          <el-table-column prop="memory" label="Memory" width="110" />
        </el-table>
        <el-descriptions v-if="detailData?.service" class="mt-4" :column="4" border>
          <el-descriptions-item label="Service">{{ detailData.service.name || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Type">{{ detailData.service.serviceType || '-' }}</el-descriptions-item>
          <el-descriptions-item label="Port">{{ detailData.service.port ?? '-' }}</el-descriptions-item>
          <el-descriptions-item label="Target port">{{ detailData.service.targetPort ?? 8000 }}</el-descriptions-item>
        </el-descriptions>
      </el-card>
    </el-tab-pane>
    <el-tab-pane label="Pods" name="pods">
      <WorkloadPodsTable
        :pods="detailData?.pods"
        :nodes-history="detailData?.nodesHistory"
        :workload-phase="detailData?.phase"
        :refresh-loading="detailLoading"
        :show-ssh="false"
        @refresh="getDetail"
      />
    </el-tab-pane>
    <el-tab-pane label="Timeline" name="timeline">
      <WorkloadTimeline :conditions="detailData?.conditions" />
    </el-tab-pane>
  </el-tabs>

  <AddDialog v-model:visible="addVisible" :wlid="curWlId" :action="curAction" @success="getDetail" />
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue'
import WorkloadHeader from '@/components/Workload/WorkloadHeader.vue'
import WorkloadPodsTable from '@/components/Workload/WorkloadPodsTable.vue'
import WorkloadTimeline from '@/components/Workload/WorkloadTimeline.vue'
import { useWorkloadDetail } from '@/composables/useWorkloadDetail'
import AddDialog from './Components/AddDialog.vue'
import { ATOM_ROLES } from './atomPayload'

const { detailData, detailLoading, getDetail, onDelete, onStop, onResume } = useWorkloadDetail({
  redirectPath: '/atom',
})

const activeTab = ref('overview')
const addVisible = ref(false)
const curWlId = ref('')
const curAction = ref<'Edit' | 'Clone'>('Edit')

const roleRows = computed(() =>
  ATOM_ROLES.map((role, index) => {
    const resource = detailData.value?.resources?.[index] || {}
    return {
      role,
      image: detailData.value?.images?.[index] || '-',
      entryPoint: detailData.value?.entryPoints?.[index] || '-',
      replica: resource.replica ?? '-',
      cpu: resource.cpu ?? '-',
      gpu: resource.gpu || '-',
      memory: resource.memory ?? '-',
    }
  }),
)

function onEdit() {
  curWlId.value = detailData.value?.workloadId || ''
  curAction.value = 'Edit'
  addVisible.value = true
}

function onClone() {
  curWlId.value = detailData.value?.workloadId || ''
  curAction.value = 'Clone'
  addVisible.value = true
}
</script>

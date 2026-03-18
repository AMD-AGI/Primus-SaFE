<template>
  <div class="pyspy-profiler-tab">
    <!-- Status Alerts -->
    <div v-if="!isWorkloadRunning || runningPods.length === 0">
      <el-alert
        v-if="!isWorkloadRunning"
        type="info"
        title="Workload Not Running"
        description="New profiling tasks cannot be created. You can still view historical profiling results."
        show-icon
        :closable="false"
      />
      <el-alert
        v-else
        type="warning"
        title="No Running Pods"
        description="All pods are in non-running state. Wait for pods to be Running to start profiling."
        show-icon
        :closable="false"
      />
    </div>

    <!-- Profiling History - Always Visible at Top -->
    <ProfilingHistoryTable
      :workload-uid="workloadUid"
      :cluster="cluster"
      @view-flamegraph="handleViewFlamegraph"
      ref="historyTableRef"
    />

    <!-- New Profiling Section - Only when workload is running and has running pods -->
    <el-card v-if="canShowNewProfiling" class="new-profiling-card">
      <template #header>
        <div class="card-header">
          <span class="section-title">New Profiling Task</span>
          <el-tag type="success" size="small">
            {{ runningPods.length }} running / {{ props.pods.length }} total
          </el-tag>
        </div>
      </template>

      <!-- Top Row: Pod Selection -->
      <PodSelector
        :pods="pods"
        :selected-pod="selectedPod"
        @select="handlePodSelect"
        class="pod-selection-row"
      />

      <!-- Bottom Row: Process Tree + Sampling Config -->
      <el-row :gutter="20" class="mt-4">
        <!-- Left Column: Process Tree -->
        <el-col :span="24" :lg="12">
          <ProcessTreePanel
            v-if="selectedPod && selectedPod.status === 'Running'"
            :workload-uid="workloadUid"
            :pod="selectedPod"
            :cluster="cluster"
            :selected-process="selectedProcess"
            @select="handleProcessSelect"
            @refresh="handleRefreshProcesses"
          />
          <el-card v-else class="stat-card empty-placeholder">
            <template #header>
              <span>Process Tree</span>
            </template>
            <el-empty
              description="Select a running pod to view its process tree"
              :image-size="100"
            />
          </el-card>
        </el-col>

        <!-- Right Column: Sampling Config -->
        <el-col :span="24" :lg="12" class="mt-4 lg:mt-0">
          <SamplingConfigPanel
            :selected-process="selectedProcess"
            :disabled="!canStartProfiling"
            :loading="isCreatingTask"
            @start="handleStartProfiling"
          />
        </el-col>
      </el-row>
    </el-card>

    <!-- Flamegraph Viewer Modal -->
    <FlamegraphViewer
      v-model:visible="flamegraphVisible"
      :task-id="viewingTaskId"
      :format="viewingFormat"
      :cluster="cluster"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import ProfilingHistoryTable from './ProfilingHistoryTable.vue'
import PodSelector from './PodSelector.vue'
import ProcessTreePanel from './ProcessTreePanel.vue'
import SamplingConfigPanel from './SamplingConfigPanel.vue'
import FlamegraphViewer from './FlamegraphViewer.vue'
import { createPySpyTask, type NormalizedProcessInfo } from '@/services/pyspy'

interface PodInfo {
  uid: string
  name: string
  namespace: string
  nodeName: string
  status: 'Running' | 'Pending' | 'Succeeded' | 'Failed' | 'Unknown'
  ip?: string
  gpuAllocated?: number
  createdAt?: number
  updatedAt?: number
}

interface SamplingConfig {
  duration: number
  rate: number
  format: 'flamegraph' | 'speedscope' | 'raw'
  native: boolean
  subprocesses: boolean
}

interface Props {
  workloadUid: string
  workloadStatus: 'running' | 'completed' | 'failed' | 'pending'
  pods: PodInfo[]
  cluster?: string
}

const props = defineProps<Props>()

// State
const selectedPod = ref<PodInfo | null>(null)
const selectedProcess = ref<NormalizedProcessInfo | null>(null)
const isCreatingTask = ref(false)
const historyTableRef = ref()

// Flamegraph viewer state
const flamegraphVisible = ref(false)
const viewingTaskId = ref('')
const viewingFormat = ref<'flamegraph' | 'speedscope'>('flamegraph')

// Computed
const isWorkloadRunning = computed(() => props.workloadStatus === 'running')

const runningPods = computed(() => 
  props.pods.filter(pod => pod.status === 'Running')
)

const canShowNewProfiling = computed(() => 
  isWorkloadRunning.value && runningPods.value.length > 0
)

const canStartProfiling = computed(() => 
  isWorkloadRunning.value && 
  selectedPod.value !== null && 
  selectedPod.value.status === 'Running' &&
  selectedProcess.value !== null
)

// Methods
const handlePodSelect = (pod: PodInfo) => {
  if (pod.status !== 'Running') {
    ElMessage.warning('Only running pods can be profiled')
    return
  }
  selectedPod.value = pod
  selectedProcess.value = null
}

const handleProcessSelect = (process: NormalizedProcessInfo) => {
  selectedProcess.value = process
}

const handleRefreshProcesses = () => {
  // Process tree will handle its own refresh
}

const handleStartProfiling = async (config: SamplingConfig) => {
  if (!canStartProfiling.value || !selectedPod.value || !selectedProcess.value) {
    ElMessage.warning('Please select a pod and process first')
    return
  }

  isCreatingTask.value = true
  try {
    const res: any = await createPySpyTask({
      workloadUid: props.workloadUid,
      podUid: selectedPod.value.uid,
      podName: selectedPod.value.name,
      podNamespace: selectedPod.value.namespace,
      nodeName: selectedPod.value.nodeName,
      pid: selectedProcess.value.hostPid,
      duration: config.duration,
      rate: config.rate,
      format: config.format,
      native: config.native,
      subprocesses: config.subprocesses,
      cluster: props.cluster
    })

    ElMessage.success(`Profiling task created: ${res.taskId || res.task_id || 'Success'}`)
    
    // Refresh history table
    if (historyTableRef.value) {
      historyTableRef.value.refresh()
    }
  } catch (error: any) {
    ElMessage.error(error?.message || 'Failed to create profiling task')
  } finally {
    isCreatingTask.value = false
  }
}

const handleViewFlamegraph = (taskId: string, format: 'flamegraph' | 'speedscope') => {
  viewingTaskId.value = taskId
  viewingFormat.value = format
  flamegraphVisible.value = true
}

// Auto-select first running pod when pods change
watch(runningPods, (newPods) => {
  if (newPods.length > 0 && !selectedPod.value) {
    selectedPod.value = newPods[0]
  }
}, { immediate: true })
</script>

<style scoped lang="scss">
@import '@/styles/stats-layout.scss';

.pyspy-profiler-tab {
  .mt-4 {
    margin-top: 16px;
  }

  .new-profiling-card {
    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }

    .section-title {
      font-size: 16px;
      font-weight: 600;
    }

    .pod-selection-row {
      margin-bottom: 20px;
      padding: 16px;
      background: var(--el-fill-color-light);
      border-radius: 8px;
    }

    .empty-placeholder {
      height: 100%;
      min-height: 400px;
      
      :deep(.el-card__body) {
        display: flex;
        align-items: center;
        justify-content: center;
        min-height: 350px;
      }
    }

    @media (max-width: 1024px) {
      .lg\:mt-0 {
        margin-top: 16px !important;
      }
    }
  }
}
</style>

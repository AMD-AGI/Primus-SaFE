<template>
  <el-card class="process-tree-panel stat-card" v-loading="loading">
    <template #header>
      <div class="card-header">
        <span>Process Tree</span>
        <el-button
          size="small"
          :icon="Refresh"
          @click="handleRefresh"
          :loading="loading"
        >
          Refresh
        </el-button>
      </div>
    </template>

    <div v-if="!loading && processes.length > 0">
      <el-input
        v-model="searchText"
        placeholder="Search processes..."
        :prefix-icon="Search"
        clearable
        size="default"
        class="mb-3"
      />

      <div class="process-list">
        <ProcessTreeNode
          v-for="process in filteredProcesses"
          :key="process.hostPid"
          :process="process"
          :selected-pid="selectedProcess?.hostPid"
          @select="handleSelect"
        />
      </div>
    </div>

    <el-empty
      v-else-if="!loading && processes.length === 0"
      description="No processes found"
      :image-size="100"
    />

    <el-alert
      v-if="!loading && !hasPythonProcesses"
      type="warning"
      title="No Python Processes Found"
      description="Make sure Python processes are running in this pod."
      show-icon
      :closable="false"
      class="mt-3"
    />
  </el-card>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Search } from '@element-plus/icons-vue'
import ProcessTreeNode from './ProcessTreeNode.vue'
import { 
  getProcessTree, 
  normalizeProcessInfo,
  type PodProcessTree,
  type NormalizedProcessInfo 
} from '@/services/pyspy'

interface PodInfo {
  uid: string
  name: string
  namespace: string
  nodeName: string
  status: string
}

interface Props {
  workloadUid: string
  pod: PodInfo
  cluster?: string
  selectedProcess?: NormalizedProcessInfo | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  select: [process: NormalizedProcessInfo]
  refresh: []
}>()

const loading = ref(false)
const processes = ref<NormalizedProcessInfo[]>([])
const searchText = ref('')

const hasPythonProcesses = computed(() => {
  const checkPython = (procs: NormalizedProcessInfo[]): boolean => {
    return procs.some(p => p.isPython || checkPython(p.children || []))
  }
  return checkPython(processes.value)
})

const filteredProcesses = computed(() => {
  if (!searchText.value) return processes.value
  
  const search = searchText.value.toLowerCase()
  const filter = (procs: NormalizedProcessInfo[]): NormalizedProcessInfo[] => {
    return procs.filter(p => {
      const matches = p.command.toLowerCase().includes(search) ||
                     p.cmdline.toLowerCase().includes(search) ||
                     p.hostPid.toString().includes(search)
      const hasMatchingChildren = (p.children || []).length > 0 && 
                                  filter(p.children || []).length > 0
      return matches || hasMatchingChildren
    }).map(p => ({
      ...p,
      children: filter(p.children || [])
    }))
  }
  return filter(processes.value)
})

const loadProcesses = async () => {
  loading.value = true
  try {
    const res: PodProcessTree = await getProcessTree({
      workloadUid: props.workloadUid,
      podUid: props.pod.uid,
      podName: props.pod.name,
      podNamespace: props.pod.namespace,
      cluster: props.cluster
    })
    
    // Extract processes from containers[*].rootProcess structure
    // Note: API response is converted from snake_case to camelCase by request interceptor
    const extractedProcesses: NormalizedProcessInfo[] = []
    if (res.containers && Array.isArray(res.containers)) {
      for (const container of res.containers) {
        // Use camelCase property names (response interceptor transforms snake_case to camelCase)
        const rootProc = container.rootProcess
        if (rootProc) {
          // Normalize and add container context
          const normalized = normalizeProcessInfo(rootProc)
          normalized.containerName = container.containerName
          extractedProcesses.push(normalized)
        }
      }
    }
    
    processes.value = extractedProcesses
  } catch (error) {
    ElMessage.error('Failed to load process tree')
    console.error('Load processes error:', error)
  } finally {
    loading.value = false
  }
}

const handleRefresh = () => {
  loadProcesses()
  emit('refresh')
}

const handleSelect = (process: NormalizedProcessInfo) => {
  emit('select', process)
}

watch(() => props.pod.uid, () => {
  loadProcesses()
}, { immediate: true })
</script>

<style scoped lang="scss">
@import '@/styles/stats-layout.scss';

.process-tree-panel {
  height: 100%;

  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .mb-3 {
    margin-bottom: 12px;
  }

  .mt-3 {
    margin-top: 12px;
  }

  .process-list {
    max-height: 400px;
    overflow-y: auto;
  }
}
</style>

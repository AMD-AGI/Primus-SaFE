import request from '@/services/request'
import { useGlobalCluster } from '@/composables/useGlobalCluster'

// Helper to get cluster from global state if not provided
const getCluster = (cluster?: string): string | undefined => {
  if (cluster) return cluster
  const { selectedCluster } = useGlobalCluster()
  return selectedCluster.value || undefined
}

// ProcessInfo matches backend pyspy.ProcessInfo structure
export interface ProcessInfo {
  // Host-level information
  host_pid: number
  host_ppid: number
  // Container-level information
  container_pid?: number
  container_ppid?: number
  // Process details
  cmdline: string
  comm: string
  exe?: string
  args?: string[]
  env?: string[]
  cwd?: string
  // Process state
  state: string
  threads: number
  // Resource usage
  cpu_time?: number
  memory_rss?: number
  memory_virtual?: number
  // Container context
  container_id?: string
  container_name?: string
  pod_uid?: string
  pod_name?: string
  pod_namespace?: string
  // Process classification
  is_python: boolean
  is_java?: boolean
  // GPU binding information
  gpu_devices?: Array<{
    device_index: number
    device_uuid?: string
    device_name?: string
  }>
  has_gpu?: boolean
  // Timestamps
  start_time?: number
  // Tree structure
  children?: ProcessInfo[]
}

// Normalized ProcessInfo for frontend usage (camelCase)
export interface NormalizedProcessInfo {
  pid: number           // Uses host_pid
  hostPid: number
  hostPpid: number
  containerPid?: number
  command: string       // Uses comm
  cmdline: string
  args?: string[]
  isPython: boolean
  isJava?: boolean
  cpuTime?: number
  memoryRss?: number
  state: string
  threads: number
  containerName?: string
  children: NormalizedProcessInfo[]
}

// Container process tree (camelCase - after response interceptor transforms)
export interface ContainerProcessTree {
  containerId: string
  containerName: string
  imageName?: string
  rootProcess: ProcessInfo | null
  processCount: number
  pythonCount: number
  // Legacy snake_case fields for compatibility
  container_id?: string
  container_name?: string
  image_name?: string
  root_process?: ProcessInfo | null
  process_count?: number
  python_count?: number
}

// Pod process tree response from backend (camelCase - after response interceptor transforms)
export interface PodProcessTree {
  podName: string
  podNamespace: string
  podUid: string
  nodeName?: string
  containers: ContainerProcessTree[]
  totalProcesses: number
  totalPython: number
  collectedAt: string
  // Legacy snake_case fields for compatibility
  pod_name?: string
  pod_namespace?: string
  pod_uid?: string
  node_name?: string
  total_processes?: number
  total_python?: number
  collected_at?: string
}

export interface PySpyTask {
  taskId: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  podUid: string
  podName: string
  podNamespace: string
  nodeName: string
  pid: number
  duration: number
  format: string
  outputFile?: string
  fileSize?: number
  error?: string
  createdAt: string
  startedAt?: string
  completedAt?: string
  filePath?: string
}

export interface CreateTaskParams {
  workloadUid?: string  // Parent workload UID (for querying history)
  podUid: string
  podName?: string
  podNamespace?: string
  nodeName: string
  pid: number
  duration?: number
  rate?: number
  format?: string
  native?: boolean
  subprocesses?: boolean
  cluster?: string
}

// Helper function to normalize ProcessInfo from backend to frontend (camelCase)
// Note: Response interceptor converts snake_case to camelCase, so we need to handle both formats
export function normalizeProcessInfo(proc: ProcessInfo | any): NormalizedProcessInfo {
  // Handle both snake_case (original) and camelCase (after interceptor transform) field names
  const hostPid = proc.hostPid ?? proc.host_pid
  const hostPpid = proc.hostPpid ?? proc.host_ppid
  const containerPid = proc.containerPid ?? proc.container_pid
  const isPython = proc.isPython ?? proc.is_python
  const isJava = proc.isJava ?? proc.is_java
  const cpuTime = proc.cpuTime ?? proc.cpu_time
  const memoryRss = proc.memoryRss ?? proc.memory_rss
  const containerName = proc.containerName ?? proc.container_name
  
  return {
    pid: hostPid,
    hostPid: hostPid,
    hostPpid: hostPpid,
    containerPid: containerPid,
    command: proc.comm || proc.cmdline?.split(' ')[0] || 'unknown',
    cmdline: proc.cmdline || '',
    args: proc.args,
    isPython: isPython ?? false,
    isJava: isJava,
    cpuTime: cpuTime,
    memoryRss: memoryRss,
    state: proc.state,
    threads: proc.threads,
    containerName: containerName,
    children: (proc.children || []).map(normalizeProcessInfo)
  }
}

// Helper function to flatten process tree for display
export function flattenProcessTree(proc: ProcessInfo, result: ProcessInfo[] = []): ProcessInfo[] {
  result.push(proc)
  if (proc.children) {
    for (const child of proc.children) {
      flattenProcessTree(child, result)
    }
  }
  return result
}

// Get process tree for a pod
export function getProcessTree(params: {
  workloadUid: string
  podUid: string
  podName?: string
  podNamespace?: string
  cluster?: string
}): Promise<PodProcessTree> {
  return request.post(`/workloads/${params.workloadUid}/process-tree`, {
    pod_uid: params.podUid,
    pod_name: params.podName,
    pod_namespace: params.podNamespace,
    cluster: getCluster(params.cluster)
  })
}

// Create py-spy sampling task
export function createPySpyTask(params: CreateTaskParams) {
  return request.post('/pyspy/sample', {
    workload_uid: params.workloadUid,
    pod_uid: params.podUid,
    pod_name: params.podName,
    pod_namespace: params.podNamespace,
    node_name: params.nodeName,
    pid: params.pid,
    duration: params.duration || 30,
    rate: params.rate || 100,
    format: params.format || 'flamegraph',
    native: params.native || false,
    subprocesses: params.subprocesses || false,
    cluster: getCluster(params.cluster)
  })
}

// List py-spy tasks
export function listPySpyTasks(params: {
  workloadUid?: string
  podUid?: string
  podNamespace?: string
  status?: string
  cluster?: string
  limit?: number
  offset?: number
}) {
  return request.post('/pyspy/tasks', {
    workload_uid: params.workloadUid,
    pod_uid: params.podUid,
    pod_namespace: params.podNamespace,
    status: params.status,
    cluster: getCluster(params.cluster),
    limit: params.limit,
    offset: params.offset
  })
}

// Get task details
export function getPySpyTask(taskId: string, cluster?: string) {
  return request.get(`/pyspy/task/${taskId}`, {
    params: { cluster: getCluster(cluster) }
  })
}

// Get file content (for SVG/text viewing)
export function getPySpyFileContent(taskId: string, filename: string, cluster?: string) {
  return request.get(`/pyspy/file/${taskId}/${filename}`, {
    params: { cluster: getCluster(cluster) },
    responseType: 'text'
  })
}

// Cancel task
export function cancelPySpyTask(taskId: string, cluster?: string, reason?: string) {
  return request.post(`/pyspy/task/${taskId}/cancel`, {
    reason
  }, {
    params: { cluster: getCluster(cluster) }
  })
}

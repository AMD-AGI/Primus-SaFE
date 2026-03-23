import request from '@/services/request'
import { lensRequest, rootCauseRequest } from '@/services/request'
import type {
  WorkloadParams,
  SubmitWorkloadRequest,
  GetLogParams,
  GetLogResponse,
  LogTableResult,
  DownloadParams,
  EditWorkloadRequest,
  RootCauseAnalysisResult,
  PendingCauseAnalysisResult,
  CreatePendingCauseJobRequest,
  CreatePendingCauseJobResponse,
} from './type'

const LONG_TIMEOUT = 30_000

export const getWorkloadsList = (params: WorkloadParams): Promise<any> => {
  // Special handling for phase parameter: convert array to comma-separated string
  const processedParams = { ...params, src: 'console' }
  if (Array.isArray(params.phase) && params.phase.length > 0) {
    // Convert phase array to comma-separated string format: "Pending, Running"
    processedParams.phase = params.phase.join(',') as any
  }
  return request.get('/workloads', { params: processedParams, timeout: LONG_TIMEOUT })
}

export const getWorkloadDetail = (id: string): Promise<any> =>
  request.get(`/workloads/${id}`, { params: { src: 'console' }, timeout: LONG_TIMEOUT })

export const getWorkloadService = (id: string): Promise<any> =>
  request.get(`/workloads/${id}/service`, { timeout: LONG_TIMEOUT })

// Log download
export const downloadWlLogs = (data: DownloadParams): Promise<{ jobId: string }> =>
  request.post('/opsjobs', data)
export const getDownloadWlLogsUrl = (id: string): Promise<any> =>
  request.get(`/opsjobs/${id}`, { timeout: LONG_TIMEOUT })

export const getWorkloadLogs = (wlId: string, data: GetLogParams): Promise<LogTableResult> =>
  request
    .post<GetLogResponse>(`/workloads/${wlId}/logs`, data, { timeout: LONG_TIMEOUT })
    .then((res: any) => {
      const hits = res?.hits?.hits ?? []
      const rows = hits.map((h: any) => ({
        id: h._id,
        timestamp: h._source['@timestamp'],
        message: h._source.message,
        pod_name: h._source.kubernetes?.pod_name ?? '',
        host: h._source.kubernetes?.host ?? '',
      }))
      return {
        rows,
        total: res?.hits?.total?.value ?? 0,
        took: res?.took ?? 0,
      }
    })

// export type LogContextRow = { id: string; line: number; timestamp: string; message: string }
// interface GetLogContextResponse {
//   hits: { hits: Array<{ _source: LogContextRow & { '@timestamp': string } }> }
// }

interface LogContextParams {
  limit: number
  since: string
  dispatchCount?: number
  nodeNames?: string
}
export const getLogContext = (wlId: string, docId: string, data: LogContextParams): Promise<any> =>
  request
    .post<any>(`/workloads/${wlId}/logs/${docId}/context`, data, { timeout: LONG_TIMEOUT })
    .then((res: any) => {
      const hits = res?.hits?.hits ?? []
      return hits.map((h: any) => ({
        id: h._id,
        line: h._source.line,
        timestamp: h._source['@timestamp'],
        message: h._source.message,
      }))
    })

export const getWorkloadLogsByPod = (
  wlId: string,
  podId: string,
  params: { tailLines: number },
): Promise<any> =>
  request.get(`/workloads/${wlId}/pods/${podId}/logs`, { params, timeout: LONG_TIMEOUT })
// workloads/wlid/pods/podid/logs?tailLines=1000

export const addWorkload = (data: SubmitWorkloadRequest): Promise<any> =>
  request.post('/workloads', data, { timeout: LONG_TIMEOUT })

export const getNodeFlavorAvail = (id: string): Promise<any> =>
  request.get(`/nodeflavors/${id}/avail`)

export const deleteWorkload = (id: string): Promise<any> => request.delete(`/workloads/${id}`)

export const stopWorkload = (id: string): Promise<any> => request.post(`/workloads/${id}/stop`)

export const editWorkload = (id: string, data: EditWorkloadRequest): Promise<any> =>
  request.patch(`/workloads/${id}`, data, { timeout: LONG_TIMEOUT })

// Batch operations
export const batchDelWorkload = (data: { workloadIds: string[] }): Promise<any> =>
  request.post('/workloads/delete', data)
export const batchStopWorkload = (data: { workloadIds: string[] }): Promise<any> =>
  request.post('/workloads/stop', data)

function toQuery(obj?: Record<string, any>) {
  const usp = new URLSearchParams()
  if (!obj) return ''
  for (const [k, v] of Object.entries(obj)) {
    if (v == null) continue
    if (Array.isArray(v)) v.forEach((i) => usp.append(k, String(i)))
    else if (v instanceof Date) usp.append(k, v.toISOString())
    else usp.append(k, String(v))
  }
  return usp.toString()
}
export const getLensHourlyStats = (p: {
  cluster: string
  namespace: string
  start_time?: string
  end_time?: string
  page?: number
  page_size?: number
  order_by?: string
  workload_name?: string
  order_direction?: 'asc' | 'desc'
}) => {
  const qs = toQuery(p)
  return lensRequest.get(`/gpu-aggregation/workloads/hourly-stats${qs ? `?${qs}` : ''}`, {
    params: undefined as any,
  })
}

export const getGPUAggregation = (p: {
  cluster: string
  namespace?: string
  start_time?: string
  end_time?: string
  page?: number
  page_size?: number
  order_by?: string
  order_direction?: 'asc' | 'desc'
}) => {
  const qs = toQuery(p)
  return lensRequest.get(`/gpu-aggregation/namespaces/hourly-stats${qs ? `?${qs}` : ''}`, {
    params: undefined as any,
  })
}

// Root Cause Analysis
export const getRootCauseAnalysis = (workloadId: string): Promise<RootCauseAnalysisResult> =>
  rootCauseRequest.get(`/api/workloads/${workloadId}`)

// Pending Cause Analysis
export const createPendingCauseJob = (
  data: CreatePendingCauseJobRequest,
): Promise<CreatePendingCauseJobResponse> => request.post('/agent/pending-cause/api/jobs', data)

export const getPendingCauseJob = (jobId: string): Promise<PendingCauseAnalysisResult> =>
  request.get(`/agent/pending-cause/api/jobs/${jobId}`)

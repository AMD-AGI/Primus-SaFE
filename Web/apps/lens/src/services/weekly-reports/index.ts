import request from '@/services/request'
import { useGlobalCluster } from '@/composables/useGlobalCluster'

// Helper to get cluster and append to params
const withCluster = <T extends Record<string, any>>(params?: T): T & { cluster?: string } => {
  const { selectedCluster } = useGlobalCluster()
  const cluster = selectedCluster.value
  return {
    ...params,
    ...(cluster ? { cluster } : {}),
  } as T & { cluster?: string }
}

// Get weekly reports list
export const getWeeklyReports = (params?: {
  page?: number
  pageSize?: number
  clusterName?: string
  status?: string
  sortBy?: string
  sortOrder?: string
}) => {
  return request({
    url: '/weekly-reports/gpu_utilization',
    method: 'get',
    params: withCluster(params)
  })
}

// Get weekly report detail
export const getWeeklyReportDetail = (id: string) => {
  return request({
    url: `/weekly-reports/gpu_utilization/${id}`,
    method: 'get',
    params: withCluster()
  })
}

// Generate weekly report
export const generateWeeklyReport = (data: {
  clusterName: string
  weekStartDate: string
  weekEndDate: string
}) => {
  const { selectedCluster } = useGlobalCluster()
  return request({
    url: '/weekly-reports/gpu_utilization/generate',
    method: 'post',
    data: {
      ...data,
      cluster: selectedCluster.value
    }
  })
}

// Export weekly report
export const exportWeeklyReport = (id: string) => {
  return request({
    url: `/weekly-reports/gpu_utilization/${id}/export`,
    method: 'get',
    responseType: 'blob',
    params: withCluster()
  })
}

// Delete weekly report
export const deleteWeeklyReport = (id: string) => {
  return request({
    url: `/weekly-reports/gpu_utilization/${id}`,
    method: 'delete',
    params: withCluster()
  })
}
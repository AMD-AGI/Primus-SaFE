import request from '@/services/request'
import type { CreateOpsjobsResponse, SubmitOpsjobsRequest } from './type'
import type { AxiosRequestConfig } from 'axios'

const LONG_TIMEOUT = 30_000

export const addOpsjobs = (
  data: SubmitOpsjobsRequest,
  config?: AxiosRequestConfig,
): Promise<CreateOpsjobsResponse> =>
  request.post<CreateOpsjobsResponse>('/opsjobs', data, config) as Promise<CreateOpsjobsResponse>

export const getOpsjobs = (params: { 
  type: string
  workspaceId?: string
  clusterId?: string
  page?: number
  limit?: number
  since?: string
  until?: string
}): Promise<any> =>
  request.get('/opsjobs', { params, timeout: LONG_TIMEOUT })

// export const editWorkspace = (id: string, data: BaseSubmitWsData): Promise<any> =>
//   request.patch(`/workspaces/${id}`, data)

export const getOpsjobsDetail = (id: string): Promise<any> => request.get(`/opsjobs/${id}`)

export const deleteOpsjobs = (id: string) => request.delete(`/opsjobs/${id}`)

export const stopOpsjob = (id: string) => request.post(`/opsjobs/${id}/stop`)

import request from '@/services/request'
import type {
  A2AServiceListResponse,
  A2AService,
  A2ARegisterRequest,
  A2ACallLogListResponse,
  A2ACallLogParams,
  A2ATopologyResponse,
} from './type'

export const getA2AServices = (params?: { status?: string }): Promise<A2AServiceListResponse> =>
  request.get('/a2a/services', { params })

export const getA2AServiceDetail = (serviceName: string): Promise<A2AService> =>
  request.get(`/a2a/services/${serviceName}`)

export const registerA2AService = (data: A2ARegisterRequest): Promise<A2AService> =>
  request.post('/a2a/services', data)

export const deleteA2AService = (serviceName: string): Promise<void> =>
  request.delete(`/a2a/services/${serviceName}`)

export const getA2ACallLogs = (params?: A2ACallLogParams): Promise<A2ACallLogListResponse> =>
  request.get('/a2a/call-logs', { params })

export const getA2ATopology = (): Promise<A2ATopologyResponse> =>
  request.get('/a2a/topology')

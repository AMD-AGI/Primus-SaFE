import request from '@/services/request'
import type { FaultParams, FaultsItemResp } from './type'

export const getFaultsList = (params: FaultParams): Promise<FaultsItemResp> =>
  request.get(`/faults`, { params })

export const deleteFault = (id: string) => request.delete(`/faults/${id}`)

export const stopFault = (id: string) => request.post(`/faults/${id}/stop`)

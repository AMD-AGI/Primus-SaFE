import request from '@/services/request'
import type { KeysParams } from './type'

export const getPodContainers = (wsid: string, podName: string): Promise<any> =>
  request.get(`/workloads/${wsid}/pods/${podName}/containers`)

export const getPublicKeysList = (params: KeysParams): Promise<any> =>
  request.get('/publickeys', { params })

export const addPublickey = (data: { description: string; publicKey: string }) =>
  request.post('/publickeys', data)

export const deletePublickey = (id: string) => request.delete(`/publickeys/${id}`)

export const editPublickeyStatus = (id: string, data: { status: boolean }) =>
  request.patch(`/publickeys/${id}/status`, data)

export const editPublickeyDesc = (id: string, data: { description: string }) =>
  request.patch(`/publickeys/${id}/description`, data)

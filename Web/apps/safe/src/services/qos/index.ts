import request from '@/services/request'
import type { ListQosResp, QosItem, QosScope } from './type'

/** listQos returns every cluster ceiling. */
export const listQos = (): Promise<ListQosResp> => request.get('/qos')

/** getQos returns one cluster ceiling. */
export const getQos = (name: string): Promise<QosItem> =>
  request.get(`/qos/${encodeURIComponent(name)}`)

/** createQos creates the ceiling for one cluster. */
export const createQos = (name: string, data: QosScope): Promise<QosItem> =>
  request.post(`/qos/${encodeURIComponent(name)}`, data)

/** updateQos replaces the ceiling for one cluster. Omitted fields are cleared. */
export const updateQos = (name: string, data: QosScope): Promise<QosItem> =>
  request.put(`/qos/${encodeURIComponent(name)}`, data)

/** deleteQos removes one cluster ceiling. */
export const deleteQos = (name: string): Promise<void> =>
  request.delete(`/qos/${encodeURIComponent(name)}`)

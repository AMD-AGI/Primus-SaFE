import request from '@/services/request'
import type { ListQosResp, QosScope } from './type'

/** listQos returns every default ceiling. */
export const listQos = (): Promise<ListQosResp> => request.get('/qos')

/** putQos creates or replaces the ceiling named default or a cluster id. */
export const putQos = (name: string, data: QosScope): Promise<void> =>
  request.put(`/qos/${encodeURIComponent(name)}`, data)

/** deleteQos removes one default ceiling. */
export const deleteQos = (name: string): Promise<void> =>
  request.delete(`/qos/${encodeURIComponent(name)}`)

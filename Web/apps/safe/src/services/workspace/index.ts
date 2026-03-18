import request from '@/services/request'
import type { SubmitWorkspaceRequest, BaseSubmitWsData } from './type'

export const addWorkspace = (data: SubmitWorkspaceRequest): Promise<any> =>
  request.post('/workspaces', data)

export const editWorkspace = (id: string, data: BaseSubmitWsData): Promise<any> =>
  request.patch(`/workspaces/${id}`, data)

export const getWorkspaceDetail = (id: string): Promise<any> => request.get(`/workspaces/${id}`)

export const deleteWorkspace = (id: string): Promise<any> => request.delete(`/workspaces/${id}`)

export const getPersistentVolumes = (workspaceId: string): Promise<any> =>
  request.get('/persistentvolumes', { params: { workspaceId } })

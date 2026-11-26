import request from '@/services/request'
import type {
  SubmitWorkspaceRequest,
  BaseSubmitWsData,
  WorkspaceAccessPolicy,
  WorkspaceMembersResp,
  UpdateWorkspaceTeamsRequest,
  UpdateWorkspaceTeamsResp,
} from './type'

export const addWorkspace = (data: SubmitWorkspaceRequest): Promise<any> =>
  request.post('/workspaces', data)

export const editWorkspace = (id: string, data: BaseSubmitWsData): Promise<any> =>
  request.patch(`/workspaces/${id}`, data)

export const getWorkspaceDetail = (id: string): Promise<any> => request.get(`/workspaces/${id}`)

export const deleteWorkspace = (id: string): Promise<any> => request.delete(`/workspaces/${id}`)

export const getPersistentVolumes = (workspaceId: string): Promise<any> =>
  request.get('/persistentvolumes', { params: { workspaceId } })

/** getWorkspaceMembers returns the effective members of a workspace: users, bound teams and platform groups. */
export const getWorkspaceMembers = (id: string): Promise<WorkspaceMembersResp> =>
  request.get(`/workspaces/${encodeURIComponent(id)}/members`)

/** updateWorkspaceTeams adds, re-roles or removes team bindings of a workspace incrementally. */
export const updateWorkspaceTeams = (
  id: string,
  data: UpdateWorkspaceTeamsRequest,
): Promise<UpdateWorkspaceTeamsResp> =>
  request.put(`/workspaces/${encodeURIComponent(id)}/teams`, data)

/** patchWorkspaceAccessPolicy changes the access policy of a workspace. System administrators only. */
export const patchWorkspaceAccessPolicy = (
  id: string,
  accessPolicy: WorkspaceAccessPolicy,
): Promise<unknown> => request.patch(`/workspaces/${encodeURIComponent(id)}`, { accessPolicy })

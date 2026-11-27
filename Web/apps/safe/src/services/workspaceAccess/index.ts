import request from '@/services/request'
import type {
  CreateWorkspaceAccessRequest,
  ListWorkspaceAccessRequestsResp,
  WorkspaceAccessRequest,
  WorkspaceAccessRequestDetail,
  WorkspaceAccessRequestView,
  WorkspaceCatalogResp,
} from './type'

/** getWorkspaceCatalog lists the workspaces that accept access requests, with the caller's status. */
export const getWorkspaceCatalog = (): Promise<WorkspaceCatalogResp> =>
  request.get('/workspaces/catalog')

/** createWorkspaceAccessRequest asks for member access to a workspace. */
export const createWorkspaceAccessRequest = (
  data: CreateWorkspaceAccessRequest,
): Promise<WorkspaceAccessRequest> => request.post('/workspace-access-requests', data)

/** listWorkspaceAccessRequests lists the caller's requests, or the pending requests the caller may decide. */
export const listWorkspaceAccessRequests = (
  view: WorkspaceAccessRequestView,
): Promise<ListWorkspaceAccessRequestsResp> =>
  request.get('/workspace-access-requests', { params: { view } })

/** getWorkspaceAccessRequest reads one request. A refusal (403) is left to the caller. */
export const getWorkspaceAccessRequest = (id: string): Promise<WorkspaceAccessRequestDetail> =>
  request.get(`/workspace-access-requests/${encodeURIComponent(id)}`, { silentErrors: true })

/** approveWorkspaceAccessRequest grants the requested access. Approvers only. */
export const approveWorkspaceAccessRequest = (
  id: string | number,
  comment: string,
): Promise<WorkspaceAccessRequest> =>
  request.post(`/workspace-access-requests/${encodeURIComponent(id)}/approve`, { comment })

/** rejectWorkspaceAccessRequest declines the request. Approvers only. */
export const rejectWorkspaceAccessRequest = (
  id: string | number,
  comment: string,
): Promise<WorkspaceAccessRequest> =>
  request.post(`/workspace-access-requests/${encodeURIComponent(id)}/reject`, { comment })

/** cancelWorkspaceAccessRequest withdraws a pending request. The requester only. */
export const cancelWorkspaceAccessRequest = (
  id: string | number,
): Promise<WorkspaceAccessRequest> =>
  request.post(`/workspace-access-requests/${encodeURIComponent(id)}/cancel`)

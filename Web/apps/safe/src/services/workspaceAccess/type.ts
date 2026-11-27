// Status of a workspace access request. Every status but pending is final.
export type WorkspaceAccessRequestStatus =
  | 'pending'
  | 'approved'
  | 'rejected'
  | 'cancelled'
  | 'auto_closed'

// A request of one user for member access to one workspace
export interface WorkspaceAccessRequest {
  id: number
  workspace: string
  workspaceName: string
  userId: string
  userName: string
  userEmail: string
  reason: string
  status: WorkspaceAccessRequestStatus
  decidedBy: string
  decidedByName: string
  decisionComment: string
  decidedAt: string
  creationTime: string
  updateTime: string
  approvalUrl: string
}

// GET /workspace-access-requests/:id: the request and what the caller may do with it
export interface WorkspaceAccessRequestDetail extends WorkspaceAccessRequest {
  canDecide: boolean
  isRequester: boolean
}

export interface ListWorkspaceAccessRequestsResp {
  totalCount: number
  items: WorkspaceAccessRequest[]
}

// mine: the caller's own requests; approver: pending requests the caller may decide
export type WorkspaceAccessRequestView = 'mine' | 'approver'

export interface CreateWorkspaceAccessRequest {
  workspace: string
  reason: string
}

// A manager of a workspace in the catalog
export interface CatalogManager {
  id: string
  name: string
  email: string
}

// A team bound to a workspace in the catalog
export interface CatalogTeam {
  id: string
  displayName: string
  role: string
}

// member: the caller has access; pending: the caller has a pending request; none: neither
export type CatalogMyStatus = 'member' | 'pending' | 'none'

// GET /workspaces/catalog: a workspace that accepts access requests
export interface CatalogWorkspace {
  workspaceId: string
  workspaceName: string
  description: string
  managers: CatalogManager[]
  teams: CatalogTeam[]
  myStatus: CatalogMyStatus
  pendingRequestId?: number
}

export interface WorkspaceCatalogResp {
  totalCount: number
  items: CatalogWorkspace[]
}

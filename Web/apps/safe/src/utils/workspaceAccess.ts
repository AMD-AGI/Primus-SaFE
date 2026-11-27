import type { WorkspaceAccessRequestStatus } from '@/services/workspaceAccess/type'

type TagType = 'success' | 'warning' | 'danger' | 'info' | 'primary'

const STATUS_LABELS: Record<WorkspaceAccessRequestStatus, string> = {
  pending: 'Pending',
  approved: 'Approved',
  rejected: 'Rejected',
  cancelled: 'Cancelled',
  auto_closed: 'Auto-closed',
}

const STATUS_TAGS: Record<WorkspaceAccessRequestStatus, TagType> = {
  pending: 'warning',
  approved: 'success',
  rejected: 'danger',
  cancelled: 'info',
  auto_closed: 'info',
}

/** Display label of a request status; an unknown status is shown as it is. */
export function requestStatusLabel(status: string | undefined): string {
  return STATUS_LABELS[status as WorkspaceAccessRequestStatus] ?? (status || '-')
}

/** Element Plus tag type of a request status; 'info' for an unknown status. */
export function requestStatusTag(status: string | undefined): TagType {
  return STATUS_TAGS[status as WorkspaceAccessRequestStatus] ?? 'info'
}

/** Whether a request is still waiting for a decision. */
export function isPendingRequest(status: string | undefined): boolean {
  return status === 'pending'
}

/** Console path of the approval page of a request. */
export function approvalPath(id: string | number): string {
  return `/workspace-access/requests/${encodeURIComponent(String(id))}`
}

/**
 * The approval link to share for a request: the server's approvalUrl when it is an http(s) URL,
 * otherwise the approval page on `origin`.
 */
export function approvalLink(
  req: { id: string | number; approvalUrl?: string },
  origin: string,
): string {
  const url = (req.approvalUrl ?? '').trim()
  if (/^https?:\/\//i.test(url)) return url
  return `${origin}${approvalPath(req.id)}`
}

import request from '@/services/request'
import type {
  GetUsageWorkspaceResp,
  ListUsageClustersResp,
  ListUsageWorkspacesResp,
  UsageQuery,
} from './type'

// The workspace endpoint may wait for the GPU metrics backend, so allow more than the default.
const WORKSPACE_TIMEOUT_MS = 30000

/** Usage of every cluster the caller can see. */
export const listUsageClusters = (params: UsageQuery): Promise<ListUsageClustersResp> =>
  request.get('/usage/clusters', { params })

/** Usage of the workspaces of one cluster. */
export const listUsageWorkspaces = (
  cluster: string,
  params: UsageQuery,
): Promise<ListUsageWorkspacesResp> =>
  request.get(`/usage/clusters/${encodeURIComponent(cluster)}/workspaces`, { params })

/** Usage, series and workload lists of one workspace. */
export const getUsageWorkspace = (
  workspace: string,
  params: UsageQuery,
): Promise<GetUsageWorkspaceResp> =>
  request.get(`/usage/workspaces/${encodeURIComponent(workspace)}`, {
    params,
    timeout: WORKSPACE_TIMEOUT_MS,
  })

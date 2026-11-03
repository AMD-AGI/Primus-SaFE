import request from '@/services/request'
import type { ClusterItem } from './type'

// cluster
// With `quiet` a failure is left to the caller as the raw axios error (so that its status can be
// read): no toast, and a 401 is not handled by the response interceptor.
export const getClusters = (quiet = false): Promise<{ items: ClusterItem[]; totalCount: number }> =>
  request.get('/clusters', quiet ? { skipErrorHandler: true } : undefined)

// workspace
export const getWorkspace = (): Promise<any> => request.get('/workspaces')

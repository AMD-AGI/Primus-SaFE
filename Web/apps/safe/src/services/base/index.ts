import request from '@/services/request'
import type { ClusterItem } from './type'

// cluster
export const getClusters = (): Promise<{ items: ClusterItem[]; totalCount: number }> =>
  request.get('/clusters')

// workspace
export const getWorkspace = (): Promise<any> => request.get('/workspaces')

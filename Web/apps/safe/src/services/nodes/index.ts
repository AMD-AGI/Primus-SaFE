import request from '@/services/request'
// import { withFieldDefaults } from '@/utils/index'
import type {
  GetNodePodLogResponse,
  NodeEditData,
  CreateClusterPayload,
  NodesParams,
  FlavorItemResp,
  CreateFlavorPayload,
  PatchNodeFlavorRequest,
  CreateSecretPayload,
  RebootNodesParams,
  NodeRebootData,
  GetRebootLogsResp,
} from './type'

export interface Node {
  id: number
  name: string
  status: string
}

interface ManageParams {
  action: string
  nodeIds: string[]
}

const LONG_TIMEOUT = 30_000

// add dialog options
export const getNodeTemps = (): Promise<any> =>
  request.get('/nodetemplates', { timeout: LONG_TIMEOUT })

export const getNodeFlavors = (): Promise<FlavorItemResp> =>
  request.get('/nodeflavors', { timeout: LONG_TIMEOUT })

export const addNodeFlavor = (data: CreateFlavorPayload) => request.post('/nodeflavors', data)

export const editNodeFlavor = (id: string, data: PatchNodeFlavorRequest) =>
  request.patch(`/nodeflavors/${id}`, data)

export const deleteNodeFlavor = (id: string) => request.delete(`/nodeflavors/${id}`)

// secrets
export const getSecrets = (params: {
  type?: string
  labels?: Record<string, string>
  [key: string]: any
}): Promise<any> => request.get('/secrets', { params })

export const addSecret = (data: CreateSecretPayload) => request.post('/secrets', data)

export const deleteSecret = (id: string) => request.delete(`/secrets/${id}`)

export const getSecretDetail = (id: string): Promise<any> => request.get(`/secrets/${id}`)

export const editSecret = (id: string, data: any) => request.patch(`/secrets/${id}`, data)

// node list
export const getNodesList = (params: NodesParams): Promise<any> =>
  request.get('/nodes', { params, timeout: LONG_TIMEOUT })

export const getNodeDetail = (id: string): Promise<any> =>
  request.get(`/nodes/${id}`, { timeout: LONG_TIMEOUT })

export const getNodeDetailLogs = (id: string): Promise<GetNodePodLogResponse> =>
  request.get(`/nodes/${id}/logs`, { timeout: LONG_TIMEOUT })

export const addNode = (data: Record<string, any>): Promise<any> => request.post('/nodes', data)

export const editNode = (id: string, data: NodeEditData) => request.patch(`/nodes/${id}`, data)

export const deleteNode = (id: string): Promise<any> => request.delete(`/nodes/${id}`)
export const deleteNodes = (data: { nodeIds: string[] }): Promise<any> =>
  request.post('/nodes/delete', data)

export const rebootNodes = (data: RebootNodesParams) => request.post('/opsjobs', data)

export const getRebootLogs = (id: string, params: NodeRebootData): Promise<GetRebootLogsResp> =>
  request.get(`/nodes/${id}/reboot/logs`, { params })
export const mockGetRebootLogs = async (
  id: string,
  params: NodeRebootData,
): Promise<GetRebootLogsResp> => {
  return {
    totalCount: 1,
    items: [
      {
        userId: '63a9f0ea7bb98050796b649e85481845',
        userName: '63a9f0ea7bb98050796b649e85481845',
        creationTime: '2025-10-02 02:22:21',
      },
    ],
  }
}

interface NodeRelateData {
  action: string
  nodeIds: string[]
}
export const relateNodeToWs = (id: string, data: NodeRelateData): Promise<any> =>
  request.post(`/workspaces/${id}/nodes`, data)

// cluster actions
export const manageNodes = (data: ManageParams, id: string): Promise<any> =>
  request.post(`/clusters/${id}/nodes`, data)

export const editClusterProtected = (
  id: string,
  data: { isProtected?: boolean; imageSecretId?: string },
) => request.patch(`/clusters/${id}`, data)

export const addCluster = (data: CreateClusterPayload): Promise<any> =>
  request.post('/clusters', data)

export const deleteCluster = (id: string): Promise<any> => request.delete(`/clusters/${id}`)

export const getClusterDetail = (id: string): Promise<any> => request.get(`/clusters/${id}`)

// Export nodes
export async function exportNodes(params: any) {
  const resp: any = await request.get('/nodes/export', {
    responseType: 'blob',
    params,
    rawResponse: true,
  })

  // Adapt to common axios wrapper
  const data: Blob = resp?.data ?? resp
  const cd = resp?.headers?.['content-disposition'] ?? ''

  const m = /filename\*?=(?:UTF-8''|")?([^";]+)/i.exec(cd)
  const filename = decodeURIComponent(m?.[1] || 'nodes.csv')

  const url = URL.createObjectURL(data)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}
// retry nodes (batch)
export const retryNodes = (data: { nodeIds: string[] }): Promise<any> =>
  request.post('/nodes/retry', data, { timeout: LONG_TIMEOUT })

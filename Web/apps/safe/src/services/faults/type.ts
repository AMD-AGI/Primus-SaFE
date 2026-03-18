export interface FaultParams {
  offset?: number
  limit?: number
  sortBy?: string
  order?: 'asc' | 'desc'

  nodeId?: string
  monitorId?: string

  onlyOpen?: boolean
}

export interface FaultsItemResp {
  totalCount: number
  items: FaultsData[]
}
export interface FaultsData {
  id: string
  nodeId: string
  monitorId: string
  message: string
  action: string
  phase: string
  clusterId: string
  creationTime: string
  deletionTime: string
}

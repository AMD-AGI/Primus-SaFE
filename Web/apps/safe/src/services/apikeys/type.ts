export interface APIKey {
  id: number
  name: string
  userId: string
  apiKey?: string
  expirationTime: string
  creationTime: string
  whitelist: string[]
  deleted: boolean
  deletionTime?: string | null
}

export interface CreateAPIKeyRequest {
  name: string
  ttlDays: number
  whitelist?: string[]
}

export interface CreateAPIKeyResponse extends APIKey {
  apiKey: string
}

export interface ListAPIKeysParams {
  offset?: number
  limit?: number
  sortBy?: 'creationTime' | 'expirationTime'
  order?: 'desc' | 'asc'
}

export interface ListAPIKeysResponse {
  totalCount: number
  items: APIKey[]
}


import request from '@/services/request'
import type {
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
  ListAPIKeysParams,
  ListAPIKeysResponse,
} from './type'

export const createAPIKey = (data: CreateAPIKeyRequest): Promise<CreateAPIKeyResponse> =>
  request.post('/apikeys', data)

export const deleteAPIKey = (id: number): Promise<any> => request.delete(`/apikeys/${id}`)

export const listAPIKeys = (params?: ListAPIKeysParams): Promise<ListAPIKeysResponse> =>
  request.get('/apikeys', { params })

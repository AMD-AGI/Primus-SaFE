import request from '@/services/request'
import type {
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
  ListAPIKeysParams,
  ListAPIKeysResponse,
} from './type'

/**
 * Where the API key routes are called. With an external identity provider (`external`, see
 * isExternalAuth) the provider issues, lists and revokes the keys, and serves them at the public
 * `/api/v1/apikeys`, which is the path a gateway can route to it. `/v1/apikeys` under the request
 * base `/api` is that path; the console's usual `/apikeys` is the apiserver's compatibility alias,
 * which always reaches the apiserver, and the apiserver refuses to issue keys in that mode. With
 * the built-in accounts nothing changes.
 */
export const apiKeysPath = (external: boolean): string => (external ? '/v1/apikeys' : '/apikeys')

export const createAPIKey = (
  data: CreateAPIKeyRequest,
  external = false,
): Promise<CreateAPIKeyResponse> => request.post(apiKeysPath(external), data)

export const deleteAPIKey = (id: number, external = false): Promise<any> =>
  request.delete(`${apiKeysPath(external)}/${id}`)

export const listAPIKeys = (
  params?: ListAPIKeysParams,
  external = false,
): Promise<ListAPIKeysResponse> => request.get(apiKeysPath(external), { params })

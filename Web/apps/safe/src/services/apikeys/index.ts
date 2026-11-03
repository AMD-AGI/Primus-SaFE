import request from '@/services/request'
import type {
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
  ListAPIKeysParams,
  ListAPIKeysResponse,
} from './type'

/**
 * The API key routes. Without `providerUrl` they are the apiserver's `/apikeys`: the keys of this
 * deployment's own accounts and, with an external identity provider, the keys the apiserver issued
 * before the switch (they still authenticate, so they stay listed and revocable).
 *
 * `providerUrl` is the path where the deployment routes the identity provider's keys (GET /envs
 * `apiKeysUrl`, see apiKeysProviderUrl): a path on this console's own origin, so it is called as it
 * is, not under the request base. The apiserver refuses to issue keys in external mode: a key is
 * created at the provider, and the keys the provider issued are listed and revoked there.
 */
const onOrigin = { baseURL: '' }

export const createAPIKey = (
  data: CreateAPIKeyRequest,
  providerUrl = '',
): Promise<CreateAPIKeyResponse> =>
  providerUrl ? request.post(providerUrl, data, onOrigin) : request.post('/apikeys', data)

export const deleteAPIKey = (id: number, providerUrl = ''): Promise<any> =>
  providerUrl ? request.delete(`${providerUrl}/${id}`, onOrigin) : request.delete(`/apikeys/${id}`)

export const listAPIKeys = (
  params?: ListAPIKeysParams,
  providerUrl = '',
): Promise<ListAPIKeysResponse> =>
  providerUrl
    ? request.get(providerUrl, { params, ...onOrigin })
    : request.get('/apikeys', { params })

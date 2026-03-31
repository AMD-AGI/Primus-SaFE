import axios from 'axios'
import type {
  SandboxTemplateListParams,
  SandboxTemplateListResponse,
  SandboxSessionListParams,
  SandboxSessionListResponse,
} from './type'

const isOciCluster = window.location.origin === 'https://oci-slc.primus-safe.amd.com'

const sandboxRequest = axios.create({
  baseURL: isOciCluster ? '/sandbox' : '/x-flannel/sandbox',
  timeout: 15000,
  withCredentials: true,
})

sandboxRequest.interceptors.response.use(
  (response) => response.data,
  (error) => {
    const status = error?.response?.status
    if (status === 401) {
      const from = encodeURIComponent(location.pathname + location.search)
      location.href = `/login?redirect=${from}`
    }
    return Promise.reject(error?.response?.data?.error || error.message || 'Request failed')
  },
)

export const getSandboxTemplates = (
  params?: SandboxTemplateListParams,
): Promise<SandboxTemplateListResponse> => sandboxRequest.get('/v1/templates', { params })

export const getSandboxSessions = (
  params?: SandboxSessionListParams,
): Promise<SandboxSessionListResponse> =>
  sandboxRequest.get('/v1/code-interpreter/sessions', { params })

export * from './type'

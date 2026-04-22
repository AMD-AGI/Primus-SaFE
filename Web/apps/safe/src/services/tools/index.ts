import { clawRequest } from '@/services/request'
import type {
  GetToolsParams,
  GetToolsResponse,
  UpsertToolRequest,
  UpsertResponse,
  UpdateToolRequest,
  ToolDetail,
  SearchToolsParams,
  SearchToolsResponse,
  SkillDiscoverResponse,
  ImportCommitRequest,
  ImportCommitResponse,
  GetPluginsParams,
  GetPluginsResponse,
  Plugin,
  UpsertPluginRequest,
  PluginUpdateRequest,
  GetResourcesParams,
  GetResourcesResponse,
  Resource,
  UpsertResourceRequest,
  ResourceUpdateRequest,
} from './type'

// ─── Tools ───

export const getTools = (params?: GetToolsParams): Promise<GetToolsResponse> =>
  clawRequest.get('/tools', { params })

export const getMCPTools = (params?: GetToolsParams): Promise<GetToolsResponse> =>
  clawRequest.get('/tools/mcp', { params })

export const searchTools = (params: SearchToolsParams): Promise<SearchToolsResponse> =>
  clawRequest.get('/tools/search', { params })

export const getTool = (id: number): Promise<ToolDetail> =>
  clawRequest.get(`/tools/${id}`)

export const upsertTool = (data: UpsertToolRequest): Promise<UpsertResponse> =>
  clawRequest.post('/tools/upsert', data)

export const updateTool = (id: number, data: UpdateToolRequest): Promise<void> =>
  clawRequest.put(`/tools/${id}`, data, { timeout: 3e4 })

export const deleteTool = (id: number): Promise<void> =>
  clawRequest.delete(`/tools/${id}`)

export const getToolContent = (id: number): Promise<string> =>
  clawRequest.get(`/tools/${id}/content`, {
    responseType: 'text',
    rawResponse: false,
  })

export const uploadSkill = (formData: FormData): Promise<{ id: number }> =>
  clawRequest.post('/tools/skill/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 3e4,
  })

export const uploadRule = (formData: FormData): Promise<{ id: number }> =>
  clawRequest.post('/tools/rule/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 3e4,
  })

// ─── Import (zip / GitHub) ───

export const discoverImport = (formData: FormData): Promise<SkillDiscoverResponse> =>
  clawRequest.post('/tools/import/discover', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 3e4,
  })

export const commitImport = (data: ImportCommitRequest): Promise<ImportCommitResponse> =>
  clawRequest.post('/tools/import/commit', data, { timeout: 3e4 })

// ─── Plugins ───

export const getPlugins = (params?: GetPluginsParams): Promise<GetPluginsResponse> =>
  clawRequest.get('/plugins', { params })

export const getPlugin = (id: number): Promise<Plugin> =>
  clawRequest.get(`/plugins/${id}`)

export const upsertPlugin = (data: UpsertPluginRequest): Promise<UpsertResponse> =>
  clawRequest.post('/plugins/upsert', data)

export const updatePlugin = (id: number, data: PluginUpdateRequest): Promise<void> =>
  clawRequest.put(`/plugins/${id}`, data)

export const deletePlugin = (id: number): Promise<void> =>
  clawRequest.delete(`/plugins/${id}`)

// ─── Resources ───

export const getResources = (params?: GetResourcesParams): Promise<GetResourcesResponse> =>
  clawRequest.get('/resources', { params })

export const getResource = (id: number): Promise<Resource> =>
  clawRequest.get(`/resources/${id}`)

export const upsertResource = (data: UpsertResourceRequest): Promise<UpsertResponse> =>
  clawRequest.post('/resources/upsert', data)

export const updateResource = (id: number, data: ResourceUpdateRequest): Promise<void> =>
  clawRequest.put(`/resources/${id}`, data)

export const deleteResource = (id: number): Promise<void> =>
  clawRequest.delete(`/resources/${id}`)

export * from './type'

import request from '@/services/request'
import type {
  GetToolsParams,
  GetToolsResponse,
  CreateMCPRequest,
  UpdateMCPRequest,
  ToolDetail,
  SkillDiscoverResponse,
  SkillCommitRequest,
  SkillCommitResponse,
  SearchToolsParams,
  SearchToolsResponse
} from './type'

// Fetch tool list
export const getTools = (params?: GetToolsParams): Promise<GetToolsResponse> =>
  request.get('/tools/api/v1/tools', { params })

// Search tools
export const searchTools = (params: SearchToolsParams): Promise<SearchToolsResponse> =>
  request.get('/tools/api/v1/tools/search', { params })

// Fetch tool details
export const getTool = (id: number): Promise<ToolDetail> =>
  request.get(`/tools/api/v1/tools/${id}`)

// Create MCP
export const createMCP = (data: CreateMCPRequest): Promise<{ id: number }> =>
  request.post('/tools/api/v1/tools/mcp', data)

// Update MCP
export const updateMCP = (id: number, data: UpdateMCPRequest): Promise<void> =>
  request.put(`/tools/api/v1/tools/${id}`, data, {
    timeout: 3e4
  })

// Skills import - step 1: discover
export const discoverSkills = (formData: FormData): Promise<SkillDiscoverResponse> =>
  request.post('/tools/api/v1/tools/import/discover', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    timeout: 3e4
  })

// Skills import - step 2: confirm
export const commitSkills = (data: SkillCommitRequest): Promise<SkillCommitResponse> => {
  return request.post('/tools/api/v1/tools/import/commit', data, {
    timeout: 3e4
  })
}

// Download tool
export const downloadTool = async (id: number): Promise<void> => {
  const response = await request.get(`/tools/api/v1/tools/${id}/download`, {
    responseType: 'blob',
    rawResponse: true,
  })

  // Get filename from response headers
  const contentDisposition = response.headers['content-disposition']
  let filename = `tool-${id}`

  if (contentDisposition) {
    const filenameMatch = contentDisposition.match(/filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/)
    if (filenameMatch && filenameMatch[1]) {
      filename = filenameMatch[1].replace(/['"]/g, '')
    }
  }

  // Create download link
  const blob = new Blob([response.data])
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  window.URL.revokeObjectURL(url)
}

// Like tool
export const likeTool = (id: number): Promise<void> =>
  request.post(`/tools/api/v1/tools/${id}/like`)

// Unlike tool
export const unlikeTool = (id: number): Promise<void> =>
  request.delete(`/tools/api/v1/tools/${id}/like`)

// Delete tool
export const deleteTool = (id: number): Promise<void> =>
  request.delete(`/tools/api/v1/tools/${id}`)

// Upload icon
export const uploadIcon = async (file: File): Promise<{ icon_url: string }> => {
  const formData = new FormData()
  formData.append('file', file)

  return request.post('/tools/api/v1/tools/icon', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

// Get skill content (only for type = "skill")
export const getSkillContent = (id: number): Promise<string> =>
  request.get(`/tools/api/v1/tools/${id}/content`, {
    responseType: 'text',
    rawResponse: false,
  })

export * from './type'

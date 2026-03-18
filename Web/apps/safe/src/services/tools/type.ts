export interface Tool {
  id: number
  type: 'skill' | 'mcp'
  name: string
  description: string
  tags: string[]
  author: string
  icon_url?: string
  run_count: number
  download_count: number
  like_count?: number
  is_liked?: boolean
  is_public: boolean
  status: 'active' | 'inactive'
  created_at: string
  updated_at: string
}

export interface GetToolsParams {
  type?: 'skill' | 'mcp'
  status?: 'active' | 'inactive'
  sort?: 'created_at' | 'updated_at' | 'run_count' | 'download_count'
  order?: 'desc' | 'asc'
  offset?: number
  limit?: number
}

export interface GetToolsResponse {
  tools: Tool[]
  total: number
  offset: number
  limit: number
  sort: string
  order: string
}

// MCP Create
export interface CreateMCPRequest {
  name: string
  description: string
  config: {
    mcpServers: Record<string, {
      command: string
      args?: string[]
      env?: Record<string, string>
    }>
  }
  display_name?: string
  tags?: string[]
  icon_url?: string
  author?: string
  is_public?: boolean
}

// Skills import - Discover
export interface SkillDiscoverRequest {
  file?: File
  github_url?: string
}

export interface SkillCandidate {
  relative_path: string
  skill_name: string | null
  skill_description: string | null
  requires_name: boolean
  will_overwrite: boolean
  owned_by_other?: boolean
}

export interface SkillDiscoverResponse {
  archive_key: string
  candidates: SkillCandidate[]
}

// Skills import - Commit
export interface SkillSelection {
  relative_path: string
  name_override?: string
}

export interface SkillCommitRequest {
  archive_key: string
  selections: SkillSelection[]
}

export interface SkillCommitItem {
  relative_path: string
  skill_name: string
  status: 'success' | 'failed'
  tool_id?: number
  error?: string
}

export interface SkillCommitResponse {
  items: SkillCommitItem[]
}

// Tool details
export interface ToolDetail extends Tool {
  config?: {
    mcpServers: Record<string, {
      command: string
      args?: string[]
      env?: Record<string, string>
    }>
  }
}

// Update MCP
export interface UpdateMCPRequest {
  name?: string
  description?: string
  config?: ToolDetail['config']
  tags?: string[]
  icon_url?: string
  is_public?: boolean
}

// Search tools
export interface SearchToolsParams {
  q: string
  mode?: 'semantic'
  type?: 'skill' | 'mcp'
  limit?: number
}

export interface SearchToolsResponse {
  tools: Tool[]
  total: number
}

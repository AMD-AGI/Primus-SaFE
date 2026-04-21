// ─── Tool types ───

export type ToolType = 'skill' | 'mcp' | 'hooks' | 'rule'

export interface Tool {
  id: number
  type: ToolType
  name: string
  version: string
  display_name: string
  description: string
  tags: string[]
  author: string
  icon_url?: string | null
  tool_source: 'upload' | 'github'
  tool_source_url?: string | null
  is_public: boolean
  status: 'active' | 'inactive'
  created_at: string
  updated_at: string
}

export interface ToolDetail extends Tool {
  config?: Record<string, any>
}

export interface GetToolsParams {
  type?: ToolType
  status?: 'active' | 'inactive'
  owner?: string
  tag?: string
  name?: string
  name_exact?: string
  latest_per_name?: boolean
  sort?: 'created_at' | 'updated_at'
  order?: 'desc' | 'asc'
  offset?: number
  limit?: number
  include_deleted?: boolean
}

export interface GetToolsResponse {
  tools: Tool[]
  total: number
  offset: number
  limit: number
}

// ─── Tool CRUD ───

export interface UpsertToolRequest {
  name: string
  type?: ToolType
  description?: string
  version?: string
  display_name?: string
  tags?: string[]
  icon_url?: string
  is_public?: boolean
  config?: Record<string, any>
}

export interface UpsertResponse {
  id: number
  upsert_action: 'created' | 'version_created'
}

export interface UpdateToolRequest {
  display_name?: string
  description?: string
  version?: string
  tags?: string[]
  icon_url?: string
  config?: Record<string, any>
  is_public?: boolean
  status?: string
}

// ─── Search ───

export interface SearchToolsParams {
  q: string
  type?: ToolType
  mode?: 'keyword' | 'semantic'
  limit?: number
  offset?: number
}

export interface SearchToolsResponse {
  tools: Tool[]
  total: number
}

// ─── Import (zip / GitHub) ───

export interface HooksScript {
  relative_path: string
  name: string
  description: string
  requires_name: boolean
}

export interface SkillCandidate {
  relative_path?: string
  type?: ToolType
  name: string | null
  description: string | null
  skill_name?: string | null
  skill_description?: string | null
  requires_name: boolean
  will_overwrite: boolean
  owned_by_other?: boolean
  is_forbidden?: boolean
  // hooks-specific
  hooks_json_relative_path?: string
  scripts?: HooksScript[]
}

export interface SkillDiscoverResponse {
  archive_key: string
  candidates: SkillCandidate[]
}

export interface ImportSelection {
  type: ToolType
  relative_path?: string
  hooks_json_relative_path?: string
  name_override?: string
}

export interface ImportCommitRequest {
  archive_key: string
  version?: string
  tags?: string[]
  selections: ImportSelection[]
}

export interface ImportCommitItem {
  type: ToolType
  name: string
  status: 'ok' | 'failed'
  tool_id?: number
  upsert_action?: 'created' | 'version_created'
  relative_path?: string
  relative_paths?: string[]
  error?: string
}

export interface ImportCommitResponse {
  items: ImportCommitItem[]
}

// ─── Plugin types ───

export interface PluginToolRef {
  id: number
  type: ToolType
  version: string
  name?: string
  description?: string
  config?: Record<string, any>
}

export interface PluginResourceRef {
  id: number
  type: string
  version: string
  name?: string
}

export interface Plugin {
  id: number
  name: string
  description: string
  version: string
  author?: string
  tools: PluginToolRef[]
  resources: PluginResourceRef[]
  is_public: boolean
  status: 'active' | 'inactive'
  created_at: string
  updated_at: string
}

export interface UpsertPluginRequest {
  name: string
  description?: string
  version?: string
  tools: { id: number; type: string; version: string }[]
  resources?: { id: number; type: string; version: string }[]
  is_public?: boolean
  status?: string
}

export interface PluginUpdateRequest {
  name?: string
  description?: string
  version?: string
  tools?: { id: number; type: string; version: string }[]
  resources?: { id: number; type: string; version: string }[]
  is_public?: boolean
  status?: string
}

export interface GetPluginsParams {
  status?: string
  owner?: string
  name?: string
  name_exact?: string
  latest_per_name?: boolean
  limit?: number
  offset?: number
  include_deleted?: boolean
}

export interface GetPluginsResponse {
  plugins: Plugin[]
  total: number
  offset: number
  limit: number
}

// ─── Resource types ───

export interface ResourceEnvVar {
  key: string
  val: string
}

export interface Resource {
  id: number
  name: string
  type: 'gpu' | 'cpu'
  image: string
  env: ResourceEnvVar[]
  version: string
  resources: {
    gpu?: string
    cpu?: string
    memory?: string
    ephemeralStorage?: string
  }
  timeout: number
  labels?: Record<string, string>
  annotations?: Record<string, string>
  created_at: string
  updated_at: string
}

export interface UpsertResourceRequest {
  name: string
  type: 'gpu' | 'cpu'
  image: string
  env?: ResourceEnvVar[]
  version?: string
  resources: Resource['resources']
  timeout?: number
  labels?: Record<string, string>
  annotations?: Record<string, string>
}

export interface ResourceUpdateRequest {
  name?: string
  type?: 'gpu' | 'cpu'
  image?: string
  env?: ResourceEnvVar[]
  version?: string
  resources?: Resource['resources']
  timeout?: number
  labels?: Record<string, string>
  annotations?: Record<string, string>
}

export interface GetResourcesParams {
  type?: 'gpu' | 'cpu'
  limit?: number
  offset?: number
  include_deleted?: boolean
}

export interface GetResourcesResponse {
  resources: Resource[]
  total: number
  offset: number
  limit: number
}

// ─── Run ───

export interface RunToolRequest {
  plugin_id?: number
  tool_ids?: number[]
}

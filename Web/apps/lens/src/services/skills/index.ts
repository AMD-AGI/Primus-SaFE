import agentRequest from '@/services/agentRequest'

// Skill type definition
export interface Skill {
  id: number
  name: string
  description: string
  category: string
  version: string
  source: string // platform, team, user
  license: string
  content: string
  file_path: string
  metadata: Record<string, any>
  created_at: string
  updated_at: string
}

// Skill list response
export interface SkillListResponse {
  skills: Skill[]
  total: number
  offset: number
  limit: number
}

// Search request
export interface SkillSearchRequest {
  query: string
  limit?: number
}

// Search result
export interface SkillSearchResult {
  name: string
  description: string
  category: string
  relevance_score: number
}

export interface SkillSearchResponse {
  skills: SkillSearchResult[]
  total: number
  hint: string
}

// Create/Update skill request
export interface CreateSkillRequest {
  name: string
  description: string
  category?: string
  version?: string
  source?: string
  license?: string
  content?: string
  metadata?: Record<string, any>
}

export interface UpdateSkillRequest {
  name?: string
  description?: string
  category?: string
  version?: string
  license?: string
  content?: string
  metadata?: Record<string, any>
}

// API functions

// List all skills with pagination
export function listSkills(params?: { offset?: number; limit?: number; category?: string; source?: string }) {
  return agentRequest.get<any, SkillListResponse>('/skills', { params })
}

// Get a skill by name
export function getSkill(name: string) {
  return agentRequest.get<any, Skill>(`/skills/${name}`)
}

// Get skill content (SKILL.md)
export function getSkillContent(name: string) {
  return agentRequest.get<any, string>(`/skills/${name}/content`, {
    headers: {
      'Accept': 'text/markdown'
    },
    transformResponse: [(data) => data] // Return raw text
  })
}

// Create a new skill
export function createSkill(data: CreateSkillRequest) {
  return agentRequest.post<any, Skill>('/skills', data)
}

// Update a skill
export function updateSkill(name: string, data: UpdateSkillRequest) {
  return agentRequest.put<any, Skill>(`/skills/${name}`, data)
}

// Delete a skill
export function deleteSkill(name: string) {
  return agentRequest.delete<any, { success: boolean; message: string }>(`/skills/${name}`)
}

// Semantic search for skills
export function searchSkills(data: SkillSearchRequest) {
  return agentRequest.post<any, SkillSearchResponse>('/skills/search', data)
}

// Health check
export function healthCheck() {
  return agentRequest.get<any, { status: string }>('/skills/health')
}

// Import from GitHub
export interface ImportGitHubRequest {
  url: string
  github_token?: string
}

export interface ImportResult {
  message: string
  imported: string[]
  skipped: string[]
  errors: string[]
}

export function importFromGitHub(data: ImportGitHubRequest) {
  return agentRequest.post<any, ImportResult>('/skills/import/github', data)
}

// Import from file upload
export function importFromFile(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return agentRequest.post<any, ImportResult>('/skills/import/file', formData, {
    headers: {
      'Content-Type': 'multipart/form-data'
    }
  })
}

// Skill categories
export const SKILL_CATEGORIES = [
  { value: 'k8s', label: 'Kubernetes' },
  { value: 'database', label: 'Database' },
  { value: 'cloud', label: 'Cloud' },
  { value: 'devops', label: 'DevOps' },
  { value: 'security', label: 'Security' },
  { value: 'monitoring', label: 'Monitoring' },
  { value: 'networking', label: 'Networking' },
  { value: 'ai', label: 'AI/ML' },
  { value: 'other', label: 'Other' }
]

// Skill sources
export const SKILL_SOURCES = [
  { value: 'platform', label: 'Platform' },
  { value: 'team', label: 'Team' },
  { value: 'user', label: 'User' }
]

// ======================== Skillset Types ========================

export interface Skillset {
  id: number
  name: string
  description: string
  owner: string
  is_default: boolean
  metadata: Record<string, any>
  created_at: string
  updated_at: string
}

export interface SkillsetListResponse {
  skillsets: Skillset[]
  total: number
  offset: number
  limit: number
}

export interface CreateSkillsetRequest {
  name: string
  description?: string
  owner?: string
  is_default?: boolean
  metadata?: Record<string, any>
}

export interface UpdateSkillsetRequest {
  description?: string
  owner?: string
  is_default?: boolean
  metadata?: Record<string, any>
}

export interface SkillsetSkillsRequest {
  skills: string[]
}

export interface SkillsetSkillsListResponse {
  skills: Skill[]
  total: number
  offset: number
  limit: number
}

export interface SkillsetSearchRequest {
  query: string
  limit?: number
}

export interface SkillsetSearchResponse {
  skills: SkillSearchResult[]
  total: number
  skillset: string
  hint: string
}

// ======================== Skillset API Functions ========================

// List all skillsets
export function listSkillsets(params?: { offset?: number; limit?: number; owner?: string }) {
  return agentRequest.get<any, SkillsetListResponse>('/skillsets', { params })
}

// Get a skillset by name
export function getSkillset(name: string) {
  return agentRequest.get<any, Skillset>(`/skillsets/${name}`)
}

// Create a new skillset
export function createSkillset(data: CreateSkillsetRequest) {
  return agentRequest.post<any, Skillset>('/skillsets', data)
}

// Update a skillset
export function updateSkillset(name: string, data: UpdateSkillsetRequest) {
  return agentRequest.put<any, Skillset>(`/skillsets/${name}`, data)
}

// Delete a skillset
export function deleteSkillset(name: string) {
  return agentRequest.delete<any, { message: string }>(`/skillsets/${name}`)
}

// List skills in a skillset
export function listSkillsetSkills(skillsetName: string, params?: { offset?: number; limit?: number }) {
  return agentRequest.get<any, SkillsetSkillsListResponse>(`/skillsets/${skillsetName}/skills`, { params })
}

// Add skills to a skillset
export function addSkillsToSkillset(skillsetName: string, data: SkillsetSkillsRequest) {
  return agentRequest.post<any, { message: string }>(`/skillsets/${skillsetName}/skills`, data)
}

// Remove skills from a skillset
export function removeSkillsFromSkillset(skillsetName: string, data: SkillsetSkillsRequest) {
  return agentRequest.delete<any, { message: string }>(`/skillsets/${skillsetName}/skills`, { data })
}

// Search skills in a skillset
export function searchSkillsInSkillset(skillsetName: string, data: SkillsetSearchRequest) {
  return agentRequest.post<any, SkillsetSearchResponse>(`/skillsets/${skillsetName}/skills/search`, data)
}

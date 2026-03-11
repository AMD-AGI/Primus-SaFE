export interface SandboxTemplateMetadata {
  name: string
  namespace: string
  creationTimestamp: string
  labels?: Record<string, string>
  annotations?: Record<string, string>
}

export interface SandboxTemplateResources {
  limits?: Record<string, string>
  requests?: Record<string, string>
}

export interface SandboxTemplateSpec {
  template: {
    fromImage: string
    resources?: SandboxTemplateResources
  }
  warmPoolSize?: number
  sessionTimeout?: string
  maxSessionDuration?: string
  authMode?: string
  gpu?: {
    product: string
    count: number
  }
}

export interface SandboxTemplate {
  metadata: SandboxTemplateMetadata
  spec: SandboxTemplateSpec
  status: {
    ready: boolean
  }
}

export interface SandboxTemplateListResponse {
  totalCount: number
  items: SandboxTemplate[]
}

export interface SandboxSession {
  sessionId: string
  sandboxName: string
  namespace: string
  status: string
  podIp: string
  entryPoints: Record<string, string>
  createdAt: string
  lastActivity: string
  expiresAt: string
  userId: string
  userName: string
}

export interface SandboxSessionListResponse {
  totalCount: number
  items: SandboxSession[]
}

export interface SandboxTemplateListParams {
  namespace?: string
  userId?: string
  userName?: string
  name?: string
  offset?: number
  limit?: number
  sortBy?: string
  order?: string
}

export interface SandboxSessionListParams {
  userId?: string
  userName?: string
  namespace?: string
  status?: string
  sessionId?: string
  sandboxName?: string
  offset?: number
  limit?: number
  sortBy?: string
  order?: string
}

import request from '@/services/request'

/**
 * System configuration item
 */
export interface SystemConfig {
  id: number
  key: string
  value: any
  description: string
  category: string
  isEncrypted: boolean
  version: number
  isReadonly: boolean
  createdAt: string
  updatedAt: string
  createdBy: string
  updatedBy: string
}

/**
 * Registry configuration
 * Note: field names use camelCase since axios interceptor converts snake_case to camelCase
 */
export interface RegistryConfig {
  registry: string
  namespace: string
  harborExternalUrl?: string
  imageVersions?: Record<string, string>
}

/**
 * List all system configs
 */
export function listSystemConfigs(cluster: string, category?: string): Promise<SystemConfig[]> {
  const params: Record<string, string> = { cluster }
  if (category) {
    params.category = category
  }
  return request.get<any, SystemConfig[]>('/system-config', { params })
}

/**
 * Get a specific system config
 */
export function getSystemConfig(key: string, cluster: string): Promise<SystemConfig> {
  return request.get<any, SystemConfig>(`/system-config/${key}`, {
    params: { cluster }
  })
}

/**
 * Set a system config
 */
export function setSystemConfig(
  key: string,
  value: any,
  cluster: string,
  options?: {
    description?: string
    category?: string
  }
): Promise<void> {
  return request.put(`/system-config/${key}`, {
    value,
    ...options
  }, {
    params: { cluster }
  })
}

/**
 * Delete a system config
 */
export function deleteSystemConfig(key: string, cluster: string): Promise<void> {
  return request.delete(`/system-config/${key}`, {
    params: { cluster }
  })
}

// ============ Registry Config ============

/**
 * Get registry configuration
 * Note: response field names use camelCase after axios interceptor conversion
 */
export function getRegistryConfig(cluster: string): Promise<{
  config: RegistryConfig
  defaults: { registry: string; namespace: string }
  imageNames: Record<string, string>
}> {
  return request.get('/registry/config', {
    params: { cluster }
  })
}

/**
 * Set registry configuration
 */
export function setRegistryConfig(config: RegistryConfig, cluster: string): Promise<{
  message: string
  config: RegistryConfig
}> {
  return request.put('/registry/config', config, {
    params: { cluster }
  })
}

/**
 * Sync registry config from Harbor URL
 */
export function syncFromHarbor(harborExternalUrl: string, cluster: string): Promise<{
  message: string
  config: RegistryConfig
}> {
  return request.post('/registry/sync-from-harbor', {
    harbor_external_url: harborExternalUrl
  }, {
    params: { cluster }
  })
}

/**
 * Get resolved image URL
 */
export function getImageUrl(imageName: string, tag: string, cluster: string): Promise<{
  image_name: string
  tag: string
  image_url: string
}> {
  return request.get('/registry/image-url', {
    params: { cluster, image: imageName, tag }
  })
}

// ============ Common Config Categories ============

export const CONFIG_CATEGORIES = [
  { value: '', label: 'All Categories' },
  { value: 'infrastructure', label: 'Infrastructure' },
  { value: 'detection', label: 'Detection' },
  { value: 'notification', label: 'Notification' },
  { value: 'integration', label: 'Integration' },
  { value: 'general', label: 'General' }
]

// ============ Known Config Keys ============

export const KNOWN_CONFIGS = {
  CONTAINER_REGISTRY: 'container_registry',
  // Add more known config keys here
}


import request from '@/services/request'
import type { SftConfigResponse, CreateSftJobRequest, CreateSftJobResponse } from './types'

export * from './types'

export function getSftConfig(modelId: string, workspace: string) {
  return request.get<SftConfigResponse>(`/playground/models/${modelId}/sft-config`, {
    params: { workspace },
  })
}

export function createSftJob(data: CreateSftJobRequest) {
  return request.post<CreateSftJobResponse>('/sft/jobs', data, { timeout: 60000 })
}

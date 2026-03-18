import request from '@/services/request'
import type {
  LLMGatewayBinding,
  LLMGatewayBindingRequest,
  LLMGatewayUsage,
  LLMGatewayUsageParams,
  LLMGatewaySummary,
} from './type'

export const getLLMGatewayBinding = (): Promise<LLMGatewayBinding> =>
  request.get('/llm-gateway/binding')

export const createLLMGatewayBinding = (
  data: LLMGatewayBindingRequest,
): Promise<LLMGatewayBinding> => request.post('/llm-gateway/binding', data)

export const updateLLMGatewayBinding = (
  data: LLMGatewayBindingRequest,
): Promise<LLMGatewayBinding> => request.put('/llm-gateway/binding', data)

export const getLLMGatewayUsage = (params: LLMGatewayUsageParams): Promise<LLMGatewayUsage> =>
  request.get('/llm-gateway/usage', { params })

export const getLLMGatewaySummary = (): Promise<LLMGatewaySummary> =>
  request.get('/llm-gateway/summary')

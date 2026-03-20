export interface LLMGatewayBinding {
  user_email: string
  key_alias?: string
  has_apim_key: boolean
  apim_key_hint?: string
  virtual_key?: string
  created_at?: string
  updated_at?: string
}

export interface LLMGatewayBindingRequest {
  apim_key: string
}

export interface LLMGatewayModelUsage {
  spend: number
  prompt_tokens: number
  completion_tokens: number
  api_requests: number
  successful_requests: number
  failed_requests: number
}

export interface LLMGatewayDailyUsage {
  date: string
  spend: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  api_requests: number
  successful_requests: number
  failed_requests: number
  models: Record<string, LLMGatewayModelUsage>
}

export interface LLMGatewayUsage {
  user_email: string
  total_spend: number
  total_prompt_tokens: number
  total_completion_tokens: number
  total_tokens: number
  total_api_requests: number
  total_successful_requests: number
  total_failed_requests: number
  daily: LLMGatewayDailyUsage[]
}

export interface LLMGatewayUsageParams {
  start_date: string
  end_date: string
  timezone?: string
}

export interface LLMGatewaySummary {
  user_email: string
  total_spend: number
  model_spend: Record<string, number>
}

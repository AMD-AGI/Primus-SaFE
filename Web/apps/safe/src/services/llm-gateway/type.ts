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
}

export interface LLMGatewaySummary {
  user_email: string
  total_spend: number
  model_spend: Record<string, number>
}

export interface LLMGatewayTagUsageParams {
  start_date: string
  end_date: string
  timezone?: string
  page?: number
  page_size?: number
  tag?: string
}

export interface LLMGatewayBudget {
  user_email: string
  spend: number
  max_budget: number | null
  remaining: number | null
  budget_exceeded: boolean
  usage_percent: number | null
  message?: string
}

export interface LLMGatewayBudgetRequest {
  max_budget: number
}

export interface LLMGatewayTagItem {
  tag_name: string | null
  spend: number
  api_requests: number
  successful_requests: number
  failed_requests: number
  prompt_tokens: number
  completion_tokens: number
}

export interface LLMGatewayTagDailySpend {
  date: string
  spend: number
}

export interface LLMGatewayTagUsage {
  user_email: string
  start_date: string
  end_date: string
  total_spend: number
  total_requests: number
  total_successful_requests: number
  total_failed_requests: number
  total_tokens: number
  daily: LLMGatewayTagDailySpend[]
  tags: LLMGatewayTagItem[]
  page: number
  page_size: number
  total: number
  total_pages: number
}

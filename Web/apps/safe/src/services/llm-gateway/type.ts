export interface LLMGatewayBinding {
  user_email: string
  key_alias?: string
  has_apim_key: boolean
  created_at?: string
  updated_at?: string
}

export interface LLMGatewayBindingRequest {
  apim_key: string
}

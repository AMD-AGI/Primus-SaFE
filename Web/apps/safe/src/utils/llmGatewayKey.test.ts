import { describe, expect, it } from 'vitest'
import { isInvalidLLMGatewayBindingKey } from './llmGatewayKey'

describe('llmGatewayKey', () => {
  it('rejects SaFE-style keys when binding an AMD LLM API key', () => {
    expect(isInvalidLLMGatewayBindingKey('ak-123')).toBe(true)
    expect(isInvalidLLMGatewayBindingKey('SK_123')).toBe(true)
    expect(isInvalidLLMGatewayBindingKey('  sk_live_value  ')).toBe(true)
  })

  it('allows non-SaFE-style LLM keys to continue to backend validation', () => {
    expect(isInvalidLLMGatewayBindingKey('llm-123')).toBe(false)
    expect(isInvalidLLMGatewayBindingKey('')).toBe(false)
  })
})

export function isInvalidLLMGatewayBindingKey(value: string): boolean {
  const key = value.trim().toLowerCase()
  return key.startsWith('ak') || key.startsWith('sk')
}

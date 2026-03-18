export interface InputsNV {
  name: string
  value: string
}

export interface SubmitOpsjobsRequest {
  name: string
  inputs?: InputsNV[]
  type: string
  timeoutSecond?: number
  excludedNodes?: string[]
  env?: Record<string, string>
  isTolerateAll?: boolean
  entryPoint?: string
  image?: string
  resource?: {
    cpu: string
    gpu: string
    memory: string
    ephemeralStorage: string
  }
}

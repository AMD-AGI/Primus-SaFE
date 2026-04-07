import request from '@/services/request'

export interface PlaygroundMessage {
  role: 'user' | 'assistant' | 'system'
  content: string
  timestamp?: string
}

export interface ChatRequest {
  serviceId: string
  messages: PlaygroundMessage[]
  modelName?: string
  stream?: boolean
  temperature?: number
  topP?: number
  topK?: number
  maxTokens?: number
  frequencyPenalty?: number
  presencePenalty?: number
  n?: number
}

export interface ChatResponseChoice {
  index: number
  message: PlaygroundMessage
}

export interface ChatResponse {
  // Adjust based on actual response
  choices: ChatResponseChoice[]
}

export interface SessionUpsertReq {
  id?: number // Omit or 0 to create new
  modelName: string
  displayName: string
  systemPrompt?: string
  messages: PlaygroundMessage[]
}

export interface SessionListItem {
  id: number
  modelName: string
  displayName: string
  systemPrompt?: string
  creationTime: string
  updateTime: string
  // Ignore messages if it's a string
}

export interface SessionListResp {
  total: number
  items: SessionListItem[]
}

export interface SessionDetailResp {
  id: number
  modelName: string
  displayName: string
  systemPrompt?: string
  messages: PlaygroundMessage[]
  createdAt: string
  updatedAt: string
}

export function playgroundChat(data: ChatRequest) {
  return request.post<ChatResponse>(`/playground/chat`, data, { timeout: 60000 })
}

// Stream chat - use the same endpoint, just pass stream: true
export async function playgroundChatStream(
  data: ChatRequest,
  onMessage: (content: string) => void,
  onError?: (error: unknown) => void,
  onFinish?: () => void,
  signal?: AbortSignal,
) {
  try {
    const response = await fetch(`${import.meta.env.VITE_API_BASE_URL || '/api'}/playground/chat`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(localStorage.getItem('token')
          ? { Authorization: `Bearer ${localStorage.getItem('token')}` }
          : {}),
      },
      body: JSON.stringify({ ...data, stream: true }),
      signal,
    })

    if (!response.ok) {
      // Try to read error response body
      const errorText = await response.text()
      let errorMessage = `HTTP error! status: ${response.status}`

      try {
        const errorData = JSON.parse(errorText)
        // Handle Kubernetes Status object format
        if (errorData.message) {
          errorMessage = errorData.message
        } else if (errorData.error) {
          errorMessage = errorData.error
        }
      } catch {
        // If not JSON, use raw text
        if (errorText) {
          errorMessage = errorText
        }
      }

      throw new Error(errorMessage)
    }

    const reader = response.body?.getReader()
    const decoder = new TextDecoder()

    if (!reader) {
      throw new Error('Failed to get response stream')
    }

    let buffer = ''
    let fullResponse = '' // Accumulate all response content for error detection
    let hasValidSSE = false // Flag whether valid SSE data was received

    while (true) {
      const { done, value } = await reader.read()
      if (done) {
        // If no valid SSE data received, check if it's an error response
        if (!hasValidSSE && fullResponse.trim()) {
          // Try to parse as JSON error
          try {
            const errorData = JSON.parse(fullResponse)
            if (errorData.status === 'Failure' || errorData.message || errorData.error) {
              const errorMsg = errorData.message || errorData.error || JSON.stringify(errorData)
              throw new Error(errorMsg)
            }
          } catch (e) {
            // If not JSON, check for error keywords
            if (
              fullResponse.includes('failed') ||
              fullResponse.includes('error') ||
              fullResponse.includes('Error')
            ) {
              // Extract first line as error message
              const firstLine = fullResponse.split('\n')[0]
              throw new Error(firstLine || fullResponse)
            }
          }
        }
        onFinish?.()
        break
      }

      const chunk = decoder.decode(value, { stream: true })
      fullResponse += chunk
      buffer += chunk
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        const trimmedLine = line.trim()
        if (!trimmedLine) continue

        // Skip event: lines (SSE event type)
        if (trimmedLine.startsWith('event:')) continue

        // Handle SSE data lines
        if (trimmedLine.startsWith('data:')) {
          const jsonData = trimmedLine.slice(5).trim() // Remove 'data:' prefix

          // Check for end signal
          if (jsonData === '[DONE]') {
            onFinish?.()
            return
          }

          try {
            const parsed = JSON.parse(jsonData)
            hasValidSSE = true // Mark valid data received

            const content =
              parsed.choices?.[0]?.delta?.content || parsed.choices?.[0]?.message?.content || ''
            if (content) {
              onMessage(content)
            }
            // Check if stream is finished
            if (parsed.choices?.[0]?.finish_reason) {
              onFinish?.()
              return
            }
          } catch (e) {
            console.error('Failed to parse SSE data:', e, jsonData)
          }
        }
      }
    }
  } catch (error) {
    // Don't log AbortError as it's intentional
    if (error instanceof Error && error.name === 'AbortError') {
      return
    }
    console.error('Stream error:', error)
    onError?.(error)
  }
}

export function upsertSession(data: SessionUpsertReq) {
  return request.post<{ id: number }>(`/playground/sessions`, data)
}

export function listSessions(params: { limit: number; offset: number; modelName: string }) {
  return request.get<SessionListResp>(`/playground/sessions`, { params })
}

export function getSession(id: number) {
  return request.get<SessionDetailResp>(`/playground/sessions/${id}`)
}

export function deleteSession(id: number) {
  return request.delete<void>(`/playground/sessions/${id}`)
}

// Model related APIs
export interface PlaygroundModel {
  id: string
  displayName: string
  description?: string
  accessMode?: 'remote_api' | 'local' | 'local_path' | 'cloud'
  phase: 'Ready' | 'Pending' | 'Failed' | 'Running' | 'Stopped'
  message?: string
  icon?: string
  tags?: string // Comma-separated string
  categorizedTags?: Array<{
    value: string
    color: string
  }>

  // Resource configuration
  cpu?: string
  gpu?: string
  memory?: string

  // Inference related
  serviceID?: string
  inferencePhase?: string
  workloadID?: string

  // Source related
  sourceURL?: string
  sourceToken?: string
  downloadType?: string
  localPath?: string
  s3Config?: string

  // Time related
  createdAt?: string
  updatedAt?: string
  deletionTime?: {
    Time: string
    Valid: boolean
  }

  // Others
  label?: string
  version?: string
  isDeleted?: boolean

  // Fine-tuning metadata
  origin?: 'external' | 'fine_tuned' | 'rl_trained'
  sftJobId?: string
  baseModel?: string
  userId?: string
  userName?: string
  workspace?: string
}

/**
 * Whether the model is a deployable local model (supports both imported and SFT-produced).
 */
export function isDeployableLocalModel(model: PlaygroundModel): boolean {
  return model.accessMode === 'local' || model.accessMode === 'local_path'
}

/**
 * Whether the model supports SFT (only HuggingFace-imported base models).
 */
export function canSft(model: PlaygroundModel): boolean {
  return model.accessMode === 'local' && model.phase === 'Ready'
}

/**
 * Whether the model supports training (SFT + RL).
 */
export function canTrain(model: PlaygroundModel): boolean {
  return (model.accessMode === 'local' || model.accessMode === 'local_path') && model.phase === 'Ready'
}

export interface ModelsListParams {
  inferenceStatus?: string
  accessMode?: string
  origin?: string
  workspace?: string
}

export interface ModelsListResp {
  total: number
  items: PlaygroundModel[]
}

export function getModelsList(params?: ModelsListParams) {
  return request.get<ModelsListResp>(`/playground/models`, { params })
}

export function getModelDetail(id: string) {
  return request.get<PlaygroundModel>(`/playground/models/${id}`)
}

export function createModel(data: Partial<PlaygroundModel>) {
  return request.post<PlaygroundModel>(`/playground/models`, data)
}

export function deleteModel(id: string) {
  return request.delete<void>(`/playground/models/${id}`)
}

export interface ToggleModelParams {
  enabled: boolean
  resource?: {
    workspace: string
    replica: number
    cpu: number | string
    memory: number | string
    gpu: string
  }
  config?:
    | {
        image: string
        entryPoint: string
      }
    | {
        apiKey: string
        model: string
      }
}

export function toggleModel(id: string, params: ToggleModelParams) {
  return request.post<void>(`/playground/models/${id}/toggle`, params)
}

export function retryModel(id: string) {
  return request.post<void>(`/playground/models/${id}/retry`, {}, { timeout: 60000 })
}

export interface WorkloadConfig {
  displayName: string
  description: string
  labels?: Record<string, string>
  env?: Record<string, string>
  modelId: string
  modelName: string
  modelPath: string
  accessMode: string
  maxTokens: number
  image: string
  entryPoint: string
  workspace: string
  cpu: string
  memory: string
  gpu: string
}

export function getModelWorkloadConfig(id: string, workspaceId: string) {
  return request.get<WorkloadConfig>(`/playground/models/${id}/workload-config`, {
    params: { workspace: workspaceId },
  })
}

export interface ModelWorkload {
  workloadId: string
  displayName: string
  workspace: string
  phase?: string
  createdat?: string
  [key: string]: unknown
}

export interface ModelWorkloadsResp {
  total: number
  items: ModelWorkload[]
}

export function getModelWorkloads(id: string) {
  return request.get<ModelWorkloadsResp>(`/playground/models/${id}/workloads`)
}

export interface PlaygroundService {
  type: 'remote_api' | 'local'
  id: string
  displayName: string
  modelName: string
  phase: string
  workspace: string
}

export interface PlaygroundServicesResp {
  items: PlaygroundService[]
}

export interface PlaygroundServicesParams {
  workspace?: string
}

export function getPlaygroundServices(params?: PlaygroundServicesParams) {
  return request.get<PlaygroundServicesResp>(`/playground/services`, { params })
}

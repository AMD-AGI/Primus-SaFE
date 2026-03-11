import agentRequest from '@/services/agentRequest'
import safeRequest from '@/services/safe-api'

// Type definitions
export interface ChatMessage {
  role: 'user' | 'assistant'
  content: string
  timestamp?: string
  messageId?: number  // For feedback system
  voteType?: 'up' | 'down' | null  // Current vote state
  feedbackId?: number  // Feedback record ID
  showFeedbackForm?: boolean  // Show feedback form for downvote
  selectedReasons?: string[]  // Selected feedback reasons
  customReason?: string  // Custom feedback reason input
}

export interface ChatRequest {
  query: string
  conversationHistory?: ChatMessage[]
  clusterName?: string
  sessionId?: string
  saveHistory?: boolean
}

export interface ChatResponse {
  answer: string
  insights: string[]
  dataCollected: any[]
  conversationHistory: ChatMessage[]
  debugInfo?: any
  timestamp: string
}

export interface AgentCapability {
  type: string
  name: string
  description: string
  examples: string[]
}

export interface AgentCapabilitiesResponse {
  capabilities: AgentCapability[]
  supportedDimensions: string[]
  supportedMetrics: string[]
}

// Conversation list item (simplified version)
export interface ConversationListItem {
  session_id: string
  name: string  // conversation title (from first user query)
  created_at: string
  metadata: {
    cluster_name?: string
    timestamp: string
    status?: string
  }
}

// Message type (may contain extra data)
export interface ConversationMessage extends ChatMessage {
  data?: any
  insights?: string[]
}

// Full conversation detail
export interface ConversationDetail {
  session_id: string
  name: string
  messages: ConversationMessage[]
  created_at: string
  updated_at: string
  metadata: {
    cluster_name?: string
    timestamp?: string
    status?: string
  }
}

export interface ConversationListResponse {
  conversations: ConversationListItem[]
  limit: number
  offset: number
  count: number
}

export interface SearchConversationResponse {
  results: ConversationListItem[]
  query: string
  count: number
}

export interface StorageStatsResponse {
  enabled: boolean
  backend?: string
  retentionDays?: number
  stats?: {
    totalConversations: number
    totalSize: number
  }
}

// Chat with GPU analysis agent
export function chat(data: ChatRequest) {
  return agentRequest.post<any, ChatResponse>('/agent/chat', data)
}

// Standardized step info from streaming protocol
export interface StepInfo {
  id: string
  name: string
  description?: string
  index: number
  total: number
  progress?: number
}

// Standardized content info from streaming protocol
export interface ContentInfo {
  type?: 'text' | 'json' | 'markdown' | 'html'
  delta?: string
  complete?: string
  accumulated?: boolean
}

// Standardized error info from streaming protocol
export interface ErrorInfo {
  code?: string
  message: string
  details?: Record<string, any>
}

// SSE Event type
export interface SSEEvent {
  type: 'session' | 'start' | 'routing' | 'routing_complete' | 'clarification' |
        'crew_execution' | 'progress' | 'agent_thinking' | 'tool_execution' |
        'final' | 'complete' | 'error' | 'done' |
        // New standardized event types
        'crew_start' | 'crew_complete' |
        'step_start' | 'step_progress' | 'step_complete' | 'step_error' |
        'content_start' | 'content_delta' | 'content_complete' |
        'data' | 'metadata' |
        'approval_request' | 'approval_resolved' |
        'tool_start' | 'tool_result'

  // Legacy fields
  session_id?: string
  query?: string
  crew?: string
  message?: string
  answer?: string
  result?: any
  data?: any
  routing_decision?: any
  debug_info?: any
  error_type?: string
  error_message?: string
  traceback?: string
  agent?: string  // currently working agent
  tool?: string   // currently used tool
  tool_input?: any  // tool input
  tool_output?: any // tool output

  // New standardized fields
  step?: StepInfo
  content?: ContentInfo
  error?: ErrorInfo
  metadata?: Record<string, any>
  timestamp?: string
  sequence?: number
}

// Stream chat (SSE)
export async function chatStream(
  data: ChatRequest,
  onEvent: (event: SSEEvent) => void,
  onError?: (error: Error) => void,
  onComplete?: () => void,
  signal?: AbortSignal,
  onResponse?: (response: Response) => void
) {
  const baseURL = `${import.meta.env.BASE_URL}`
  const url = `${baseURL}v1/agent/chat/stream`

  let reader: ReadableStreamDefaultReader<Uint8Array> | null = null

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        query: data.query,
        conversation_history: data.conversationHistory,
        cluster_name: data.clusterName,
        session_id: data.sessionId,
        save_history: data.saveHistory ?? true
      }),
      signal // Add AbortSignal support
    })

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    // Callback response to caller for extracting headers (e.g. X-Run-Id)
    if (onResponse) {
      onResponse(response)
    }

    const streamReader = response.body?.getReader()
    if (!streamReader) {
      throw new Error('Failed to get response stream')
    }

    reader = streamReader
    const decoder = new TextDecoder()

    let buffer = ''

    while (true) {
      // Check if aborted
      if (signal?.aborted) {
        await reader.cancel()
        break
      }

      const { done, value } = await reader.read()

      if (done) {
        break
      }

      buffer += decoder.decode(value, { stream: true })

      // Process complete SSE messages
      const lines = buffer.split('\n')
      buffer = lines.pop() || '' // Keep incomplete line

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          try {
            const data = JSON.parse(line.substring(6))
            onEvent(data)

            // Exit early if error or done
            if (data.type === 'error' || data.type === 'done') {
              if (data.type === 'error' && onError) {
                onError(new Error(data.error_message || 'Unknown error'))
              }
              break
            }
          } catch (e) {
            console.error('Failed to parse SSE data:', line, e)
          }
        }
      }
    }

    if (onComplete) {
      onComplete()
    }
  } catch (error) {
    // If abort error, no need to report
    if (error instanceof Error && error.name === 'AbortError') {
      if (reader) {
        try {
          await reader.cancel()
        } catch {
          // Ignore cancel errors
        }
      }
      return
    }

    console.error('SSE connection error:', error)
    if (onError) {
      onError(error as Error)
    }
    throw error
  }
}

// Get agent capabilities
export function getCapabilities() {
  return agentRequest.get<any, AgentCapabilitiesResponse>('/agent/capabilities')
}

// Health check for agent service
export function healthCheck() {
  return agentRequest.get<any, any>('/agent/health')
}

// ===== Conversation History Management =====

// Get conversation list
export function listConversations(params?: { limit?: number; offset?: number }) {
  return agentRequest.get<any, ConversationListResponse>('/agent/storage/conversations', { params })
}

// Get conversation detail by ID
export function getConversation(sessionId: string) {
  return agentRequest.get<any, ConversationDetail>(`/agent/storage/conversations/${sessionId}`)
}

// Delete conversation
export function deleteConversation(sessionId: string) {
  return agentRequest.delete<any, { success: boolean; message: string }>(`/agent/storage/conversations/${sessionId}`)
}

// Search conversations
export function searchConversations(params: { query: string; limit?: number }) {
  return agentRequest.get<any, SearchConversationResponse>('/agent/storage/search', { params })
}

// Get storage statistics
export function getStorageStats() {
  return agentRequest.get<any, StorageStatsResponse>('/agent/storage/stats')
}

// Clean up old conversations
export function cleanupOldConversations(days?: number) {
  return agentRequest.post<any, { success: boolean; removedCount: number; retentionDays: number }>('/agent/storage/cleanup', { days })
}

// ===== Answer Feedback Management =====

// Submit feedback (upvote/downvote)
export interface SubmitFeedbackRequest {
  vote_type: 'up' | 'down'
  message_id: number
  reason?: string
}

export interface SubmitFeedbackResponse {
  success: boolean
  message: string
  data: {
    id: number
    user_id: string
    user_name: string
    vote_type: 'up' | 'down'
    message_id: number
    status: 'pending' | 'resolved' | 'ignored'
    created_at: string
  }
}

export function submitFeedback(data: SubmitFeedbackRequest) {
  return safeRequest.post<any, SubmitFeedbackResponse>('/answer-feedback', data)
}

// Cancel vote
export interface CancelVoteRequest {
  message_id: number
}

export function cancelVote(data: CancelVoteRequest) {
  return safeRequest.post<any, { success: boolean; message: string }>('/answer-feedback/cancel', data)
}

// ===== Approval API =====

export function resolveApproval(requestId: string, decision: 'approved' | 'rejected' | 'modified', modifiedArgs?: Record<string, any>) {
  return agentRequest.post(`/agent/approval/${requestId}`, {
    decision,
    modified_args: modifiedArgs || null
  })
}

// ===== Abort API =====

export function abortRun(runId: string) {
  return agentRequest.post(`/agent/runs/${runId}/abort`)
}

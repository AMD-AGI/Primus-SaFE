// ========== PrimusClaw API Types ==========

// Session
export interface PocoSession {
  session_id: string
  name?: string
  title?: string // backward compat alias
  agent_id?: string
  system_prompt?: string
  config?: Record<string, unknown>
  status?: string
  created_at: string
  updated_at: string
}

export interface CreateSessionRequest {
  name?: string
  agent_id?: string
  system_prompt?: string
  config?: Record<string, unknown>
}

// Send Message (POST /v1/sessions/{session_id}/messages)
export interface SendMessageRequest {
  id?: string
  timestamp?: number
  content?: string
  contents?: Array<{ type: string; value: string }>
  messageType?: string
  taskMode?: string
  attachments?: unknown[]
  tools?: number[]
  extData?: Record<string, unknown>
}

// Skills (CRUD, no install concept)
export interface PocoSkill {
  name: string
  description?: string
  parameters?: Record<string, unknown>
  created_at?: string
  updated_at?: string
}

// Chat request
export interface PocoChatRequest {
  query: string
  session_id: string
  tools?: number[]
}

// ========== Display Types (unchanged for UI) ==========

export interface ToolCallDetail {
  toolUseId: string
  name: string
  tool?: string
  status?: string
  input?: Record<string, unknown>
  output?: string
  brief?: string
  description?: string
  isError?: boolean
  expanded?: boolean
}

export interface DisplaySegment {
  type: 'text' | 'tool-execution'
  text?: string
  toolCount?: number
  toolCalls?: ToolCallDetail[]
  expanded?: boolean
}

export interface PocoChatMessage {
  role: 'user' | 'assistant'
  content: string
  segments?: DisplaySegment[]
}

// ========== SSE Event Data Types ==========

export interface ChatDeltaEventData {
  type: string
  delta: { content: string; thought?: string }
  finished: boolean
  sender?: string
  targetEventId?: string
}

export interface ChatEventData {
  type: string
  id?: string
  role?: string
  sender?: string
  messageType?: string
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  content: any
  contents?: Array<{ type: string; value: string }>
  text_preview?: string
}

export interface ToolUsedEventData {
  type: string
  tool: string
  actionId: string
  status: string // 'start' | 'streaming' | 'argumentsFinished' | 'success' | 'error'
  planStepId?: string
  brief?: string
  description?: string
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  message?: any
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  argumentsDetail?: any
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  detail?: any
}

export interface StatusUpdateEventData {
  type: string
  agentStatus: string
  brief?: string
  description?: string
}

export interface LiveStatusEventData {
  type: string
  text: string
}

export interface EventsReplayData {
  type: string
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  events: any[]
}

export interface SSEEventHandlers {
  onConnected?: () => void
  onChatDelta?: (data: ChatDeltaEventData) => void
  onChat?: (data: ChatEventData) => void
  onToolUsed?: (data: ToolUsedEventData) => void
  onStatusUpdate?: (data: StatusUpdateEventData) => void
  onLiveStatus?: (data: LiveStatusEventData) => void
  onEventsReplay?: (data: EventsReplayData) => void
  onError?: (error: unknown) => void
  onFinish?: () => void
}

// Session history message (for SSE replay compatibility)
export interface SessionMessage {
  id: string
  role: string
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  content: any
  text_preview?: string
  created_at: string
  updated_at: string
  type?: string
  sender?: string
  messageType?: string
}

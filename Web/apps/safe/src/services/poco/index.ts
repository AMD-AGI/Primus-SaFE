import axios from 'axios'
import type {
  PocoSession,
  PocoSkill,
  CreateSessionRequest,
  SendMessageRequest,
  PocoChatRequest,
  SSEEventHandlers,
  ToolCallDetail,
  DisplaySegment,
  PocoChatMessage,
} from './type'

// PrimusClaw dedicated axios instance, baseURL points to /claw/v1
const clawRequest = axios.create({
  baseURL: '/claw/v1',
  timeout: 30000,
  withCredentials: true,
})

clawRequest.interceptors.response.use(
  (response) => response.data,
  (error) => {
    console.error('PrimusClaw API error:', error)
    return Promise.reject(error)
  },
)

const BASE_URL = '/claw/v1'

// ========== Sessions ==========

export const getSessions = (): Promise<{ code: number; data: PocoSession[] }> =>
  clawRequest.get('/sessions')

export const createSession = (
  data?: CreateSessionRequest,
): Promise<{ code: number; data: PocoSession }> =>
  clawRequest.post('/sessions', data || {})

export const deleteSession = (
  sessionId: string,
): Promise<{ code: number }> =>
  clawRequest.delete(`/sessions/${sessionId}`)

// ========== Skills (CRUD, no install concept) ==========

export const getSkills = (): Promise<{ code: number; data: PocoSkill[] }> =>
  clawRequest.get('/skills')

// ========== Send Message ==========

export const sendSessionMessage = (
  sessionId: string,
  data: SendMessageRequest,
): Promise<{ code: number; data: unknown }> =>
  clawRequest.post(`/sessions/${sessionId}/messages`, data)

// ========== SSE Utilities ==========

/** Normalize SSE event type: snake_case → camelCase */
function normalizeEventType(type: string): string {
  return type.replace(/_([a-z])/g, (_, c: string) => c.toUpperCase())
}

/** Dispatch parsed SSE event to the appropriate handler */
function dispatchSSEEvent(
  eventType: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: any,
  handlers: SSEEventHandlers,
) {
  const type = normalizeEventType(eventType)
  switch (type) {
    case 'chatDelta':
      handlers.onChatDelta?.(data)
      break
    case 'chat':
      handlers.onChat?.(data)
      break
    case 'toolUsed':
      handlers.onToolUsed?.(data)
      break
    case 'statusUpdate':
      handlers.onStatusUpdate?.(data)
      break
    case 'liveStatus':
      handlers.onLiveStatus?.(data)
      break
    case 'eventsNotifyEventsAfter':
      handlers.onEventsReplay?.(data)
      break
    case 'error':
      handlers.onError?.(data)
      break
    default:
      break
  }
}

/** Parse a single SSE text block (lines between blank lines) */
function processSSEBlock(block: string, handlers: SSEEventHandlers) {
  const lines = block.split('\n')
  let eventType = ''
  let dataStr = ''

  for (const line of lines) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith(':')) continue // empty or keepalive comment
    if (trimmed.startsWith('event:')) {
      eventType = trimmed.slice(6).trim()
    } else if (trimmed.startsWith('data:')) {
      const piece = trimmed.slice(5).trim()
      dataStr += (dataStr ? '\n' : '') + piece
    }
    // id: lines are silently consumed (could be used for reconnection)
  }

  if (!dataStr) return
  if (dataStr === '[DONE]') {
    handlers.onFinish?.()
    return
  }

  try {
    const parsed = JSON.parse(dataStr)
    const type = eventType || parsed.type || ''
    if (type) {
      dispatchSSEEvent(type, parsed, handlers)
    }
  } catch {
    // non-JSON SSE data, skip
  }
}

// ========== SSE Subscription ==========

/**
 * Subscribe to session SSE events (GET /v1/chat/sessions/{id}/messages).
 *
 * Use cases:
 *  1. Load session history: SSE replays `events_notify_events_after`
 *  2. Persistent subscription: keep connection open for live events
 *
 * Returns a Promise that resolves when the stream ends or is aborted.
 */
export async function subscribeSessionSSE(
  sessionId: string,
  handlers: SSEEventHandlers,
  afterEventId?: string,
  signal?: AbortSignal,
) {
  const url =
    `${BASE_URL}/chat/sessions/${sessionId}/messages` +
    (afterEventId ? `?after_event_id=${encodeURIComponent(afterEventId)}` : '')

  try {
    const response = await fetch(url, {
      headers: { Accept: 'text/event-stream' },
      credentials: 'include',
      signal,
    })

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`SSE HTTP ${response.status}: ${errorText}`)
    }

    handlers.onConnected?.()

    const reader = response.body?.getReader()
    if (!reader) throw new Error('Failed to get SSE response stream')

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) {
        if (buffer.trim()) processSSEBlock(buffer, handlers)
        handlers.onFinish?.()
        break
      }
      buffer += decoder.decode(value, { stream: true })
      const blocks = buffer.split('\n\n')
      buffer = blocks.pop() || ''
      for (const block of blocks) {
        if (block.trim()) processSSEBlock(block, handlers)
      }
    }
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') return
    console.error('[SSE] stream error:', error)
    handlers.onError?.(error)
  }
}

// ========== Load Session Messages (via SSE history replay) ==========

/**
 * Load session messages by subscribing to SSE and waiting for
 * the `events_notify_events_after` event, then close.
 *
 * Returns the raw event array for the caller to process.
 */
export async function getSessionMessages(
  sessionId: string,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
): Promise<{ data: any[] }> {
  const controller = new AbortController()

  return new Promise((resolve) => {
    let settled = false

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const settle = (events: any[]) => {
      if (settled) return
      settled = true
      controller.abort()
      resolve({ data: events })
    }

    subscribeSessionSSE(
      sessionId,
      {
        onEventsReplay: (data) => settle(data.events || []),
        onError: () => settle([]),
        onFinish: () => settle([]),
      },
      undefined,
      controller.signal,
    ).catch(() => settle([]))

    // Timeout fallback
    setTimeout(() => settle([]), 15000)
  })
}

// ========== Chat (two-channel: SSE subscribe + POST message) ==========

/**
 * Send a chat message and stream the response via SSE.
 *
 * Flow:
 *  1. Subscribe to SSE: GET /v1/chat/sessions/{session_id}/messages
 *  2. Wait for SSE connection to establish
 *  3. Send message:     POST /v1/sessions/{session_id}/messages
 *  4. Read SSE events → forward chatDelta content to `onMessage`
 *
 * Signature is kept compatible so the page needs minimal changes.
 */
export async function pocoChat(
  data: PocoChatRequest,
  onMessage: (content: string) => void,
  onError?: (error: unknown) => void,
  onFinish?: () => void,
  signal?: AbortSignal,
  extraHandlers?: Partial<SSEEventHandlers>,
) {
  const sseUrl = `${BASE_URL}/chat/sessions/${data.session_id}/messages`

  try {
    // 1. Open SSE subscription first
    const sseResponse = await fetch(sseUrl, {
      headers: { Accept: 'text/event-stream' },
      credentials: 'include',
      signal,
    })

    if (!sseResponse.ok) {
      const errorText = await sseResponse.text()
      throw new Error(`SSE HTTP ${sseResponse.status}: ${errorText}`)
    }

    // 2. SSE connected — now send the message
    try {
      await fetch(`${BASE_URL}/sessions/${data.session_id}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          content: data.query,
          contents: [{ type: 'text', value: data.query }],
          messageType: 'text',
          taskMode: 'agent',
          attachments: [],
          tools: data.tools || [],
        }),
        signal,
      })
    } catch (postErr) {
      if (postErr instanceof Error && postErr.name === 'AbortError') return
      console.error('[pocoChat] send message error:', postErr)
      // Don't return — still try to read SSE in case message was received
    }

    // 3. Read SSE events
    const reader = sseResponse.body?.getReader()
    if (!reader) throw new Error('Failed to get SSE response stream')

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) {
        onFinish?.()
        break
      }

      buffer += decoder.decode(value, { stream: true })
      const blocks = buffer.split('\n\n')
      buffer = blocks.pop() || ''

      for (const block of blocks) {
        const lines = block.split('\n')
        let eventType = ''
        let dataStr = ''

        for (const line of lines) {
          const trimmed = line.trim()
          if (!trimmed || trimmed.startsWith(':')) continue
          if (trimmed.startsWith('event:')) {
            eventType = trimmed.slice(6).trim()
          } else if (trimmed.startsWith('data:')) {
            dataStr += (dataStr ? '\n' : '') + trimmed.slice(5).trim()
          }
        }

        if (!dataStr) continue
        if (dataStr === '[DONE]') {
          onFinish?.()
          return
        }

        try {
          const parsed = JSON.parse(dataStr)
          const type = normalizeEventType(eventType || parsed.type || '')

          switch (type) {
            case 'chatDelta': {
              const content = parsed.delta?.content || ''
              if (content) onMessage(content)
              if (parsed.finished) {
                onFinish?.()
                return
              }
              break
            }
            case 'chat': {
              // Skip user echo; for assistant final message chatDelta already built it
              break
            }
            case 'toolUsed':
              extraHandlers?.onToolUsed?.(parsed)
              break
            case 'statusUpdate':
              extraHandlers?.onStatusUpdate?.(parsed)
              if (parsed.agentStatus === 'stopped') {
                onFinish?.()
                return
              }
              break
            case 'liveStatus':
              extraHandlers?.onLiveStatus?.(parsed)
              break
            case 'eventsNotifyEventsAfter':
              // History replay on fresh SSE — skip during live chat
              break
            case 'error':
              onError?.(parsed)
              return
            default: {
              // Fallback: try to extract plain content
              const content =
                parsed.content ||
                parsed.delta?.content ||
                parsed.text ||
                parsed.choices?.[0]?.delta?.content ||
                ''
              if (content && typeof content === 'string') {
                onMessage(content)
              }
              break
            }
          }
        } catch {
          // Non-JSON — treat as plain text
          if (dataStr) onMessage(dataStr)
        }
      }
    }
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') return
    console.error('[pocoChat] stream error:', error)
    onError?.(error)
  }
}

// ========== History Event Processing ==========

/**
 * Extract display text from a `chat` event.
 * Handles both the current implementation format (with _type wrappers)
 * and the design spec format (flat string content).
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function extractTextFromEvent(event: any): string {
  if (typeof event.content === 'string') return event.content
  if (event.text_preview) return event.text_preview
  if (event.contents && Array.isArray(event.contents)) {
    return event.contents
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      .filter((c: any) => c.type === 'text')
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      .map((c: any) => c.value)
      .join('\n')
  }
  // Structured content with TextBlock (current backend format)
  if (event.content?.content && Array.isArray(event.content.content)) {
    return event.content.content
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      .filter((b: any) => (b._type === 'TextBlock' || b.type === 'text') && (b.text || b.value))
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      .map((b: any) => b.text || b.value)
      .join('\n')
  }
  return ''
}

/**
 * Process raw SSE history events (from `events_notify_events_after`)
 * into merged display messages.
 *
 * Persisted event types: chat / tool_used / error
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function processHistoryEvents(events: any[]): PocoChatMessage[] {
  const result: PocoChatMessage[] = []
  let currentAssistant: PocoChatMessage | null = null
  let pendingToolCalls: ToolCallDetail[] = []

  const flushToolCalls = () => {
    if (pendingToolCalls.length === 0) return
    if (!currentAssistant) currentAssistant = { role: 'assistant', content: '', segments: [] }
    if (!currentAssistant.segments) currentAssistant.segments = []
    currentAssistant.segments.push({
      type: 'tool-execution',
      toolCount: pendingToolCalls.length,
      toolCalls: [...pendingToolCalls],
      expanded: false,
    })
    pendingToolCalls = []
  }

  const flushAssistant = () => {
    if (!currentAssistant) return
    flushToolCalls()
    if ((currentAssistant.segments && currentAssistant.segments.length > 0) || currentAssistant.content) {
      result.push(currentAssistant)
    }
    currentAssistant = null
  }

  for (const rawEvent of events) {
    // Handle wrapped events: { id, event, data: {...} } or flat events: { type, ... }
    const evt = rawEvent.data || rawEvent
    const type = normalizeEventType(evt.type || rawEvent.event || '')

    if (type === 'chat') {
      const sender = evt.sender || evt.role || ''

      if (sender === 'user') {
        flushAssistant()
        const text = extractTextFromEvent(evt)
        if (text) result.push({ role: 'user', content: text })
      } else if (sender === 'assistant') {
        if (!currentAssistant) currentAssistant = { role: 'assistant', content: '', segments: [] }
        flushToolCalls()
        const text = extractTextFromEvent(evt)
        if (text) {
          currentAssistant.content += (currentAssistant.content ? '\n' : '') + text
          currentAssistant.segments!.push({ type: 'text', text })
        }
      }
    } else if (type === 'toolUsed') {
      if (evt.tool === 'suggestion') continue

      // Show start / running / success / error tool calls in history
      if (evt.status === 'start' || evt.status === 'running' || evt.status === 'success' || evt.status === 'error') {
        if (!currentAssistant) currentAssistant = { role: 'assistant', content: '', segments: [] }

        // Extract input from argumentsDetail if present
        const input = evt.argumentsDetail || evt.input || undefined

        const existingIdx = pendingToolCalls.findIndex(t => t.toolUseId === evt.actionId)
        if (existingIdx >= 0) {
          // Merge into existing entry — preserve fields that success event may omit
          const existing = pendingToolCalls[existingIdx]
          existing.status = evt.status
          existing.isError = evt.status === 'error'
          // Only overwrite if new value is non-empty
          if (evt.tool) { existing.name = evt.tool; existing.tool = evt.tool }
          if (evt.brief) existing.brief = evt.brief
          if (evt.description) { existing.description = evt.description; existing.output = evt.description }
          if (input && !existing.input) existing.input = input
        } else {
          pendingToolCalls.push({
            toolUseId: evt.actionId || '',
            name: evt.tool || evt.brief || 'Unknown',
            tool: evt.tool || '',
            status: evt.status,
            brief: evt.brief || '',
            description: evt.description || '',
            output: evt.description || '',
            input,
            isError: evt.status === 'error',
            expanded: false,
          })
        }
      }
    }
    // 'error' events could be rendered as assistant error messages — skip for now
  }

  flushAssistant()
  return result
}

export type {
  PocoSession,
  PocoSkill,
  PocoChatMessage,
  DisplaySegment,
  ToolCallDetail,
  PocoChatRequest,
  SendMessageRequest,
  CreateSessionRequest,
  SessionMessage,
  SSEEventHandlers,
  ChatDeltaEventData,
  ChatEventData,
  ToolUsedEventData,
  StatusUpdateEventData,
  LiveStatusEventData,
  EventsReplayData,
} from './type'

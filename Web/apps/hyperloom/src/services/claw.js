/**
 * PrimusClaw API Service
 *
 * Dual-channel SSE chat architecture:
 *   1. GET  /claw/v1/chat/sessions/{id}/messages  (SSE long-connection, receive)
 *   2. POST /claw/v1/sessions/{id}/messages        (send user message)
 *
 * Session CRUD + Skills + Tools
 */

const BASE_URL = import.meta.env.VITE_CLAW_BASE_URL || '/claw-api/v1';

// ========== Sessions ==========

export async function getSessions() {
  const res = await fetch(`${BASE_URL}/sessions`, { credentials: 'include' });
  if (!res.ok) throw new Error(`getSessions: ${res.status}`);
  return res.json();
}

export async function createSession(data = {}) {
  const res = await fetch(`${BASE_URL}/sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(`createSession: ${res.status}`);
  return res.json();
}

export async function deleteSession(sessionId) {
  const res = await fetch(`${BASE_URL}/sessions/${sessionId}`, {
    method: 'DELETE',
    credentials: 'include',
  });
  if (!res.ok) throw new Error(`deleteSession: ${res.status}`);
  return res.json();
}

// ========== Skills ==========

export async function getSkills() {
  const res = await fetch(`${BASE_URL}/skills`, { credentials: 'include' });
  if (!res.ok) throw new Error(`getSkills: ${res.status}`);
  return res.json();
}

// ========== Tools ==========

export async function getTools({
  offset = 0,
  limit = 50,
  order = 'desc',
  sort,
  status,
  type,
  owner,
} = {}) {
  const paramsObject = {
    offset: String(offset),
    limit: String(limit),
    order,
  };
  if (sort) paramsObject.sort = String(sort);
  if (status) paramsObject.status = String(status);
  if (type) paramsObject.type = String(type);
  if (owner) paramsObject.owner = String(owner);

  const params = new URLSearchParams(paramsObject);
  const res = await fetch(`/api/tools/api/v1/tools?${params}`, { credentials: 'include' });
  if (!res.ok) throw new Error(`getTools: ${res.status}`);
  return res.json();
}

// ========== SSE Utilities ==========

function normalizeEventType(type) {
  return type.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
}

function processSSEBlock(block, handlers) {
  const lines = block.split('\n');
  let eventType = '';
  let dataStr = '';

  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith(':')) continue;
    if (trimmed.startsWith('event:')) {
      eventType = trimmed.slice(6).trim();
    } else if (trimmed.startsWith('data:')) {
      const piece = trimmed.slice(5).trim();
      dataStr += (dataStr ? '\n' : '') + piece;
    }
  }

  if (!dataStr) return;
  if (dataStr === '[DONE]') {
    handlers.onFinish?.();
    return;
  }

  try {
    const parsed = JSON.parse(dataStr);
    const type = normalizeEventType(eventType || parsed.type || '');
    if (type) dispatchSSEEvent(type, parsed, handlers);
  } catch {
    // non-JSON SSE data
  }
}

function dispatchSSEEvent(eventType, data, handlers) {
  const type = normalizeEventType(eventType);
  switch (type) {
    case 'chatDelta':    handlers.onChatDelta?.(data); break;
    case 'chat':         handlers.onChat?.(data); break;
    case 'toolUsed':     handlers.onToolUsed?.(data); break;
    case 'statusUpdate': handlers.onStatusUpdate?.(data); break;
    case 'liveStatus':   handlers.onLiveStatus?.(data); break;
    case 'eventsNotifyEventsAfter': handlers.onEventsReplay?.(data); break;
    case 'error':        handlers.onError?.(data); break;
  }
}

// ========== Load Session Messages (via SSE history replay) ==========

export async function getSessionMessages(sessionId) {
  const controller = new AbortController();

  return new Promise((resolve) => {
    let settled = false;
    const settle = (events) => {
      if (settled) return;
      settled = true;
      controller.abort();
      resolve({ data: events });
    };

    subscribeSessionSSE(sessionId, {
      onEventsReplay: (data) => settle(data.events || []),
      onError: () => settle([]),
      onFinish: () => settle([]),
    }, undefined, controller.signal).catch(() => settle([]));

    setTimeout(() => settle([]), 15000);
  });
}

// ========== SSE Subscription ==========

export async function subscribeSessionSSE(sessionId, handlers, afterEventId, signal) {
  const url = `${BASE_URL}/chat/sessions/${sessionId}/messages` +
    (afterEventId ? `?after_event_id=${encodeURIComponent(afterEventId)}` : '');

  try {
    const response = await fetch(url, {
      headers: { Accept: 'text/event-stream' },
      credentials: 'include',
      signal,
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`SSE HTTP ${response.status}: ${errorText}`);
    }

    handlers.onConnected?.();

    const reader = response.body?.getReader();
    if (!reader) throw new Error('Failed to get SSE response stream');

    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        if (buffer.trim()) processSSEBlock(buffer, handlers);
        handlers.onFinish?.();
        break;
      }
      buffer += decoder.decode(value, { stream: true });
      const blocks = buffer.split('\n\n');
      buffer = blocks.pop() || '';
      for (const block of blocks) {
        if (block.trim()) processSSEBlock(block, handlers);
      }
    }
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') return;
    console.error('[SSE] stream error:', error);
    handlers.onError?.(error);
  }
}

// ========== Chat (two-channel: SSE subscribe + POST message) ==========

export async function clawChat(data, onMessage, onError, onFinish, signal, extraHandlers) {
  const sseUrl = `${BASE_URL}/chat/sessions/${data.session_id}/messages`;

  try {
    const sseResponse = await fetch(sseUrl, {
      headers: { Accept: 'text/event-stream' },
      credentials: 'include',
      signal,
    });

    if (!sseResponse.ok) {
      const errorText = await sseResponse.text();
      throw new Error(`SSE HTTP ${sseResponse.status}: ${errorText}`);
    }

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
      });
    } catch (postErr) {
      if (postErr instanceof Error && postErr.name === 'AbortError') return;
      console.error('[clawChat] send message error:', postErr);
    }

    const reader = sseResponse.body?.getReader();
    if (!reader) throw new Error('Failed to get SSE response stream');

    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) { onFinish?.(); break; }

      buffer += decoder.decode(value, { stream: true });
      const blocks = buffer.split('\n\n');
      buffer = blocks.pop() || '';

      for (const block of blocks) {
        const lines = block.split('\n');
        let eventType = '';
        let dataStr = '';

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed || trimmed.startsWith(':')) continue;
          if (trimmed.startsWith('event:')) eventType = trimmed.slice(6).trim();
          else if (trimmed.startsWith('data:')) dataStr += (dataStr ? '\n' : '') + trimmed.slice(5).trim();
        }

        if (!dataStr) continue;
        if (dataStr === '[DONE]') { onFinish?.(); return; }

        try {
          const parsed = JSON.parse(dataStr);
          const type = normalizeEventType(eventType || parsed.type || '');

          switch (type) {
            case 'chatDelta': {
              const content = parsed.delta?.content || '';
              if (content) onMessage(content);
              if (parsed.finished) { onFinish?.(); return; }
              break;
            }
            case 'chat': break;
            case 'toolUsed':     extraHandlers?.onToolUsed?.(parsed); break;
            case 'statusUpdate':
              extraHandlers?.onStatusUpdate?.(parsed);
              if (parsed.agentStatus === 'stopped') { onFinish?.(); return; }
              break;
            case 'liveStatus':   extraHandlers?.onLiveStatus?.(parsed); break;
            case 'eventsNotifyEventsAfter': break;
            case 'error':        onError?.(parsed); return;
            default: {
              const content = parsed.content || parsed.delta?.content || parsed.text ||
                parsed.choices?.[0]?.delta?.content || '';
              if (content && typeof content === 'string') onMessage(content);
              break;
            }
          }
        } catch {
          if (dataStr) onMessage(dataStr);
        }
      }
    }
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') return;
    console.error('[clawChat] stream error:', error);
    onError?.(error);
  }
}

// ========== History Event Processing ==========

export function extractTextFromEvent(event) {
  if (typeof event.content === 'string') return event.content;
  if (event.text_preview) return event.text_preview;
  if (event.contents && Array.isArray(event.contents)) {
    return event.contents.filter(c => c.type === 'text').map(c => c.value).join('\n');
  }
  if (event.content?.content && Array.isArray(event.content.content)) {
    return event.content.content
      .filter(b => (b._type === 'TextBlock' || b.type === 'text') && (b.text || b.value))
      .map(b => b.text || b.value)
      .join('\n');
  }
  return '';
}

export function processHistoryEvents(events) {
  const result = [];
  let currentAssistant = null;
  let pendingToolCalls = [];

  const flushToolCalls = () => {
    if (pendingToolCalls.length === 0) return;
    if (!currentAssistant) currentAssistant = { role: 'assistant', content: '', segments: [] };
    if (!currentAssistant.segments) currentAssistant.segments = [];
    currentAssistant.segments.push({
      type: 'tool-execution',
      toolCount: pendingToolCalls.length,
      toolCalls: [...pendingToolCalls],
      expanded: false,
    });
    pendingToolCalls = [];
  };

  const flushAssistant = () => {
    if (!currentAssistant) return;
    flushToolCalls();
    if ((currentAssistant.segments?.length > 0) || currentAssistant.content) {
      result.push(currentAssistant);
    }
    currentAssistant = null;
  };

  for (const rawEvent of events) {
    const evt = rawEvent.data || rawEvent;
    const type = normalizeEventType(evt.type || rawEvent.event || '');

    if (type === 'chat') {
      const sender = evt.sender || evt.role || '';
      if (sender === 'user') {
        flushAssistant();
        const text = extractTextFromEvent(evt);
        if (text) result.push({ role: 'user', content: text });
      } else if (sender === 'assistant') {
        if (!currentAssistant) currentAssistant = { role: 'assistant', content: '', segments: [] };
        flushToolCalls();
        const text = extractTextFromEvent(evt);
        if (text) {
          currentAssistant.content += (currentAssistant.content ? '\n' : '') + text;
          currentAssistant.segments.push({ type: 'text', text });
        }
      }
    } else if (type === 'toolUsed') {
      if (evt.tool === 'suggestion') continue;
      if (['start', 'running', 'success', 'error'].includes(evt.status)) {
        if (!currentAssistant) currentAssistant = { role: 'assistant', content: '', segments: [] };
        const input = evt.argumentsDetail || evt.input || undefined;
        const existingIdx = pendingToolCalls.findIndex(t => t.toolUseId === evt.actionId);
        if (existingIdx >= 0) {
          const existing = pendingToolCalls[existingIdx];
          existing.status = evt.status;
          existing.isError = evt.status === 'error';
          if (evt.tool) { existing.name = evt.tool; existing.tool = evt.tool; }
          if (evt.brief) existing.brief = evt.brief;
          if (evt.description) { existing.description = evt.description; existing.output = evt.description; }
          if (input && !existing.input) existing.input = input;
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
          });
        }
      }
    }
  }

  flushAssistant();
  return result;
}

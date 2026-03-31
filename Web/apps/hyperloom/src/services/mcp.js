/**
 * MCP Agent Service — TraceLens & GEAK
 *
 * Protocol: JSON-RPC 2.0 over HTTP POST
 *
 * TraceLens: GPU kernel profiling & trace analysis
 *   POST /mcp/tracelens → rewritten to /control-plane/.../trace-lens-agent-bwrmr/mcp
 *
 * GEAK: GPU kernel auto-optimization
 *   POST /mcp/geak → rewritten to /control-plane/.../geak-agent-wvsbv/mcp/sse
 *   Requires Bearer token (VITE_MCP_GEAK_API_KEY)
 *   Returns SSE stream for long-running optimizations
 */

const TRACELENS_URL = import.meta.env.VITE_MCP_TRACELENS_URL || '/mcp/tracelens';
const GEAK_URL = import.meta.env.VITE_MCP_GEAK_URL || '/mcp/geak';
const GEAK_API_KEY = import.meta.env.VITE_MCP_GEAK_API_KEY || '';

let sessionIds = { tracelens: '', geak: '' };
let rpcId = 0;

function buildRpcBody(method, params = {}) {
  return { jsonrpc: '2.0', id: ++rpcId, method, params };
}

// ========== TraceLens Agent ==========

export async function initTraceLens() {
  const result = await callTraceLens('initialize', {
    protocolVersion: '2024-11-05',
    capabilities: {},
    clientInfo: { name: 'PRISM-Dashboard', version: '1.0.0' },
  });
  await callTraceLens('notifications/initialized', {});
  return result;
}

export async function callTraceLens(method, params = {}) {
  const headers = {
    'Content-Type': 'application/json',
    'Accept': 'application/json, text/event-stream',
  };
  if (sessionIds.tracelens) {
    headers['Mcp-Session-Id'] = sessionIds.tracelens;
  }

  const res = await fetch(TRACELENS_URL, {
    method: 'POST',
    headers,
    body: JSON.stringify(buildRpcBody(method, params)),
  });

  const sid = res.headers.get('mcp-session-id') || res.headers.get('Mcp-Session-Id');
  if (sid) sessionIds.tracelens = sid;

  const contentType = res.headers.get('content-type') || '';

  if (contentType.includes('text/event-stream')) {
    return parseSSEResponse(res);
  }

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`TraceLens ${method}: HTTP ${res.status} — ${text}`);
  }

  return res.json();
}

// ========== GEAK Agent ==========

export async function initGEAK() {
  const result = await callGEAK('initialize', {
    protocolVersion: '2024-11-05',
    capabilities: {},
    clientInfo: { name: 'PRISM-Dashboard', version: '1.0.0' },
  });
  await callGEAK('notifications/initialized', {});
  return result;
}

export async function callGEAK(method, params = {}) {
  const headers = {
    'Content-Type': 'application/json',
    'Accept': 'application/json, text/event-stream',
  };
  if (GEAK_API_KEY) {
    headers['Authorization'] = `Bearer ${GEAK_API_KEY}`;
  }
  if (sessionIds.geak) {
    headers['Mcp-Session-Id'] = sessionIds.geak;
  }

  const res = await fetch(GEAK_URL, {
    method: 'POST',
    headers,
    body: JSON.stringify(buildRpcBody(method, params)),
  });

  const sid = res.headers.get('mcp-session-id') || res.headers.get('Mcp-Session-Id');
  if (sid) sessionIds.geak = sid;

  const contentType = res.headers.get('content-type') || '';

  if (contentType.includes('text/event-stream')) {
    return parseSSEResponse(res);
  }

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`GEAK ${method}: HTTP ${res.status} — ${text}`);
  }

  return res.json();
}

// ========== Convenience: tools/call ==========

export async function traceLensToolCall(toolName, args = {}) {
  return callTraceLens('tools/call', { name: toolName, arguments: args });
}

export async function geakToolCall(toolName, args = {}) {
  return callGEAK('tools/call', { name: toolName, arguments: args });
}

export async function listTraceLensTools() {
  return callTraceLens('tools/list', {});
}

export async function listGEAKTools() {
  return callGEAK('tools/list', {});
}

// ========== SSE Response Parser ==========

async function parseSSEResponse(response) {
  const reader = response.body?.getReader();
  if (!reader) throw new Error('No readable stream in SSE response');

  const decoder = new TextDecoder();
  const results = [];
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const blocks = buffer.split('\n\n');
    buffer = blocks.pop() || '';

    for (const block of blocks) {
      let dataStr = '';
      for (const line of block.split('\n')) {
        const trimmed = line.trim();
        if (trimmed.startsWith('data:')) {
          dataStr += trimmed.slice(5).trim();
        }
      }
      if (!dataStr || dataStr === '[DONE]') continue;
      try {
        results.push(JSON.parse(dataStr));
      } catch { /* skip non-JSON */ }
    }
  }

  return results.length === 1 ? results[0] : results;
}

// ========== Streaming SSE (for long-running GEAK optimizations) ==========

export async function callGEAKStream(method, params = {}, onData, onDone, signal) {
  const headers = { 'Content-Type': 'application/json' };
  if (GEAK_API_KEY) headers['Authorization'] = `Bearer ${GEAK_API_KEY}`;
  if (sessionIds.geak) headers['Mcp-Session-Id'] = sessionIds.geak;

  const res = await fetch(GEAK_URL, {
    method: 'POST',
    headers,
    body: JSON.stringify(buildRpcBody(method, params)),
    signal,
  });

  const sid = res.headers.get('mcp-session-id') || res.headers.get('Mcp-Session-Id');
  if (sid) sessionIds.geak = sid;

  const reader = res.body?.getReader();
  if (!reader) throw new Error('No readable stream');

  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) { onDone?.(); break; }

    buffer += decoder.decode(value, { stream: true });
    const blocks = buffer.split('\n\n');
    buffer = blocks.pop() || '';

    for (const block of blocks) {
      let dataStr = '';
      for (const line of block.split('\n')) {
        const trimmed = line.trim();
        if (trimmed.startsWith('data:')) dataStr += trimmed.slice(5).trim();
      }
      if (!dataStr) continue;
      if (dataStr === '[DONE]') { onDone?.(); return; }
      try {
        onData?.(JSON.parse(dataStr));
      } catch { /* skip */ }
    }
  }
}

// ========== Reset ==========

export function resetMcpSessions() {
  sessionIds = { tracelens: '', geak: '' };
  rpcId = 0;
}

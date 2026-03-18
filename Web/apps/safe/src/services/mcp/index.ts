// MCP server configuration
// Local dev: /mcp-proxy (proxied by Vite dev server)
// Production: set via VITE_MCP_BASE_URL env var, or add /mcp-proxy in nginx
const MCP_SERVER_URL =
import.meta.env.VITE_MCP_BASE_URL || '/mcp-proxy'
const API_KEY = import.meta.env.VITE_MCP_API_KEY || ''

// Session ID for MCP protocol session management
let sessionId: string | null = null
let requestId = 1
let isInitialized = false

// ============ SSE Parsing ============

/**
 * Parse SSE (Server-Sent Events) formatted response text
 * SSE format: "event: message\ndata: {json}\n\n"
 */
function parseSSEResponse(text: string): any {
  const lines = text.split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (trimmed.startsWith('data:')) {
      const jsonStr = trimmed.substring(5).trim()
      if (jsonStr) {
        try {
          return JSON.parse(jsonStr)
        } catch {
          console.warn('Failed to parse SSE data line:', jsonStr)
        }
      }
    }
  }
  return null
}

// ============ Core Request Method ============

/**
 * Send MCP request (using native fetch, correctly handles SSE responses)
 */
async function mcpFetch(payload: Record<string, any>): Promise<any> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${API_KEY}`,
    'Accept': 'application/json, text/event-stream',
  }

  // Attach session ID to request headers if available
  if (sessionId) {
    headers['Mcp-Session-Id'] = sessionId
  }

  const response = await fetch(MCP_SERVER_URL, {
    method: 'POST',
    headers,
    body: JSON.stringify(payload),
  })

  // Capture session ID from response
  const newSessionId = response.headers.get('mcp-session-id')
  if (newSessionId) {
    sessionId = newSessionId
  }

  if (!response.ok) {
    const errorText = await response.text()
    console.error(`[MCP] HTTP Error ${response.status}:`, errorText)
    throw new Error(`MCP HTTP Error ${response.status}: ${errorText}`)
  }

  // Read response text
  const responseText = await response.text()

  if (!responseText) {
    return null
  }

  // Parse response based on Content-Type
  const contentType = response.headers.get('content-type') || ''
  let data: any

  if (contentType.includes('text/event-stream')) {
    data = parseSSEResponse(responseText)
    if (!data) {
      console.error('[MCP] Failed to parse SSE response:', responseText)
      throw new Error('Failed to parse SSE response')
    }
  } else {
    // Try parsing as JSON
    try {
      data = JSON.parse(responseText)
    } catch {
      // Content-Type may be incorrect; try SSE parse as fallback
      if (responseText.includes('data:')) {
        data = parseSSEResponse(responseText)
      }
      if (!data) {
        console.error('[MCP] Cannot parse response:', responseText)
        throw new Error(`Cannot parse MCP response: ${responseText.substring(0, 200)}`)
      }
    }
  }

  // Check for JSON-RPC errors
  if (data?.error) {
    console.error('[MCP] JSON-RPC Error:', data.error)
    throw new Error(data.error.message || JSON.stringify(data.error))
  }

  return data
}

// ============ MCP Protocol Methods ============

/**
 * Initialize MCP session
 * Per MCP protocol, initialize must be called before any other method
 */
async function initializeSession(): Promise<void> {
  if (isInitialized) return

  try {
    // Step 1: Send initialize request
    const initPayload = {
      jsonrpc: '2.0',
      method: 'initialize',
      params: {
        protocolVersion: '2024-11-05',
        capabilities: {},
        clientInfo: {
          name: 'primus-frontend',
          version: '1.0.0',
        },
      },
      id: requestId++,
    }

    await mcpFetch(initPayload)

    // Step 2: Send initialized notification (no id field = notification)
    await mcpFetch({
      jsonrpc: '2.0',
      method: 'notifications/initialized',
      params: {},
    })

    isInitialized = true
  } catch (error) {
    console.error('[MCP] ❌ Failed to initialize session:', error)
    throw error
  }
}

/**
 * Call an MCP tool
 * Automatically ensures the session is initialized first
 */
async function callMCPTool<T = any>(toolName: string, args: Record<string, any> = {}): Promise<T> {
  // Ensure session is initialized
  await initializeSession()

  const payload = {
    jsonrpc: '2.0',
    method: 'tools/call',
    params: {
      name: toolName,
      arguments: args,
    },
    id: requestId++,
  }

  const response = await mcpFetch(payload)

  // Extract result
  const result = response?.result
  if (result?.content && Array.isArray(result.content)) {
    // Extract text-type content
    const textContent = result.content
      .filter((item: any) => item.type === 'text')
      .map((item: any) => item.text)
      .join('\n')
    return textContent as T
  }

  return result as T
}

// ============ MCP Tool Interfaces ============

/**
 * Look up a user's NTID
 * @param userIdentifier User identifier (email, NTID, employeeID, etc.)
 */
export function getUserNTID(userIdentifier: string) {
  return callMCPTool('get_user_ntid', { user_identifier: userIdentifier })
}

/**
 * List groups accessible by the API key
 */
export function listAllowedGroups() {
  return callMCPTool('list_allowed_groups', {})
}

/**
 * List group members
 * @param groupCn Group CN (Common Name)
 */
export function listGroupMembers(groupCn: string) {
  return callMCPTool('list_group_members', { group_cn: groupCn })
}

/**
 * Add a user to a group
 * @param userIdentifier User identifier
 * @param groupCn Group CN (Common Name)
 */
export function addUserToGroup(userIdentifier: string, groupCn: string) {
  return callMCPTool('add_user_to_group', {
    user_identifier: userIdentifier,
    group_cn: groupCn,
  })
}

/**
 * Check whether a user belongs to a group
 * @param userIdentifier User identifier
 * @param groupCn Group CN (Common Name)
 */
export function checkUserInGroup(userIdentifier: string, groupCn: string) {
  return callMCPTool('check_user_in_group', {
    user_identifier: userIdentifier,
    group_cn: groupCn,
  })
}

/**
 * Remove a user from a group
 * @param userIdentifier User identifier
 * @param groupCn Group CN (Common Name)
 */
export function removeUserFromGroup(userIdentifier: string, groupCn: string) {
  return callMCPTool('remove_user_from_group', {
    user_identifier: userIdentifier,
    group_cn: groupCn,
  })
}

/**
 * Reset MCP session (for debugging)
 */
export function resetSession() {
  sessionId = null
  isInitialized = false
  requestId = 1
}

export default {
  getUserNTID,
  listAllowedGroups,
  listGroupMembers,
  addUserToGroup,
  checkUserInGroup,
  removeUserFromGroup,
  resetSession,
}

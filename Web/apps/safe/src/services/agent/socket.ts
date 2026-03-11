import { io, Socket } from 'socket.io-client'
import type {
  ConnectionEstablishedData,
  MessageEvent,
  UserMessagePayload,
  UserSelectionPayload,
  UserConfirmationPayload,
  UserCancelPayload,
} from './types'

export class AgentSocketService {
  private socket: Socket | null = null
  private userId: string = ''
  private userName: string = ''
  private sessionId: string = ''

  // Event handlers
  private onConnectionEstablished?: (data: ConnectionEstablishedData) => void
  private onMessage?: (data: MessageEvent) => void
  private onDisconnect?: () => void
  private onError?: (error: any) => void

  constructor() {}

  /**
   * Connect to WebSocket server
   */
  connect(userId: string, userName: string = '') {
    if (this.socket?.connected) {
      console.warn('Socket already connected')
      return
    }

    this.userId = userId
    this.userName = userName

    // Use relative path for proxy in development, or configured URL in production
    const baseURL = import.meta.env.VITE_AGENT_BASE_URL || ''
    // const path = import.meta.env.VITE_AGENT_PATH || '/agent/ops/api/v1/agent/ops/socket.io'

    const path = import.meta.env.VITE_AGENT_PATH || '/api/v1/agent/ops/socket.io'

    this.socket = io(baseURL, {
      path: path,
      query: {
        userid: userId,
        username: userName,
      },
      // Use websocket directly
      transports: ['websocket'],
      reconnection: true,
      reconnectionAttempts: 5,
      reconnectionDelay: 1000,
      reconnectionDelayMax: 5000,
      timeout: 10000,
      autoConnect: true,
      // Force websocket upgrade
      upgrade: false,
    })

    this.setupEventListeners()
  }

  /**
   * Setup socket event listeners
   */
  private setupEventListeners() {
    if (!this.socket) return

    // Connect event
    this.socket.on('connect', () => {
    })

    // Connection established
    this.socket.on('connection_established', (data: ConnectionEstablishedData) => {
      this.sessionId = data.session_id
      this.onConnectionEstablished?.(data)
    })

    // Message event
    this.socket.on('message', (data: MessageEvent) => {
      this.onMessage?.(data)
    })

    // Disconnect
    this.socket.on('disconnect', (reason: string) => {
      this.onDisconnect?.()
    })

    // Error
    this.socket.on('error', (error: any) => {
      console.error('Socket error:', error)
      this.onError?.(error)
    })

    // Connect error
    this.socket.on('connect_error', (error: any) => {
      console.error('Socket connect error:', error)
      console.error('Please ensure the Agent backend is running')
      this.onError?.(error)
    })

    // Reconnect attempt
    this.socket.io.on('reconnect_attempt', (attempt: number) => {
    })

    // Reconnect failed
    this.socket.io.on('reconnect_failed', () => {
      console.error('Reconnection failed after max attempts')
      this.onError?.(new Error('Failed to reconnect to Agent backend'))
    })
  }

  /**
   * Send user message
   */
  sendMessage(message: string, operationId?: string) {
    if (!this.socket?.connected) {
      throw new Error('Socket not connected')
    }

    const payload: UserMessagePayload = {
      message,
      operation_id: operationId,
    }

    this.socket.emit('user_message', payload)
  }

  /**
   * Send user selection
   */
  sendSelection(selections: Record<string, any>, operationId?: string) {
    if (!this.socket?.connected) {
      throw new Error('Socket not connected')
    }

    const payload: UserSelectionPayload = {
      selections,
      operation_id: operationId,
    }

    this.socket.emit('user_selection', payload)
  }

  /**
   * Send user confirmation
   */
  sendConfirmation(approved: boolean, confirmationId?: string, operationId?: string) {
    if (!this.socket?.connected) {
      throw new Error('Socket not connected')
    }

    const payload: UserConfirmationPayload = {
      approved,
      confirmation_id: confirmationId,
      operation_id: operationId,
    }

    this.socket.emit('user_confirmation', payload)
  }

  /**
   * Cancel current operation
   */
  cancelOperation(operationId?: string) {
    if (!this.socket?.connected) {
      throw new Error('Socket not connected')
    }

    const payload: UserCancelPayload = {
      operation_id: operationId,
    }

    this.socket.emit('user_cancel', payload)
  }

  /**
   * Disconnect socket
   */
  disconnect() {
    if (this.socket) {
      this.socket.disconnect()
      this.socket = null
    }
  }

  /**
   * Check if socket is connected
   */
  isConnected(): boolean {
    return this.socket?.connected || false
  }

  /**
   * Get session ID
   */
  getSessionId(): string {
    return this.sessionId
  }

  /**
   * Set event handlers
   */
  setEventHandlers(handlers: {
    onConnectionEstablished?: (data: ConnectionEstablishedData) => void
    onMessage?: (data: MessageEvent) => void
    onDisconnect?: () => void
    onError?: (error: any) => void
  }) {
    this.onConnectionEstablished = handlers.onConnectionEstablished
    this.onMessage = handlers.onMessage
    this.onDisconnect = handlers.onDisconnect
    this.onError = handlers.onError
  }
}

// Export singleton instance
export const agentSocket = new AgentSocketService()

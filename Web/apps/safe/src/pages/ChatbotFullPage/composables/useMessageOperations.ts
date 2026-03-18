import type { Message } from '../types'
import type {
  WorkflowMessageData,
  ActionMessageData,
  ConfirmMessageData,
} from '@/services/agent'

// Serialize agent data for saving
export function serializeAgentData(message: Message): string | null {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const agentData: Record<string, any> = {}

  if (message.workflow) {
    agentData.workflow = message.workflow
  }

  if (message.actions && message.actions.length > 0) {
    agentData.actions = message.actions
  }

  if (message.confirmData) {
    agentData.confirmData = message.confirmData
  }

  if (message.confirmedSelections) {
    agentData.confirmedSelections = message.confirmedSelections
  }

  if (message.savedSelectionConfirm) {
    agentData.savedSelectionConfirm = message.savedSelectionConfirm
  }

  return Object.keys(agentData).length > 0 ? JSON.stringify(agentData) : null
}

// Deserialize agent data when loading
export function deserializeAgentData(thinkingStr: string | null): Partial<Message> {
  if (!thinkingStr) return {}

  try {
    const agentData = JSON.parse(thinkingStr)
    const result: Partial<Message> = {}

    if (agentData.workflow) {
      result.workflow = agentData.workflow as WorkflowMessageData
    }

    if (agentData.actions) {
      result.actions = agentData.actions as ActionMessageData[]
    }

    if (agentData.confirmData) {
      result.confirmData = agentData.confirmData as ConfirmMessageData
      result.confirmReadonly = true
    }

    if (agentData.confirmedSelections) {
      result.confirmedSelections = agentData.confirmedSelections
    }

    if (agentData.savedSelectionConfirm) {
      result.savedSelectionConfirm = agentData.savedSelectionConfirm
    }

    return result
  } catch (_e) {
    // If parsing fails, treat as regular thinking content
    return { thinking: thinkingStr, thinkingExpanded: false }
  }
}

// Generate conversation ID
export function generateConversationId(): string {
  const timestamp = Date.now()
  const randomStr = Math.random().toString(36).substring(2, 10)
  return `${timestamp}_${randomStr}`
}

// Generate operation ID
export function generateOperationId(): string {
  return `op-${Date.now()}-${Math.random().toString(36).substring(2, 10)}`
}

import type {
  WorkflowMessageData,
  ActionMessageData,
  ConfirmMessageData,
} from '@/services/agent'
import type { SourceItem } from '@/services/chatbot'

export interface Message {
  role: 'user' | 'assistant'
  content: string
  messageId?: number
  agentHasSteps?: boolean
  agentSaved?: boolean
  statusMessages?: string[]
  thinking?: string
  thinkingExpanded?: boolean
  thinkingTime?: number
  thinkingStartTime?: number
  sources?: SourceItem[]
  sourcesLoading?: boolean
  voteType?: 'up' | 'down' | null
  feedbackId?: number | null
  showFeedbackForm?: boolean
  selectedReasons?: string[]
  customReason?: string
  // Agent mode fields
  workflow?: WorkflowMessageData
  actions?: ActionMessageData[]
  confirmData?: ConfirmMessageData
  confirmLoading?: boolean
  confirmReadonly?: boolean
  confirmedSelections?: Record<string, unknown>
  savedSelectionConfirm?: ConfirmMessageData
}

export interface HistoryItem {
  question: string
  answer: string
}

// Connection types
export interface ConnectionEstablishedData {
  session_id: string
  user_id: string
  user_name: string
}

// Message types
export interface MessageEvent {
  type: 'content' | 'workflow' | 'action' | 'confirm' | 'error' | 'complete' | 'timeout'
  operation_id?: string
  data: any
}

export interface ContentMessageData {
  text: string
  streaming?: boolean
  done?: boolean
  block_style?: string
}

export interface WorkflowMessageData {
  workflow_id: string
  workflow_name: string
  steps: WorkflowStep[]
  current_step: number
}

export interface WorkflowStep {
  name: string
  status: 'pending' | 'running' | 'success' | 'failed'
}

export interface ActionMessageData {
  action_type: 'api_call' | 'llm_call' | 'processing'
  action_name: string
  status: 'running' | 'success' | 'failed'
  details?: Record<string, any>
  timestamp: number
}

export interface ConfirmMessageData {
  id: string
  title: string
  message: string
  confirm_type: 'selection' | 'execution'
  selections?: Record<string, SelectionField>
  details?: Record<string, any>
}

export interface SelectionField {
  type?: 'input' | 'select' | 'multi-select' // Optional, can be inferred from multiple/options
  label: string
  options?: SelectionOption[]
  multiple?: boolean // true for multi-select, false/undefined for single select
  required?: boolean
  placeholder?: string
  default?: string | string[] | null
}

export interface SelectionOption {
  value: string
  label: string
  description?: string
}

export interface ErrorMessageData {
  message: string
  details?: any
}

export interface TimeoutMessageData {
  message: string
  timeout_seconds?: number
}

export interface CompleteMessageData {
  result?: any
  message?: string
}

// User payload types
export interface UserMessagePayload {
  message: string
  operation_id?: string
}

export interface UserSelectionPayload {
  selections: Record<string, any>
  operation_id?: string
}

export interface UserConfirmationPayload {
  approved: boolean
  confirmation_id?: string
  operation_id?: string
}

export interface UserCancelPayload {
  operation_id?: string
}

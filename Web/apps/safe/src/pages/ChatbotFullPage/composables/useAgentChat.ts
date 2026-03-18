import { ref } from 'vue'
import type { Ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  agentSocket,
  type WorkflowMessageData,
  type ActionMessageData,
  type ConfirmMessageData,
  type MessageEvent,
  type TimeoutMessageData,
} from '@/services/agent'
import { saveMessage } from '@/services/chatbot'
import type { Message } from '../types'

export function useAgentChat(
  messages: Ref<Message[]>,
  currentConversationId: Ref<string>,
  loading: Ref<boolean>,
  currentOperationId: Ref<string>,
  serializeAgentData: (message: Message) => string | null,
) {
  const agentConnected = ref(false)
  const agentSessionId = ref('')

  // Agent: Handle message event
  const handleAgentMessage = (event: MessageEvent) => {
    const { type, data, operation_id } = event

    // Ignore messages if operation was cancelled or from different operation
    if (operation_id) {
      if (!currentOperationId.value || operation_id !== currentOperationId.value) {
        return
      }
    }

    // Find the last assistant message
    const lastAssistantIndex = messages.value.length - 1
    const lastMessage = messages.value[lastAssistantIndex]

    if (!lastMessage || lastMessage.role !== 'assistant') {
      console.warn('No assistant message to update')
      return
    }

    switch (type) {
      case 'content':
        handleContentMessage(lastAssistantIndex, data)
        break
      case 'workflow':
        handleWorkflowMessage(lastAssistantIndex, data)
        break
      case 'action':
        handleActionMessage(lastAssistantIndex, data)
        break
      case 'confirm':
        handleConfirmMessage(data)
        break
      case 'error':
        handleErrorMessage(lastAssistantIndex, data)
        break
      case 'timeout':
        handleTimeoutMessage(lastAssistantIndex, data)
        break
      case 'complete':
        handleCompleteMessage(lastAssistantIndex, data)
        break
    }
  }

  // Agent: Handle content message
  const handleContentMessage = async (messageIndex: number, data: Record<string, unknown>) => {
    const message = messages.value[messageIndex]
    if (!message) return

    const text = typeof data.text === 'string' ? data.text : ''

    if (data.streaming) {
      message.content += text
    } else if (text) {
      message.content = text
    }

    if (data.done) {
      loading.value = false

      // Save assistant message when done (only for non-step messages)
      if (currentConversationId.value && message && !message.agentHasSteps && !message.agentSaved) {
        try {
          const questionMessageId =
            messageIndex > 0 ? messages.value[messageIndex - 1]?.messageId : undefined

          const saveAssistantMsgResponse = await saveMessage({
            conversation_id: currentConversationId.value,
            role: 'assistant',
            content: message.content,
            thinking: serializeAgentData(message),
            question_message_id: questionMessageId,
            message_type: 'Agent',
          })
          messages.value[messageIndex].messageId = saveAssistantMsgResponse.data.id
          messages.value[messageIndex].agentSaved = true
        } catch (error) {
          console.error('Failed to save assistant message:', error)
        }
      }
    }
  }

  // Agent: Handle workflow message
  const handleWorkflowMessage = (messageIndex: number, data: WorkflowMessageData) => {
    const message = messages.value[messageIndex]
    if (!message) return

    message.workflow = data
    message.agentHasSteps = true
  }

  // Agent: Handle action message
  const handleActionMessage = (messageIndex: number, data: ActionMessageData) => {
    const message = messages.value[messageIndex]
    if (!message) return

    if (!message.actions) {
      message.actions = []
    }

    const existingIndex = message.actions.findIndex((a) => a.action_name === data.action_name)
    if (existingIndex >= 0) {
      message.actions[existingIndex] = data
    } else {
      message.actions.push(data)
    }
    message.agentHasSteps = true
  }

  // Agent: Handle confirm message
  const handleConfirmMessage = (data: ConfirmMessageData) => {
    const lastAssistantIndex = messages.value.length - 1
    const lastMessage = messages.value[lastAssistantIndex]

    if (lastMessage && lastMessage.role === 'assistant') {
      lastMessage.confirmData = data
      lastMessage.confirmLoading = false
      lastMessage.agentHasSteps = true
    }

    loading.value = false
  }

  // Agent: Handle error message
  const handleErrorMessage = async (messageIndex: number, data: Record<string, unknown>) => {
    const message = messages.value[messageIndex]
    if (!message) return

    const errorMsg = typeof data.message === 'string' ? data.message : 'Unknown error'
    message.content = `❌ Error: ${errorMsg}`
    if (data.details) {
      message.content += `\n\nDetails: ${JSON.stringify(data.details, null, 2)}`
    }
    loading.value = false
    ElMessage.error(errorMsg)

    // Save error message
    if (currentConversationId.value && !message.agentSaved) {
      try {
        const questionMessageId =
          messageIndex > 0 ? messages.value[messageIndex - 1]?.messageId : undefined

        const saveAssistantMsgResponse = await saveMessage({
          conversation_id: currentConversationId.value,
          role: 'assistant',
          content: message.content,
          thinking: serializeAgentData(message),
          question_message_id: questionMessageId,
          message_type: 'Agent',
        })
        messages.value[messageIndex].messageId = saveAssistantMsgResponse.data.id
        messages.value[messageIndex].agentSaved = true
      } catch (error) {
        console.error('Failed to save error message:', error)
      }
    }
  }

  // Agent: Handle timeout message
  const handleTimeoutMessage = async (messageIndex: number, data: TimeoutMessageData) => {
    const message = messages.value[messageIndex]
    if (!message) return

    const rawMessage = typeof data.message === 'string' ? data.message : 'Operation timed out'
    const timeoutPrefix = rawMessage.startsWith('⚠️') ? '' : '⚠️ '
    message.content = `${timeoutPrefix}${rawMessage}`
    loading.value = false
    ElMessage.warning(rawMessage)

    // Save timeout message
    if (currentConversationId.value && !message.agentSaved) {
      try {
        const questionMessageId =
          messageIndex > 0 ? messages.value[messageIndex - 1]?.messageId : undefined

        const saveAssistantMsgResponse = await saveMessage({
          conversation_id: currentConversationId.value,
          role: 'assistant',
          content: message.content,
          thinking: serializeAgentData(message),
          question_message_id: questionMessageId,
          message_type: 'Agent',
        })
        messages.value[messageIndex].messageId = saveAssistantMsgResponse.data.id
        messages.value[messageIndex].agentSaved = true
      } catch (error) {
        console.error('Failed to save timeout message:', error)
      }
    }
  }

  // Agent: Handle complete message
  const handleCompleteMessage = (messageIndex: number, data: Record<string, unknown>) => {
    loading.value = false

    const message = messages.value[messageIndex]
    if (message && message.savedSelectionConfirm && message.confirmedSelections) {
      message.confirmData = message.savedSelectionConfirm
      message.confirmReadonly = true
    }

    // Save assistant message (only for messages with steps)
    if (currentConversationId.value && message && message.agentHasSteps && !message.agentSaved) {
      ;(async () => {
        try {
          const questionMessageId =
            messageIndex > 0 ? messages.value[messageIndex - 1]?.messageId : undefined

          const saveAssistantMsgResponse = await saveMessage({
            conversation_id: currentConversationId.value,
            role: 'assistant',
            content: message.content,
            thinking: serializeAgentData(message),
            question_message_id: questionMessageId,
            message_type: 'Agent',
          })
          messages.value[messageIndex].messageId = saveAssistantMsgResponse.data.id
          messages.value[messageIndex].agentSaved = true
        } catch (error) {
          console.error('Failed to save assistant message:', error)
        }
      })()
    }
  }

  // Agent: Handle inline confirm submit
  const handleInlineConfirmSubmit = (
    messageIndex: number,
    data: { selections?: Record<string, unknown>; approved?: boolean },
  ) => {
    const message = messages.value[messageIndex]
    if (!message || !message.confirmData) return

    message.confirmLoading = true

    try {
      if (data.selections) {
        agentSocket.sendSelection(data.selections, currentOperationId.value)
        message.savedSelectionConfirm = { ...message.confirmData }
        message.confirmedSelections = data.selections
        message.confirmData = undefined
        message.confirmLoading = false
      } else if (data.approved !== undefined) {
        agentSocket.sendConfirmation(
          data.approved,
          message.confirmData.id,
          currentOperationId.value,
        )
        message.confirmData = undefined
      }

      loading.value = true
    } catch (error) {
      console.error('Failed to send response:', error)
      ElMessage.error('Failed to send response')
      message.confirmLoading = false
    }
  }

  // Agent: Handle inline confirm cancel
  const handleInlineConfirmCancel = async (
    messageIndex: number,
    payload?: { confirmType?: string },
  ) => {
    const message = messages.value[messageIndex]
    if (!message) return

    const confirmType = payload?.confirmType ?? message.confirmData?.confirm_type

    if (confirmType === 'execution') {
      handleInlineConfirmSubmit(messageIndex, { approved: false })
    } else {
      // Notify backend user cancelled (agent mode only)
      if (agentConnected.value && currentOperationId.value) {
        try {
          agentSocket.cancelOperation(currentOperationId.value)
        } catch (error) {
          console.error('Failed to send user cancel:', error)
        }
      }

      message.confirmData = undefined
      loading.value = false

      // Save cancelled selection
      if (currentConversationId.value && !message.agentSaved) {
        try {
          const questionMessageId =
            messageIndex > 0 ? messages.value[messageIndex - 1]?.messageId : undefined

          const cancelledContent = message.content
            ? `${message.content}\n\n_[Selection cancelled by user]_`
            : '_[Selection cancelled by user]_'

          const saveAssistantMsgResponse = await saveMessage({
            conversation_id: currentConversationId.value,
            role: 'assistant',
            content: cancelledContent,
            thinking: serializeAgentData(message),
            question_message_id: questionMessageId,
            message_type: 'Agent',
          })
          messages.value[messageIndex].messageId = saveAssistantMsgResponse.data.id
          messages.value[messageIndex].agentSaved = true
        } catch (error) {
          console.error('Failed to save cancelled selection message:', error)
        }
      }
    }
  }

  return {
    agentConnected,
    agentSessionId,
    handleAgentMessage,
    handleInlineConfirmSubmit,
    handleInlineConfirmCancel,
  }
}

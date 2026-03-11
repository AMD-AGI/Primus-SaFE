import { ref, nextTick } from 'vue'
import type { Ref } from 'vue'
import { ElMessage } from 'element-plus'
import { chatBotAsk, createConversation, saveMessage, type SourceItem } from '@/services/chatbot'
import type { Message, HistoryItem } from '../types'
import { generateConversationId } from './useMessageOperations'

export function useAskChat(
  messages: Ref<Message[]>,
  currentConversationId: Ref<string>,
  loading: Ref<boolean>,
  messagesContainer: Ref<HTMLElement | undefined>,
) {
  const enableThinking = ref(false)
  let abortController: AbortController | null = null

  // Build history from messages
  const buildHistory = (): HistoryItem[] => {
    const history: HistoryItem[] = []
    for (let i = 0; i < messages.value.length - 1; i += 2) {
      if (messages.value[i].role === 'user' && messages.value[i + 1]?.role === 'assistant') {
        history.push({
          question: messages.value[i].content,
          answer: messages.value[i + 1].content,
        })
      }
    }
    return history
  }

  // Scroll to new question
  const scrollToNewQuestion = () => {
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    }
  }

  // Send Ask message
  const sendAskMessage = async (query: string) => {
    // Add user message
    messages.value.push({
      role: 'user',
      content: query,
    })

    // Create assistant message placeholder
    const assistantMessageIndex = messages.value.length
    messages.value.push({
      role: 'assistant',
      content: '',
      statusMessages: [],
      thinking: '',
      thinkingExpanded: true,
      thinkingTime: 0,
      thinkingStartTime: undefined,
      sources: [],
      voteType: null,
      feedbackId: null,
    })

    await nextTick()
    scrollToNewQuestion()

    loading.value = true
    abortController = new AbortController()

    // Create conversation if needed
    if (!currentConversationId.value) {
      try {
        const conversationId = generateConversationId()
        const title = query.slice(0, 50)

        const response = await createConversation({
          conversation_id: conversationId,
          title: title,
        })

        currentConversationId.value = response.data.conversation_id
      } catch (error) {
        console.error('Failed to create conversation:', error)
        ElMessage.error('Failed to create conversation')
        messages.value.splice(messages.value.length - 2, 2)
        loading.value = false
        return
      }
    }

    // Save user message
    try {
      const saveUserMsgResponse = await saveMessage({
        conversation_id: currentConversationId.value,
        role: 'user',
        content: query,
        thinking: null,
        message_type: 'Ask',
      })
      messages.value[messages.value.length - 2].messageId = saveUserMsgResponse.data.id
    } catch (error) {
      console.error('Failed to save user message:', error)
    }

    try {
      const history = buildHistory()

      await chatBotAsk(
        {
          question: query,
          stream: true,
          history: history,
          enable_thinking: enableThinking.value,
        },
        (content: string) => {
          messages.value[assistantMessageIndex].content += content
        },
        (error: unknown) => {
          console.error('Chat error:', error)
          messages.value[assistantMessageIndex].content =
            'Sorry, I encountered an error. Please try again later.'
          loading.value = false
        },
        async () => {
          loading.value = false

          // Save assistant message
          try {
            const sourceRefs = messages.value[assistantMessageIndex].sources
              ? messages.value[assistantMessageIndex].sources!.map((source) => ({
                  source: 'qa_items',
                  item_id: source.item_id,
                }))
              : []

            const questionMessageId = messages.value[assistantMessageIndex - 1]?.messageId

            const saveAssistantMsgResponse = await saveMessage({
              conversation_id: currentConversationId.value,
              role: 'assistant',
              content: messages.value[assistantMessageIndex].content,
              thinking: messages.value[assistantMessageIndex].thinking || null,
              source_refs: sourceRefs,
              question_message_id: questionMessageId,
              message_type: 'Ask',
            })
            messages.value[assistantMessageIndex].messageId = saveAssistantMsgResponse.data.id
          } catch (error) {
            console.error('Failed to save assistant message:', error)
          }
        },
        abortController.signal,
        (statusMessage: string) => {
          if (!messages.value[assistantMessageIndex].statusMessages) {
            messages.value[assistantMessageIndex].statusMessages = []
          }
          messages.value[assistantMessageIndex].statusMessages!.push(statusMessage)
        },
        (sources: SourceItem[]) => {
          messages.value[assistantMessageIndex].sources = sources
        },
        (thinkingContent: string) => {
          if (!messages.value[assistantMessageIndex].thinking) {
            messages.value[assistantMessageIndex].thinking = ''
            messages.value[assistantMessageIndex].thinkingStartTime = Date.now()
          }
          messages.value[assistantMessageIndex].thinking += thinkingContent

          if (messages.value[assistantMessageIndex].thinkingStartTime) {
            messages.value[assistantMessageIndex].thinkingTime =
              Date.now() - messages.value[assistantMessageIndex].thinkingStartTime
          }
        },
      )
    } catch (err) {
      console.error('Send message error:', err)
      messages.value[assistantMessageIndex].content =
        'Sorry, I encountered an error. Please try again later.'
      loading.value = false
    }
  }

  // Stop generation (Ask mode)
  const stopAskGeneration = () => {
    if (abortController) {
      abortController.abort()
      loading.value = false
      ElMessage.info('Generation stopped')
    }
  }

  return {
    enableThinking,
    sendAskMessage,
    stopAskGeneration,
  }
}

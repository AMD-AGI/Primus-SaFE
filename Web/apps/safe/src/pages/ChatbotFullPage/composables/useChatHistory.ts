import { ref, nextTick } from 'vue'
import type { Ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  getConversationList,
  getMessageList,
  batchGetMessages,
  getQAItemDetail,
  type ConversationListItem,
} from '@/services/chatbot'
import type { Message } from '../types'
import { deserializeAgentData } from './useMessageOperations'

export function useChatHistory(
  messages: Ref<Message[]>,
  currentConversationId: Ref<string>,
  isSharedMode: Ref<boolean>,
  messagesContainer: Ref<HTMLElement | undefined>,
) {
  const conversationList = ref<ConversationListItem[]>([])
  const loadingHistory = ref(false)
  const loadingMoreConversations = ref(false)
  const conversationCurrentPage = ref(1)
  const conversationPageSize = ref(20)
  const conversationHasNoMore = ref(false)

  const loadingMoreMessages = ref(false)
  const messageCurrentPage = ref(1)
  const messagePageSize = ref(50)
  const messageHasNoMore = ref(false)

  // Fetch conversation list
  const fetchConversationList = async (reset = true) => {
    if (reset) {
      loadingHistory.value = true
      conversationCurrentPage.value = 1
      conversationHasNoMore.value = false
    } else {
      loadingMoreConversations.value = true
    }

    try {
      const response = await getConversationList({
        page: conversationCurrentPage.value,
        page_size: conversationPageSize.value,
      })

      if (reset) {
        conversationList.value = response.data.items
      } else {
        conversationList.value = [...conversationList.value, ...response.data.items]
      }

      const { page, page_size, total } = response.data.pagination
      conversationHasNoMore.value = page * page_size >= total
    } catch (error) {
      console.error('Failed to fetch conversation list:', error)
      ElMessage.error('Failed to load conversation history')
    } finally {
      loadingHistory.value = false
      loadingMoreConversations.value = false
    }
  }

  // Load more conversations
  const loadMoreConversations = async () => {
    if (loadingMoreConversations.value || conversationHasNoMore.value) {
      return
    }

    conversationCurrentPage.value += 1
    await fetchConversationList(false)
  }

  // Load conversation
  const loadConversation = async (conversationId: string, silent = false) => {
    try {
      messageCurrentPage.value = 1
      messageHasNoMore.value = false

      const response = await getMessageList(conversationId, {
        page: messageCurrentPage.value,
        page_size: messagePageSize.value,
      })
      const messageData = response.data.items

      messages.value = messageData.map((msg) => {
        const baseMessage = {
          role: msg.role as 'user' | 'assistant',
          content: msg.content,
          messageId: msg.id,
          statusMessages: [],
          thinking: '',
          thinkingExpanded: false,
          thinkingTime: 0,
          sources: msg.sources || [],
          sourcesLoading: !msg.sources && msg.source_refs && msg.source_refs.length > 0,
          voteType: msg.user_vote_type || null,
          feedbackId: msg.feedback_id || null,
        }

        const agentData = deserializeAgentData(msg.thinking)

        return {
          ...baseMessage,
          ...agentData,
        }
      })

      const { page, page_size, total } = response.data.pagination
      messageHasNoMore.value = page * page_size >= total

      currentConversationId.value = conversationId
      isSharedMode.value = false

      await nextTick()
      if (messagesContainer.value) {
        messagesContainer.value.scrollTop = 0
      }

      if (!silent) {
        ElMessage.success('Conversation loaded')
      }

      // Load sources asynchronously
      messageData.forEach(async (msg, index) => {
        if (!msg.sources && msg.source_refs && msg.source_refs.length > 0) {
          const sources: any[] = []
          for (const ref of msg.source_refs) {
            if (ref.source === 'qa_items' && ref.item_id) {
              try {
                const res = await getQAItemDetail(ref.item_id)
                const primaryQuestion =
                  res.questions?.find((q) => q.is_primary)?.question ??
                  res.questions?.[0]?.question ??
                  ''
                sources.push({
                  type: 'qa_item',
                  collection_id: res.answer.collection_id,
                  collection_name: res.answer.collection_name || 'SaFE-QA',
                  item_id: res.answer.id,
                  question: primaryQuestion,
                  similarity: 0.95,
                })
              } catch (error) {
                console.error(`Failed to load source ${ref.item_id}:`, error)
              }
            }
          }

          if (messages.value[index]?.messageId === msg.id) {
            messages.value[index].sources = sources
            messages.value[index].sourcesLoading = false
          }
        }
      })
    } catch (error) {
      console.error('Failed to load conversation:', error)
      ElMessage.error('Failed to load conversation')
    }
  }

  // Load more messages
  const loadMoreMessages = async () => {
    if (loadingMoreMessages.value || messageHasNoMore.value || !currentConversationId.value) {
      return
    }

    loadingMoreMessages.value = true

    try {
      messageCurrentPage.value += 1

      const response = await getMessageList(currentConversationId.value, {
        page: messageCurrentPage.value,
        page_size: messagePageSize.value,
      })
      const messageData = response.data.items

      const currentMessagesCount = messages.value.length

      const newMessages = messageData.map((msg) => {
        const baseMessage = {
          role: msg.role as 'user' | 'assistant',
          content: msg.content,
          messageId: msg.id,
          statusMessages: [],
          thinking: '',
          thinkingExpanded: false,
          thinkingTime: 0,
          sources: msg.sources || [],
          sourcesLoading: !msg.sources && msg.source_refs && msg.source_refs.length > 0,
          voteType: msg.user_vote_type || null,
          feedbackId: msg.feedback_id || null,
        }

        const agentData = deserializeAgentData(msg.thinking)

        return {
          ...baseMessage,
          ...agentData,
        }
      })

      messages.value = [...messages.value, ...newMessages]

      const { page, page_size, total } = response.data.pagination
      messageHasNoMore.value = page * page_size >= total

      // Load sources asynchronously
      messageData.forEach(async (msg, index) => {
        if (!msg.sources && msg.source_refs && msg.source_refs.length > 0) {
          const sources: any[] = []
          for (const ref of msg.source_refs) {
            if (ref.source === 'qa_items' && ref.item_id) {
              try {
                const res = await getQAItemDetail(ref.item_id)
                const primaryQuestion =
                  res.questions?.find((q) => q.is_primary)?.question ??
                  res.questions?.[0]?.question ??
                  ''
                sources.push({
                  type: 'qa_item',
                  collection_id: res.answer.collection_id,
                  collection_name: res.answer.collection_name || 'SaFE-QA',
                  item_id: res.answer.id,
                  question: primaryQuestion,
                  similarity: 0.95,
                })
              } catch (error) {
                console.error(`Failed to load source ${ref.item_id}:`, error)
              }
            }
          }

          const absoluteIndex = currentMessagesCount + index
          if (messages.value[absoluteIndex]?.messageId === msg.id) {
            messages.value[absoluteIndex].sources = sources
            messages.value[absoluteIndex].sourcesLoading = false
          }
        }
      })
    } catch (error) {
      console.error('Failed to load more messages:', error)
      ElMessage.error('Failed to load more messages')
    } finally {
      loadingMoreMessages.value = false
    }
  }

  // Load shared messages
  const loadSharedMessages = async (questionMessageId: number, answerMessageId: number) => {
    try {
      const response = await batchGetMessages([questionMessageId, answerMessageId])

      if (!response.success || response.data.length !== 2) {
        ElMessage.error('Invalid share link')
        return
      }

      const questionMsg = response.data.find((msg) => msg.id === questionMessageId)
      const answerMsg = response.data.find((msg) => msg.id === answerMessageId)

      if (
        !questionMsg ||
        !answerMsg ||
        questionMsg.role !== 'user' ||
        answerMsg.role !== 'assistant'
      ) {
        ElMessage.error('Invalid share link')
        return
      }

      const answerAgentData = deserializeAgentData(answerMsg.thinking)

      messages.value = [
        {
          role: 'user',
          content: questionMsg.content,
          messageId: questionMsg.id,
        },
        {
          role: 'assistant',
          content: answerMsg.content,
          messageId: answerMsg.id,
          thinking: '',
          thinkingExpanded: false,
          statusMessages: [],
          sources: answerMsg.sources || [],
          voteType: answerMsg.user_vote_type || null,
          feedbackId: answerMsg.feedback_id || null,
          ...answerAgentData,
        },
      ]

      isSharedMode.value = true
      currentConversationId.value = ''

      await nextTick()
      if (messagesContainer.value) {
        messagesContainer.value.scrollTop = 0
      }
    } catch (error) {
      console.error('Failed to load shared messages:', error)
      ElMessage.error('Failed to load shared content')
    }
  }

  return {
    conversationList,
    loadingHistory,
    loadingMoreConversations,
    conversationHasNoMore,
    loadingMoreMessages,
    messageHasNoMore,
    fetchConversationList,
    loadMoreConversations,
    loadConversation,
    loadMoreMessages,
    loadSharedMessages,
  }
}

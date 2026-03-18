import type { Ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  submitFeedback,
  cancelVote,
  updateConversation,
  deleteConversation,
} from '@/services/chatbot'
import { copyText } from '@/utils'
import type { Message } from '../types'

export function useFeedback(
  messages: Ref<Message[]>,
  currentConversationId: Ref<string>,
  fetchConversationList: () => Promise<void>,
) {
  const router = useRouter()

  const feedbackReasons = [
    'Incorrect information',
    'Incomplete answer',
    'Not relevant to question',
    'Poor formatting',
    'Incorrect workflow',
    'Incorrect action result',
    'Other',
  ]

  // Toggle thinking expansion
  const toggleThinking = (message: Message) => {
    message.thinkingExpanded = !message.thinkingExpanded
  }

  // Handle vote
  const handleVote = async (messageIndex: number, voteType: 'up' | 'down') => {
    const message = messages.value[messageIndex]

    if (!message || !message.messageId) {
      ElMessage.warning('Cannot vote on this message')
      return
    }

    try {
      if (message.voteType === voteType) {
        // Cancel vote
        await cancelVote({ message_id: message.messageId })
        message.voteType = null
        message.feedbackId = null
        message.showFeedbackForm = false
        ElMessage.success('Vote cancelled')
      } else {
        // Submit vote
        const response = await submitFeedback({
          vote_type: voteType,
          message_id: message.messageId,
        })

        message.voteType = voteType
        message.feedbackId = response.data.id

        if (voteType === 'down') {
          message.showFeedbackForm = true
        } else {
          ElMessage.success('Thank you for your feedback!')
        }
      }
    } catch (error) {
      console.error('Vote error:', error)
      ElMessage.error('Failed to submit vote')
    }
  }

  // Submit feedback reasons
  const submitFeedbackReasons = async (messageIndex: number) => {
    const message = messages.value[messageIndex]
    if (!message || !message.feedbackId) return

    try {
      const reasons = [...(message.selectedReasons || [])]
      if (message.customReason?.trim()) {
        reasons.push(message.customReason.trim())
      }

      await submitFeedback({
        vote_type: 'down',
        message_id: message.messageId!,
        reason: reasons.join('; '),
      })

      message.showFeedbackForm = false
      message.selectedReasons = []
      message.customReason = ''
      ElMessage.success('Thank you for your detailed feedback!')
    } catch (error) {
      console.error('Submit feedback error:', error)
      ElMessage.error('Failed to submit feedback')
    }
  }

  // Cancel feedback form
  const cancelFeedbackForm = (messageIndex: number) => {
    const message = messages.value[messageIndex]
    message.showFeedbackForm = false
    message.selectedReasons = []
    message.customReason = ''
  }

  // Handle share
  const handleShare = async (questionMessageId: number, answerMessageId: number) => {
    try {
      const baseUrl = window.location.origin
      const shareUrl = `${baseUrl}${router.currentRoute.value.path}?share=1&qid=${questionMessageId}&aid=${answerMessageId}`
      await copyText(shareUrl)
    } catch (error) {
      console.error('Failed to copy share link:', error)
    }
  }

  // Edit conversation title
  const handleEditTitle = async (item: any) => {
    try {
      const { value: newTitle } = (await ElMessageBox.prompt('Enter new title', 'Edit Title', {
        confirmButtonText: 'Save',
        cancelButtonText: 'Cancel',
        inputValue: item.title,
        inputPattern: /.+/,
        inputErrorMessage: 'Title cannot be empty',
      })) as { value: string }

      if (newTitle) {
        await updateConversation(item.conversation_id, { title: newTitle })
        item.title = newTitle
        ElMessage.success('Title updated')
        await fetchConversationList()
      }
    } catch (err) {
      if (err !== 'cancel') {
        console.error('Update title error:', err)
        ElMessage.error('Failed to update title')
      }
    }
  }

  // Delete conversation
  const handleDeleteConversation = async (item: any) => {
    try {
      await ElMessageBox.confirm(
        'Are you sure you want to delete this conversation? This action cannot be undone.',
        'Confirm Deletion',
        {
          confirmButtonText: 'Delete',
          cancelButtonText: 'Cancel',
          type: 'warning',
        },
      )

      await deleteConversation(item.conversation_id)
      ElMessage.success('Conversation deleted')

      if (currentConversationId.value === item.conversation_id) {
        messages.value = []
        currentConversationId.value = ''
      }

      await fetchConversationList()
    } catch (err) {
      if (err !== 'cancel') {
        console.error('Delete conversation error:', err)
        ElMessage.error('Failed to delete conversation')
      }
    }
  }

  return {
    feedbackReasons,
    toggleThinking,
    handleVote,
    submitFeedbackReasons,
    cancelFeedbackForm,
    handleShare,
    handleEditTitle,
    handleDeleteConversation,
  }
}

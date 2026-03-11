// ChatBot service for floating chat assistant
import request from '@/services/request'

export interface ChatBotRequest {
  question: string
  stream: boolean
  history: HistoryItem[]
  enable_thinking: boolean
}

export interface HistoryItem {
  question: string
  answer: string
}

// ========== Helper Functions ==========

/**
 * Get userId for request headers
 */
function getUserId(): string {
  try {
    const userStr = localStorage.getItem('user')
    if (userStr) {
      const user = JSON.parse(userStr)
      return user.userId || user.id || ''
    }
  } catch (e) {
    console.warn('Failed to get userId from localStorage:', e)
  }
  return ''
}

// ========== Agent QA API Types ==========

export interface CreateConversationRequest {
  conversation_id: string
  title: string
}

export interface ConversationData {
  id: number
  conversation_id: string
  user_id: string
  title: string
  created_at: string
  updated_at: string
  deleted: boolean
}

export interface CreateConversationResponse {
  success: boolean
  data: ConversationData
}

export interface SaveMessageRequest {
  conversation_id: string
  role: 'user' | 'assistant'
  content: string
  thinking: string | null
  source_refs?: SourceRef[] // Reference sources, consistent with ask response sources
  question_message_id?: number // Question ID corresponding to the answer
  message_type?: 'Ask' | 'Agent' // Message type: Ask or Agent
}

export interface MessageData {
  id: number
  conversation_id: string
  role: string
  content: string
  thinking: string | null
  created_at: string
  user_vote_type?: 'up' | 'down' | null // User vote type (synced from backend)
  feedback_id?: number | null // Feedback ID (synced from backend)
  sources?: SourceItem[] // Message sources (synced from backend, full format)
  source_refs?: (SourceRef & Partial<SourceItem>)[] // Message source references (supports both full and simplified formats)
}

export interface SaveMessageResponse {
  success: boolean
  data: MessageData
}

export interface ConversationListItem {
  id: number
  conversation_id: string
  user_id: string
  title: string
  created_at: string
  updated_at: string
  deleted: boolean
}

export interface PaginationInfo {
  page: number
  page_size: number
  total: number
}

export interface ConversationListResponse {
  success: boolean
  data: {
    items: ConversationListItem[]
    pagination: PaginationInfo
  }
}

export interface UpdateConversationRequest {
  title: string
}

export interface UpdateConversationResponse {
  success: boolean
  message: string
}

export interface DeleteConversationResponse {
  success: boolean
  message: string
}

export interface MessageListResponse {
  success: boolean
  data: {
    items: MessageData[]
    pagination: PaginationInfo
  }
}

export interface SourceItem {
  type: string
  collection_id: number
  collection_name: string
  item_id: number
  question: string
  similarity: number
}

// ========== QA Collection API Types ==========

export interface CreateQACollectionRequest {
  name: string
  description?: string
  is_active?: boolean
}

export interface QACollectionData {
  id: number
  user_id: string
  name: string
  description: string
  is_active: boolean
  created_at: string
  updated_at: string
  deleted: boolean
}

export interface QACollectionListItem extends QACollectionData {
  item_count: number
}

export interface CreateQACollectionResponse {
  success: boolean
  data: QACollectionData
}

export interface QACollectionListResponse {
  success: boolean
  data: {
    items: QACollectionListItem[]
    pagination: PaginationInfo
  }
}

export interface QACollectionDetailResponse {
  success: boolean
  data: QACollectionData
}

export interface UpdateQACollectionRequest {
  name?: string
  description?: string
  is_active?: boolean
}

export interface UpdateQACollectionResponse {
  success: boolean
  message: string
}

export interface DeleteQACollectionResponse {
  success: boolean
  message: string
}

// ========== QA Item API Types ==========

export type QAAnswerType = 'plaintext' | 'markdown' | 'richtext'

export type RichTextParagraph = { type: 'paragraph'; content: string }
export type RichTextHeading = { type: 'heading'; level: number; content: string }
export type RichTextList = { type: 'list'; style: 'ordered' | 'unordered'; items: string[] }
export type RichTextListUI = { type: 'list'; style: 'ordered' | 'unordered'; itemsText: string }
export type RichTextCode = { type: 'code'; language: string; content: string }
export type RichTextImage = { type: 'image'; url: string; alt: string; description: string }
export type RichTextBlock =
  | RichTextParagraph
  | RichTextHeading
  | RichTextList
  | RichTextCode
  | RichTextImage
export type RichTextBlockUI =
  | RichTextParagraph
  | RichTextHeading
  | RichTextListUI
  | RichTextCode
  | RichTextImage
export type RichTextDoc = {
  version: '1.0'
  blocks: RichTextBlockUI[]
}
export type RichTextDocPayload = {
  version: '1.0'
  blocks: RichTextBlock[]
}

export interface CreateQAItemRequest {
  collection_id: number
  answer: string
  answer_type: QAAnswerType
  questions: string[]
  primary_question_index?: number
  priority?: 'low' | 'medium' | 'high'
  is_active?: boolean
}

export interface QAItemQuestion {
  id: number
  question: string
  is_primary?: boolean
  match_count?: number
  total_match_count?: number
  has_embedding?: boolean
}

export type QAQuestionVariant = QAItemQuestion

export interface QAItemAnswer {
  id: number
  collection_id: number
  collection_name?: string
  user_id?: string
  user_name?: string
  answer: string
  answer_type?: QAAnswerType
  source?: string
  priority?: 'low' | 'medium' | 'high'
  is_active?: boolean
  has_embedding?: boolean
  deleted?: boolean
  created_at: string
  updated_at: string
}

export interface QAAnswerEntity extends QAItemAnswer {
  question?: string
  primary_question?: string
  primary_question_id?: number
  primaryQuestion?: string
  primaryQuestionId?: number
}

export interface QAAnswerDetailData {
  answer: QAItemAnswer
  questions: QAItemQuestion[]
}

export interface QAItemData {
  answer: QAItemAnswer
  questions: QAItemQuestion[]
}

export interface CreateQAItemResponse {
  success: boolean
  data: QAItemData
}

export interface BatchCreateQAItemRequest {
  collection_id: number
  items: Array<{
    question: string
    answer: string
    source?: string
    priority?: 'low' | 'medium' | 'high'
    is_active?: boolean
  }>
}

export interface BatchCreateQAItemResponse {
  success: boolean
  data: {
    created_count: number
    items: QAItemData[]
  }
}

export interface QAItemListResponse {
  items: QAItemData[]
  page: number
  page_size: number
  total: number
}

export type QAItemDetailResponse = QAAnswerDetailData

export interface QAAnswerDetailResponse {
  success: boolean
  data: QAAnswerDetailData
}

export interface UpdateQAItemRequest {
  answer?: string
  answer_type?: QAAnswerType
  /**
   * New parameter: questions supports object array; new questions have no id
   */
  questions?: Array<{
    id?: number
    question: string
    is_primary: boolean
  }>
  priority?: 'low' | 'medium' | 'high'
  is_active?: boolean
}

export interface UpdateQAItemResponse {
  success: boolean
  message: string
}

export interface DeleteQAItemResponse {
  success: boolean
  message: string
}

export interface GenerateQuestionsRequest {
  answer: string
  max_questions?: number
  primary_question?: string
}

export interface GenerateQuestionsResponse {
  questions: string[]
  is_composite?: boolean
}

export interface UploadQAImageResponse {
  success?: boolean
  url?: string
  path?: string
  message?: string
}

export interface UploadQAFileResponse {
  success?: boolean
  url?: string
  path?: string
  message?: string
}

// ========== QA Item Search API Types ==========

export interface SearchQAItemRequest {
  query: string
  limit?: number
  min_similarity?: number
  collection_id?: number
}

export interface SearchQAItemResult {
  answer_id: number
  question_id: number
  collection_id: number
  collection_name: string
  answer: string
  answer_type: QAAnswerType
  match_count?: number
  question: string
  similarity: number
}

export interface SearchQAItemResponse {
  query: string
  results: SearchQAItemResult[]
  total: number
}

// ========== Health Check API Types ==========

export interface HealthCheckResponse {
  status: string
  version: string
  timestamp: string
}

/**
 * Stream chat with the chatbot
 * @param data Request data
 * @param onMessage Callback when receiving message chunks
 * @param onError Callback when error occurs
 * @param onFinish Callback when stream finishes
 * @param signal AbortSignal for cancelling the request
 * @param onStatus Callback when receiving status messages (displayed as list)
 * @param onSources Callback when receiving knowledge base sources
 * @param onThinking Callback when receiving thinking content (streamed)
 */
export async function chatBotAsk(
  data: ChatBotRequest,
  onMessage: (content: string) => void,
  onError?: (error: unknown) => void,
  onFinish?: () => void,
  signal?: AbortSignal,
  onStatus?: (message: string) => void,
  onSources?: (sources: SourceItem[]) => void,
  onThinking?: (content: string) => void,
) {
  try {
    let userId = ''
    try {
      const userStr = localStorage.getItem('user')
      if (userStr) {
        const user = JSON.parse(userStr)
        userId = user.userId || user.id || ''
      }
    } catch (e) {
      console.warn('Failed to get userId from localStorage:', e)
    }

    const response = await fetch(`${import.meta.env.VITE_API_BASE_URL || '/api'}/agent/qa/api/v1/ask`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(localStorage.getItem('token')
          ? { Authorization: `Bearer ${localStorage.getItem('token')}` }
          : {}),
        // ...(userId ? { userId } : {}),
      },
      body: JSON.stringify(data),
      signal,
    })

    if (!response.ok) {
      // Try to read error response
      const errorText = await response.text()
      let errorMessage = `HTTP error! status: ${response.status}`

      try {
        const errorData = JSON.parse(errorText)
        if (errorData.message) {
          errorMessage = errorData.message
        } else if (errorData.error) {
          errorMessage = errorData.error
        }
      } catch {
        if (errorText) {
          errorMessage = errorText
        }
      }

      throw new Error(errorMessage)
    }

    const reader = response.body?.getReader()
    const decoder = new TextDecoder()

    if (!reader) {
      throw new Error('Failed to get response stream')
    }

    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()

      if (done) {
        onFinish?.()
        break
      }

      const chunk = decoder.decode(value, { stream: true })
      buffer += chunk
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(window as any).__askChunks ??= []
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(window as any).__askChunks.push({
        t: performance.now(),
        chunk,
      })


      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        const trimmedLine = line.trim()
        if (!trimmedLine) continue

        if (trimmedLine.startsWith('event:')) continue

        if (trimmedLine.startsWith('data:')) {
          const jsonData = trimmedLine.slice(5).trim()

          if (jsonData === '[DONE]') {
            onFinish?.()
            return
          }

          try {
            const parsed = JSON.parse(jsonData)

            if (parsed.type === 'done') {
              // Handle sources if available
              if (parsed.sources && Array.isArray(parsed.sources)) {
                onSources?.(parsed.sources)
              }
              onFinish?.()
              return
            }

            // Handle thinking messages (streamed)
            if (parsed.type === 'thinking' && parsed.content) {
              onThinking?.(parsed.content)
            }
            // Handle status messages (displayed as list)
            else if ((parsed.type === 'start' || parsed.type === 'status') && parsed.message) {
              onStatus?.(parsed.message)
            }
            // Handle content messages
            else if (parsed.type === 'content' && parsed.content) {
              onMessage(parsed.content)
              await new Promise((resolve) => setTimeout(resolve, 30))
            }
            // Fallback for other content formats
            else {
              const content =
                parsed.content ||
                parsed.answer ||
                parsed.delta ||
                parsed.choices?.[0]?.delta?.content ||
                parsed.choices?.[0]?.message?.content ||
                ''

              if (content) {
                onMessage(content)
                await new Promise((resolve) => setTimeout(resolve, 30))
              }
            }

            if (parsed.finish_reason || parsed.done || parsed.choices?.[0]?.finish_reason) {
              onFinish?.()
              return
            }
          } catch (e) {
            console.error('Failed to parse SSE data:', e, jsonData)
          }
        }
      }
    }
  } catch (error) {
    if (error instanceof Error && error.name === 'AbortError') {
      return
    }
    console.error('Stream error:', error)
    onError?.(error)
  }
}

// ========== Agent QA API ==========

/**
 * Create conversation
 */
export function createConversation(
  data: CreateConversationRequest,
): Promise<CreateConversationResponse> {
  return request.post('/agent/qa/api/v1/conversations', data, {})
}

/**
 * Save message
 */
export function saveMessage(data: SaveMessageRequest): Promise<SaveMessageResponse> {
  const userId = getUserId()
  return request.post('/agent/qa/api/v1/messages', data, {
    headers: {
      userId,
    },
  })
}

/**
 * Get conversation list
 */
export function getConversationList(params?: {
  page?: number
  page_size?: number
}): Promise<ConversationListResponse> {
  const userId = getUserId()
  return request.get('/agent/qa/api/v1/conversations', {
    params,
    headers: {
      userId,
    },
  })
}

/**
 * Get conversation details
 */
export function getConversationDetail(conversationId: string): Promise<CreateConversationResponse> {
  const userId = getUserId()
  return request.get(`/agent/qa/api/v1/conversations/${conversationId}`, {
    headers: {
      userId,
    },
  })
}

/**
 * Get message list for a conversation
 */
export function getMessageList(
  conversationId: string,
  params?: {
    page?: number
    page_size?: number
  },
): Promise<MessageListResponse> {
  const userId = getUserId()
  return request.get(`/agent/qa/api/v1/messages/${conversationId}`, {
    params,
    headers: {
      userId,
    },
  })
}

/**
 * Get single message details
 */
export interface GetMessageDetailResponse {
  success: boolean
  data: MessageData
}

export function getMessageDetail(messageId: number): Promise<GetMessageDetailResponse> {
  const userId = getUserId()
  return request.get(`/agent/qa/api/v1/message/${messageId}`, {
    headers: {
      userId,
    },
  })
}

/**
 * Batch get message details
 */
export interface BatchGetMessagesResponse {
  success: boolean
  data: MessageData[]
  message: string
}

export function batchGetMessages(messageIds: number[]): Promise<BatchGetMessagesResponse> {
  const userId = getUserId()
  return request.post('/agent/qa/api/v1/messages/batch', messageIds, {
    headers: {
      userId,
    },
  })
}

/**
 * Update conversation
 */
export function updateConversation(
  conversationId: string,
  data: UpdateConversationRequest,
): Promise<UpdateConversationResponse> {
  const userId = getUserId()
  return request.patch(`/agent/qa/api/v1/conversations/${conversationId}`, data, {
    headers: {
      userId,
    },
  })
}

/**
 * Delete conversation
 */
export function deleteConversation(conversationId: string): Promise<DeleteConversationResponse> {
  const userId = getUserId()
  return request.delete(`/agent/qa/api/v1/conversations/${conversationId}`, {
    headers: {
      userId,
    },
  })
}

/**
 * Health check
 */
export function checkHealth(): Promise<HealthCheckResponse> {
  return request.get('/agent/qa/api/v1/health')
}

// ========== QA Collection API ==========

/**
 * Create Q&A collection
 */
export function createQACollection(
  data: CreateQACollectionRequest,
): Promise<CreateQACollectionResponse> {
  const userId = getUserId()
  return request.post('/agent/qa/api/v1/qa-collections', data, {
    headers: {
      userId,
    },
  })
}

/**
 * Get Q&A collection list
 */
export function getQACollectionList(params?: {
  page?: number
  page_size?: number
}): Promise<QACollectionListResponse> {
  const userId = getUserId()
  return request.get('/agent/qa/api/v1/qa-collections', {
    params,
    headers: {
      userId,
    },
  })
}

/**
 * Get single Q&A collection
 */
export function getQACollectionDetail(id: number): Promise<QACollectionDetailResponse> {
  const userId = getUserId()
  return request.get(`/agent/qa/api/v1/qa-collections/${id}`, {
    headers: {
      userId,
    },
  })
}

/**
 * Update Q&A collection
 */
export function updateQACollection(
  id: number,
  data: UpdateQACollectionRequest,
): Promise<UpdateQACollectionResponse> {
  const userId = getUserId()
  return request.patch(`/agent/qa/api/v1/qa-collections/${id}`, data, {
    headers: {
      userId,
    },
  })
}

/**
 * Delete Q&A collection
 */
export function deleteQACollection(id: number): Promise<DeleteQACollectionResponse> {
  const userId = getUserId()
  return request.delete(`/agent/qa/api/v1/qa-collections/${id}`, {
    headers: {
      userId,
    },
  })
}

// ========== QA Item API ==========

function normalizeSuccess<T>(raw: unknown, data?: T): { success: boolean; data: T } {
  if (raw && typeof raw === 'object' && 'success' in (raw as Record<string, unknown>))
    return raw as { success: boolean; data: T }
  return { success: true, data: (data ?? raw) as T }
}

function getUserName(): string {
  try {
    const userStr = localStorage.getItem('user')
    if (!userStr) return ''
    const user = JSON.parse(userStr)
    return user?.profile?.name || user?.profile?.username || user?.userName || user?.username || ''
  } catch {
    return ''
  }
}

function toRecord(raw: unknown): Record<string, unknown> {
  return raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {}
}

function detailToItem(detail: QAAnswerDetailData): QAItemData {
  return {
    answer: detail.answer,
    questions: detail.questions,
  }
}

function toListItem(raw: unknown): QAItemData {
  const r = toRecord(raw)
  const answerEntity = (r.answer ? toRecord(r.answer) : r) as unknown as QAItemAnswer
  const questions = (Array.isArray(r.questions) ? r.questions : []) as QAItemQuestion[]
  return {
    answer: answerEntity,
    questions,
  }
}

/**
 * Create Q&A item
 */
export function createQAItem(data: CreateQAItemRequest): Promise<CreateQAItemResponse> {
  const userId = getUserId()
  return request.post('/agent/qa/api/v1/qa-answers', data, {
    headers: {
      userId,
    },
    timeout: 60_000,
  })
}

/**
 * Batch create Q&A items
 */
// export function batchCreateQAItems(
//   data: BatchCreateQAItemRequest,
// ): Promise<BatchCreateQAItemResponse> {
//   const userId = getUserId()
//   return request.post('/agent/qa/api/v1/qa-answers/batch', data, {
//     headers: {
//       userId,
//     },
//   })
// }

/**
 * Get Q&A item list
 */
export function getQAItemList(params?: {
  collection_id?: number
  page?: number
  page_size?: number
}): Promise<QAItemListResponse> {
  const userId = getUserId()
  return request.get('/agent/qa/api/v1/qa-answers', {
    params,
    headers: {
      userId,
    },
    timeout: 10000,
  })
}

/**
 * Get single Q&A item
 */
export function getQAItemDetail(id: number): Promise<QAItemDetailResponse> {
  const userId = getUserId()
  return request.get(`/agent/qa/api/v1/qa-answers/${id}`, {
    headers: {
      userId,
    },
  })
}

/**
 * Get answer details (with question variants)
 */
export function getQAAnswerDetail(id: number): Promise<QAAnswerDetailResponse> {
  const userId = getUserId()
  return request.get(`/agent/qa/api/v1/qa-answers/${id}`, {
    headers: {
      userId,
    },
  })
}

/**
 * Update Q&A item
 */
export function updateQAItem(id: number, data: UpdateQAItemRequest): Promise<UpdateQAItemResponse> {
  const userId = getUserId()
  return request.put(`/agent/qa/api/v1/qa-answers/${id}`, data, {
    headers: {
      userId,
    },
  })
}

/**
 * Delete Q&A item
 */
export function deleteQAItem(id: number): Promise<DeleteQAItemResponse> {
  const userId = getUserId()
  return request.delete(`/agent/qa/api/v1/qa-answers/${id}`, {
    headers: {
      userId,
    },
  })
}

/**
 * Generate questions
 */
export function generateQAQuestions(
  data: GenerateQuestionsRequest,
): Promise<GenerateQuestionsResponse> {
  const userId = getUserId()
  return request.post('/agent/qa/api/v1/qa-answers/generate-questions', data, {
    headers: {
      userId,
    },
  })
}

/**
 * Upload rich text image
 */
export function uploadQAImage(file: File, description: string): Promise<UploadQAImageResponse> {
  const userId = getUserId()
  const formData = new FormData()
  formData.append('file', file)
  formData.append('description', description)
  return request.post('/agent/qa/api/v1/upload', formData, {
    headers: {
      userId,
      'Content-Type': 'multipart/form-data',
    },
    timeout: 60_000,
  })
}

/**
 * Upload file
 */
export function uploadQAFile(file: File, description: string): Promise<UploadQAFileResponse> {
  const userId = getUserId()
  const formData = new FormData()
  formData.append('file', file)
  formData.append('description', description)
  return request.post('/agent/qa/api/v1/upload', formData, {
    headers: {
      userId,
      'Content-Type': 'multipart/form-data',
    },
    timeout: 60_000,
  })
}

/**
 * Vector similarity search for Q&A items
 */
export function searchQAItems(data: SearchQAItemRequest): Promise<SearchQAItemResponse> {
  return request.post('/agent/qa/api/v1/qa-answers/search', data)
}

// ========== Answer Feedback API Types ==========

export interface SourceRef {
  source: string
  item_id?: number
  doc_id?: string
}

export interface SubmitFeedbackRequest {
  vote_type: 'up' | 'down'
  message_id: number // Answer message ID
  reason?: string
}

export interface FeedbackData {
  id: number
  user_id: string
  user_name: string
  vote_type: 'up' | 'down'
  reason?: string
  query: string
  answer: string
  source_refs?: (SourceRef & Partial<SourceItem>)[] // Support both minimal and full source info
  status: 'pending' | 'resolved' | 'ignored'
  resolved_by?: string
  resolved_by_name?: string
  resolved_at?: string
  resolution_note?: string
  created_at: string
}

export interface SubmitFeedbackResponse {
  success: boolean
  data: FeedbackData
  message: string
}

export interface CancelVoteRequest {
  message_id: number // Answer message ID
}

export interface CancelVoteResponse {
  success: boolean
  message: string
}

export interface FeedbackListParams {
  status?: 'pending' | 'resolved' | 'ignored'
  vote_type?: 'up' | 'down'
  page?: number
  page_size?: number
}

export interface FeedbackListResponse {
  success: boolean
  data: {
    items: FeedbackData[]
    pagination: {
      page: number
      page_size: number
      total: number
      total_pages: number
    }
  }
  message: string
}

export interface FeedbackStatsResponse {
  success: boolean
  data: {
    total: number
    pending: number
    resolved: number
    ignored: number
    upvotes: number
    downvotes: number
  }
}

export interface FeedbackDetailResponse {
  success: boolean
  data: FeedbackData
}

export interface ResolveFeedbackRequest {
  status: 'resolved' | 'ignored'
  note?: string
}

export interface ResolveFeedbackResponse {
  success: boolean
  message: string
}

// ========== Answer Feedback API ==========

/**
 * Submit feedback
 */
export function submitFeedback(data: SubmitFeedbackRequest): Promise<SubmitFeedbackResponse> {
  const userId = getUserId()
  return request.post('/agent/qa/api/v1/answer-feedback', data, {
    headers: {
      userId,
    },
  })
}

/**
 * Cancel vote
 */
export function cancelVote(data: CancelVoteRequest): Promise<CancelVoteResponse> {
  const userId = getUserId()
  return request.post('/agent/qa/api/v1/answer-feedback/cancel', data, {
    headers: {
      userId,
    },
  })
}

/**
 * Get feedback list
 */
export function getFeedbackList(params?: FeedbackListParams): Promise<FeedbackListResponse> {
  const userId = getUserId()
  return request.get('/agent/qa/api/v1/answer-feedback', {
    params,
    headers: {
      userId,
    },
  })
}

/**
 * Get feedback statistics
 */
export function getFeedbackStats(): Promise<FeedbackStatsResponse> {
  const userId = getUserId()
  return request.get('/agent/qa/api/v1/answer-feedback/stats', {
    headers: {
      userId,
    },
  })
}

/**
 * Get single feedback
 */
export function getFeedbackDetail(feedbackId: number): Promise<FeedbackDetailResponse> {
  const userId = getUserId()
  return request.get(`/agent/qa/api/v1/answer-feedback/${feedbackId}`, {
    headers: {
      userId,
    },
  })
}

/**
 * Resolve feedback
 */
export function resolveFeedback(
  feedbackId: number,
  data: ResolveFeedbackRequest,
): Promise<ResolveFeedbackResponse> {
  const userId = getUserId()
  return request.post(`/agent/qa/api/v1/answer-feedback/${feedbackId}/resolve`, data, {
    headers: {
      userId,
    },
  })
}

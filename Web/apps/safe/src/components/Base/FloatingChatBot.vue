<template>
  <div
    v-show="!isOnChatbotFullPage"
    class="floating-chatbot-wrapper"
    :class="{ dragging: isDragging, expanded: showDialog }"
    :style="{ bottom: position.y + 'px', left: position.x + 'px' }"
    ref="wrapperRef"
  >
    <!-- Floating Button -->
    <div
      v-show="!showDialog"
      class="chatbot-button"
      @mousedown="startDrag"
      @click="handleClick"
      @contextmenu.prevent="resetPosition"
    >
      <img :src="sparklesIcon" class="bot-icon" alt="Primus-SaFE Agent" />
    </div>

    <!-- Chat Dialog -->
    <div v-show="showDialog" class="chat-dialog" @click.stop @mousedown="startDialogDrag">
      <!-- Header (Draggable) -->
      <div class="chat-header">
        <div class="header-left">
          <img :src="sparklesIcon" class="header-icon" alt="Primus-SaFE Agent" />
          <span class="header-title">Primus-SaFE Agents</span>
          <span class="header-status" :class="{ offline: !currentModeOnline }">
            {{ currentModeLabel }} {{ currentModeOnline ? 'Online' : 'Offline' }}
          </span>
          <el-tooltip content="Star us on GitHub" placement="bottom">
            <el-icon class="header-star-icon" @click.stop="openGitHub">
              <Star />
            </el-icon>
          </el-tooltip>
        </div>
        <div class="header-actions" @mousedown.stop>
          <el-tooltip content="New conversation" placement="bottom">
            <el-icon class="action-icon new-icon" @click="startNewConversation">
              <Plus />
            </el-icon>
          </el-tooltip>
          <el-tooltip
            :content="loading ? 'Please wait for response to complete' : 'Open full conversation'"
            placement="bottom"
          >
            <span>
              <el-icon
                class="action-icon expand-icon"
                :class="{ disabled: loading }"
                @click="!loading && openFullPage()"
              >
                <FullScreen />
              </el-icon>
            </span>
          </el-tooltip>
          <el-icon class="action-icon close-icon" @click="closeDialog">
            <Close />
          </el-icon>
        </div>
      </div>

      <!-- Scrollable Content Area -->
      <div class="content-area" ref="messagesContainer">
        <!-- Quick Actions -->
        <div class="quick-actions" v-if="messages.length === 0">
          <div class="welcome-text">
            <el-icon class="wave-icon"><ChatLineRound /></el-icon>
            <span>How can I help you today?</span>
          </div>
          <QuickStartCards
            v-if="mode === 'ask'"
            :config="askModeQuickStart"
            :show-header="false"
            @card-click="setQuestion"
          />
          <QuickStartCards
            v-else-if="mode === 'agent'"
            :config="agentQuickStartConfig"
            :show-header="false"
            @card-click="setInputText"
          />
        </div>

        <!-- Messages -->
        <div class="messages-container">
          <div v-for="(message, index) in messages" :key="index" class="message-wrapper">
            <!-- Assistant message group (includes avatar, state, thinking, answer) -->
            <div v-if="message.role === 'assistant'" class="assistant-message-group">
              <!-- Avatar (shown only once) -->
              <div class="message-avatar">
                <img :src="sparklesIcon" class="avatar-icon" alt="Primus-SaFE Agent" />
              </div>

              <div class="assistant-content-wrapper">
                <!-- State messages (displayed line by line) -->
                <div
                  v-if="message.statusMessages && message.statusMessages.length > 0"
                  class="status-content"
                >
                  <div
                    v-for="(statusMsg, statusIndex) in message.statusMessages"
                    :key="statusIndex"
                    class="status-item"
                  >
                    {{ statusMsg }}
                  </div>
                </div>

                <!-- Thinking process (streaming cumulative display) -->
                <div v-if="message.thinking && message.thinking.trim()" class="thinking-content">
                  <div class="thinking-header" @click="toggleThinking(message)">
                    <div class="thinking-header-left">
                      <el-icon class="thinking-icon"><View /></el-icon>
                      <span class="thinking-label">Thinking</span>
                      <span v-if="message.thinkingTime" class="thinking-time">
                        ({{ formatThinkingTime(message.thinkingTime) }})
                      </span>
                    </div>
                    <el-icon class="thinking-toggle-icon">
                      <ArrowUp v-if="message.thinkingExpanded" />
                      <ArrowDown v-else />
                    </el-icon>
                  </div>
                  <div v-show="message.thinkingExpanded" class="thinking-text">
                    {{ message.thinking }}
                  </div>
                </div>

                <!-- Agent: Workflow Progress -->
                <WorkflowProgress
                  v-if="message.workflow"
                  :workflow-name="message.workflow.workflow_name"
                  :steps="message.workflow.steps"
                  :current-step="message.workflow.current_step"
                />

                <!-- Agent: Action Status -->
                <ActionStatus
                  v-if="message.actions && message.actions.length > 0"
                  :actions="message.actions"
                />

                <!-- Agent: Inline Confirm Form -->
                <InlineConfirmForm
                  v-if="message.confirmData"
                  :data="message.confirmData"
                  :loading="message.confirmLoading || false"
                  :readonly="message.confirmReadonly || false"
                  :confirmed-selections="message.confirmedSelections"
                  @submit="handleInlineConfirmSubmit(index, $event)"
                  @cancel="handleInlineConfirmCancel(index, $event)"
                />

                <!-- Answer content (bubble) -->
                <div
                  v-if="message.content || (loading && index === messages.length - 1)"
                  class="message-content"
                >
                  <!-- Show typing indicator if content is empty and loading -->
                  <div
                    v-if="!message.content && loading && index === messages.length - 1"
                    class="typing-indicator"
                  >
                    <span></span>
                    <span></span>
                    <span></span>
                  </div>
                  <!-- Otherwise show message content -->
                  <div
                    v-else-if="message.content"
                    class="message-text"
                    v-html="formatMessage(message.content)"
                  ></div>
                </div>
              </div>
            </div>

            <!-- User message -->
            <div v-else class="message user">
              <div class="message-avatar">
                <el-icon><User /></el-icon>
              </div>

              <div class="message-content">
                <div
                  v-if="message.content"
                  class="message-text"
                  v-html="formatMessage(message.content)"
                ></div>
              </div>
            </div>

            <!-- Knowledge Base sources (displayed below the bubble) -->
            <div
              v-if="message.role === 'assistant' && message.sources && message.sources.length > 0"
              class="sources-section"
            >
              <div class="sources-title">
                <el-icon class="sources-icon"><Collection /></el-icon>
                <span>Knowledge Base Sources ({{ message.sources.length }})</span>
              </div>
              <div class="sources-list">
                <div
                  v-for="(source, sourceIndex) in message.sources"
                  :key="sourceIndex"
                  class="source-item"
                  @click="handleViewSourceDetail(source.item_id)"
                >
                  <div class="source-header">
                    <span class="source-collection">{{ source.collection_name }}</span>
                    <!-- <span class="source-similarity"
                      >{{ (source.similarity * 100).toFixed(1) }}%</span
                    > -->
                  </div>
                  <div class="source-question">{{ source.question }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- QA Detail Dialog -->
      <QADetailDialog
        v-model="qaDetailDialogVisible"
        :loading="qaDetailLoading"
        :data="qaDetailData"
      />

      <!-- Input -->
      <div class="input-container">
        <div class="input-wrapper" @click="focusInput">
          <!-- Input -->
          <textarea
            ref="inputRef"
            v-model="userInput"
            placeholder="Ask me anything..."
            @keydown.enter="handleEnterKey"
            :disabled="loading"
            class="message-input"
            rows="1"
          />

          <!-- Bottom Controls -->
          <div class="bottom-controls" @click.stop>
            <div class="left-controls">
              <!-- Mode Selector Dropdown -->
              <el-dropdown trigger="click" @command="handleModeChange">
                <div class="mode-selector-button">
                  <el-icon v-if="mode === 'ask'"><ChatDotRound /></el-icon>
                  <el-icon v-else><MagicStick /></el-icon>
                  <span class="mode-text">{{ mode === 'ask' ? 'Ask' : 'Agent' }}</span>
                </div>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="ask" :class="{ active: mode === 'ask' }">
                      <el-icon class="mr-2"><ChatDotRound /></el-icon>
                      <span>Ask</span>
                    </el-dropdown-item>
                    <el-dropdown-item command="agent" :class="{ active: mode === 'agent' }">
                      <el-icon class="mr-2"><MagicStick /></el-icon>
                      <span>Agent</span>
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>

              <!-- Deep Think Toggle -->
              <el-tooltip
                :content="enableThinking ? 'Deep Thinking Enabled' : 'Enable Deep Thinking'"
                placement="top"
              >
                <div
                  class="control-button"
                  :class="{ active: enableThinking }"
                  @click="enableThinking = !enableThinking"
                >
                  <img :src="deepThinkIcon" class="control-icon" alt="Deep Thinking" />Think
                </div>
              </el-tooltip>
            </div>

            <div class="right-controls">
              <!-- Stop button when loading -->
              <img
                v-if="loading"
                :src="stopIcon"
                class="stop-icon"
                @click="stopGeneration"
                alt="Stop"
              />
              <!-- Send button when not loading -->
              <el-icon
                v-else
                class="send-icon"
                @click="sendMessage"
                :class="{ active: userInput.trim() }"
              >
                <Position />
              </el-icon>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted, onUnmounted, nextTick, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ChatDotRound,
  Close,
  FullScreen,
  MagicStick,
  ChatLineRound,
  User,
  Position,
  Plus,
  Collection,
  View,
  ArrowDown,
  ArrowUp,
  Star,
} from '@element-plus/icons-vue'
import stopIcon from '@/assets/icons/stop.png'
import deepThinkIcon from '@/assets/icons/deepthink.png'
import sparklesIcon from '@/assets/icons/sparkles.png'
import {
  createConversation,
  saveMessage,
  checkHealth,
  getQAItemDetail,
  type QAAnswerDetailData,
} from '@/services/chatbot'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import { useChatbotUIStore } from '@/stores/chatbotUI'
import QADetailDialog from './QADetailDialog.vue'
import QuickStartCards from '@/pages/ChatbotFullPage/Components/QuickStartCards.vue'
import WorkflowProgress from '@/pages/ChatbotFullPage/Components/WorkflowProgress.vue'
import ActionStatus from '@/pages/ChatbotFullPage/Components/ActionStatus.vue'
import InlineConfirmForm from '@/pages/ChatbotFullPage/Components/InlineConfirmForm.vue'
import {
  askModeQuickStart,
  agentModeQuickStart,
  normalUserQuickStart,
  workspaceAdminQuickStart,
} from '@/pages/ChatbotFullPage/constants/quickStartData'
import {
  formatMessage,
  formatThinkingTime,
} from '@/pages/ChatbotFullPage/composables/useFormatters'
import {
  generateConversationId,
  generateOperationId,
  serializeAgentData,
} from '@/pages/ChatbotFullPage/composables/useMessageOperations'
import { useAskChat } from '@/pages/ChatbotFullPage/composables/useAskChat'
import { useAgentChat } from '@/pages/ChatbotFullPage/composables/useAgentChat'
import { agentSocket, checkAgentHealth } from '@/services/agent'
import type { Message } from '@/pages/ChatbotFullPage/types'

// Message type is now imported from ChatbotFullPage types

// Local state
const wrapperRef = ref<HTMLElement>()
const messagesContainer = ref<HTMLElement>()
const inputRef = ref<HTMLTextAreaElement>()
const showDialog = ref(false)
const mode = ref<'ask' | 'agent'>('ask')
const userInput = ref('')
const messages = ref<Message[]>([])
const loading = ref(false)
const askOnline = ref(true)
const agentOnline = ref(false)
const currentModeOnline = computed(() =>
  mode.value === 'ask' ? askOnline.value : agentOnline.value,
)
const currentModeLabel = computed(() => (mode.value === 'ask' ? 'Ask' : 'Agent'))

// Router
const router = useRouter()
const route = useRoute()

// User store
const userStore = useUserStore()
const wsStore = useWorkspaceStore()
const chatbotUIStore = useChatbotUIStore()

// Agent mode quick start config based on user role
const agentQuickStartConfig = computed(() => {
  if (userStore.isManager) {
    return agentModeQuickStart
  }

  return wsStore.isCurrentWorkspaceAdmin() ? workspaceAdminQuickStart : normalUserQuickStart
})

// Check if current route is ChatbotFullPage
const isOnChatbotFullPage = computed(() => route.name === 'ChatbotFullPage')

// Conversation state
const currentConversationId = ref<string>('')

// Agent state
const currentOperationId = ref('')

// QA Detail Dialog state
const qaDetailDialogVisible = ref(false)
const qaDetailLoading = ref(false)
const qaDetailData = ref<QAAnswerDetailData | null>(null)

// Dragging state - position in bottom-left
const position = reactive({ x: 20, y: 20 })
const isDragging = ref(false)
const dragStart = reactive({ x: 0, y: 0 })
const clickTime = ref(0)

// Use composables
const { enableThinking, sendAskMessage, stopAskGeneration } = useAskChat(
  messages,
  currentConversationId,
  loading,
  messagesContainer,
)

const {
  agentConnected,
  agentSessionId,
  handleAgentMessage,
  handleInlineConfirmSubmit,
  handleInlineConfirmCancel,
} = useAgentChat(messages, currentConversationId, loading, currentOperationId, serializeAgentData)

// Toggle thinking expansion
const toggleThinking = (message: Message) => {
  message.thinkingExpanded = !message.thinkingExpanded
}

// Agent: Connect to WebSocket
const connectAgent = () => {
  if (!userStore.userId) {
    ElMessage.warning('Please login first')
    return
  }

  agentSocket.setEventHandlers({
    onConnectionEstablished: (data) => {
      agentConnected.value = true
      agentSessionId.value = data.session_id
      agentOnline.value = true
      ElMessage.success('Agent connected successfully')
    },
    onMessage: handleAgentMessage,
    onDisconnect: () => {
      agentConnected.value = false
      agentOnline.value = false
      ElMessage.warning('Agent disconnected')
    },
    onError: (error) => {
      console.error('Agent error:', error)
      agentConnected.value = false
      agentOnline.value = false

      const errorMsg = error?.message || 'Unknown error'
      if (errorMsg.includes('websocket error') || errorMsg.includes('xhr poll error')) {
        ElMessage.error(
          'Cannot connect to Agent backend. Please ensure the backend service is running.',
        )
      } else {
        ElMessage.error(`Agent connection error: ${errorMsg}`)
      }
    },
  })

  try {
    agentSocket.connect(userStore.userId, userStore.profile?.name || '')
    ElMessage.info('Connecting to Agent...')
  } catch (error) {
    console.error('Failed to initialize Agent connection:', error)
    ElMessage.error('Failed to initialize Agent connection')
  }
}

// Agent: Disconnect WebSocket
const disconnectAgent = () => {
  agentSocket.disconnect()
  agentConnected.value = false
  agentOnline.value = false
}

// Agent: Send message
const sendAgentMessage = async (query: string) => {
  if (!agentConnected.value) {
    ElMessage.error('Agent not connected')
    return
  }

  // Add user message
  messages.value.push({
    role: 'user',
    content: query,
  })
  const userMessageIndex = messages.value.length - 1

  // Create assistant message placeholder
  messages.value.push({
    role: 'assistant',
    content: '',
    workflow: undefined,
    actions: [],
  })

  await nextTick()
  scrollToNewQuestion()

  loading.value = true

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
    }
  }

  // Save user message
  if (currentConversationId.value) {
    try {
      const saveUserMsgResponse = await saveMessage({
        conversation_id: currentConversationId.value,
        role: 'user',
        content: query,
        thinking: null,
        message_type: 'Agent',
      })
      messages.value[userMessageIndex].messageId = saveUserMsgResponse.data.id
    } catch (error) {
      console.error('Failed to save user message:', error)
    }
  }

  // Generate operation ID
  currentOperationId.value = generateOperationId()

  try {
    agentSocket.sendMessage(query, currentOperationId.value)
  } catch (error) {
    console.error('Failed to send agent message:', error)
    ElMessage.error('Failed to send message')
    loading.value = false
  }
}

// Stop generation (Agent mode)
const stopAgentGeneration = () => {
  if (agentConnected.value && currentOperationId.value) {
    try {
      agentSocket.cancelOperation(currentOperationId.value)
      loading.value = false
      ElMessage.info('Operation cancelled')
    } catch (error) {
      console.error('Failed to cancel operation:', error)
      ElMessage.error('Failed to cancel operation')
    }
  } else {
    loading.value = false
  }
}

// Handle click to toggle dialog
const handleClick = () => {
  const clickDuration = Date.now() - clickTime.value
  if (clickDuration < 200 && !isDragging.value) {
    showDialog.value = true
    // Wait for DOM update then adjust position
    nextTick(() => {
      adjustDialogPosition()
    })
  }
}

// Adjust dialog position to prevent being obscured by screen edges
const adjustDialogPosition = () => {
  if (!wrapperRef.value) return

  const dialogWidth = 400 // Dialog width
  const dialogHeight = 600 // Dialog height
  const buffer = 20

  // Check if exceeds right edge
  const rightEdge = position.x + dialogWidth
  if (rightEdge > window.innerWidth - buffer) {
    position.x = window.innerWidth - dialogWidth - buffer
  }

  // Check if exceeds top edge (dialog positioned from bottom)
  const topEdge = window.innerHeight - position.y - dialogHeight
  if (topEdge < buffer) {
    position.y = window.innerHeight - dialogHeight - buffer
  }

  // Ensure not exceeding left and bottom edges
  position.x = Math.max(buffer, position.x)
  position.y = Math.max(buffer, position.y)
}

// Close dialog
const closeDialog = () => {
  showDialog.value = false
}

// Open GitHub
const openGitHub = () => {
  window.open('https://github.com/AMD-AGI/Primus-SaFE', '_blank')
}

// Start new conversation
const startNewConversation = () => {
  messages.value = []
  currentConversationId.value = ''
  ElMessage.success('New conversation started')
}

// Open full page
const openFullPage = () => {
  // Close dialog
  showDialog.value = false

  // Navigate to full-screen chat page with current mode
  const query: Record<string, string> = {
    mode: mode.value,
  }

  if (currentConversationId.value) {
    query.conversationId = currentConversationId.value
  }

  router.push({
    name: 'ChatbotFullPage',
    query,
  })
}

// Handle mode change from dropdown
const handleModeChange = (command: 'ask' | 'agent') => {
  if (mode.value === command) return

  mode.value = command

  if (command === 'agent') {
    // Switch to agent mode
    if (!agentConnected.value) {
      connectAgent()
    }
    ElMessage.success('Switched to Agent mode')
  } else {
    // Switch to ask mode
    if (agentConnected.value) {
      disconnectAgent()
    }
    ElMessage.success('Switched to Ask mode')
  }
}

// Set question from quick actions (Ask mode - auto send)
const setQuestion = (question: string) => {
  userInput.value = question
  nextTick(() => {
    sendMessage()
  })
}

// Set input text (Agent mode - only fill input)
const setInputText = (text: string) => {
  userInput.value = text
  nextTick(() => {
    inputRef.value?.focus()
  })
}

// Focus the input field
const focusInput = () => {
  inputRef.value?.focus()
}

// View source detail
const handleViewSourceDetail = async (itemId: number) => {
  qaDetailDialogVisible.value = true
  qaDetailLoading.value = true
  qaDetailData.value = null

  try {
    const res = await getQAItemDetail(itemId)
    qaDetailData.value = res
  } catch (error) {
    console.error('Failed to load QA detail:', error)
    ElMessage.error('Failed to load details: ' + (error as Error).message)
    qaDetailDialogVisible.value = false
  } finally {
    qaDetailLoading.value = false
  }
}

// Stop generation
const stopGeneration = () => {
  if (mode.value === 'agent') {
    stopAgentGeneration()
  } else {
    stopAskGeneration()
  }
}

// Scroll to new question (helper for agent mode)
const scrollToNewQuestion = () => {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}

// Auto-adjust textarea height
const adjustTextareaHeight = () => {
  if (inputRef.value) {
    inputRef.value.style.height = 'auto'
    inputRef.value.style.height = inputRef.value.scrollHeight + 'px'
  }
}

// External prefill (e.g. from Nodes page tooltip button)
watch(
  () => chatbotUIStore.requestId,
  async () => {
    const text = (chatbotUIStore.prefillText || '').trim()
    if (!text) return

    // If not expanded, expand first
    if (!showDialog.value) {
      showDialog.value = true
      await nextTick()
      adjustDialogPosition()
    }

    // Fill input and focus
    userInput.value = text
    await nextTick()
    adjustTextareaHeight()
    inputRef.value?.focus()
  },
)

// Watch userInput to auto-adjust height
watch(userInput, () => {
  nextTick(() => {
    adjustTextareaHeight()
  })
})

// Handle Enter key press
const handleEnterKey = (event: KeyboardEvent) => {
  // Shift + Enter: allow new line (default behavior)
  if (event.shiftKey) {
    return
  }

  // Enter only: send message and prevent default
  event.preventDefault()
  sendMessage()
}

// Send message
const sendMessage = async () => {
  const query = userInput.value.trim()
  if (!query || loading.value) return

  if (!userStore.userId) {
    ElMessage.warning('Please login first')
    return
  }

  // Clear input
  userInput.value = ''

  // Reset textarea height
  nextTick(() => {
    if (inputRef.value) {
      inputRef.value.style.height = 'auto'
    }
  })

  // Use different handlers based on mode
  if (mode.value === 'agent') {
    await sendAgentMessage(query)
  } else {
    await sendAskMessage(query)
  }
}

// Dragging methods - for button
const startDrag = (e: MouseEvent) => {
  isDragging.value = true
  dragStart.x = e.clientX - position.x
  dragStart.y = window.innerHeight - e.clientY - position.y
  clickTime.value = Date.now()

  document.addEventListener('mousemove', onDrag)
  document.addEventListener('mouseup', endDrag)
  e.preventDefault()
}

// Dragging methods - for dialog
const startDialogDrag = (e: MouseEvent) => {
  const target = e.target as HTMLElement

  // Prevent dragging if clicking on interactive elements or text content
  const excludedSelectors = [
    '.header-actions',
    'input',
    'button',
    '.el-icon',
    '.mode-option',
    '.action-item',
    '.message-input',
    '.send-icon',
    '.stop-icon',
    '.message-content', // Exclude message content
    '.message-text', // Exclude message text
    '.messages-container', // Exclude entire messages area
    '.input-wrapper', // Exclude input wrapper
    '.bottom-controls', // Exclude bottom controls
    '.sources-section', // Exclude Knowledge Base sources
    'a',
  ]

  // Check if the clicked element or its parent matches any excluded selector
  for (const selector of excludedSelectors) {
    if (target.closest(selector)) {
      return
    }
  }

  // Only allow dragging from header
  const allowedAreas = ['.chat-header']
  const isInAllowedArea = allowedAreas.some((selector) => target.closest(selector))

  if (!isInAllowedArea) {
    return
  }

  isDragging.value = true
  dragStart.x = e.clientX - position.x
  dragStart.y = window.innerHeight - e.clientY - position.y

  document.addEventListener('mousemove', onDrag)
  document.addEventListener('mouseup', endDrag)
  e.preventDefault()
}

const onDrag = (e: MouseEvent) => {
  if (!isDragging.value) return

  const newX = e.clientX - dragStart.x
  const newY = window.innerHeight - e.clientY - dragStart.y

  // Keep within viewport bounds
  const buffer = 20
  const elementWidth = wrapperRef.value?.offsetWidth || 60
  const elementHeight = wrapperRef.value?.offsetHeight || 60

  const maxX = window.innerWidth - elementWidth - buffer
  const maxY = window.innerHeight - elementHeight - buffer

  position.x = Math.max(buffer, Math.min(newX, maxX))
  position.y = Math.max(buffer, Math.min(newY, maxY))
}

const endDrag = () => {
  isDragging.value = false
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', endDrag)

  // Save position to localStorage
  localStorage.setItem('chatbotButtonPosition', JSON.stringify(position))
}

// Ensure button stays within viewport
const ensureInViewport = () => {
  const buffer = 20
  const elementWidth = wrapperRef.value?.offsetWidth || 60
  const elementHeight = wrapperRef.value?.offsetHeight || 60
  const maxX = window.innerWidth - elementWidth - buffer
  const maxY = window.innerHeight - elementHeight - buffer

  position.x = Math.max(buffer, Math.min(position.x, maxX))
  position.y = Math.max(buffer, Math.min(position.y, maxY))
}

// Reset position to default (right-click)
const resetPosition = () => {
  position.x = 20
  position.y = 20
  localStorage.removeItem('chatbotButtonPosition')
  ElMessage.success('Position reset')
}

// Ask mode health check
const checkHealthStatus = async () => {
  try {
    const response = await checkHealth()
    askOnline.value = response.status === 'healthy'
  } catch (error) {
    console.error('Health check failed:', error)
    askOnline.value = false
  }
}

const checkAgentHealthStatus = async () => {
  try {
    const response = await checkAgentHealth()
    agentOnline.value = response.status === 'healthy'
  } catch (error) {
    console.error('Agent health check failed:', error)
    agentOnline.value = false
  }
}

// Health check interval
let healthCheckInterval: number | null = null
let agentHealthCheckInterval: number | null = null

const startAskHealthCheck = () => {
  if (healthCheckInterval) {
    clearInterval(healthCheckInterval)
  }
  checkHealthStatus()
  healthCheckInterval = setInterval(checkHealthStatus, 30000)
}

const stopAskHealthCheck = () => {
  if (!healthCheckInterval) return
  clearInterval(healthCheckInterval)
  healthCheckInterval = null
}

const startAgentHealthCheck = () => {
  if (agentHealthCheckInterval) {
    clearInterval(agentHealthCheckInterval)
  }
  checkAgentHealthStatus()
  agentHealthCheckInterval = setInterval(checkAgentHealthStatus, 30000)
}

const stopAgentHealthCheck = () => {
  if (!agentHealthCheckInterval) return
  clearInterval(agentHealthCheckInterval)
  agentHealthCheckInterval = null
}

const shouldPollAskHealth = computed(() => showDialog.value && mode.value === 'ask')
const shouldPollAgentHealth = computed(() => showDialog.value && mode.value === 'agent')

watch(
  shouldPollAskHealth,
  (shouldPoll) => {
    if (shouldPoll) {
      startAskHealthCheck()
    } else {
      stopAskHealthCheck()
    }
  },
  { immediate: true },
)

watch(
  shouldPollAgentHealth,
  (shouldPoll) => {
    if (shouldPoll) {
      startAgentHealthCheck()
    } else {
      stopAgentHealthCheck()
    }
  },
  { immediate: true },
)

onMounted(() => {
  // Load saved position
  const savedPosition = localStorage.getItem('chatbotButtonPosition')
  if (savedPosition) {
    try {
      const pos = JSON.parse(savedPosition)
      position.x = pos.x
      position.y = pos.y
    } catch (_e) {
      // Use default position
    }
  }

  // Add window resize listener
  window.addEventListener('resize', ensureInViewport)
})

onUnmounted(() => {
  window.removeEventListener('resize', ensureInViewport)
  stopAskHealthCheck()
  stopAgentHealthCheck()
  // Disconnect agent if connected
  if (agentConnected.value) {
    disconnectAgent()
  }
})
</script>

<style scoped lang="scss">
.floating-chatbot-wrapper {
  position: fixed;
  z-index: 99;
  user-select: none;
}

.chatbot-button {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  background: linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%);
  color: #fff;
  border-radius: 50%;
  box-shadow:
    0 4px 12px rgba(59, 130, 246, 0.4),
    0 2px 4px rgba(0, 0, 0, 0.1);
  cursor: pointer;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  backdrop-filter: blur(8px);

  .bot-icon {
    width: 28px;
    height: 28px;
    object-fit: contain;
    filter: brightness(0) invert(1);
    transition: all 0.3s ease;
  }

  &:hover {
    transform: translateY(-2px) scale(1.05);
    box-shadow:
      0 6px 20px rgba(59, 130, 246, 0.5),
      0 4px 8px rgba(0, 0, 0, 0.15);

    .bot-icon {
      transform: scale(1.1);
    }
  }

  &:active {
    transform: scale(0.95);
  }
}

.chat-dialog {
  width: 400px;
  height: 600px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.95) 0%, rgba(248, 250, 252, 0.95) 100%);
  border-radius: 16px;
  box-shadow:
    0 12px 40px rgba(0, 0, 0, 0.15),
    0 0 0 1px rgba(148, 163, 184, 0.1);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  backdrop-filter: blur(30px) saturate(180%);
  animation: slideIn 0.3s ease;
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.chat-dialog {
  // Remove overall move cursor, only set on header

  // Restore correct cursor for interactive elements
  input,
  button,
  a,
  .el-icon,
  .action-icon,
  .send-icon,
  .stop-icon {
    cursor: pointer !important;
  }

  input {
    cursor: text !important;
  }
}

.chat-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px;
  background: linear-gradient(135deg, rgba(248, 250, 252, 0.98) 0%, rgba(241, 245, 249, 0.98) 100%);
  backdrop-filter: blur(20px) saturate(180%);
  border-bottom: 1px solid rgba(148, 163, 184, 0.15);
  user-select: none;
  cursor: move;
  position: relative;

  .header-left {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .header-icon {
    width: 20px;
    height: 20px;
    object-fit: contain;
    filter: brightness(0) saturate(100%) invert(42%) sepia(93%) saturate(1352%) hue-rotate(201deg)
      brightness(103%) contrast(97%);
  }

  .header-title {
    font-size: 14px;
    font-weight: 600;
    color: #0f172a;
    letter-spacing: -0.3px;
  }

  .header-status {
    font-size: 10px;
    color: #059669;
    margin-left: 6px;
    padding: 2px 6px;
    background: linear-gradient(135deg, #d1fae5 0%, #a7f3d0 100%);
    border-radius: 10px;
    font-weight: 500;
    transition: all 0.3s ease;

    &.offline {
      color: #64748b;
      background: rgba(100, 116, 139, 0.15);
    }
  }

  .header-star-icon {
    margin-left: 8px;
    font-size: 16px;
    color: #ffd700;
    cursor: pointer;
    transition: all 0.2s ease;
    display: flex;
    align-items: center;
    justify-content: center;

    :deep(svg) {
      filter: drop-shadow(0 0 2px rgba(255, 215, 0, 0.3));
    }

    &:hover {
      transform: scale(1.15) rotate(10deg);
      color: #ffd33d;

      :deep(svg) {
        filter: drop-shadow(0 0 3px rgba(255, 215, 0, 0.5));
      }
    }
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .action-icon {
    width: 30px;
    height: 30px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 17px;
    cursor: pointer;
    transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    border-radius: 8px;
    color: #64748b;
    background: rgba(255, 255, 255, 0.5);
    border: 1px solid rgba(148, 163, 184, 0.15);

    &:hover {
      background: rgba(255, 255, 255, 0.9);
      border-color: rgba(148, 163, 184, 0.25);
      transform: translateY(-1px) scale(1.05);
      box-shadow: 0 3px 6px rgba(0, 0, 0, 0.1);
    }

    &:active {
      transform: translateY(0) scale(0.95);
    }

    &.disabled {
      opacity: 0.4;
      cursor: not-allowed;
      pointer-events: none;

      &:hover {
        background: rgba(255, 255, 255, 0.5);
        border-color: rgba(148, 163, 184, 0.15);
        transform: none;
        box-shadow: none;
      }
    }

    &.new-icon,
    &.expand-icon {
      background: linear-gradient(
        135deg,
        rgba(59, 130, 246, 0.08) 0%,
        rgba(139, 92, 246, 0.08) 100%
      );
      border-color: rgba(59, 130, 246, 0.2);
      color: #3b82f6;

      &:hover {
        color: #2563eb;
        background: linear-gradient(
          135deg,
          rgba(59, 130, 246, 0.15) 0%,
          rgba(139, 92, 246, 0.15) 100%
        );
        border-color: rgba(59, 130, 246, 0.35);
        box-shadow: 0 2px 6px rgba(59, 130, 246, 0.15);
      }
    }

    &.close-icon {
      background: rgba(239, 68, 68, 0.08);
      border-color: rgba(239, 68, 68, 0.2);
      color: #ef4444;

      &:hover {
        background: rgba(239, 68, 68, 0.15);
        color: #dc2626;
        border-color: rgba(239, 68, 68, 0.35);
        box-shadow: 0 2px 6px rgba(239, 68, 68, 0.15);
      }
    }
  }
}

.content-area {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  cursor: default; // Default cursor, not drag

  &::-webkit-scrollbar {
    width: 6px;
  }

  &::-webkit-scrollbar-track {
    background: rgba(0, 0, 0, 0.02);
  }

  &::-webkit-scrollbar-thumb {
    background: rgba(0, 0, 0, 0.15);
    border-radius: 3px;

    &:hover {
      background: rgba(0, 0, 0, 0.25);
    }
  }
}

.quick-actions {
  padding: 16px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.6) 0%, transparent 100%);
  border-bottom: 1px solid rgba(148, 163, 184, 0.1);

  .welcome-text {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 16px;
    font-size: 14px;
    font-weight: 600;
    color: #1e293b;

    .wave-icon {
      color: #3b82f6;
      font-size: 18px;
    }
  }

  // QuickStartCards component style override, adapted to floating window size
  :deep(.quick-start-section) {
    margin-bottom: 0;

    .quick-start-header {
      padding: 0;
      margin-bottom: 12px;

      .emoji-icon {
        font-size: 20px;
      }

      .quick-title {
        font-size: 14px;
        font-weight: 600;
      }
    }

    .quick-cards {
      gap: 8px;

      .quick-card {
        padding: 10px 12px;
        border-radius: 10px;
        gap: 8px;

        .card-bullet {
          font-size: 14px;
        }

        .card-icon {
          font-size: 16px;
        }

        .card-text {
          font-size: 13px;
        }
      }
    }
  }
}

// Dark mode styles for FloatingChatBot
.dark {
  .quick-actions {
    background: linear-gradient(180deg, rgba(30, 41, 59, 0.4) 0%, transparent 100%);
    border-bottom-color: rgba(148, 163, 184, 0.1);

    .welcome-text {
      color: #e2e8f0;

      .wave-icon {
        color: #60a5fa;
      }
    }

    :deep(.quick-start-section) {
      .quick-start-header {
        .quick-title {
          color: #e2e8f0;
        }
      }

      .quick-card {
        background: rgba(30, 41, 59, 0.6);
        border-color: rgba(148, 163, 184, 0.15);
        color: #cbd5e1;

        .card-icon {
          color: #94a3b8;
        }

        &:hover {
          background: rgba(59, 130, 246, 0.15);
          border-color: #3b82f6;
          color: #f1f5f9;
          box-shadow: 0 2px 8px rgba(59, 130, 246, 0.15);

          .card-icon {
            color: #60a5fa;
          }
        }
      }
    }
  }

  .messages-container {
    .message-wrapper {
      .assistant-message-group {
        .assistant-content-wrapper {
          .status-content {
            .status-item {
              color: #94a3b8;
            }
          }

          .thinking-content {
            border-left-color: #60a5fa;
            background: rgba(59, 130, 246, 0.08);

            &:hover {
              background: rgba(59, 130, 246, 0.12);
            }

            .thinking-header {
              &:hover {
                .thinking-label {
                  color: #60a5fa;
                }

                .thinking-toggle-icon {
                  color: #60a5fa;
                }
              }

              .thinking-icon {
                color: #60a5fa;
              }

              .thinking-label {
                color: #cbd5e1;
              }

              .thinking-time {
                color: #94a3b8;
              }

              .thinking-toggle-icon {
                color: #94a3b8;
              }
            }

            .thinking-text {
              color: #94a3b8;
            }
          }

          .message-text {
            background: rgba(40, 40, 40, 0.9);
            border-color: rgba(255, 255, 255, 0.1);
            color: #eee;
          }
        }
      }

      .sources-section {
        .sources-title {
          color: #aaa;

          .sources-icon {
            color: #a78bfa;
          }
        }

        .sources-list {
          .source-item {
            background: rgba(40, 40, 40, 0.6);
            border-color: rgba(255, 255, 255, 0.08);

            &:hover {
              background: rgba(139, 92, 246, 0.15);
              border-color: rgba(139, 92, 246, 0.4);
            }

            .source-header {
              .source-collection {
                color: #a78bfa;
              }

              .source-similarity {
                color: #888;
              }
            }

            .source-question {
              color: #aaa;
            }
          }
        }
      }
    }
  }
}

.messages-container {
  padding: 16px;
  cursor: default; // Default cursor
  user-select: text; // Allow text selection

  .message-wrapper {
    margin-bottom: 16px;

    &:last-child {
      margin-bottom: 0;
    }

    .assistant-message-group {
      display: flex;
      gap: 10px;
      margin-bottom: 16px;

      .message-avatar {
        width: 32px;
        height: 32px;
        border-radius: 50%;
        display: flex;
        align-items: center;
        justify-content: center;
        color: #fff;
        flex-shrink: 0;
        background: linear-gradient(135deg, #8b5cf6 0%, #7c3aed 100%);

        .el-icon {
          font-size: 18px;
        }

        .avatar-icon {
          width: 18px;
          height: 18px;
          object-fit: contain;
          filter: brightness(0) invert(1);
        }
      }

      .assistant-content-wrapper {
        flex: 1;
        display: flex;
        flex-direction: column;
        gap: 8px;
        max-width: 70%;

        .status-content {
          font-size: 12px;

          .status-item {
            color: #475569;
            line-height: 1.6;
            margin-bottom: 4px;
            animation: fadeInStep 0.3s ease;

            &:last-child {
              margin-bottom: 0;
            }
          }
        }

        .thinking-content {
          margin-top: 8px;
          margin-bottom: 10px;
          border-left: 3px solid #3b82f6;
          padding-left: 10px;
          background: rgba(59, 130, 246, 0.03);
          border-radius: 0 6px 6px 0;
          transition: all 0.3s ease;

          &:hover {
            background: rgba(59, 130, 246, 0.05);
          }

          .thinking-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 8px 10px 8px 0;
            cursor: pointer;
            user-select: none;
            transition: all 0.2s ease;

            &:hover {
              .thinking-label {
                color: #3b82f6;
              }

              .thinking-toggle-icon {
                color: #3b82f6;
              }
            }

            .thinking-header-left {
              display: flex;
              align-items: center;
              gap: 4px;
              flex: 1;
            }

            .thinking-icon {
              font-size: 13px;
              color: #3b82f6;
            }

            .thinking-label {
              font-size: 12px;
              font-weight: 600;
              color: #475569;
              transition: color 0.2s ease;
            }

            .thinking-time {
              font-size: 11px;
              color: #64748b;
              font-weight: 400;
            }

            .thinking-toggle-icon {
              font-size: 13px;
              color: #94a3b8;
              transition: all 0.2s ease;
            }
          }

          .thinking-text {
            padding: 0 10px 10px 0;
            color: #64748b;
            line-height: 1.6;
            font-size: 12px;
            white-space: pre-wrap;
            word-wrap: break-word;
            animation: fadeIn 0.3s ease;
          }
        }

        @keyframes fadeIn {
          from {
            opacity: 0;
            transform: translateY(-5px);
          }
          to {
            opacity: 1;
            transform: translateY(0);
          }
        }

        .message-content {
          max-width: 100%;

          .message-text {
            background: #fff;
            color: #333;
            border-radius: 12px 12px 12px 4px;
            padding: 10px 14px;
            font-size: 14px;
            line-height: 1.5;
            word-wrap: break-word;
            border: 1px solid #e2e8f0;

            :deep(p) {
              margin: 0;
              margin-bottom: 8px;

              &:last-child {
                margin-bottom: 0;
              }
            }

            :deep(code) {
              background: rgba(0, 0, 0, 0.08);
              padding: 2px 6px;
              border-radius: 4px;
              font-size: 13px;
            }

            :deep(pre) {
              background: rgba(0, 0, 0, 0.08);
              padding: 12px;
              border-radius: 6px;
              overflow-x: auto;
              margin: 8px 0;

              code {
                background: none;
                padding: 0;
              }
            }
          }
        }
      }
    }

    .message.user {
      display: flex;
      gap: 10px;
      flex-direction: row-reverse;
      margin-bottom: 16px;

      .message-avatar {
        width: 32px;
        height: 32px;
        border-radius: 50%;
        display: flex;
        align-items: center;
        justify-content: center;
        color: #fff;
        flex-shrink: 0;
        background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);

        .el-icon {
          font-size: 18px;
        }
      }

      .message-content {
        max-width: 70%;

        .message-text {
          background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
          color: #fff;
          border-radius: 12px 12px 4px 12px;
          padding: 10px 14px;
          font-size: 14px;
          line-height: 1.5;
          word-wrap: break-word;

          :deep(p) {
            margin: 0;
            margin-bottom: 8px;

            &:last-child {
              margin-bottom: 0;
            }
          }

          :deep(code) {
            background: rgba(255, 255, 255, 0.2);
            padding: 2px 6px;
            border-radius: 4px;
            font-size: 13px;
          }

          :deep(pre) {
            background: rgba(0, 0, 0, 0.2);
            padding: 12px;
            border-radius: 6px;
            overflow-x: auto;
            margin: 8px 0;

            code {
              background: none;
              padding: 0;
            }
          }
        }
      }
    }

    .sources-section {
      margin-top: 10px;
      margin-left: 42px;
      max-width: min(calc(100% - 42px), 70%); // Keep width consistent with message-content
      font-size: 12px;

      .sources-title {
        display: flex;
        align-items: center;
        gap: 6px;
        margin-bottom: 8px;
        color: #666;
        font-weight: 500;

        .sources-icon {
          font-size: 14px;
          color: #8b5cf6;
        }
      }

      .sources-list {
        display: flex;
        flex-direction: column;
        gap: 8px;

        .source-item {
          padding: 8px 10px;
          background: rgba(248, 249, 250, 0.8);
          border: 1px solid rgba(0, 0, 0, 0.06);
          border-radius: 6px;
          transition: all 0.2s;
          cursor: pointer;

          &:hover {
            background: rgba(139, 92, 246, 0.05);
            border-color: rgba(139, 92, 246, 0.3);
            transform: translateX(2px);
          }

          .source-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 4px;

            .source-collection {
              font-weight: 500;
              color: #8b5cf6;
              font-size: 11px;
            }

            .source-similarity {
              font-size: 11px;
              color: #999;
              font-weight: 500;
            }
          }

          .source-question {
            color: #666;
            line-height: 1.5;
            font-size: 12px;
            overflow: hidden;
            text-overflow: ellipsis;
            display: -webkit-box;
            -webkit-line-clamp: 2;
            line-clamp: 2;
            -webkit-box-orient: vertical;
          }
        }
      }
    }
  }
}

.typing-indicator {
  display: flex;
  gap: 4px;
  padding: 4px 0;

  span {
    width: 8px;
    height: 8px;
    background: #999;
    border-radius: 50%;
    animation: typing 1.4s infinite;

    &:nth-child(1) {
      animation-delay: 0s;
    }

    &:nth-child(2) {
      animation-delay: 0.2s;
    }

    &:nth-child(3) {
      animation-delay: 0.4s;
    }
  }
}

@keyframes typing {
  0%,
  60%,
  100% {
    opacity: 0.3;
    transform: scale(0.8);
  }
  30% {
    opacity: 1;
    transform: scale(1);
  }
}

@keyframes fadeInStep {
  from {
    opacity: 0;
    transform: translateY(-5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.input-container {
  flex-shrink: 0;
  padding: 12px 16px;
  background: #fff;
  border-top: 1px solid rgba(0, 0, 0, 0.06);
  cursor: default; // Default cursor, not drag

  .input-wrapper {
    display: flex;
    flex-direction: column;
    padding: 14px 16px;
    background: rgba(248, 249, 250, 0.5);
    border: 1.5px solid rgba(0, 0, 0, 0.1);
    border-radius: 12px;
    transition: all 0.2s;
    gap: 12px;
    cursor: text; // Show text cursor when clicking the entire area

    &:focus-within {
      border-color: #3b82f6;
      background: #fff;
      box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.08);
    }

    .message-input {
      width: 100%;
      border: none;
      outline: none;
      background: transparent;
      font-size: 14px;
      color: #333;
      cursor: text !important;
      padding: 6px 0;
      min-height: 24px;
      line-height: 24px;
      resize: none;
      overflow-y: hidden;
      max-height: 150px;
      font-family: inherit;

      &::placeholder {
        color: #999;
      }

      &:disabled {
        opacity: 0.6;
        cursor: not-allowed;
      }
    }

    .bottom-controls {
      display: flex;
      align-items: center;
      justify-content: space-between;

      .left-controls {
        display: flex;
        align-items: center;
        gap: 8px;
      }

      .right-controls {
        display: flex;
        align-items: center;
      }
    }

    .mode-selector-button {
      height: 28px;
      padding: 0 10px;
      border-radius: 14px;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 5px;
      background: #fff;
      border: 1.5px solid rgba(148, 163, 184, 0.2);
      cursor: pointer !important;
      transition: all 0.2s ease;
      color: #3b82f6;
      font-size: 12px;
      font-weight: 500;

      .el-icon {
        font-size: 14px;
      }

      .mode-text {
        line-height: 1;
      }

      &:hover {
        border-color: #3b82f6;
        background: rgba(59, 130, 246, 0.05);
      }
    }

    .control-button {
      width: 74px;
      height: 28px;
      border-radius: 20px;
      display: flex;
      align-items: center;
      gap: 5px;
      justify-content: center;
      background: rgba(248, 249, 250, 0.8);
      border: 1px solid rgba(0, 0, 0, 0.08);
      cursor: pointer !important;
      transition: all 0.2s ease;
      color: #666;
      font-size: 12px;

      .el-icon {
        font-size: 14px;
      }

      .control-icon {
        width: 16px;
        height: 16px;
        object-fit: contain;
        opacity: 0.6;
        transition: all 0.2s ease;
      }

      &:hover {
        background: rgba(59, 130, 246, 0.1);
        border-color: rgba(59, 130, 246, 0.3);
        color: #3b82f6;
        transform: scale(1.05);

        .control-icon {
          opacity: 1;
          filter: brightness(0) saturate(100%) invert(42%) sepia(93%) saturate(1352%)
            hue-rotate(201deg) brightness(103%) contrast(97%);
        }
      }

      &.active {
        background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
        border-color: transparent;
        color: #fff;
        box-shadow: 0 2px 8px rgba(59, 130, 246, 0.3);

        .control-icon {
          opacity: 1;
          filter: brightness(0) invert(1);
        }
      }
    }

    .send-icon {
      font-size: 28px;
      color: #cbd5e1;
      cursor: pointer;
      transition: all 0.2s;
      transform: rotate(45deg);

      &.active {
        color: #3b82f6;

        &:hover {
          color: #2563eb;
          transform: rotate(45deg) scale(1.1);
        }
      }

      &:not(.active) {
        cursor: not-allowed;
      }
    }

    .stop-icon {
      width: 28px;
      height: 28px;
      cursor: pointer;
      transition: all 0.2s;
      object-fit: contain;

      &:hover {
        transform: scale(1.1);
        opacity: 0.8;
      }
    }
  }
}

// History drawer styles
.history-content {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.conversation-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}

.conversation-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px;
  margin-bottom: 8px;
  background: #fff;
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 8px;
  transition: all 0.2s;
  cursor: pointer;

  &:hover {
    border-color: #3b82f6;
    box-shadow: 0 2px 8px rgba(59, 130, 246, 0.1);
    transform: translateX(-2px);
  }

  &.active {
    background: rgba(59, 130, 246, 0.08);
    border-color: #3b82f6;
  }

  .conversation-content {
    flex: 1;
    min-width: 0;
  }

  .conversation-title {
    font-size: 14px;
    font-weight: 500;
    color: #333;
    margin-bottom: 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .conversation-time {
    font-size: 12px;
    color: #999;
  }

  .conversation-actions {
    display: flex;
    gap: 8px;
    opacity: 0;
    transition: opacity 0.2s;
  }

  &:hover .conversation-actions {
    opacity: 1;
  }

  .action-btn {
    font-size: 16px;
    color: #666;
    cursor: pointer;
    padding: 4px;
    border-radius: 4px;
    transition: all 0.2s;

    &:hover {
      background: rgba(59, 130, 246, 0.1);
      color: #3b82f6;
    }

    &.delete-btn:hover {
      background: rgba(255, 71, 87, 0.1);
      color: #ff4757;
    }
  }
}

.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #999;

  .empty-icon {
    font-size: 48px;
    color: #ddd;
    margin-bottom: 12px;
  }

  p {
    font-size: 14px;
  }
}

// Dragging state
.floating-chatbot-wrapper.dragging {
  .chatbot-button {
    transition: none !important;
    opacity: 0.7;
    cursor: grabbing;
  }

  .chat-dialog {
    .chat-header {
      cursor: grabbing;
    }
  }
}

// Dark mode
.dark {
  .chatbot-button {
    background: linear-gradient(135deg, #2563eb 0%, #7c3aed 100%);
    box-shadow:
      0 4px 16px rgba(37, 99, 235, 0.4),
      0 2px 8px rgba(0, 0, 0, 0.3);

    &:hover {
      background: linear-gradient(135deg, #3b82f6 0%, #8b5cf6 100%);
      box-shadow:
        0 6px 24px rgba(59, 130, 246, 0.5),
        0 4px 12px rgba(0, 0, 0, 0.4);
    }
  }

  .chat-dialog {
    background: linear-gradient(180deg, rgba(30, 41, 59, 0.98) 0%, rgba(15, 23, 42, 0.98) 100%);
    box-shadow:
      0 12px 40px rgba(0, 0, 0, 0.5),
      0 0 0 1px rgba(148, 163, 184, 0.15);
    backdrop-filter: blur(30px) saturate(180%);
  }

  .chat-header {
    background: rgba(30, 41, 59, 0.95);
    border-bottom-color: rgba(148, 163, 184, 0.15);

    .header-icon {
      filter: brightness(0) saturate(100%) invert(58%) sepia(83%) saturate(2456%) hue-rotate(200deg)
        brightness(103%) contrast(98%);
    }

    .header-title {
      color: #f1f5f9;
    }

    .header-status {
      color: #6ee7b7;
      background: linear-gradient(135deg, rgba(16, 185, 129, 0.2) 0%, rgba(5, 150, 105, 0.2) 100%);

      &.offline {
        color: #94a3b8;
        background: rgba(100, 116, 139, 0.25);
      }
    }

    .header-star-icon {
      color: #ffd700;

      :deep(svg) {
        filter: drop-shadow(0 0 3px rgba(255, 215, 0, 0.4));
      }

      &:hover {
        color: #ffd33d;

        :deep(svg) {
          filter: drop-shadow(0 0 4px rgba(255, 215, 0, 0.6));
        }
      }
    }

    .action-icon {
      background: rgba(30, 41, 59, 0.6);
      border-color: rgba(148, 163, 184, 0.2);
      color: #94a3b8;

      &:hover {
        background: rgba(59, 130, 246, 0.15);
        border-color: rgba(59, 130, 246, 0.3);
        color: #60a5fa;
        box-shadow: 0 2px 6px rgba(59, 130, 246, 0.2);
      }

      &.disabled {
        opacity: 0.3;
        cursor: not-allowed;
        pointer-events: none;

        &:hover {
          background: rgba(30, 41, 59, 0.6);
          border-color: rgba(148, 163, 184, 0.2);
          color: #94a3b8;
          box-shadow: none;
        }
      }

      &.new-icon,
      &.expand-icon {
        background: linear-gradient(
          135deg,
          rgba(59, 130, 246, 0.12) 0%,
          rgba(139, 92, 246, 0.12) 100%
        );
        border-color: rgba(59, 130, 246, 0.3);
        color: #60a5fa;

        &:hover {
          background: linear-gradient(
            135deg,
            rgba(59, 130, 246, 0.2) 0%,
            rgba(139, 92, 246, 0.2) 100%
          );
          border-color: rgba(59, 130, 246, 0.45);
          color: #93c5fd;
          box-shadow: 0 2px 6px rgba(59, 130, 246, 0.25);
        }
      }

      &.close-icon {
        background: rgba(239, 68, 68, 0.12);
        border-color: rgba(239, 68, 68, 0.3);
        color: #f87171;

        &:hover {
          background: rgba(239, 68, 68, 0.2);
          color: #fca5a5;
          border-color: rgba(239, 68, 68, 0.45);
          box-shadow: 0 2px 6px rgba(239, 68, 68, 0.25);
        }
      }
    }
  }

  .input-container {
    background: rgba(30, 30, 30, 0.95);
    border-top-color: rgba(255, 255, 255, 0.06);

    .input-wrapper {
      background: rgba(40, 40, 40, 0.5);
      border-color: rgba(255, 255, 255, 0.1);

      &:focus-within {
        background: rgba(50, 50, 50, 0.8);
        border-color: #3b82f6;
        box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
      }

      .message-input {
        color: #eee;

        &::placeholder {
          color: #888;
        }
      }

      .mode-selector-button {
        background: rgba(70, 70, 70, 0.9);
        border-color: rgba(148, 163, 184, 0.2);
        color: #60a5fa;

        &:hover {
          border-color: #3b82f6;
          background: rgba(59, 130, 246, 0.15);
        }
      }

      .control-button {
        background: rgba(60, 60, 60, 0.8);
        border-color: rgba(148, 163, 184, 0.15);
        color: #ccc;

        &:hover {
          background: rgba(59, 130, 246, 0.2);
          border-color: rgba(59, 130, 246, 0.4);
          color: #60a5fa;
        }

        &.active {
          background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
          border-color: transparent;
          color: #fff;
        }
      }
    }
  }

  .conversation-item {
    background: rgba(40, 40, 40, 0.8);
    border-color: rgba(255, 255, 255, 0.08);

    &:hover {
      border-color: #3b82f6;
      box-shadow: 0 2px 8px rgba(59, 130, 246, 0.2);
    }

    &.active {
      background: rgba(59, 130, 246, 0.15);
    }

    .conversation-title {
      color: #eee;
    }

    .conversation-time {
      color: #888;
    }

    .action-btn {
      color: #999;

      &:hover {
        color: #60a5fa;
      }

      &.delete-btn:hover {
        color: #ff4757;
      }
    }
  }

  .empty-state {
    color: #888;

    .empty-icon {
      color: #555;
    }
  }

  .quick-actions {
    background: linear-gradient(180deg, rgba(30, 41, 59, 0.4) 0%, transparent 100%);
    border-bottom-color: rgba(148, 163, 184, 0.1);

    .welcome-text {
      color: #e2e8f0;

      .wave-icon {
        color: #60a5fa;
      }
    }

    .action-item {
      background: rgba(30, 41, 59, 0.6);
      border-color: rgba(148, 163, 184, 0.15);
      color: #cbd5e1;

      .el-icon {
        color: #94a3b8;
      }

      &:hover {
        background: rgba(59, 130, 246, 0.15);
        border-color: #3b82f6;
        color: #f1f5f9;
        box-shadow: 0 2px 8px rgba(59, 130, 246, 0.15);

        .el-icon {
          color: #60a5fa;
        }
      }
    }
  }

  .messages-container {
    .message-wrapper {
      .assistant-message-group {
        .assistant-content-wrapper {
          .status-content {
            .status-item {
              color: #94a3b8;
            }
          }

          .thinking-content {
            border-left-color: #60a5fa;
            background: rgba(59, 130, 246, 0.08);

            &:hover {
              background: rgba(59, 130, 246, 0.12);
            }

            .thinking-header {
              &:hover {
                .thinking-label {
                  color: #60a5fa;
                }

                .thinking-toggle-icon {
                  color: #60a5fa;
                }
              }

              .thinking-icon {
                color: #60a5fa;
              }

              .thinking-label {
                color: #cbd5e1;
              }

              .thinking-time {
                color: #94a3b8;
              }

              .thinking-toggle-icon {
                color: #94a3b8;
              }
            }

            .thinking-text {
              color: #94a3b8;
            }
          }

          .message-text {
            background: rgba(40, 40, 40, 0.9);
            border-color: rgba(255, 255, 255, 0.1);
            color: #eee;
          }
        }
      }

      .sources-section {
        .sources-title {
          color: #aaa;

          .sources-icon {
            color: #a78bfa;
          }
        }

        .sources-list {
          .source-item {
            background: rgba(40, 40, 40, 0.6);
            border-color: rgba(255, 255, 255, 0.08);

            &:hover {
              background: rgba(139, 92, 246, 0.15);
              border-color: rgba(139, 92, 246, 0.4);
            }

            .source-header {
              .source-collection {
                color: #a78bfa;
              }

              .source-similarity {
                color: #888;
              }
            }

            .source-question {
              color: #aaa;
            }
          }
        }
      }
    }
  }
}

// Utility classes
.mr-2 {
  margin-right: 8px;
}

.ml-2 {
  margin-left: 8px;
}

// Dropdown menu styles
.el-dropdown-menu {
  border-radius: 8px;
  padding: 4px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.15);

  .el-dropdown-menu__item {
    display: flex;
    align-items: center;
    padding: 8px 12px;
    border-radius: 6px;
    transition: all 0.2s;

    &.active {
      background: rgba(59, 130, 246, 0.1);
      color: #3b82f6;
      font-weight: 500;

      .el-icon {
        color: #3b82f6;
      }
    }

    &:hover:not(.is-disabled) {
      background: rgba(59, 130, 246, 0.05);
    }

    .el-icon {
      font-size: 16px;
    }
  }
}

// Responsive
@media (max-width: 768px) {
  .chat-dialog {
    width: 320px;
    height: 500px;
  }

  .chatbot-button {
    width: 48px;
    height: 48px;

    .bot-icon {
      font-size: 24px;
    }
  }
}
</style>

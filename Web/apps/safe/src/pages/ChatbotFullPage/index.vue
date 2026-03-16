<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <div class="chatbot-fullpage">
    <!-- Left Sidebar -->
    <div class="sidebar" :class="{ collapsed: !sidebarVisible }">
      <div class="sidebar-header">
        <div class="sidebar-title">
          <img :src="sparklesIcon" class="title-icon" alt="Primus-SaFE Agent" />
          <span>Primus-SaFE</span>
        </div>

        <div class="sidebar-header-actions">
          <el-button text class="collapse-btn" @click="sidebarVisible = false">
            <img :src="sidebarIcon" class="collapse-icon" alt="Collapse" />
          </el-button>
        </div>
      </div>

      <!-- New Chat Button -->
      <div class="sidebar-new-chat">
        <el-button text class="new-chat-btn" @click="startNewConversation">
          <el-icon><Edit /></el-icon>
          <span>New Chat</span>
        </el-button>
      </div>

      <!-- History Section Header -->
      <div class="sidebar-section-header" @click="historyCollapsed = !historyCollapsed">
        <div class="section-header-left">
          <el-icon class="section-icon" :class="{ collapsed: historyCollapsed }">
            <ArrowRight />
          </el-icon>
          <span class="section-title">History</span>
        </div>
      </div>

      <!-- Loading -->
      <div v-if="loadingHistory && !historyCollapsed" class="sidebar-loading">
        <el-skeleton :rows="5" animated />
      </div>

      <!-- Conversation List -->
      <div
        v-else-if="conversationList.length > 0 && !historyCollapsed"
        class="sidebar-list"
        ref="conversationListRef"
        @scroll="(e) => handleScrollLoadMore(e, loadMoreConversations)"
      >
        <div
          v-for="item in conversationList"
          :key="item.conversation_id"
          class="sidebar-item"
          :class="{ active: currentConversationId === item.conversation_id }"
          @click="loadConversation(item.conversation_id)"
        >
          <div class="sidebar-item-content">
            <el-icon class="item-icon"><ChatDotRound /></el-icon>
            <div class="item-info">
              <div class="item-title">{{ item.title }}</div>
            </div>
          </div>
          <div class="sidebar-item-actions" @click.stop>
            <el-dropdown trigger="click">
              <el-icon class="more-icon"><MoreFilled /></el-icon>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item @click="handleShareConversation(item.conversation_id)">
                    <el-icon><Upload /></el-icon>
                    Share
                  </el-dropdown-item>
                  <el-dropdown-item @click="handleEditTitle(item)">
                    <el-icon><Edit /></el-icon>
                    Edit Title
                  </el-dropdown-item>
                  <el-dropdown-item @click="handleDeleteConversation(item)">
                    <el-icon><Delete /></el-icon>
                    Delete
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
        <!-- Loading more -->
        <div v-if="loadingMoreConversations" class="sidebar-loading-more">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>Loading more...</span>
        </div>
        <!-- No more -->
        <div v-if="conversationHasNoMore && conversationList.length > 0" class="sidebar-no-more">
          No more conversations
        </div>
      </div>

      <!-- Empty State -->
      <div v-else-if="!historyCollapsed" class="sidebar-empty">
        <el-icon><ChatDotRound /></el-icon>
        <p>No conversations yet</p>
      </div>
    </div>

    <!-- Main Content -->
    <div class="main-content" :class="{ 'sidebar-collapsed': !sidebarVisible }">
      <!-- Top Bar -->
      <div class="topbar">
        <div class="topbar-left">
          <el-button
            v-if="!sidebarVisible"
            text
            class="expand-sidebar-btn"
            @click="sidebarVisible = true"
          >
            <img :src="sidebarIcon" class="expand-icon" alt="Expand" />
          </el-button>
        </div>
        <div class="topbar-right">
          <div class="status-indicator" :class="{ offline: !currentModeOnline }">
            <span class="status-dot"></span>
            <span class="status-text"
              >{{ currentModeLabel }} {{ currentModeOnline ? 'Online' : 'Offline' }}</span
            >
          </div>
          <el-button class="back-button" @click="goBack">
            <el-icon class="back-icon"><Back /></el-icon>
            <span class="back-text">Back</span>
          </el-button>
        </div>
      </div>

      <!-- Messages Area -->
      <div class="chat-container" :class="{ 'has-messages': messages.length > 0 }">
        <div
          class="messages-scroll"
          ref="messagesContainer"
          @scroll="(e) => handleScrollLoadMore(e, loadMoreMessages)"
        >
          <!-- Welcome Screen (when no messages) -->
          <div v-if="messages.length === 0" class="welcome-screen">
            <!-- Ask Mode Quick Start -->
            <QuickStartCards
              v-if="mode === 'ask'"
              :config="askModeQuickStart"
              @card-click="setQuestion"
            />

            <!-- Agent Mode Quick Start -->
            <QuickStartCards
              v-else-if="mode === 'agent'"
              :config="agentQuickStartConfig"
              @card-click="setInputText"
            />
          </div>

          <!-- Messages List -->
          <div v-else class="messages-list">
            <div
              v-for="(message, index) in messages"
              :key="index"
              class="message-wrapper"
              :class="message.role"
            >
              <!-- Assistant message group (avatar, state, thinking, response) -->
              <div v-if="message.role === 'assistant'" class="assistant-message-group">
                <!-- Avatar (shown only once) -->
                <div class="message-avatar">
                  <img :src="sparklesIcon" class="avatar-icon" alt="Primus-SaFE Agent" />
                </div>

                <div class="assistant-content-wrapper">
                  <!-- Message sender -->
                  <div class="message-header">
                    <span class="message-sender">Primus-SaFE Agent</span>
                  </div>

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

                  <!-- Thinking process (streaming accumulation) -->
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

                  <!-- Response content (bubble) -->
                  <div v-if="message.content || (loading && index === messages.length - 1)">
                    <!-- Loading indicator (only for the last message when loading and no content yet) -->
                    <div
                      v-if="!message.content && loading && index === messages.length - 1"
                      class="typing-indicator"
                    >
                      <span></span>
                      <span></span>
                      <span></span>
                    </div>
                    <!-- Message content -->
                    <div
                      v-else-if="message.content"
                      class="message-text"
                      v-html="formatMessage(message.content)"
                      @click="handleImageClick"
                    ></div>
                  </div>
                </div>
              </div>

              <!-- User message -->
              <div v-else class="message-row user">
                <div class="message-avatar">
                  <el-icon><User /></el-icon>
                </div>

                <div class="message-content">
                  <div class="message-header">
                    <span class="message-sender">You</span>
                  </div>
                  <div
                    v-if="message.content"
                    class="message-text"
                    v-html="formatMessage(message.content)"
                    @click="handleImageClick"
                  ></div>
                </div>
              </div>

              <!-- Knowledge Base sources (displayed below the bubble) -->
              <div
                v-if="
                  message.role === 'assistant' &&
                  ((message.sources && message.sources.length > 0) || message.sourcesLoading)
                "
                class="sources-section"
              >
                <div class="sources-title">
                  <el-icon class="sources-icon"><Collection /></el-icon>
                  <span v-if="!message.sourcesLoading"
                    >Knowledge Base Sources ({{ message.sources?.length || 0 }})</span
                  >
                  <span v-else>Knowledge Base Sources</span>
                </div>

                <!-- Loading state -->
                <div v-if="message.sourcesLoading" class="sources-loading">
                  <el-icon class="is-loading"><Loading /></el-icon>
                  <span>Loading sources...</span>
                </div>

                <!-- Sources list -->
                <div v-else class="sources-list">
                  <div
                    v-for="(source, sourceIndex) in message.sources"
                    :key="sourceIndex"
                    class="source-item"
                    @click="source.item_id && handleViewSourceDetail(source.item_id)"
                  >
                    <div class="source-header">
                      <span class="source-collection">{{ source.collection_name }}</span>
                      <!-- <span class="source-similarity"
                        >{{
                          source.similarity ? (source.similarity * 100).toFixed(1) : 'N/A'
                        }}%</span
                      > -->
                    </div>
                    <div class="source-question">{{ source.question }}</div>
                  </div>
                </div>
              </div>

              <!-- Actions section (Share & Feedback buttons) -->
              <div
                v-if="
                  message.role === 'assistant' &&
                  message.messageId &&
                  messages[index - 1]?.messageId
                "
                class="actions-section"
              >
                <!-- Copy button -->
                <el-tooltip content="Copy" placement="top">
                  <el-button text class="action-button" @click="copyText(message.content)">
                    <el-icon><DocumentCopy /></el-icon>
                  </el-button>
                </el-tooltip>

                <!-- Vote buttons -->
                <div class="vote-buttons">
                  <el-tooltip content="Upvote" placement="top">
                    <button
                      class="vote-button"
                      :class="{ active: message.voteType === 'up' }"
                      @click="handleVote(index, 'up')"
                      :disabled="loadingVote"
                    >
                      <span v-if="message.voteType === 'up'" class="vote-emoji">👍</span>
                      <img v-else :src="likeLight" class="vote-icon" alt="Like" />
                    </button>
                  </el-tooltip>
                  <el-tooltip content="Downvote" placement="top">
                    <button
                      class="vote-button"
                      :class="{ active: message.voteType === 'down' }"
                      @click="handleVote(index, 'down')"
                      :disabled="loadingVote"
                    >
                      <img
                        :src="message.voteType === 'down' ? dislikeActive : dislikeLight"
                        class="vote-icon"
                        alt="Dislike"
                      />
                    </button>
                  </el-tooltip>
                </div>

                <!-- Share button -->
                <el-tooltip content="Share" placement="top">
                  <el-button
                    text
                    class="action-button"
                    @click="handleShare(messages[index - 1].messageId!, message.messageId!)"
                  >
                    <el-icon><Upload /></el-icon>
                  </el-button>
                </el-tooltip>
              </div>

              <!-- Feedback Form (shown when downvoting) -->
              <div
                v-if="message.role === 'assistant' && message.showFeedbackForm && message.messageId"
                class="feedback-form"
              >
                <div class="feedback-form-header">
                  <span class="feedback-form-title"
                    >Please share more details about your feedback:</span
                  >
                </div>

                <div class="feedback-reasons">
                  <!-- Default reasons -->
                  <div
                    v-for="reason in defaultFeedbackReasons"
                    :key="reason"
                    class="feedback-reason-tag"
                    :class="{ selected: message.selectedReasons?.includes(reason) }"
                    @click="toggleFeedbackReason(message, reason)"
                  >
                    {{ reason }}
                  </div>

                  <!-- Custom reasons (already added) -->
                  <div
                    v-for="customReason in message.selectedReasons?.filter(
                      (r) => !defaultFeedbackReasons.includes(r),
                    )"
                    :key="customReason"
                    class="feedback-reason-tag custom selected"
                  >
                    {{ customReason }}
                    <el-icon
                      class="remove-icon"
                      @click.stop="removeCustomFeedbackReason(message, customReason)"
                    >
                      <Close />
                    </el-icon>
                  </div>

                  <!-- Add custom reason input -->
                  <div class="feedback-custom-input">
                    <input
                      v-model="message.customReason"
                      type="text"
                      placeholder="Add custom reason..."
                      @keypress.enter="addCustomFeedbackReason(message)"
                      class="custom-reason-input"
                    />
                    <el-button
                      v-if="message.customReason?.trim()"
                      size="small"
                      type="primary"
                      @click="addCustomFeedbackReason(message)"
                    >
                      Add
                    </el-button>
                  </div>
                </div>

                <div class="feedback-form-actions">
                  <el-button size="small" @click="cancelFeedbackForm(message)"> Cancel </el-button>
                  <el-button
                    size="small"
                    type="primary"
                    @click="submitFeedbackWithReasons(index)"
                    :loading="loadingVote"
                  >
                    Submit
                  </el-button>
                </div>
              </div>
            </div>
          </div>

          <!-- Loading more messages at bottom -->
          <div v-if="loadingMoreMessages && messages.length > 0" class="messages-loading-more">
            <el-icon class="is-loading"><Loading /></el-icon>
            <span>Loading more messages...</span>
          </div>
          <!-- No more messages indicator at bottom -->
          <div
            v-if="messageHasNoMore && messages.length > 0 && currentConversationId"
            class="messages-no-more"
          >
            No more messages
          </div>
        </div>

        <!-- QA Detail Dialog -->
        <QADetailDialog
          v-model="qaDetailDialogVisible"
          :loading="qaDetailLoading"
          :data="qaDetailData"
        />

        <!-- Input Area -->
        <div class="input-section">
          <SlashCommandMenu
            :groups="slashGroups"
            :display-items="slashDisplayItems"
            :active-index="slashActiveIndex"
            :visible="slashMenuVisible"
            :is-searching="slashIsSearching"
            @select="handleSlashSelect"
            @update:active-index="slashActiveIndex = $event"
          />

          <div class="input-wrapper" @click="focusInput">
            <!-- Input -->
            <textarea
              ref="inputRef"
              v-model="userInput"
              placeholder="Ask me anything... (type / for commands)"
              @keydown="onInputKeydown"
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

    <ImagePreviewOverlay
      :visible="imagePreviewVisible"
      :url="imagePreviewUrl"
      @close="closeImagePreview"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ChatDotRound,
  User,
  Delete,
  Edit,
  Back,
  MoreFilled,
  Collection,
  View,
  ArrowDown,
  ArrowUp,
  ArrowRight,
  MagicStick,
  Position,
  Upload,
  Loading,
  Close,
  DocumentCopy,
} from '@element-plus/icons-vue'
import deepThinkIcon from '@/assets/icons/deepthink.png'
import sparklesIcon from '@/assets/icons/sparkles.png'
import stopIcon from '@/assets/icons/stop.png'
import likeLight from '@/assets/icons/like-light.png'
import dislikeLight from '@/assets/icons/dislike-light.png'
import dislikeActive from '@/assets/icons/dislike-active.png'
import sidebarIcon from '@/assets/icons/sidebar.png'
import {
  chatBotAsk,
  createConversation,
  saveMessage,
  getConversationList,
  getMessageList,
  updateConversation,
  deleteConversation,
  checkHealth,
  getQAItemDetail,
  batchGetMessages,
  submitFeedback,
  cancelVote,
  type ConversationListItem,
  type SourceItem,
  type QAAnswerDetailData,
  type SourceRef,
  type MessageData,
} from '@/services/chatbot'
import { marked } from 'marked'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import { copyText, buildMessageShareUrl, buildConversationShareUrl } from '@/utils'
import QADetailDialog from '@/components/Base/QADetailDialog.vue'
import ImagePreviewOverlay from '@/components/Base/ImagePreviewOverlay.vue'
import { useImagePreview } from '@/composables/useImagePreview'
import WorkflowProgress from './Components/WorkflowProgress.vue'
import ActionStatus from './Components/ActionStatus.vue'
import InlineConfirmForm from './Components/InlineConfirmForm.vue'
import QuickStartCards from './Components/QuickStartCards.vue'
import SlashCommandMenu from './Components/SlashCommandMenu.vue'
import { useSlashCommands } from './composables/useSlashCommands'
import type { SlashCommandHandlers } from './composables/slashCommandExecutor'
import { builtinSlashCommands } from './constants/slashCommands'
import {
  askModeQuickStart,
  agentModeQuickStart,
  normalUserQuickStart,
  workspaceAdminQuickStart,
} from './constants/quickStartData'
import {
  agentSocket,
  checkAgentHealth,
  type WorkflowMessageData,
  type ActionMessageData,
  type ConfirmMessageData,
  type MessageEvent,
  type TimeoutMessageData,
} from '@/services/agent'

// Types
interface Message {
  role: 'user' | 'assistant'
  content: string
  messageId?: number // Message ID
  agentHasSteps?: boolean // Whether Agent contains steps/actions
  agentSaved?: boolean // Whether Agent message has been saved
  statusMessages?: string[] // State messages (displayed line by line)
  thinking?: string // Thinking process (streaming accumulation)
  thinkingExpanded?: boolean // Whether thinking process is expanded
  thinkingTime?: number // Thinking time (milliseconds)
  thinkingStartTime?: number // Thinking start time
  sources?: (SourceRef & Partial<SourceItem>)[] // Knowledge Base sources (supports full and simplified formats)
  sourcesLoading?: boolean // Knowledge Base sources loading
  voteType?: 'up' | 'down' | null // Vote type
  feedbackId?: number | null // Feedback ID
  showFeedbackForm?: boolean // Whether to show feedback form
  selectedReasons?: string[] // Selected feedback reasons
  customReason?: string // Custom feedback reason (being typed)
  // Agent mode fields
  workflow?: WorkflowMessageData // Workflow progress
  actions?: ActionMessageData[] // Action state list
  confirmData?: ConfirmMessageData // Confirm form data
  confirmLoading?: boolean // Confirm form submitting
  confirmReadonly?: boolean // Whether confirm form is readonly (already submitted)
  confirmedSelections?: Record<string, unknown> // Submitted selections
  savedSelectionConfirm?: ConfirmMessageData // Saved selection form data
}

interface HistoryItem {
  question: string
  answer: string
}

// Router
const router = useRouter()

// User store
const userStore = useUserStore()
const wsStore = useWorkspaceStore()

// Agent mode quick start config based on user role
const agentQuickStartConfig = computed(() => {
  if (userStore.isManager) {
    return agentModeQuickStart
  }

  return wsStore.isCurrentWorkspaceAdmin() ? workspaceAdminQuickStart : normalUserQuickStart
})

// State
const messagesContainer = ref<HTMLElement>()
const inputRef = ref<HTMLTextAreaElement>()
const userInput = ref('')
const messages = ref<Message[]>([])
const loading = ref(false)
const enableThinking = ref(false)
const currentConversationId = ref<string>('')
const sidebarVisible = ref(true)
const askOnline = ref(true)
const agentOnline = ref(false)
const mode = ref<'ask' | 'agent'>('ask')
const currentModeOnline = computed(() =>
  mode.value === 'ask' ? askOnline.value : agentOnline.value,
)
const currentModeLabel = computed(() => (mode.value === 'ask' ? 'Ask' : 'Agent'))

// Slash commands
const slashHandlers: SlashCommandHandlers = {
  onClear: () => {
    messages.value = []
    ElMessage.success('Conversation cleared')
  },
  onNewChat: () => startNewConversation(),
  onSwitchMode: (m) => handleModeChange(m),
  onToggleThink: () => {
    enableThinking.value = !enableThinking.value
    ElMessage.success(`Deep thinking ${enableThinking.value ? 'enabled' : 'disabled'}`)
  },
  onHelp: () => {
    const helpLines = builtinSlashCommands
      .filter((cmd) => cmd.mode === 'all' || cmd.mode === mode.value)
      .map((cmd) => `<b>/${cmd.command}</b> — ${cmd.description}`)
      .join('<br/>')
    ElMessage({
      message: helpLines,
      type: 'info',
      duration: 5000,
      dangerouslyUseHTMLString: true,
    })
  },
  onNavigate: (route) => router.push(route),
  onNavigateWithAction: (route, action) => router.push({ path: route, query: { action } }),
  onFillInput: (text) => { userInput.value = text },
}

const {
  showMenu: slashMenuVisible,
  isSearching: slashIsSearching,
  displayItems: slashDisplayItems,
  groupedCommands: slashGroups,
  activeIndex: slashActiveIndex,
  handleSlashKeydown,
  selectDisplayItem: handleSlashSelect,
} = useSlashCommands(userInput, mode, slashHandlers)

// Agent state
const agentConnected = ref(false)
const agentSessionId = ref('')
const currentOperationId = ref('')

// History state
const conversationList = ref<ConversationListItem[]>([])
const loadingHistory = ref(false)
const conversationListRef = ref<HTMLElement>()
const loadingMoreConversations = ref(false)
const conversationCurrentPage = ref(1)
const conversationPageSize = ref(20)
const conversationHasNoMore = ref(false)
const historyCollapsed = ref(false)

// QA Detail Dialog state
const qaDetailDialogVisible = ref(false)
const qaDetailLoading = ref(false)
const qaDetailData = ref<QAAnswerDetailData | null>(null)

// Share state
const isSharedMode = ref(false) // Whether in share mode (entered from share link)
const sharedConversationId = ref('') // Used when sharing entire conversation (to avoid writing back to original)

// Vote state
const loadingVote = ref(false)

// Default feedback reasons
const defaultFeedbackReasons = [
  'Incorrect workflow',
  'Outdated or invalid information',
  'Not relevant to the question',
  'Misunderstood user intent',
  'Factually incorrect',
  'Too generic to be useful',
]

// Message pagination state
const loadingMoreMessages = ref(false)
const messageCurrentPage = ref(1)
const messagePageSize = ref(20)
const messageHasNoMore = ref(false)

// AbortController
let abortController: AbortController | null = null

// Generate conversation ID
const generateConversationId = () => {
  const timestamp = Date.now()
  const randomStr = Math.random().toString(36).substring(2, 10)
  return `${timestamp}_${randomStr}`
}

// Generate operation ID
const generateOperationId = () => {
  return `op-${Date.now()}-${Math.random().toString(36).substring(2, 10)}`
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

      // More user-friendly error message
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

// Agent: Handle incoming messages
const handleAgentMessage = (event: MessageEvent) => {
  const { type, data, operation_id } = event

  // Ignore messages if operation was cancelled (currentOperationId is empty)
  // or if message is from a different operation
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
    // Streaming mode - append text
    message.content += text

    // Auto scroll to bottom during streaming
    nextTick(() => {
      if (messagesContainer.value) {
        const container = messagesContainer.value
        const isNearBottom =
          container.scrollHeight - container.scrollTop - container.clientHeight < 200

        // Only auto-scroll if user is near bottom (not scrolling up to read)
        if (isNearBottom) {
          container.scrollTop = container.scrollHeight
        }
      }
    })
  } else if (text) {
    // Non-streaming mode with content - replace text
    message.content = text
  }
  // If not streaming and no text, keep existing content (don't clear it)

  if (data.done) {
    loading.value = false

    // Save assistant message to history when done
    if (currentConversationId.value && message && !message.agentHasSteps && !message.agentSaved) {
      try {
        // Get question message ID (previous message should be the user message)
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
        // Store message ID
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

  // Find existing action with same name and update it, or add new one
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
  // Find the last assistant message and add confirm form to it
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

  // Save error message to history
  if (currentConversationId.value && !message.agentSaved) {
    try {
      // Get question message ID (previous message should be the user message)
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
      // Store message ID
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

  // Save timeout message to history
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

  // When operation completes, restore selection forms as readonly
  const message = messages.value[messageIndex]
  if (message && message.savedSelectionConfirm && message.confirmedSelections) {
    // Restore the selection form as readonly
    message.confirmData = message.savedSelectionConfirm
    message.confirmReadonly = true
  }

  // Save assistant message to history only when steps are involved
  if (currentConversationId.value && message && message.agentHasSteps && !message.agentSaved) {
    ;(async () => {
      try {
        // Get question message ID (previous message should be the user message)
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
        // Store message ID
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
      // Send selection
      agentSocket.sendSelection(data.selections, currentOperationId.value)
      // Save the selection form data and user's selections
      message.savedSelectionConfirm = { ...message.confirmData }
      message.confirmedSelections = data.selections
      // Hide the form temporarily (will show readonly version after complete)
      message.confirmData = undefined
      message.confirmLoading = false
    } else if (data.approved !== undefined) {
      // Send confirmation
      agentSocket.sendConfirmation(data.approved, message.confirmData.id, currentOperationId.value)
      // For execution type, hide the form
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

  // For execution type, send rejection
  if (confirmType === 'execution') {
    handleInlineConfirmSubmit(messageIndex, { approved: false })
  } else {
    // Notify backend user cancelled (agent mode only)
    if (mode.value === 'agent' && agentConnected.value && currentOperationId.value) {
      try {
        agentSocket.cancelOperation(currentOperationId.value)
      } catch (error) {
        console.error('Failed to send user cancel:', error)
      }
    }

    // For selection type, just hide the form and save the message
    message.confirmData = undefined
    loading.value = false

    // Save the message with cancelled selection to history
    if (currentConversationId.value && !message.agentSaved) {
      try {
        // Get question message ID (previous message should be the user message)
        const questionMessageId =
          messageIndex > 0 ? messages.value[messageIndex - 1]?.messageId : undefined

        // Add cancellation note to content
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
        // Store message ID
        messages.value[messageIndex].messageId = saveAssistantMsgResponse.data.id
        messages.value[messageIndex].agentSaved = true
      } catch (error) {
        console.error('Failed to save cancelled selection message:', error)
      }
    }
  }
}

// Agent: Send message
const sendAgentMessage = async (query: string) => {
  if (!agentConnected.value) {
    ElMessage.error('Agent not connected')
    return
  }

  // Add user message immediately
  messages.value.push({
    role: 'user',
    content: query,
  })
  const userMessageIndex = messages.value.length - 1
  userInput.value = ''

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

  // Create conversation if it doesn't exist
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
      // Continue even if conversation creation fails
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
      // Store message ID
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

// Format structured list items (with | separators)
const formatStructuredList = (content: string): string => {
  // First, normalize line breaks (convert <br> to \n for processing)
  const normalized = content.replace(/<br\s*\/?>/gi, '\n')

  // Match lines that start with • and contain | separators
  const lines = normalized.split('\n')
  const formattedLines = lines.map((line) => {
    const trimmedLine = line.trim()

    // Check if line starts with • and contains |
    if (/^[•·]\s*.+\|.+/.test(trimmedLine)) {
      // Split by | separator
      const parts = trimmedLine
        .substring(1)
        .trim()
        .split('|')
        .map((p) => p.trim())

      if (parts.length > 1) {
        // Create structured HTML
        return `<div class="workload-item">
          <div class="workload-main">${parts[0]}</div>
          <div class="workload-details">
            ${parts
              .slice(1)
              .map((part) => `<span class="workload-detail">${part}</span>`)
              .join('')}
          </div>
        </div>`
      }
    }

    // Return original line if not a structured list item
    return line
  })

  return formattedLines.join('\n')
}

// Format message with markdown
const formatMessage = (content: string) => {
  // First, apply structured list formatting
  const formatted = formatStructuredList(content)
  // Then apply markdown
  return marked(formatted, { breaks: true })
}

const { imagePreviewVisible, imagePreviewUrl, handleImageClick, closeImagePreview } =
  useImagePreview()

// Serialize agent data for saving
const serializeAgentData = (message: Message) => {
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

  // Return JSON string if there's any agent data, otherwise null
  return Object.keys(agentData).length > 0 ? JSON.stringify(agentData) : null
}

// Deserialize agent data when loading
const deserializeAgentData = (thinkingStr: string | null): Partial<Message> => {
  if (!thinkingStr) return {}

  try {
    const agentData = JSON.parse(thinkingStr)
    const result: Partial<Message> = {}

    if (agentData.workflow) {
      result.workflow = agentData.workflow
    }

    if (agentData.actions) {
      result.actions = agentData.actions
    }

    if (agentData.confirmData) {
      result.confirmData = agentData.confirmData
      result.confirmReadonly = true // Loaded confirm forms are readonly
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

// Toggle thinking expansion
const toggleThinking = (message: Message) => {
  message.thinkingExpanded = !message.thinkingExpanded
}

// Format thinking time
const formatThinkingTime = (timeMs: number) => {
  const seconds = Math.floor(timeMs / 1000)
  if (seconds < 1) {
    return '< 1s'
  } else if (seconds < 60) {
    return `${seconds}s`
  } else {
    const minutes = Math.floor(seconds / 60)
    const remainingSeconds = seconds % 60
    return `${minutes}m ${remainingSeconds}s`
  }
}

// Set question from quick actions (Ask mode - auto send)
const setQuestion = (question: string) => {
  userInput.value = question
  nextTick(() => {
    sendMessage()
  })
}

// Set input text from quick actions (Agent mode - only fill input, don't send)
const setInputText = (text: string) => {
  userInput.value = text
  // Focus on input field so user can modify
  nextTick(() => {
    inputRef.value?.focus()
  })
}

// Scroll to top (used for loading conversation history)
const scrollToTop = () => {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = 0
  }
}

// Scroll to show new question (only once)
const scrollToNewQuestion = () => {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}

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

// Stop generation
const stopGeneration = async () => {
  // Agent mode: send cancel request via WebSocket
  if (mode.value === 'agent') {
    if (agentConnected.value && currentOperationId.value) {
      try {
        agentSocket.cancelOperation(currentOperationId.value)
        loading.value = false
        ElMessage.info('Operation cancelled')

        // Save the incomplete message to history
        const lastAssistantIndex = messages.value.length - 1
        const lastMessage = messages.value[lastAssistantIndex]
        if (
          currentConversationId.value &&
          lastMessage &&
          lastMessage.role === 'assistant' &&
          !lastMessage.agentSaved
        ) {
          try {
            // Get question message ID (previous message should be the user message)
            const questionMessageId =
              lastAssistantIndex > 0 ? messages.value[lastAssistantIndex - 1]?.messageId : undefined

            // Add cancellation note to content
            const cancelledContent = lastMessage.content
              ? `${lastMessage.content}\n\n_[Cancelled by user]_`
              : '_[Cancelled by user]_'

            const saveAssistantMsgResponse = await saveMessage({
              conversation_id: currentConversationId.value,
              role: 'assistant',
              content: cancelledContent,
              thinking: serializeAgentData(lastMessage),
              question_message_id: questionMessageId,
              message_type: 'Agent',
            })
            // Store message ID
            messages.value[lastAssistantIndex].messageId = saveAssistantMsgResponse.data.id
            messages.value[lastAssistantIndex].agentSaved = true
          } catch (error) {
            console.error('Failed to save cancelled message:', error)
          }
        }
      } catch (error) {
        console.error('Failed to cancel operation:', error)
        ElMessage.error('Failed to cancel operation')
      }
    } else {
      loading.value = false
    }
    return
  }

  // Ask mode: abort HTTP request
  if (abortController) {
    abortController.abort()
    loading.value = false
    ElMessage.info('Generation stopped')
  }
}

// Auto-adjust textarea height
const adjustTextareaHeight = () => {
  if (inputRef.value) {
    inputRef.value.style.height = 'auto'
    inputRef.value.style.height = inputRef.value.scrollHeight + 'px'
  }
}

// Watch userInput to auto-adjust height
watch(userInput, () => {
  nextTick(() => {
    adjustTextareaHeight()
  })
})

// Handle Enter key press
const onInputKeydown = (event: KeyboardEvent) => {
  if (handleSlashKeydown(event)) return
  if (event.key === 'Enter') {
    handleEnterKey(event)
  }
}

const handleEnterKey = (event: KeyboardEvent) => {
  if (event.shiftKey) {
    return
  }

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

  // Agent mode
  if (mode.value === 'agent') {
    return sendAgentMessage(query)
  }

  // Ask mode (original logic)
  // Add user message immediately (UI responds instantly)
  messages.value.push({
    role: 'user',
    content: query,
  })
  userInput.value = ''

  // Reset textarea height
  nextTick(() => {
    if (inputRef.value) {
      inputRef.value.style.height = 'auto'
    }
  })

  // Create assistant message placeholder immediately
  const assistantMessageIndex = messages.value.length
  messages.value.push({
    role: 'assistant',
    content: '',
    statusMessages: [],
    thinking: '',
    thinkingExpanded: true, // Expanded by default
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

  // If in share mode and asking first question, create new conversation and exit share mode
  if (isSharedMode.value && !currentConversationId.value) {
    isSharedMode.value = false
  }

  // Create conversation asynchronously (don't block UI)
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
      // Remove messages on error
      messages.value.splice(messages.value.length - 2, 2)
      loading.value = false
      return
    }
  }

  // Save user message and record message ID
  try {
    const saveUserMsgResponse = await saveMessage({
      conversation_id: currentConversationId.value,
      role: 'user',
      content: query,
      thinking: null,
      message_type: 'Ask',
    })
    // Record the user message's messageId
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
        // Just update content, don't auto scroll
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

        // Save assistant message and record message ID
        try {
          // Build source_refs, save complete SourceItem info
          const sourceRefs = messages.value[assistantMessageIndex].sources
            ? messages.value[assistantMessageIndex].sources!.map((source) => ({
                source: 'qa_items',
                item_id: source.item_id,
                type: source.type,
                collection_id: source.collection_id,
                collection_name: source.collection_name,
                question: source.question,
                similarity: source.similarity,
              }))
            : []

          // Get the question message's ID
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
          // Record the assistant message's messageId
          messages.value[assistantMessageIndex].messageId = saveAssistantMsgResponse.data.id
        } catch (error) {
          console.error('Failed to save assistant message:', error)
        }
      },
      abortController.signal,
      (statusMessage: string) => {
        // Handle status messages (add to list), don't auto scroll
        if (!messages.value[assistantMessageIndex].statusMessages) {
          messages.value[assistantMessageIndex].statusMessages = []
        }
        messages.value[assistantMessageIndex].statusMessages!.push(statusMessage)
      },
      (sources) => {
        // Handle knowledge base sources, don't auto scroll
        // Convert to SourceRef & SourceItem format
        messages.value[assistantMessageIndex].sources = sources.map((source) => ({
          source: 'qa_items',
          ...source,
        }))
      },
      (thinkingContent: string) => {
        // Handle thinking content (streaming accumulation), don't auto scroll
        if (!messages.value[assistantMessageIndex].thinking) {
          messages.value[assistantMessageIndex].thinking = ''
          messages.value[assistantMessageIndex].thinkingStartTime = Date.now()
        }
        messages.value[assistantMessageIndex].thinking += thinkingContent

        // Update thinking time
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

// Start new conversation
const startNewConversation = () => {
  messages.value = []
  currentConversationId.value = ''
  isSharedMode.value = false
  sharedConversationId.value = ''
  // Reset message pagination
  messageCurrentPage.value = 1
  messageHasNoMore.value = false
  ElMessage.success('New conversation started')
}

// Handle share
const handleShare = async (questionMessageId: number, answerMessageId: number) => {
  try {
    await copyText(buildMessageShareUrl(router, questionMessageId, answerMessageId))
  } catch (error) {
    console.error('Failed to copy share link:', error)
  }
}

// Handle share conversation
const handleShareConversation = async (conversationId: string) => {
  try {
    await copyText(buildConversationShareUrl(router, conversationId))
  } catch (error) {
    console.error('Failed to copy conversation share link:', error)
  }
}

// Handle vote (like/dislike)
const handleVote = async (messageIndex: number, voteType: 'up' | 'down') => {
  const message = messages.value[messageIndex]

  if (!message || !message.messageId) {
    ElMessage.error('Message not found')
    return
  }

  // If same vote type, cancel vote
  if (message.voteType === voteType) {
    loadingVote.value = true
    try {
      await cancelVote({
        message_id: message.messageId,
      })
      message.voteType = null
      message.feedbackId = null
      message.showFeedbackForm = false
      message.selectedReasons = []
      message.customReason = ''
      ElMessage.success('Vote cancelled')
    } catch (error: unknown) {
      console.error('Vote error:', error)
      const err = error as { message?: string }
      ElMessage.error('Operation failed: ' + (err?.message || 'Unknown error'))
    } finally {
      loadingVote.value = false
    }
    return
  }

  // If voting down, show feedback form
  if (voteType === 'down') {
    message.showFeedbackForm = true
    message.selectedReasons = []
    message.customReason = ''
    return
  }

  // If voting up, submit directly
  loadingVote.value = true
  try {
    const response = await submitFeedback({
      vote_type: voteType,
      message_id: message.messageId,
    })

    message.voteType = voteType
    message.feedbackId = response.data.id
    ElMessage.success('Upvoted successfully')
  } catch (error: unknown) {
    console.error('Vote error:', error)
    const err = error as { message?: string }
    ElMessage.error('Operation failed: ' + (err?.message || 'Unknown error'))
  } finally {
    loadingVote.value = false
  }
}

// Toggle feedback reason selection
const toggleFeedbackReason = (message: Message, reason: string) => {
  if (!message.selectedReasons) {
    message.selectedReasons = []
  }

  const index = message.selectedReasons.indexOf(reason)
  if (index > -1) {
    message.selectedReasons.splice(index, 1)
  } else {
    message.selectedReasons.push(reason)
  }
}

// Add custom feedback reason
const addCustomFeedbackReason = (message: Message) => {
  const customReason = message.customReason?.trim()
  if (!customReason) return

  if (!message.selectedReasons) {
    message.selectedReasons = []
  }

  // Check if already exists
  if (!message.selectedReasons.includes(customReason)) {
    message.selectedReasons.push(customReason)
  }

  message.customReason = ''
}

// Remove custom feedback reason
const removeCustomFeedbackReason = (message: Message, reason: string) => {
  if (!message.selectedReasons) return

  const index = message.selectedReasons.indexOf(reason)
  if (index > -1) {
    message.selectedReasons.splice(index, 1)
  }
}

// Submit feedback with reasons
const submitFeedbackWithReasons = async (messageIndex: number) => {
  const message = messages.value[messageIndex]

  if (!message || !message.messageId) {
    ElMessage.error('Message not found')
    return
  }

  loadingVote.value = true

  try {
    const reason =
      message.selectedReasons && message.selectedReasons.length > 0
        ? message.selectedReasons.join('; ')
        : undefined

    const response = await submitFeedback({
      vote_type: 'down',
      message_id: message.messageId,
      reason,
    })

    message.voteType = 'down'
    message.feedbackId = response.data.id
    message.showFeedbackForm = false
    ElMessage.success('Downvoted successfully')
  } catch (error: unknown) {
    console.error('Vote error:', error)
    const err = error as { message?: string }
    ElMessage.error('Operation failed: ' + (err?.message || 'Unknown error'))
  } finally {
    loadingVote.value = false
  }
}

// Cancel feedback form
const cancelFeedbackForm = (message: Message) => {
  message.showFeedbackForm = false
  message.selectedReasons = []
  message.customReason = ''
}

// Complete missing source_refs data (backward compatible with historical data)
const completeSourceRefs = async (
  sourceRefs: (SourceRef & Partial<SourceItem>)[],
): Promise<(SourceRef & Partial<SourceItem>)[]> => {
  // Check for incomplete data
  const hasIncompleteData = sourceRefs.some((ref) => !ref.similarity)
  if (!hasIncompleteData) {
    return sourceRefs
  }

  // Complete the data
  return await Promise.all(
    sourceRefs.map(async (ref) => {
      // If already has complete data, return directly
      if (ref.similarity && ref.question && ref.collection_name) {
        return ref
      }
      // If data is missing, try to fetch and complete from API
      if (ref.source === 'qa_items' && ref.item_id) {
        try {
          const res = await getQAItemDetail(ref.item_id)
          const primaryQuestion =
            res.questions?.find((q) => q.is_primary)?.question ?? res.questions?.[0]?.question ?? ''
          return {
            source: 'qa_items',
            item_id: res.answer.id,
            type: 'qa_item',
            collection_id: res.answer.collection_id,
            collection_name: res.answer.collection_name || 'SaFE-QA',
            question: primaryQuestion,
            similarity: 0.6, // Hardcoded to 60% for historical data
          }
        } catch (error) {
          console.error(`Failed to load source ${ref.item_id}:`, error)
        }
      }
      // If fetch fails, return original data
      return ref
    }),
  )
}

// Convert MessageData to Message
const convertMessageData = (msg: MessageData): Message => {
  const baseMessage = {
    role: msg.role as 'user' | 'assistant',
    content: msg.content,
    messageId: msg.id,
    statusMessages: [],
    thinking: '',
    thinkingExpanded: false,
    thinkingTime: 0,
    sources: msg.source_refs || [],
    sourcesLoading:
      msg.source_refs?.some((ref: SourceRef & Partial<SourceItem>) => !ref.similarity) || false,
    voteType: msg.user_vote_type || null,
    feedbackId: msg.feedback_id || null,
  }

  // Try to deserialize agent data from thinking field
  const agentData = deserializeAgentData(msg.thinking)

  return {
    ...baseMessage,
    ...agentData,
  }
}

// Asynchronously complete message list sources (backward compatible with historical data)
const completeMessageListSources = (messageDataList: MessageData[], startIndex = 0) => {
  messageDataList.forEach(async (msg, index) => {
    if (msg.source_refs && msg.source_refs.length > 0) {
      const completedSources = await completeSourceRefs(msg.source_refs)
      const absoluteIndex = startIndex + index
      if (messages.value[absoluteIndex]?.messageId === msg.id) {
        messages.value[absoluteIndex].sources = completedSources
        messages.value[absoluteIndex].sourcesLoading = false
      }
    }
  })
}

// Load shared messages
const loadSharedMessages = async (questionMessageId: number, answerMessageId: number) => {
  try {
    // Batch get question and answer messages
    const response = await batchGetMessages([questionMessageId, answerMessageId])

    if (!response.success || response.data.length !== 2) {
      ElMessage.error('Invalid share link')
      return
    }

    // Find question and answer from response
    const questionMsg = response.data.find((msg) => msg.id === questionMessageId)
    const answerMsg = response.data.find((msg) => msg.id === answerMessageId)

    // Validate that these messages are a valid Q&A pair
    if (
      !questionMsg ||
      !answerMsg ||
      questionMsg.role !== 'user' ||
      answerMsg.role !== 'assistant'
    ) {
      ElMessage.error('Invalid share link')
      return
    }

    // Load messages to UI
    messages.value = [convertMessageData(questionMsg), convertMessageData(answerMsg)]

    // Set to share mode, no conversation selected
    isSharedMode.value = true
    currentConversationId.value = ''
    sharedConversationId.value = ''

    await nextTick()
    scrollToTop()

    // Asynchronously complete source_refs with missing similarity (backward compatible)
    completeMessageListSources([questionMsg, answerMsg], 0)
  } catch (error) {
    console.error('Failed to load shared messages:', error)
    ElMessage.error('Failed to load shared content')
  }
}

// Load shared conversation (from share link)
const loadSharedConversation = async (conversationId: string) => {
  try {
    // Reset message pagination
    messageCurrentPage.value = 1
    messageHasNoMore.value = false

    // Record shared conversation id for pagination, but don't bind to currentConversationId
    sharedConversationId.value = conversationId

    const response = await getMessageList(conversationId, {
      page: messageCurrentPage.value,
      page_size: messagePageSize.value,
    })

    const messageData = response.data.items
    messages.value = messageData.map(convertMessageData)

    // Set to share mode, no conversation selected (avoid sending new messages into this conversation)
    isSharedMode.value = true
    currentConversationId.value = ''

    // Update pagination state
    const { page, page_size, total } = response.data.pagination
    messageHasNoMore.value = page * page_size >= total

    await nextTick()
    scrollToTop()

    // Asynchronously complete source_refs with missing similarity (backward compatible)
    completeMessageListSources(messageData, 0)
  } catch (error) {
    console.error('Failed to load shared conversation:', error)
    ElMessage.error('Failed to load shared conversation')
  }
}

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

    // Check if there are more pages
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

// Generic scroll handler - load more when scrolled to bottom
const handleScrollLoadMore = (event: Event, loadMoreFn: () => void) => {
  const target = event.target as HTMLElement
  const scrollTop = target.scrollTop
  const scrollHeight = target.scrollHeight
  const clientHeight = target.clientHeight

  // Load more when scrolled to bottom (with 50px threshold)
  if (scrollTop + clientHeight >= scrollHeight - 50) {
    loadMoreFn()
  }
}

// Load messages for a conversation
const loadMessages = async (options: {
  conversationId?: string
  reset?: boolean
  silent?: boolean
}) => {
  const { conversationId, reset = false, silent = false } = options

  // Determine the conversationId to use
  const targetConversationId = conversationId || currentConversationId.value
  if (!targetConversationId) {
    console.error('No conversation ID provided')
    return
  }

  // Check if loading more messages
  if (!reset && (loadingMoreMessages.value || messageHasNoMore.value)) {
    return
  }

  // Set loading state
  if (!reset) {
    loadingMoreMessages.value = true
  }

  try {
    // Update pagination state
    if (reset) {
      messageCurrentPage.value = 1
      messageHasNoMore.value = false
    } else {
      messageCurrentPage.value += 1
    }

    // Get message list
    const response = await getMessageList(targetConversationId, {
      page: messageCurrentPage.value,
      page_size: messagePageSize.value,
    })
    const messageData = response.data.items

    // Record current message list length (for append mode)
    const currentMessagesCount = messages.value.length

    // Convert message data
    const newMessages = messageData.map(convertMessageData)

    // Update message list
    if (reset) {
      messages.value = newMessages
      currentConversationId.value = targetConversationId
      isSharedMode.value = false
      sharedConversationId.value = ''
    } else {
      messages.value = [...messages.value, ...newMessages]
    }

    // Update pagination state
    const { page, page_size, total } = response.data.pagination
    messageHasNoMore.value = page * page_size >= total

    // Scroll and notification (only in reset mode)
    if (reset) {
      await nextTick()
      scrollToTop()
      if (!silent) {
        ElMessage.success('Conversation loaded')
      }
    }

    // Asynchronously complete source_refs with missing similarity (backward compatible)
    completeMessageListSources(messageData, reset ? 0 : currentMessagesCount)
  } catch (error) {
    console.error('Failed to load messages:', error)
    ElMessage.error(reset ? 'Failed to load conversation' : 'Failed to load more messages')
  } finally {
    if (!reset) {
      loadingMoreMessages.value = false
    }
  }
}

// Load conversation (wrapper for loadMessages with reset)
const loadConversation = async (conversationId: string, silent = false) => {
  await loadMessages({ conversationId, reset: true, silent })
}

// Load more messages (wrapper for loadMessages without reset)
const loadMoreMessages = async () => {
  const conversationId = isSharedMode.value ? sharedConversationId.value : undefined
  await loadMessages({ reset: false, conversationId })
}

// Handle edit title
const handleEditTitle = async (item: ConversationListItem) => {
  const result = await ElMessageBox.prompt('Enter new title', 'Edit Title', {
    confirmButtonText: 'OK',
    cancelButtonText: 'Cancel',
    inputValue: item.title,
    inputPattern: /.+/,
    inputErrorMessage: 'Title cannot be empty',
  }).catch(() => null)
  const newTitle = result ? (result as { value: string }).value : null

  if (!newTitle) return

  try {
    await updateConversation(item.conversation_id, { title: newTitle })
    ElMessage.success('Title updated')
    await fetchConversationList()
  } catch (error) {
    console.error('Failed to update title:', error)
    ElMessage.error('Failed to update title')
  }
}

// Handle delete conversation
const handleDeleteConversation = async (item: ConversationListItem) => {
  try {
    await ElMessageBox.confirm(
      'Are you sure you want to delete this conversation?',
      'Delete Conversation',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning',
      },
    )

    await deleteConversation(item.conversation_id)
    ElMessage.success('Conversation deleted')

    if (currentConversationId.value === item.conversation_id) {
      startNewConversation()
    }

    await fetchConversationList()
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Failed to delete conversation:', error)
      ElMessage.error('Failed to delete conversation')
    }
  }
}

// Go back
const goBack = () => {
  router.back()
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
    stopAskHealthCheck()
    startAgentHealthCheck()
    ElMessage.success('Switched to Agent mode')
  } else {
    // Switch to ask mode
    if (agentConnected.value) {
      disconnectAgent()
    }
    stopAgentHealthCheck()
    startAskHealthCheck()
    ElMessage.success('Switched to Ask mode')
  }
}

// Focus input
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

// Check health status
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

// Lifecycle
onMounted(async () => {
  await fetchConversationList()

  // Check if there's a mode parameter from floating window
  const modeParam = router.currentRoute.value.query.mode as string
  if (modeParam === 'agent') {
    // Switch to agent mode
    mode.value = 'agent'
    connectAgent()
  }

  // Check if it's a share link
  const shareParam = router.currentRoute.value.query.share as string
  const qid = router.currentRoute.value.query.qid as string
  const aid = router.currentRoute.value.query.aid as string
  const cid = router.currentRoute.value.query.cid as string

  if (shareParam === '1' && qid && aid) {
    // Load shared messages from share link
    await loadSharedMessages(parseInt(qid), parseInt(aid))
  } else if (shareParam === 'conv' && cid) {
    await loadSharedConversation(cid)
  } else {
    // Check if there's a conversation ID from floating window
    const conversationId = router.currentRoute.value.query.conversationId as string
    if (conversationId) {
      // Auto-load conversation (silent mode, no success message)
      await loadConversation(conversationId, true)
    }
  }

  if (mode.value === 'ask') {
    startAskHealthCheck()
  } else {
    startAgentHealthCheck()
  }
})

onUnmounted(() => {
  if (abortController) {
    abortController.abort()
  }
  // Clear health check interval
  stopAskHealthCheck()
  stopAgentHealthCheck()
  // Disconnect agent
  if (agentConnected.value) {
    disconnectAgent()
  }
})
</script>

<style scoped lang="scss">
@import './styles.scss';
</style>

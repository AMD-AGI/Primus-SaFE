<template>
  <div class="agent-page">
    <p class="large-title">AI Agent Platform</p>
    <p class="text-gray-500">AI-Powered Analysis and Skills Management - Beta</p>

    <!-- Tabs -->
    <el-tabs v-model="activeTab" class="agent-tabs">
      <el-tab-pane label="GPU Agent" name="gpu-agent">
        <div class="agent-container">
      <!-- Left Panel - Capabilities -->
      <div class="capabilities-panel">
        <el-card class="capability-card">
          <template #header>
            <div class="card-header">
              <i i="ep-info-filled" class="mr-2" />
              <span>Agent Capabilities</span>
            </div>
          </template>

          <div v-loading="loadingCapabilities" class="capabilities-content">
            <div
              v-for="capability in capabilities"
              :key="capability.type"
              class="capability-item"
            >
              <div class="capability-header">
                <span class="capability-name">{{ capability.name }}</span>
              </div>
              <p class="capability-desc">{{ capability.description }}</p>
              <div class="capability-examples">
                <p class="examples-title">Example Queries:</p>
                <div
                  v-for="(example, idx) in capability.examples"
                  :key="idx"
                  class="example-item"
                  @click="useExample(example)"
                >
                  <i i="ep-chat-dot-round" class="example-icon" />
                  <span>{{ example }}</span>
                </div>
              </div>
            </div>
          </div>
        </el-card>
      </div>

      <!-- Right Panel - Chat Window -->
      <div class="chat-panel">
        <el-card class="chat-card">
          <template #header>
            <div class="card-header">
              <div class="flex items-center">
                <i i="ep-chat-line-round" class="mr-2" />
                <span>Chat Window</span>
                <el-tag
                  v-if="currentSessionId"
                  size="small"
                  type="success"
                  class="ml-2"
                >
                  In Session
                </el-tag>
              </div>
              <div class="flex gap-2">
                <el-button
                  size="small"
                  @click="toggleHistoryPanel"
                >
                  <i i="ep-clock" class="mr-1" />
                  History
                </el-button>
                <el-button
                  size="small"
                  @click="clearChat"
                  :disabled="messages.length === 0"
                >
                  <i i="ep-delete" class="mr-1" />
                  Clear Chat
                </el-button>
              </div>
            </div>
          </template>

          <!-- Message List -->
          <div class="messages-container" ref="messagesContainer">
            <div v-if="messages.length === 0" class="empty-state-wrapper">
              <div class="empty-state">
                <div class="example-queries-section">
                  <p class="queries-hint">Try asking about GPU utilization:</p>
                  <div class="example-queries">
                    <div class="query-item" @click="useExample('What is the GPU utilization trend over the last 7 days?')">
                      <i i="ep-trend-charts" class="query-icon" />
                      <span>What is the GPU utilization trend over the last 7 days?</span>
                    </div>
                    <div class="query-item" @click="useExample('How many GPUs are currently in use in the cluster?')">
                      <i i="ep-data-analysis" class="query-icon" />
                      <span>How many GPUs are currently in use in the cluster?</span>
                    </div>
                    <div class="query-item" @click="useExample('Which workloads have low GPU utilization?')">
                      <i i="ep-warning" class="query-icon" />
                      <span>Which workloads have low GPU utilization?</span>
                    </div>
                  </div>
                </div>

                <!-- Input Box integrated into empty state -->
                <div class="integrated-input-container">
                  <div class="chat-input-wrapper">
                    <el-input
                      v-model="userInput"
                      type="textarea"
                      :rows="2"
                      placeholder="Ask me anything about GPU utilization..."
                      @keydown.ctrl.enter="sendMessage"
                      class="chat-input"
                    />
                    <button
                      class="send-btn"
                      @click="sendMessage"
                      :disabled="!userInput.trim() || loading"
                    >
                      <el-icon v-if="!loading" :size="20">
                        <Promotion />
                      </el-icon>
                      <el-icon v-else :size="20" class="is-loading">
                        <Loading />
                      </el-icon>
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <div
              v-for="(message, index) in messages"
              :key="index"
              :class="['message', message.role]"
            >
              <div class="message-avatar">
                <i
                  v-if="message.role === 'user'"
                  i="ep-user"
                  class="avatar-icon"
                />
                <i
                  v-else
                  i="ep-cpu"
                  class="avatar-icon"
                />
              </div>
              <div class="message-content">
                <div class="message-header">
                  <span class="message-role">
                    {{ message.role === 'user' ? 'You' : 'AI Agent' }}
                  </span>
                  <span class="message-time">{{ message.timestamp }}</span>
                </div>
                <!-- Live loading content (only for the last assistant message while loading) -->
                <template v-if="loading && message.role === 'assistant' && index === messages.length - 1">
                  <!-- Step progress bars -->
                  <div v-if="currentSteps.size > 0" class="steps-progress-container">
                    <div
                      v-for="[stepId, stepInfo] in currentSteps"
                      :key="stepId"
                      class="step-progress-item"
                      :class="stepInfo.status"
                    >
                      <div class="step-header">
                        <span class="step-icon">
                          <i v-if="stepInfo.status === 'pending'" i="ep-timer" />
                          <i v-else-if="stepInfo.status === 'running'" i="ep-loading" class="rotating" />
                          <i v-else-if="stepInfo.status === 'completed'" i="ep-circle-check" />
                          <i v-else-if="stepInfo.status === 'error'" i="ep-circle-close" />
                        </span>
                        <span class="step-name">
                          [{{ stepInfo.index }}/{{ stepInfo.total }}] {{ stepInfo.name }}
                        </span>
                        <span v-if="stepInfo.progress !== undefined" class="step-progress-value">
                          {{ stepInfo.progress }}%
                        </span>
                      </div>
                      <div v-if="stepInfo.progress !== undefined && stepInfo.status === 'running'" class="progress-bar">
                        <div
                          class="progress-fill"
                          :style="{ width: `${stepInfo.progress}%` }"
                        />
                      </div>
                      <div v-if="stepInfo.description" class="step-description">
                        {{ stepInfo.description }}
                      </div>
                    </div>
                  </div>

                  <!-- Live processing steps -->
                  <div v-if="currentProcessingSteps.length > 0" class="live-processing-steps">
                    <div class="steps-timeline">
                      <div
                        v-for="(step, stepIdx) in currentProcessingSteps"
                        :key="stepIdx"
                        class="step-item"
                        :class="step.type"
                      >
                        <div class="step-dot" />
                        <div class="step-content">
                          <div class="step-message">{{ step.message }}</div>
                          <div class="step-time">{{ step.timestamp }}</div>
                        </div>
                      </div>
                    </div>
                  </div>
                </template>

                <!-- Display Processing Steps (completed, for assistant messages) -->
                <div
                  v-if="message.role === 'assistant' && message.processingSteps && message.processingSteps.length > 0 && !(loading && index === messages.length - 1)"
                  class="processing-steps"
                >
                  <el-collapse v-model="activeCollapse">
                    <el-collapse-item :name="'steps-' + index">
                      <template #title>
                        <div class="processing-header">
                          <i i="ep-view" class="mr-1" />
                          <span>View Processing Steps</span>
                          <el-tag size="small" type="info" class="ml-2">
                            {{ message.processingSteps.length }} steps
                          </el-tag>
                        </div>
                      </template>
                      <div class="steps-timeline">
                        <div
                          v-for="(step, stepIdx) in message.processingSteps"
                          :key="stepIdx"
                          class="step-item"
                          :class="step.type"
                        >
                          <div class="step-dot" />
                          <div class="step-content">
                            <div class="step-message">{{ step.message }}</div>
                            <div class="step-time">{{ step.timestamp }}</div>
                            <div v-if="step.details" class="step-details">
                              <pre>{{ JSON.stringify(step.details, null, 2) }}</pre>
                            </div>
                          </div>
                        </div>
                      </div>
                    </el-collapse-item>
                  </el-collapse>
                </div>

                <!-- Message text (hide when empty and loading) -->
                <div v-if="message.content" class="message-text" v-html="formatMessage(message.content)" />

                <!-- Loading dots (last assistant message, no content yet) -->
                <div v-if="loading && message.role === 'assistant' && index === messages.length - 1 && !message.content" class="loading-dots">
                  <span></span>
                  <span></span>
                  <span></span>
                </div>

                <!-- Approval Request Card -->
                <el-alert
                  v-if="message.role === 'assistant' && message.pendingApproval"
                  type="warning"
                  :closable="false"
                  show-icon
                  style="margin-top: 8px;"
                >
                  <template #title>
                    Tool requires approval
                  </template>
                  <div style="margin-bottom: 8px;">
                    <el-tag type="warning" size="small">{{ message.pendingApproval.toolName }}</el-tag>
                  </div>
                  <pre style="background: #f5f5f5; padding: 8px; border-radius: 4px; font-size: 12px; max-height: 200px; overflow: auto; margin: 0 0 12px 0;">{{ JSON.stringify(message.pendingApproval.toolArgs, null, 2) }}</pre>
                  <div style="display: flex; gap: 8px;">
                    <el-button type="primary" size="small" @click="handleApproval(message, 'approved')">Approve</el-button>
                    <el-button type="danger" size="small" @click="handleApproval(message, 'rejected')">Reject</el-button>
                  </div>
                </el-alert>

                <!-- Display Chart -->
                <div v-if="message.chart_data" class="chart-container">
                  <div
                    :ref="(el: any) => setChartRef(el, index)"
                    class="chart"
                    :style="{ width: '100%', height: '400px' }"
                  />
                </div>

                <!-- Display Insights -->
                <div v-if="message.insights && message.insights.length > 0" class="insights">
                  <div class="insights-header">
                    <i i="ep-info-filled" class="mr-1" />
                    <span>Key Insights</span>
                  </div>
                  <ul class="insights-list">
                    <li v-for="(insight, idx) in message.insights" :key="idx">
                      {{ insight }}
                    </li>
                  </ul>
                </div>

                <!-- Actions section (Feedback buttons) - Temporarily hidden, backend API not working -->
                <!-- Temporarily hidden, backend API not working -->
                <!--
                  v-if="message.role === 'assistant' && message.messageId && index > 0"
                  v-if="message.role === 'assistant' && message.messageId && index > 0"
                >
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
                </div>
                -->
                <!-- Feedback Form (shown when downvoting) - Temporarily hidden, backend API not working -->
                <!--
                <div
                  v-if="message.role === 'assistant' && message.showFeedbackForm && message.messageId"
                  class="feedback-form"
                >
                  <div class="feedback-form-header">
                    <span class="feedback-form-title">Please share more details about your feedback:</span>
                  </div>
                  <div class="feedback-reasons">
                    <div
                      v-for="reason in defaultFeedbackReasons"
                      :key="reason"
                      class="feedback-reason-tag"
                      :class="{ selected: message.selectedReasons?.includes(reason) }"
                      @click="toggleFeedbackReason(message, reason)"
                    >
                      {{ reason }}
                    </div>
                    <div
                      v-for="customReason in message.selectedReasons?.filter(r => !defaultFeedbackReasons.includes(r))"
                      :key="customReason"
                      class="feedback-reason-tag custom selected"
                    >
                      {{ customReason }}
                      <el-icon
                        class="remove-icon"
                        @click.stop="removeCustomFeedbackReason(message, customReason)"
                      >
                        <i i="ep-close" />
                      </el-icon>
                    </div>
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
                    <el-button size="small" @click="cancelFeedbackForm(message)">Cancel</el-button>
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
                -->
              </div>
            </div>

          </div>

          <!-- Input Box - Normal position (only show when messages exist) -->
          <div v-if="messages.length > 0" class="input-container">
            <div class="chat-input-wrapper">
              <el-input
                v-model="userInput"
                type="textarea"
                :rows="3"
                placeholder="Enter your question... (Press Ctrl+Enter to send)"
                @keydown.ctrl.enter="sendMessage"
                class="chat-input"
              />
              <button
                v-if="!loading"
                class="send-btn"
                @click="sendMessage"
                :disabled="!userInput.trim()"
              >
                <el-icon :size="20">
                  <Promotion />
                </el-icon>
              </button>
              <button
                v-else
                class="send-btn abort-btn"
                @click="abortChat"
              >
                <el-icon :size="20">
                  <CircleCloseFilled />
                </el-icon>
              </button>
            </div>
          </div>
        </el-card>
      </div>

      <!-- History Sidebar -->
      <el-drawer
        v-model="showHistoryPanel"
        title="Conversation History"
        direction="rtl"
        size="400px"
      >
        <div class="history-panel">
          <!-- Search Box -->
          <div class="search-box">
            <el-input
              v-model="searchQuery"
              placeholder="Search conversations..."
              clearable
              @input="handleSearchConversations"
              @clear="clearSearch"
            >
              <template #prefix>
                <i i="ep-search" />
              </template>
            </el-input>
          </div>

          <!-- Loading State -->
          <div v-if="loadingConversations || searching" class="loading-state">
            <el-skeleton :rows="5" animated />
          </div>

          <!-- Conversation List -->
          <div v-else-if="displayedConversations.length > 0" class="conversations-list">
            <div
              v-for="item in displayedConversations"
              :key="item.session_id"
              class="conversation-item"
              :class="{ active: currentSessionId === item.session_id }"
              @click="loadConversationHistory(item)"
            >
              <div class="conversation-header">
                <div class="conversation-title">
                  <i i="ep-chat-dot-round" class="mr-1" />
                  <span>{{ item.name && item.name.length > 30 ? item.name.slice(0, 30) + '...' : item.name || 'Untitled Conversation' }}</span>
                </div>
                <el-button
                  size="small"
                  text
                  type="danger"
                  @click.stop="handleDeleteConversation(item)"
                >
                  <i i="ep-delete" />
                </el-button>
              </div>
              <div class="conversation-meta">
                <el-tag v-if="item.metadata.cluster_name" size="small" type="info">
                  {{ item.metadata.cluster_name }}
                </el-tag>
                <span class="conversation-time">
                  {{ formatTime(item.created_at) }}
                </span>
              </div>
            </div>
          </div>

          <!-- Empty State -->
          <div v-else class="empty-state">
            <el-empty description="No conversation history">
              <template #image>
                <i i="ep-chat-line-round" style="font-size: 64px; color: var(--el-color-info-light-5);" />
              </template>
            </el-empty>
          </div>
        </div>
      </el-drawer>
    </div>
      </el-tab-pane>

      <!-- <el-tab-pane label="Skills Repository" name="skills-repository">
        <SkillsRepository />
      </el-tab-pane> -->
    </el-tabs>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick, computed, watch, type ComponentPublicInstance } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Promotion, Loading, CircleCloseFilled, WarningFilled } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'
import { useClusterSync } from '@/composables/useClusterSync'
import { useGlobalCluster } from '@/composables/useGlobalCluster'
import SkillsRepository from './SkillsRepository.vue'

// Tab state
const activeTab = ref('gpu-agent')
// Import vote icons
import likeLight from '@/assets/icons/like-light.png'
import dislikeLight from '@/assets/icons/dislike-light.png'
import dislikeActive from '@/assets/icons/dislike-active.png'
import {
  chatStream,
  getCapabilities,
  listConversations,
  getConversation,
  deleteConversation,
  searchConversations,
  submitFeedback,
  cancelVote,
  resolveApproval,
  abortRun,
  type ChatMessage,
  type AgentCapability,
  type ConversationListItem,
  type ConversationDetail,
  type SSEEvent
} from '@/services/agent'

// Get global cluster
const { selectedCluster } = useClusterSync()
const { clusterOptions } = useGlobalCluster()

// Chart data type
interface ChartData {
  title: string
  xAxis: string[]
  series: Array<{
    name: string
    data: number[]
    type: string
  }>
}

// Processing step type - display thinking process
interface ProcessingStep {
  type: 'routing' | 'crew_execution' | 'agent_thinking' | 'tool_execution' | 'progress'
  message: string
  timestamp: string
  agent?: string
  tool?: string
  details?: any
}

// Standardized step info from streaming protocol
interface StepInfo {
  id: string
  name: string
  description?: string
  index: number
  total: number
  progress?: number
  status?: 'pending' | 'running' | 'completed' | 'error'
}

// Configure marked
marked.setOptions({
  breaks: true,
  gfm: true
})

// Custom code highlight renderer
const renderer = new marked.Renderer()
renderer.code = function({ text, lang }: { text: string; lang?: string; escaped?: boolean }) {
  if (lang && hljs.getLanguage(lang)) {
    try {
      const highlighted = hljs.highlight(text, { language: lang }).value
      return `<pre><code class="hljs language-${lang}">${highlighted}</code></pre>`
    } catch (err) {
      console.error('Highlight error:', err)
    }
  }
  const autoHighlighted = hljs.highlightAuto(text).value
  return `<pre><code class="hljs">${autoHighlighted}</code></pre>`
}

marked.use({ renderer })

// State
const capabilities = ref<AgentCapability[]>([])
const loadingCapabilities = ref(false)
const userInput = ref('')
const messages = ref<Array<ChatMessage & { insights?: string[], chart_data?: ChartData, processingSteps?: ProcessingStep[], pendingApproval?: any }>>([])
const loading = ref(false)
const messagesContainer = ref<HTMLElement>()
const currentProcessingSteps = ref<ProcessingStep[]>([])  // Currently processing steps
const showProcessingDetails = ref(true)  // Whether to show processing details

// Streaming protocol state
const currentSteps = ref<Map<string, StepInfo>>(new Map())  // Track all steps
const currentContentBuffer = ref<string>('')  // Buffer for streaming content
const isStreamingContent = ref(false)  // Is content streaming

// Conversation history related
const currentSessionId = ref<string | undefined>(undefined)
const conversations = ref<ConversationListItem[]>([])
const loadingConversations = ref(false)
const showHistoryPanel = ref(false)
const searchQuery = ref('')
const searchResults = ref<ConversationListItem[]>([])
const searching = ref(false)
const activeCollapse = ref<string[]>([])  // Control processing section expand/collapse

// Upvote/downvote related
const loadingVote = ref(false)
const defaultFeedbackReasons = [
  'Inaccurate information',
  'Not helpful',
  'Missing details',
  'Too complex',
  'Outdated data'
]
// For tracking message IDs
let messageIdCounter = 1

// SSE stream control
const currentAbortController = ref<AbortController | null>(null)
const currentRunId = ref<string | null>(null)

// Load capability list
const loadCapabilities = async () => {
  loadingCapabilities.value = true
  try {
    const data = await getCapabilities()
    // Sort capabilities: Low Utilization Detection first
    const sortedCapabilities = data.capabilities.sort((a, b) => {
      if (a.type === 'low_utilization_detection') return -1
      if (b.type === 'low_utilization_detection') return 1
      return 0
    })
    capabilities.value = sortedCapabilities
  } catch (error) {
    console.error('Failed to load capabilities:', error)
    ElMessage.error('Failed to load agent capabilities')
  } finally {
    loadingCapabilities.value = false
  }
}

// Use example query
const useExample = (example: string) => {
  userInput.value = example
}

// Send message (using SSE streaming interface)
const sendMessage = async () => {
  const query = userInput.value.trim()
  if (!query || loading.value) return

  // Abort previous request (if any)
  if (currentAbortController.value) {
    currentAbortController.value.abort()
  }

  // Create new AbortController
  currentAbortController.value = new AbortController()

  // Add user message with ID
  const userMessage: ChatMessage = {
    role: 'user',
    content: query,
    timestamp: new Date().toLocaleTimeString(),
    messageId: messageIdCounter++
  }
  messages.value.push(userMessage)
  userInput.value = ''

  // Scroll to bottom
  await nextTick()
  scrollToBottom()

  // Call SSE streaming API
  loading.value = true

  // Pre-create an AI message object for streaming updates
  const assistantMessage: ChatMessage & { insights?: string[], chart_data?: ChartData, processingSteps?: ProcessingStep[], pendingApproval?: any } = {
    role: 'assistant',
    content: '',
    timestamp: new Date().toLocaleTimeString(),
    messageId: messageIdCounter++,
    insights: [],
    chart_data: undefined,
    processingSteps: [],
    pendingApproval: null
  }
  messages.value.push(assistantMessage)

  // Clear current processing steps and streaming state
  currentProcessingSteps.value = []
  currentSteps.value.clear()
  currentContentBuffer.value = ''
  isStreamingContent.value = false

  let currentContent = ''
  let hasError = false
  let needsClarification = false

  try {
    await chatStream(
      {
        query,
        conversationHistory: messages.value
          .filter(m => m !== assistantMessage) // Exclude the newly added empty message
          .map(m => ({
            role: m.role,
            content: m.content
          })),
        clusterName: selectedCluster.value || undefined,
        sessionId: currentSessionId.value,
        saveHistory: true
      },
      // onEvent - Handle SSE events
      (event: SSEEvent) => {
        switch (event.type) {
          case 'session':
            // Update session ID
            if (event.session_id && !currentSessionId.value) {
              currentSessionId.value = event.session_id
            }
            break

          case 'start':
            // Start processing
            currentProcessingSteps.value.push({
              type: 'progress',
              message: '🚀 Starting to process your query...',
              timestamp: new Date().toLocaleTimeString()
            })
            break

          case 'routing':
            // Routing analysis
            currentProcessingSteps.value.push({
              type: 'routing',
              message: '🧭 ' + (event.message || 'Analyzing query intent...'),
              timestamp: new Date().toLocaleTimeString()
            })
            assistantMessage.content = 'Understanding your request...'
            nextTick(() => scrollToBottom())
            break

          case 'routing_complete':
            // Routing complete
            const decision = event.routing_decision
            if (decision) {
              currentProcessingSteps.value.push({
                type: 'routing',
                message: `✅ Routing complete - Using ${decision.target_crew || 'default crew'} (confidence: ${(decision.confidence * 100).toFixed(1)}%)`,
                timestamp: new Date().toLocaleTimeString(),
                details: decision
              })
            }
            assistantMessage.content = 'Request identified, starting analysis...'
            nextTick(() => scrollToBottom())
            break

          case 'clarification':
            // Need clarification
            needsClarification = true
            currentProcessingSteps.value.push({
              type: 'progress',
              message: '❓ Need more information',
              timestamp: new Date().toLocaleTimeString()
            })

            // Set clarification message
            const clarificationMessage = event.message || 'Please provide more information to help you better.'
            assistantMessage.content = clarificationMessage
            currentContent = clarificationMessage

            // Save processing steps
            assistantMessage.processingSteps = [...currentProcessingSteps.value]

            nextTick(() => scrollToBottom())
            break

          case 'crew_execution':
            // Crew start execution
            currentProcessingSteps.value.push({
              type: 'crew_execution',
              message: `🔧 ${event.message || 'Executing analysis task...'}`,
              timestamp: new Date().toLocaleTimeString(),
              details: { crew: event.crew }
            })
            assistantMessage.content = `Executing ${event.crew || ''} analysis...`
            nextTick(() => scrollToBottom())
            break

          case 'agent_thinking':
            // Agent thinking process
            currentProcessingSteps.value.push({
              type: 'agent_thinking',
              message: `🤔 ${event.agent || 'Agent'}: ${event.message || 'Thinking...'}`,
              timestamp: new Date().toLocaleTimeString(),
              agent: event.agent,
              details: event.data
            })
            nextTick(() => scrollToBottom())
            break

          case 'tool_execution':
            // Tool execution
            currentProcessingSteps.value.push({
              type: 'tool_execution',
              message: `🛠️ Using tool: ${event.tool || 'Unknown tool'}`,
              timestamp: new Date().toLocaleTimeString(),
              tool: event.tool,
              details: {
                input: event.tool_input,
                output: event.tool_output
              }
            })
            nextTick(() => scrollToBottom())
            break

          case 'progress':
            // Show progress message
            if (event.message) {
              currentProcessingSteps.value.push({
                type: 'progress',
                message: `⚙️ ${event.message}`,
                timestamp: new Date().toLocaleTimeString()
              })
              currentContent = event.message
              assistantMessage.content = currentContent
              nextTick(() => scrollToBottom())
            }
            break

          case 'final':
            // Final answer - Save processing steps to message
            assistantMessage.processingSteps = [...currentProcessingSteps.value]

            // Extract final answer
            if (event.answer) {
              assistantMessage.content = event.answer
              currentContent = event.answer
            } else if (event.result?.answer) {
              // Support event.result.answer path
              assistantMessage.content = event.result.answer
              currentContent = event.result.answer
            } else if (event.result?.analysis?.answer) {
              assistantMessage.content = event.result.analysis.answer
              currentContent = event.result.analysis.answer
            } else if (event.message) {
              assistantMessage.content = event.message
              currentContent = event.message
            }

            // Extract chart data from multiple possible paths
            const resultData = event.result || event.data || {}
            const chart_data = resultData.analysis?.chart_data ||
                             resultData.cluster_trend?.chart_data ||
                             resultData.chart_data ||
                             resultData.chart_data ||  // Support camelCase
                             resultData.gpu_comparison?.chart_data

            if (chart_data) {
              assistantMessage.chart_data = chart_data
            }

            // Update session ID (if any)
            if (event.session_id && !currentSessionId.value) {
              currentSessionId.value = event.session_id
            }
            if (event.debug_info?.session_id && !currentSessionId.value) {
              currentSessionId.value = event.debug_info.session_id
            }

            // Add completion step
            currentProcessingSteps.value.push({
              type: 'progress',
              message: '✨ Analysis complete',
              timestamp: new Date().toLocaleTimeString()
            })

            nextTick(() => scrollToBottom())
            break

          case 'complete':
            // Completely finished - ensure processing steps are preserved
            if (!assistantMessage.processingSteps?.length && currentProcessingSteps.value.length > 0) {
              assistantMessage.processingSteps = [...currentProcessingSteps.value]
            }
            break

          case 'error': {
            // Error handling
            hasError = true
            const errMsg = event.message || event.error?.message || event.error_message || 'Unknown error'
            if (event.error?.code === 'aborted' || errMsg === 'Execution aborted by user') {
              assistantMessage.content = '*Execution aborted by user.*'
              assistantMessage.processingSteps = [...currentProcessingSteps.value]
              currentProcessingSteps.value.push({
                type: 'progress',
                message: '⏹️ Aborted by user',
                timestamp: new Date().toLocaleTimeString()
              })
            } else {
              assistantMessage.content = `Sorry, encountered an issue while processing your request: ${errMsg}`
              assistantMessage.processingSteps = [...currentProcessingSteps.value]
              currentProcessingSteps.value.push({
                type: 'progress',
                message: `❌ Error: ${errMsg}`,
                timestamp: new Date().toLocaleTimeString()
              })
            }
            break
          }

          case 'done':
            break

          // ===== New Standardized Streaming Protocol Events =====

          case 'crew_start':
            // Crew starts execution
            currentProcessingSteps.value.push({
              type: 'crew_execution',
              message: `🚀 ${event.data?.crew || 'Crew'} started`,
              timestamp: new Date().toLocaleTimeString()
            })
            nextTick(() => scrollToBottom())
            break

          case 'crew_complete':
            // Crew execution completed
            currentProcessingSteps.value.push({
              type: 'crew_execution',
              message: `✅ ${event.data?.crew || 'Crew'} completed`,
              timestamp: new Date().toLocaleTimeString()
            })
            nextTick(() => scrollToBottom())
            break

          case 'step_start':
            // Step started
            if (event.step) {
              const stepInfo: StepInfo = {
                ...event.step,
                status: 'running'
              }
              currentSteps.value.set(event.step.id, stepInfo)

              currentProcessingSteps.value.push({
                type: 'progress',
                message: `📝 [Step ${event.step.index}/${event.step.total}] ${event.step.name}`,
                timestamp: new Date().toLocaleTimeString()
              })

              if (event.step.description) {
                assistantMessage.content = event.step.description
              }
            }
            nextTick(() => scrollToBottom())
            break

          case 'step_progress':
            // Step progress update
            if (event.step) {
              const stepInfo = currentSteps.value.get(event.step.id)
              if (stepInfo) {
                stepInfo.progress = event.step.progress || 0
                stepInfo.status = 'running'
                currentSteps.value.set(event.step.id, stepInfo)
              }

              const progressMsg = event.metadata?.message || `Progress: ${event.step.progress}%`
              currentProcessingSteps.value.push({
                type: 'progress',
                message: `⚙️ ${event.step.name}: ${progressMsg}`,
                timestamp: new Date().toLocaleTimeString()
              })
            }
            nextTick(() => scrollToBottom())
            break

          case 'step_complete':
            // Step completed
            if (event.step) {
              const stepInfo = currentSteps.value.get(event.step.id)
              if (stepInfo) {
                stepInfo.progress = 100
                stepInfo.status = 'completed'
                currentSteps.value.set(event.step.id, stepInfo)
              }

              currentProcessingSteps.value.push({
                type: 'progress',
                message: `✅ ${event.step.name} completed`,
                timestamp: new Date().toLocaleTimeString()
              })
            }
            nextTick(() => scrollToBottom())
            break

          case 'step_error':
            // Step error
            if (event.step) {
              const stepInfo = currentSteps.value.get(event.step.id)
              if (stepInfo) {
                stepInfo.status = 'error'
                currentSteps.value.set(event.step.id, stepInfo)
              }

              currentProcessingSteps.value.push({
                type: 'progress',
                message: `❌ ${event.step.name} failed: ${event.error?.message || 'Unknown error'}`,
                timestamp: new Date().toLocaleTimeString()
              })
            }
            nextTick(() => scrollToBottom())
            break

          case 'content_start':
            // Content stream started
            isStreamingContent.value = true
            currentContentBuffer.value = ''
            currentProcessingSteps.value.push({
              type: 'progress',
              message: '📝 Generating response...',
              timestamp: new Date().toLocaleTimeString()
            })
            nextTick(() => scrollToBottom())
            break

          case 'content_delta':
            // Content increment (token by token)
            if (event.content?.delta) {
              if (event.content.accumulated) {
                // Accumulation mode: append to buffer
                currentContentBuffer.value += event.content.delta
                assistantMessage.content = currentContentBuffer.value
              } else {
                // Direct mode: display immediately
                assistantMessage.content = event.content.delta
              }
              currentContent = assistantMessage.content
              nextTick(() => scrollToBottom())
            }
            break

          case 'content_complete':
            // Content stream ended
            isStreamingContent.value = false
            if (event.content?.complete) {
              assistantMessage.content = event.content.complete
              currentContent = event.content.complete
            } else if (currentContentBuffer.value) {
              assistantMessage.content = currentContentBuffer.value
              currentContent = currentContentBuffer.value
            }

            currentProcessingSteps.value.push({
              type: 'progress',
              message: '✨ Response generated',
              timestamp: new Date().toLocaleTimeString()
            })
            nextTick(() => scrollToBottom())
            break

          case 'data':
            // Structured data (parameters, charts, etc.)
            if (event.data) {
              const dataType = event.metadata?.data_type

              if (dataType === 'chart_data') {
                // Chart data (standardized format)
                assistantMessage.chart_data = event.data
                currentProcessingSteps.value.push({
                  type: 'progress',
                  message: '📊 Chart data loaded',
                  timestamp: new Date().toLocaleTimeString()
                })
                nextTick(() => scrollToBottom())
              } else if (dataType === 'chart') {
                // Chart data (backward compatible format)
                assistantMessage.chart_data = event.data
                currentProcessingSteps.value.push({
                  type: 'progress',
                  message: '📊 Chart data loaded (legacy format)',
                  timestamp: new Date().toLocaleTimeString()
                })
                nextTick(() => scrollToBottom())
              } else if (dataType === 'user_table') {
                // User table data (Markdown format)
                if (event.data.table) {
                  // Append table to content area
                  const tableContent = '\n\n' + event.data.table
                  if (assistantMessage.content) {
                    assistantMessage.content += tableContent
                  } else {
                    assistantMessage.content = tableContent
                  }
                  currentContent += tableContent

                  currentProcessingSteps.value.push({
                    type: 'progress',
                    message: '📋 User table loaded',
                    timestamp: new Date().toLocaleTimeString()
                  })
                  nextTick(() => scrollToBottom())
                }
              } else if (dataType === 'user_detection_result') {
                // JSON format user data
                const userCount = event.data.users?.length || event.data.total_users || 0
                currentProcessingSteps.value.push({
                  type: 'progress',
                  message: `👥 Detected ${userCount} low-utilization users`,
                  timestamp: new Date().toLocaleTimeString(),
                  details: event.data
                })
              } else if (dataType === 'parameters') {
                // Parameter data
                currentProcessingSteps.value.push({
                  type: 'progress',
                  message: `📊 Parameters extracted: ${JSON.stringify(event.data)}`,
                  timestamp: new Date().toLocaleTimeString(),
                  details: event.data
                })
              } else if (dataType === 'query_result') {
                // Query results
                currentProcessingSteps.value.push({
                  type: 'progress',
                  message: `📈 Data queried: ${event.data.data_points || 'N/A'} data points`,
                  timestamp: new Date().toLocaleTimeString(),
                  details: event.data
                })
              } else {
                currentProcessingSteps.value.push({
                  type: 'progress',
                  message: `📦 Data received: ${dataType || 'unknown type'}`,
                  timestamp: new Date().toLocaleTimeString(),
                  details: event.data
                })
              }
            }
            nextTick(() => scrollToBottom())
            break

          case 'approval_request':
            if (event.data) {
              assistantMessage.pendingApproval = {
                requestId: event.data.request_id,
                toolName: event.data.tool_name,
                toolArgs: event.data.tool_args,
                timeout: event.data.timeout,
                receivedAt: Date.now(),
              }
              currentProcessingSteps.value.push({
                type: 'progress',
                message: `⏳ Waiting for approval: ${event.data.tool_name}`,
                timestamp: new Date().toLocaleTimeString()
              })
            }
            nextTick(() => scrollToBottom())
            break

          case 'approval_resolved':
            if (assistantMessage.pendingApproval) {
              assistantMessage.pendingApproval = null
            }
            if (event.data) {
              currentProcessingSteps.value.push({
                type: 'progress',
                message: `✅ Approval resolved: ${event.data.decision}`,
                timestamp: new Date().toLocaleTimeString()
              })
            }
            nextTick(() => scrollToBottom())
            break

          case 'metadata':
            break
        }
      },
      // onError
      (error: Error) => {
        console.error('SSE Error:', error)
        hasError = true

        // If no content yet, set error message
        if (!assistantMessage.content) {
          assistantMessage.content = 'Sorry, encountered an issue connecting to the server. Please check your network connection and try again.'
        }

        // Save processing steps
        assistantMessage.processingSteps = [...currentProcessingSteps.value]

        // Add error step
        currentProcessingSteps.value.push({
          type: 'progress',
          message: `❌ Connection error: ${error.message}`,
          timestamp: new Date().toLocaleTimeString()
        })

        ElMessage.error(error.message || 'Connection failed')
      },
      // onComplete
      () => {
        loading.value = false
        currentRunId.value = null

        // If no content received and no error and not clarification, display default message
        if (!currentContent && !hasError && !needsClarification) {
          assistantMessage.content = 'Sorry, no valid response received.'
        }

        // Always preserve processing steps on completion
        if (!assistantMessage.processingSteps?.length && currentProcessingSteps.value.length > 0) {
          assistantMessage.processingSteps = [...currentProcessingSteps.value]
        }

        // Refresh history list
        if (showHistoryPanel.value) {
          loadConversations()
        }

        nextTick(() => scrollToBottom())
      },
      currentAbortController.value.signal, // Pass AbortSignal
      // onResponse - extract X-Run-Id
      (response: Response) => {
        currentRunId.value = response.headers.get('X-Run-Id')
      }
    )
  } catch (error: any) {
    // If abort error, don't show error message
    if (error?.name === 'AbortError') {
      loading.value = false
      return
    }

    console.error('Chat error:', error)
    loading.value = false

    if (!assistantMessage.content) {
      assistantMessage.content = 'Sorry, I encountered some issues. Please try again later.'
    }

    // Save processing steps
    assistantMessage.processingSteps = [...currentProcessingSteps.value]

    if (!hasError) {
      ElMessage.error(error?.message || 'Conversation failed, please try again')
    }
  } finally {
    // Clear current AbortController
    currentAbortController.value = null
  }
}

// Handle approval decision
const handleApproval = async (message: any, decision: 'approved' | 'rejected') => {
  if (!message.pendingApproval) return
  const { requestId } = message.pendingApproval

  try {
    await resolveApproval(requestId, decision)
  } catch (e) {
    console.error('Approval resolve failed:', e)
    ElMessage.warning('Approval may have timed out')
  }

  message.pendingApproval = null
}

// Abort current chat run
const abortChat = async () => {
  if (currentRunId.value) {
    try {
      await abortRun(currentRunId.value)
    } catch (e) {
      console.error('Abort failed:', e)
    }
  }

  // Abort the fetch connection
  if (currentAbortController.value) {
    currentAbortController.value.abort()
    currentAbortController.value = null
  }

  // Reset loading state
  loading.value = false
  currentRunId.value = null
}

// Clear chat
const clearChat = () => {
  messages.value = []
  currentSessionId.value = undefined
  ElMessage.success('Chat cleared')
}

// Format message (supports Markdown)
const formatMessage = (content: string) => {
  try {
    return marked.parse(content) as string
  } catch (error) {
    console.error('Markdown parse error:', error)
    return content.replace(/\n/g, '<br/>')
  }
}

// Scroll to bottom
const scrollToBottom = () => {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}

// ===== Conversation History Management =====

// Load conversation list
const loadConversations = async () => {
  loadingConversations.value = true
  try {
    const data = await listConversations({ limit: 50, offset: 0 })
    conversations.value = data.conversations
  } catch (error) {
    console.error('Failed to load conversations:', error)
    ElMessage.error('Failed to load conversation history')
  } finally {
    loadingConversations.value = false
  }
}

// Toggle history panel
const toggleHistoryPanel = () => {
  showHistoryPanel.value = !showHistoryPanel.value
  if (showHistoryPanel.value && conversations.value.length === 0) {
    loadConversations()
  }
}

// Load specific conversation
const loadConversationHistory = async (item: ConversationListItem) => {
  try {
    const response = await getConversation(item.session_id) as any

    // Clear current conversation
    messages.value = []

    // Handle new API response structure
    if (response.conversation) {
      const conv = response.conversation

      // Add user message
      if (conv.query) {
        messages.value.push({
          role: 'user',
          content: conv.query,
          timestamp: response.created_at ? new Date(response.created_at).toLocaleTimeString() : new Date().toLocaleTimeString()
        })
      }

      // Add AI reply message
      if (conv.result) {
        const assistantMessage: ChatMessage & { insights?: string[], chart_data?: ChartData, processingSteps?: ProcessingStep[] } = {
          role: 'assistant',
          content: conv.result,  // conv.result is already a string (Markdown format)
          timestamp: response.created_at ? new Date(response.created_at).toLocaleTimeString() : new Date().toLocaleTimeString()
        }

        // Extract chart_data from messages metadata
        if (conv.messages && conv.messages.length > 0) {
          const firstMessage = conv.messages[0]
          if (firstMessage.metadata?.crew_result?.chart_data) {
            assistantMessage.chart_data = firstMessage.metadata.crew_result.chart_data
          }

          // Extract insights if available
          if (firstMessage.metadata?.crew_result?.insights && Array.isArray(firstMessage.metadata.crew_result.insights)) {
            assistantMessage.insights = firstMessage.metadata.crew_result.insights
          }
        }

        messages.value.push(assistantMessage)
      }
    }
    // Compatible with old messages array format
    else if (response.messages && response.messages.length > 0) {
      response.messages.forEach((msg: any) => {
        const messageItem: ChatMessage & { insights?: string[], chart_data?: ChartData } = {
          role: msg.role,
          content: msg.content,
          timestamp: msg.timestamp || new Date().toLocaleTimeString()
        }

        // If it's an assistant message, process additional data
        if (msg.role === 'assistant') {
          // Extract insights
          if (msg.insights && msg.insights.length > 0) {
            messageItem.insights = msg.insights
          }

          // Extract chart data
          if (msg.data) {
            // Try to extract chart_data from different paths
            const chart_data = msg.data.cluster_trend?.chart_data ||
                             msg.data.chart_data ||
                             msg.data.gpu_comparison?.chart_data

            if (chart_data) {
              messageItem.chart_data = chart_data
            }
          }
        }

        messages.value.push(messageItem)
      })
    }

    // Set current session ID
    currentSessionId.value = item.session_id

    // Note: We don't set cluster name from history anymore
    // as it's managed globally in the header

    ElMessage.success('Conversation history loaded')

    await nextTick()
    scrollToBottom()

    // Delay rendering charts, ensure DOM is updated
    await nextTick()
    messages.value.forEach((msg, index) => {
      if (msg.chart_data) {
        // Trigger chart re-rendering
        nextTick(() => {
          const chartContainer = document.querySelectorAll('.chart')[index]
          if (chartContainer) {
            initChart(chartContainer as HTMLElement, msg.chart_data!, index)
          }
        })
      }
    })
  } catch (error: any) {
    console.error('Failed to load conversation:', error)
    ElMessage.error(error?.message || 'Failed to load conversation')
  }
}

// Delete conversation
const handleDeleteConversation = async (item: ConversationListItem) => {
  try {
    await ElMessageBox.confirm(
      'Are you sure you want to delete this conversation? This action cannot be undone.',
      'Confirm Deletion',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning',
      }
    )

    await deleteConversation(item.session_id)
    ElMessage.success('Conversation deleted')

    // If the deleted conversation is the current one, clear current conversation
    if (currentSessionId.value === item.session_id) {
      clearChat()
    }

    // Refresh list
    loadConversations()
  } catch (error: any) {
    if (error !== 'cancel') {
      console.error('Failed to delete conversation:', error)
      ElMessage.error(error?.message || 'Failed to delete conversation')
    }
  }
}

// Search conversations
const handleSearchConversations = async () => {
  if (!searchQuery.value.trim()) {
    searchResults.value = []
    return
  }

  searching.value = true
  try {
    const data = await searchConversations({
      query: searchQuery.value.trim(),
      limit: 20
    })
    searchResults.value = data.results
  } catch (error) {
    console.error('Failed to search conversations:', error)
    ElMessage.error('Search failed')
  } finally {
    searching.value = false
  }
}

// Clear search
const clearSearch = () => {
  searchQuery.value = ''
  searchResults.value = []
}

// Format time
const formatTime = (timestamp: string) => {
  const date = new Date(timestamp)
  const now = new Date()
  const diff = now.getTime() - date.getTime()

  // Less than 1 minute
  if (diff < 60 * 1000) {
    return 'Just now'
  }

  // Less than 1 hour
  if (diff < 60 * 60 * 1000) {
    return `${Math.floor(diff / (60 * 1000))} minutes ago`
  }

  // Less than 1 day
  if (diff < 24 * 60 * 60 * 1000) {
    return `${Math.floor(diff / (60 * 60 * 1000))} hours ago`
  }

  // Less than 7 days
  if (diff < 7 * 24 * 60 * 60 * 1000) {
    return `${Math.floor(diff / (24 * 60 * 60 * 1000))} days ago`
  }

  // Show specific date
  return date.toLocaleDateString()
}

// Displayed conversation list (search results or all)
const displayedConversations = computed(() => {
  return searchResults.value.length > 0 ? searchResults.value : conversations.value
})

// ===== Chart Management =====

// Store chart instances
const chartInstances = ref<Map<number, echarts.ECharts>>(new Map())

// Set chart ref
const setChartRef = (el: HTMLElement | null, index: number) => {
  if (!el) return

  const message = messages.value[index]
  if (!message?.chart_data) return

  // Delay chart initialization, ensure DOM is rendered
  nextTick(() => {
    initChart(el as HTMLElement, message.chart_data!, index)
  })
}

// Initialize chart
const initChart = (container: HTMLElement, chart_data: ChartData, index: number) => {
  // If already initialized, dispose first
  if (chartInstances.value.has(index)) {
    chartInstances.value.get(index)?.dispose()
  }

  // Create new chart instance
  const chart = echarts.init(container)

  // Format time axis
  const formatXAxis = (dateStr: string) => {
    const date = new Date(dateStr)
    return `${date.getMonth() + 1}/${date.getDate()} ${date.getHours()}:00`
  }

  // Normalize chart data structure (handle backend format)
  let normalizedData: any = { ...chart_data }

  // If series field doesn't exist, build it from raw data fields
  if (!normalizedData.series) {
    normalizedData.series = []

    // Check for utilization_series and allocation_series (cluster analysis format)
    if (normalizedData.utilization_series) {
      normalizedData.series.push({
        name: 'GPU Utilization (%)',
        type: 'line',
        data: normalizedData.utilization_series
      })
    }

    if (normalizedData.allocation_series) {
      normalizedData.series.push({
        name: 'GPU Allocation Rate (%)',
        type: 'line',
        data: normalizedData.allocation_series
      })
    }

    // If still no series found, try to auto-detect data fields
    if (normalizedData.series.length === 0) {
      const dataKeys = Object.keys(normalizedData).filter(key =>
        Array.isArray(normalizedData[key]) &&
        key !== 'xAxis' &&
        normalizedData[key].length > 0 &&
        typeof normalizedData[key][0] === 'number'
      )

      dataKeys.forEach(key => {
        normalizedData.series.push({
          name: key.replace(/_/g, ' ').replace(/\b\w/g, (l: string) => l.toUpperCase()),
          type: 'line',
          data: normalizedData[key]
        })
      })
    }
  }

  // Generate default title if not provided
  if (!normalizedData.title) {
    if (normalizedData.cluster) {
      normalizedData.title = `${normalizedData.cluster} - GPU Usage Trend`
    } else {
      normalizedData.title = 'GPU Usage Analysis'
    }
  }

  // Ensure xAxis exists
  if (!normalizedData.xAxis || normalizedData.xAxis.length === 0) {
    console.error('Chart data missing xAxis')
    return
  }

  // Configure chart
  const option: echarts.EChartsOption = {
    title: {
      text: normalizedData.title,
      left: 'center',
      textStyle: {
        fontSize: 16,
        fontWeight: 600
      }
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross'
      },
      formatter: (params: any) => {
        if (!Array.isArray(params)) return ''

        let result = `<div style="font-weight: 600;">${formatXAxis(normalizedData.xAxis[params[0].dataIndex])}</div>`
        params.forEach((param: any) => {
          result += `<div style="margin-top: 4px;">
            ${param.marker} ${param.seriesName}: <strong>${param.value.toFixed(2)}</strong>
          </div>`
        })
        return result
      }
    },
    legend: {
      data: normalizedData.series.map((s: any) => s.name),
      top: 35,
      left: 'center',
      textStyle: { color: document.documentElement.classList.contains('dark') ? '#E5EAF3' : '#303133' }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '3%',
      top: 80,
      containLabel: true
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: normalizedData.xAxis.map(formatXAxis),
      axisLabel: {
        rotate: 45,
        fontSize: 11,
        color: document.documentElement.classList.contains('dark') ? '#E5EAF3' : '#303133'
      },
      axisLine: { lineStyle: { color: document.documentElement.classList.contains('dark') ? '#FFFFFF1A' : '#00000012' } }
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        formatter: '{value}%',
        color: document.documentElement.classList.contains('dark') ? '#E5EAF3' : '#303133'
      },
      axisLine: { lineStyle: { color: document.documentElement.classList.contains('dark') ? '#FFFFFF1A' : '#00000012' } },
      splitLine: { lineStyle: { color: document.documentElement.classList.contains('dark') ? '#FFFFFF1A' : '#00000012' } }
    },
    series: normalizedData.series.map((s: any) => ({
      name: s.name,
      type: s.type || 'line',
      smooth: true,
      data: s.data,
      emphasis: {
        focus: 'series'
      }
    })) as any
  }

  chart.setOption(option)
  chartInstances.value.set(index, chart)

  // Responsive adjustment
  window.addEventListener('resize', () => {
    chart.resize()
  })
}

// ===== Upvote/downvote feature =====

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
      message.feedbackId = undefined
      message.showFeedbackForm = false
      message.selectedReasons = []
      message.customReason = ''
      ElMessage.success('Vote cancelled')
    } catch (error: any) {
      console.error('Vote error:', error)
      ElMessage.error('Operation failed: ' + (error?.message || 'Unknown error'))
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
  } catch (error: any) {
    console.error('Vote error:', error)
    ElMessage.error('Operation failed: ' + (error?.message || 'Unknown error'))
  } finally {
    loadingVote.value = false
  }
}

// Toggle feedback reason selection
const toggleFeedbackReason = (message: ChatMessage, reason: string) => {
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
const addCustomFeedbackReason = (message: ChatMessage) => {
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
const removeCustomFeedbackReason = (message: ChatMessage, reason: string) => {
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
    const reason = message.selectedReasons && message.selectedReasons.length > 0
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
  } catch (error: any) {
    console.error('Vote error:', error)
    ElMessage.error('Operation failed: ' + (error?.message || 'Unknown error'))
  } finally {
    loadingVote.value = false
  }
}

// Cancel feedback form
const cancelFeedbackForm = (message: ChatMessage) => {
  message.showFeedbackForm = false
  message.selectedReasons = []
  message.customReason = ''
}

// Initialize
onMounted(() => {
  loadCapabilities()
})

// Abort SSE stream when component unmounts
onBeforeUnmount(() => {
  if (currentAbortController.value) {
    currentAbortController.value.abort()
    currentAbortController.value = null
  }
})
</script>

<style scoped lang="scss">
.agent-page {
  // Fully take over the entire main-content area
  position: fixed;
  top: 72px; // Header height (desktop)
  left: 0;
  right: 0;
  bottom: 0;
  padding: 20px 5%; // Reduced padding
  overflow: hidden;
  z-index: 1;
  display: flex;
  flex-direction: column;

  // Title section
  .large-title {
    font-size: 20px;
    font-weight: 600;
    color: var(--el-text-color-primary);
    margin-top: 0;
    margin-bottom: 4px;
    flex-shrink: 0;
    line-height: 1.2;
  }

  .text-gray-500 {
    margin-bottom: 12px;
    flex-shrink: 0;
    line-height: 1.2;
  }

  // Tabs styling
  .agent-tabs {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    min-height: 0;

    :deep(.el-tabs__header) {
      margin-bottom: 16px;
      flex-shrink: 0;
    }

    :deep(.el-tabs__content) {
      flex: 1;
      overflow: hidden;
      height: 100%;

      .el-tab-pane {
        height: 100%;
        overflow: hidden;
      }
    }

    :deep(.el-tabs__item) {
      font-size: 15px;
      padding: 0 20px;

      &.is-active {
        font-weight: 600;
      }
    }
  }

  // Responsive adjustments
  @media (max-width: 768px) {
    top: 60px; // Header height (mobile)
    padding: 8px 12px; // Further reduced padding

    .large-title {
      font-size: 18px; // Match other pages' mobile title size
      margin-bottom: 2px;
    }

    .text-gray-500 {
      font-size: 12px; // Slightly larger for readability
      margin-bottom: 6px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }

  .agent-container {
    display: flex;
    gap: 20px;
    flex: 1;
    height: 100%;
    min-height: 0; // Important for flex children to shrink properly
    overflow: hidden;
  }

  .capabilities-panel {
    width: 350px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;

    .capability-card {
      border-radius: 15px;
      flex: 1;
      display: flex;
      flex-direction: column;
      min-height: 0;
      overflow: hidden;

      :deep(.el-card__body) {
        flex: 1;
        overflow-y: auto;
        min-height: 0;
      }

      .capabilities-content {
        padding: 0;
      }

      .capability-item {
        margin-bottom: 20px;
        padding-bottom: 20px;
        border-bottom: 1px solid var(--el-border-color-light);

        &:last-child {
          border-bottom: none;
          margin-bottom: 0;
          padding-bottom: 0;
        }

        .capability-header {
          margin-bottom: 8px;

          .capability-name {
            font-weight: 600;
            color: var(--el-color-primary);
            font-size: 16px;
          }
        }

        .capability-desc {
          color: var(--el-text-color-secondary);
          font-size: 13px;
          margin-bottom: 12px;
          line-height: 1.5;
        }

        .capability-examples {
          .examples-title {
            font-size: 12px;
            color: var(--el-text-color-regular);
            margin-bottom: 8px;
            font-weight: 500;
          }

          .example-item {
            display: flex;
            align-items: flex-start;
            padding: 8px;
            margin: 4px 0;
            border-radius: 8px;
            font-size: 13px;
            color: var(--el-text-color-regular);
            cursor: pointer;
            transition: all 0.2s;
            background-color: var(--el-fill-color-lighter);

            &:hover {
              background-color: var(--el-color-primary-light-9);
              color: var(--el-color-primary);
            }

            .example-icon {
              margin-right: 6px;
              margin-top: 2px;
              flex-shrink: 0;
            }
          }
        }
      }
    }
  }

  .chat-panel {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;

    .chat-card {
      border-radius: 20px;
      flex: 1;
      display: flex;
      flex-direction: column;
      min-height: 0; // Allow shrinking
      background: linear-gradient(135deg, var(--el-bg-color) 0%, rgba(255, 255, 255, 0.02) 100%);
      box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08);
      border: 1px solid rgba(255, 255, 255, 0.08);

      :deep(.el-card__body) {
        flex: 1;
        display: flex;
        flex-direction: column;
        overflow: hidden;
        padding: 0;
      }

      .card-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        font-weight: 600;
      }

      .messages-container {
        flex: 1;
        overflow-y: scroll;
        overflow-x: hidden; // Prevent horizontal scrolling
        padding: 24px;
        background: linear-gradient(180deg, transparent 0%, rgba(var(--el-color-primary-rgb), 0.02) 100%);

        // Smart scroll handling in initial state
        &:has(.empty-state-wrapper) {
          overflow-y: auto;
          // Use safe center strategy: center when content is small, start from top when content overflows
          display: flex;
          flex-direction: column;
        }

        .empty-state-wrapper {
          min-height: 100%;
          display: flex;
          flex-direction: column;
          justify-content: safe center; // Smart centering: auto switch to flex-start on overflow
          align-items: center;
          padding: 20px 20px 0;

          // Provide fallback for browsers that do not support the safe keyword
          @supports not (justify-content: safe center) {
            justify-content: center;
            // Achieve similar effect via margin auto
            margin: auto 0;
            &:first-child {
              margin-top: 20px;
            }
          }
        }

        .empty-state {
          text-align: center;
          width: 100%;
          max-width: 600px;
          display: flex;
          flex-direction: column;
          gap: 40px;

          .example-queries-section {
            .queries-hint {
              font-size: 16px;
              color: var(--el-text-color-regular);
              margin-bottom: 24px;
              font-weight: 500;
            }

            .example-queries {
              display: flex;
              flex-direction: column;
              gap: 12px;

              .query-item {
                display: flex;
                align-items: center;
                gap: 12px;
                padding: 14px 18px;
                background: var(--el-fill-color-lighter);
                border-radius: 10px;
                cursor: pointer;
                transition: all 0.2s ease;
                text-align: left;
                border: 1px solid var(--el-border-color-lighter);

                &:hover {
                  background: var(--el-color-primary-light-9);
                  border-color: var(--el-color-primary-light-5);

                  .query-icon {
                    color: var(--el-color-primary);
                  }
                }

                .query-icon {
                  font-size: 18px;
                  color: var(--el-text-color-secondary);
                  transition: color 0.2s ease;
                }

                span {
                  flex: 1;
                  font-size: 14px;
                  color: var(--el-text-color-primary);
                }
              }
            }
          }

          // Integrated input container
          .integrated-input-container {
            width: 100%;

            .chat-input-wrapper {
              position: relative;
              width: 100%;

              .chat-input {
                :deep(.el-textarea__inner) {
                  font-size: 16px;
                  padding: 16px 60px 16px 24px; // Right padding for button
                  border-radius: 24px;
                  resize: none;
                  border: 1px solid var(--el-border-color);
                  background: var(--el-fill-color-blank);
                  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
                  transition: all 0.3s ease;

                  &:hover {
                    border-color: var(--el-color-primary-light-5);
                    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.1);
                  }

                  &:focus {
                    border-color: var(--el-color-primary);
                    box-shadow: 0 6px 24px rgba(64, 158, 255, 0.2);
                  }
                }
              }

              .send-btn {
                position: absolute;
                right: 12px;
                bottom: 12px;
                width: 36px;
                height: 36px;
                border-radius: 8px;
                background: var(--el-color-primary);
                color: white;
                border: none;
                cursor: pointer;
                display: flex;
                align-items: center;
                justify-content: center;
                transition: all 0.2s ease;

                &:hover:not(:disabled) {
                  background: var(--el-color-primary-dark-2);
                  transform: scale(1.05);
                }

                &:active:not(:disabled) {
                  transform: scale(0.95);
                }

                &:disabled {
                  background: var(--el-disabled-bg-color);
                  color: var(--el-disabled-text-color);
                  cursor: not-allowed;
                  opacity: 0.6;
                }

                .is-loading {
                  animation: rotate 1s linear infinite;
                }

                @keyframes rotate {
                  from { transform: rotate(0deg); }
                  to { transform: rotate(360deg); }
                }
              }
            }
          }
        }

        .message {
          display: flex;
          margin-bottom: 24px;
          animation: slideIn 0.3s ease-out;

          .message-avatar {
            width: 40px;
            height: 40px;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            flex-shrink: 0;
            margin-right: 16px;
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);

            .avatar-icon {
              font-size: 22px;
              color: var(--el-color-white);
            }
          }

          // User message styling
          &.user {
            .message-avatar {
              background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            }

            .message-text {
              background: linear-gradient(135deg, #f5f7fa 0%, #e9ecef 100%);
              color: var(--el-text-color-primary);
              margin-left: 56px;

              html.dark & {
                background: linear-gradient(135deg, rgba(255, 255, 255, 0.08) 0%, rgba(255, 255, 255, 0.04) 100%);
              }
            }
          }

          // Assistant message styling
          &.assistant {
            .message-avatar {
              background: linear-gradient(135deg, #667eea 0%, #409eff 100%);
            }

            .message-text {
              background: var(--el-bg-color);
              border: 1px solid var(--el-border-color-lighter);

              &:hover {
                box-shadow: 0 4px 20px rgba(0, 0, 0, 0.08);
                transform: translateY(-1px);
              }
            }
          }

          .message-content {
            flex: 1;
            min-width: 0;

            .message-header {
              display: flex;
              align-items: center;
              margin-bottom: 8px;
              padding: 0 2px;

              .message-role {
                font-weight: 600;
                font-size: 15px;
                margin-right: 12px;
                color: var(--el-text-color-primary);
              }

              .message-time {
                font-size: 12px;
                color: var(--el-text-color-placeholder);
                opacity: 0.7;
              }
            }

            .message-text {
              padding: 16px 20px;
              border-radius: 16px;
              line-height: 1.7;
              word-break: break-word;
              overflow-wrap: break-word; // Ensure long words wrap
              max-width: 100%; // Limit max width
              box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
              position: relative;
              transition: all 0.2s ease;

              // Markdown styles
              :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6) {
                margin: 16px 0 8px;
                font-weight: 600;
                line-height: 1.4;

                &:first-child {
                  margin-top: 0;
                }
              }

              :deep(h1) { font-size: 1.8em; }
              :deep(h2) { font-size: 1.5em; }
              :deep(h3) { font-size: 1.3em; }
              :deep(h4) { font-size: 1.1em; }

              :deep(p) {
                margin: 8px 0;

                &:first-child {
                  margin-top: 0;
                }

                &:last-child {
                  margin-bottom: 0;
                }
              }

              :deep(ul), :deep(ol) {
                margin: 8px 0;
                padding-left: 24px;

                li {
                  margin: 4px 0;
                }
              }

              :deep(code) {
                padding: 2px 6px;
                border-radius: 4px;
                font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
                font-size: 0.9em;
              }

              :deep(pre) {
                margin: 16px 0;
                padding: 16px;
                border-radius: 12px;
                overflow-x: auto;
                max-width: 100%; // Limit max width
                background-color: #1e1e1e !important;
                box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
                border: 1px solid rgba(255, 255, 255, 0.1);

                code {
                  padding: 0;
                  background: none;
                  color: #abb2bf;
                  word-break: break-all; // Break long code lines
                }
              }

              :deep(blockquote) {
                margin: 16px 0;
                padding: 12px 20px;
                border-left: 4px solid var(--el-color-primary);
                background-color: var(--el-fill-color-lighter);
                border-radius: 8px;
                box-shadow: 0 1px 4px rgba(0, 0, 0, 0.04);

                p {
                  margin: 4px 0;
                }
              }

              :deep(table) {
                border-collapse: collapse;
                margin: 16px 0;
                width: 100%;
                border-radius: 8px;
                overflow: hidden;
                box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);

                th, td {
                  border: none;
                  border-bottom: 1px solid var(--el-border-color-lighter);
                  padding: 12px 16px;
                  text-align: left;
                }

                th {
                  background-color: var(--el-fill-color);
                  font-weight: 600;
                  color: var(--el-text-color-primary);
                }

                tr:nth-child(even) {
                  background-color: var(--el-fill-color-lighter);
                }

                tr:last-child td {
                  border-bottom: none;
                }
              }

              :deep(a) {
                color: var(--el-color-primary);
                text-decoration: none;

                &:hover {
                  text-decoration: underline;
                }
              }

              :deep(hr) {
                margin: 16px 0;
                border: none;
                border-top: 1px solid var(--el-border-color);
              }

              :deep(img) {
                max-width: 100%;
                border-radius: 8px;
                margin: 8px 0;
              }
            }

            .chart-container {
              margin-top: 20px;
              padding: 20px;
              background: linear-gradient(135deg, var(--el-fill-color-blank) 0%, var(--el-fill-color-light) 100%);
              border: none;
              border-radius: 16px;
              max-width: 100%; // Prevent overflow
              overflow: hidden; // Hide overflowing content
              box-shadow: 0 4px 16px rgba(0, 0, 0, 0.06);

              .chart {
                min-height: 400px;
                width: 100% !important; // Force 100% width
              }
            }

            .insights {
              margin-top: 12px;
              padding: 12px;
              background-color: var(--el-color-warning-light-9);
              border-left: 3px solid var(--el-color-warning);
              border-radius: 8px;

              .insights-header {
                display: flex;
                align-items: center;
                font-weight: 600;
                margin-bottom: 8px;
                color: var(--el-color-warning-dark-2);
              }

              .insights-list {
                list-style: none;
                padding: 0;
                margin: 0;

                li {
                  padding: 4px 0;
                  color: var(--el-text-color-regular);

                  &::before {
                    content: "💡 ";
                    margin-right: 4px;
                  }
                }
              }
            }

            // Processing progress display
            .processing-steps {
              margin-top: 16px;
              margin-bottom: 12px;

              .processing-header {
                display: flex;
                align-items: center;
                font-size: 14px;
                color: var(--el-text-color-regular);
                font-weight: 500;
              }

              :deep(.el-collapse) {
                border: none;
              }

              :deep(.el-collapse-item__header) {
                background: linear-gradient(135deg, var(--el-fill-color-light) 0%, var(--el-fill-color-lighter) 100%);
                border-radius: 12px;
                padding: 10px 16px;
                border: none;
                box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
                transition: all 0.3s ease;

                &:hover {
                  background: linear-gradient(135deg, var(--el-fill-color) 0%, var(--el-fill-color-light) 100%);
                  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
                  transform: translateY(-1px);
                }
              }

              :deep(.el-collapse-item__wrap) {
                border: none;
                background-color: transparent;
              }

              :deep(.el-collapse-item__content) {
                padding: 16px 0 0 0;
              }
            }

            // Processing steps timeline styles
            .steps-timeline, .live-processing-steps .steps-timeline {
              padding-left: 8px;

              .step-item {
                display: flex;
                position: relative;
                padding: 8px 0;

                &:not(:last-child)::before {
                  content: '';
                  position: absolute;
                  left: 7px;
                  top: 24px;
                  bottom: -8px;
                  width: 2px;
                  background-color: var(--el-border-color-light);
                }

                .step-dot {
                  width: 16px;
                  height: 16px;
                  border-radius: 50%;
                  background-color: var(--el-color-info);
                  margin-right: 12px;
                  margin-top: 2px;
                  flex-shrink: 0;
                  border: 3px solid var(--el-fill-color-blank);
                  box-shadow: 0 0 0 2px var(--el-color-info-light-5);
                }

                &.routing .step-dot {
                  background-color: var(--el-color-primary);
                  box-shadow: 0 0 0 2px var(--el-color-primary-light-5);
                }

                &.crew_execution .step-dot {
                  background-color: var(--el-color-success);
                  box-shadow: 0 0 0 2px var(--el-color-success-light-5);
                }

                &.agent_thinking .step-dot {
                  background-color: var(--el-color-warning);
                  box-shadow: 0 0 0 2px var(--el-color-warning-light-5);
                }

                &.tool_execution .step-dot {
                  background-color: #8b5cf6;
                  box-shadow: 0 0 0 2px #8b5cf6;
                }

                .step-content {
                  flex: 1;
                  min-width: 0;

                  .step-message {
                    font-size: 14px;
                    line-height: 1.5;
                    color: var(--el-text-color-primary);
                    margin-bottom: 4px;
                  }

                  .step-time {
                    font-size: 12px;
                    color: var(--el-text-color-secondary);
                  }

                  .step-details {
                    margin-top: 8px;
                    padding: 8px;
                    background-color: var(--el-fill-color-dark);
                    border-radius: 6px;
                    font-size: 12px;
                    max-height: 200px;
                    overflow-y: auto;

                    pre {
                      margin: 0;
                      white-space: pre-wrap;
                      word-break: break-all;
                      color: var(--el-text-color-regular);
                      font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
                    }
                  }
                }
              }
            }

            // Actions section (Vote buttons)
            .actions-section {
              margin-top: 8px;
              margin-left: 56px;
              max-width: min(calc(100% - 56px), 80%);
              display: flex;
              align-items: center;
              gap: 4px;

              .vote-buttons {
                display: flex;
                align-items: center;
                gap: 4px;
              }

              .vote-button {
                display: inline-flex;
                align-items: center;
                justify-content: center;
                width: 32px;
                height: 32px;
                padding: 0;
                margin: 0;
                border: none;
                background: transparent;
                cursor: pointer;
                transition: all 0.25s ease;
                border-radius: 8px;

                .vote-icon {
                  width: 18px;
                  height: 18px;
                  object-fit: contain;
                  transition: all 0.25s ease;
                }

                .vote-emoji {
                  font-size: 18px;
                  line-height: 1;
                }

                &:hover:not(:disabled) {
                  background: rgba(59, 130, 246, 0.08);

                  .vote-icon {
                    transform: scale(1.1);
                  }

                  .vote-emoji {
                    transform: scale(1.1);
                  }
                }

                &.active {
                  background: rgba(59, 130, 246, 0.12);
                }

                &:active:not(:disabled) {
                  transform: scale(0.95);
                }

                &:disabled {
                  opacity: 0.5;
                  cursor: not-allowed;
                }
              }
            }

            // Feedback form
            .feedback-form {
              margin-top: 12px;
              margin-left: 56px;
              max-width: min(calc(100% - 56px), 80%);
              padding: 16px;
              background: var(--el-fill-color-lighter);
              backdrop-filter: blur(12px);
              border: 1px solid var(--el-border-color-lighter);
              border-radius: 12px;
              animation: fadeIn 0.3s ease;

              html.dark & {
                background: rgba(30, 41, 59, 0.4);
                border: 1px solid rgba(255, 255, 255, 0.08);
              }

              .feedback-form-header {
                margin-bottom: 12px;

                .feedback-form-title {
                  font-size: 14px;
                  font-weight: 500;
                  color: var(--el-text-color-primary);
                }
              }

              .feedback-reasons {
                display: flex;
                flex-wrap: wrap;
                gap: 8px;
                margin-bottom: 16px;

                .feedback-reason-tag {
                  display: inline-flex;
                  align-items: center;
                  gap: 6px;
                  padding: 6px 12px;
                  font-size: 13px;
                  color: var(--el-text-color-regular);
                  background: var(--el-fill-color);
                  border: 1px solid var(--el-border-color-light);
                  border-radius: 8px;
                  cursor: pointer;
                  transition: all 0.2s ease;
                  user-select: none;

                  &:hover {
                    border-color: var(--el-color-primary-light-5);
                    background: var(--el-color-primary-light-9);
                    color: var(--el-color-primary);
                  }

                  &.selected {
                    background: var(--el-color-primary-light-8);
                    border-color: var(--el-color-primary);
                    color: var(--el-color-primary);
                  }

                  &.custom {
                    padding-right: 6px;

                    .remove-icon {
                      margin-left: 4px;
                      font-size: 14px;
                      cursor: pointer;
                      transition: all 0.2s ease;

                      &:hover {
                        color: var(--el-color-danger);
                      }
                    }
                  }
                }

                .feedback-custom-input {
                  display: flex;
                  align-items: center;
                  gap: 8px;
                  flex: 1;
                  min-width: 200px;

                  .custom-reason-input {
                    flex: 1;
                    padding: 6px 12px;
                    font-size: 13px;
                    color: var(--el-text-color-primary);
                    background: var(--el-fill-color-blank);
                    border: 1px solid var(--el-border-color);
                    border-radius: 8px;
                    outline: none;
                    transition: all 0.2s ease;

                    &::placeholder {
                      color: var(--el-text-color-placeholder);
                    }

                    &:focus {
                      border-color: var(--el-color-primary);
                      background: var(--el-fill-color-blank);
                    }
                  }
                }
              }

              .feedback-form-actions {
                display: flex;
                justify-content: flex-end;
                gap: 8px;
              }
            }

            // Real-time processing steps styles
            .live-processing-steps {
              margin-bottom: 12px;
              padding: 12px;
              background-color: var(--el-fill-color-lighter);
              border-radius: 8px;
              border-left: 3px solid var(--el-color-primary);

              .steps-timeline {
                .step-item:last-child .step-dot {
                  animation: pulse 1.5s ease-in-out infinite;
                }
              }
            }

            // Step progress bar styles
            .steps-progress-container {
              margin-bottom: 16px;
              padding: 16px;
              background: linear-gradient(135deg, var(--el-fill-color-light) 0%, var(--el-fill-color-lighter) 100%);
              border-radius: 12px;
              border: 1px solid var(--el-border-color-lighter);

              .step-progress-item {
                margin-bottom: 12px;

                &:last-child {
                  margin-bottom: 0;
                }

                .step-header {
                  display: flex;
                  align-items: center;
                  gap: 8px;
                  margin-bottom: 6px;
                  font-size: 14px;

                  .step-icon {
                    display: flex;
                    align-items: center;
                    font-size: 16px;

                    .rotating {
                      animation: rotate 1s linear infinite;
                    }
                  }

                  .step-name {
                    flex: 1;
                    font-weight: 500;
                    color: var(--el-text-color-primary);
                  }

                  .step-progress-value {
                    font-size: 13px;
                    color: var(--el-text-color-secondary);
                    font-weight: 600;
                  }
                }

                .progress-bar {
                  height: 6px;
                  background-color: var(--el-fill-color);
                  border-radius: 3px;
                  overflow: hidden;
                  margin-bottom: 4px;

                  .progress-fill {
                    height: 100%;
                    background: linear-gradient(90deg, var(--el-color-primary) 0%, var(--el-color-primary-light-3) 100%);
                    border-radius: 3px;
                    transition: width 0.3s ease;
                    position: relative;

                    &::after {
                      content: '';
                      position: absolute;
                      top: 0;
                      left: 0;
                      right: 0;
                      bottom: 0;
                      background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.3), transparent);
                      animation: shimmer 1.5s infinite;
                    }
                  }
                }

                .step-description {
                  font-size: 12px;
                  color: var(--el-text-color-secondary);
                  padding-left: 24px;
                }

                &.pending {
                  .step-icon {
                    color: var(--el-color-info);
                  }
                }

                &.running {
                  .step-icon {
                    color: var(--el-color-primary);
                  }
                  .step-name {
                    color: var(--el-color-primary);
                  }
                }

                &.completed {
                  .step-icon {
                    color: var(--el-color-success);
                  }
                  opacity: 0.8;
                }

                &.error {
                  .step-icon {
                    color: var(--el-color-danger);
                  }
                  .step-name {
                    color: var(--el-color-danger);
                  }
                }
              }
            }

            // Streaming content display
            .streaming-content {
              position: relative;
              margin-top: 12px;

              .cursor-blink {
                display: inline-block;
                margin-left: 2px;
                animation: blink 1s infinite;
                color: var(--el-color-primary);
                font-weight: bold;
              }
            }
          }

          &.user {
            .message-avatar {
              background-color: var(--el-color-primary-light-9);
              color: var(--el-color-primary);
            }

            .message-text {
              background-color: var(--el-color-primary);
              color: white;

              :deep(code) {
                background-color: rgba(255, 255, 255, 0.2);
                color: rgba(255, 255, 255, 0.95);
              }

              :deep(a) {
                color: rgba(255, 255, 255, 0.9);

                &:hover {
                  color: white;
                }
              }

              :deep(blockquote) {
                border-left-color: rgba(255, 255, 255, 0.5);
                background-color: rgba(255, 255, 255, 0.1);
              }
            }
          }

          &.assistant {
            .message-avatar {
              background-color: var(--el-fill-color);
              color: var(--el-text-color-regular);
            }

            .message-text {
              background-color: var(--el-fill-color-light);
              color: var(--el-text-color-primary);

              :deep(code) {
                background-color: var(--el-fill-color-dark);
              }
            }
          }
        }

        .loading-dots {
          display: flex;
          gap: 6px;
          padding: 12px 16px;

          span {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background-color: var(--el-color-primary);
            animation: bounce 1.4s infinite ease-in-out both;

            &:nth-child(1) {
              animation-delay: -0.32s;
            }

            &:nth-child(2) {
              animation-delay: -0.16s;
            }
          }
        }
      }

      .input-container {
        border-top: 1px solid var(--el-border-color-light);
        padding: 16px 20px;
        flex-shrink: 0; // Prevent input area from shrinking

        .chat-input-wrapper {
          position: relative;
          width: 100%;

          .chat-input {
            :deep(.el-textarea__inner) {
              font-size: 15px;
              padding: 14px 50px 14px 18px; // Right padding for button
              border-radius: 16px;
              resize: none;
              border: 1px solid var(--el-border-color-lighter);
              background: var(--el-fill-color-blank);
              box-shadow: 0 2px 8px rgba(0, 0, 0, 0.05);
              transition: all 0.3s ease;

              &::placeholder {
                color: var(--el-text-color-placeholder);
                opacity: 0.7;
              }

              &:hover {
                border-color: var(--el-color-primary-light-5);
                box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
              }

              &:focus {
                border-color: var(--el-color-primary);
                box-shadow: 0 4px 16px rgba(64, 158, 255, 0.2);
                outline: none;
              }
            }
          }

          .send-btn {
            position: absolute;
            right: 10px;
            bottom: 50%; // Center vertically
            transform: translateY(50%);
            width: 32px;
            height: 32px;
            border-radius: 6px;
            background: var(--el-color-primary);
            color: white;
            border: none;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: all 0.2s ease;

            &:hover:not(:disabled) {
              background: var(--el-color-primary-dark-2);
              transform: translateY(50%) scale(1.05);
            }

            &:active:not(:disabled) {
              transform: translateY(50%) scale(0.95);
            }

            &:disabled {
              background: var(--el-disabled-bg-color);
              color: var(--el-disabled-text-color);
              cursor: not-allowed;
              opacity: 0.6;
            }

            &.abort-btn {
              background: var(--el-color-danger);

              &:hover {
                background: var(--el-color-danger-light-3);
                transform: translateY(50%) scale(1.05);
              }

              &:active {
                transform: translateY(50%) scale(0.95);
              }
            }

            .is-loading {
              animation: rotate 1s linear infinite;
            }
          }
        }
      }
    }
  }
}

// Dark mode optimization for input
.dark {
  .integrated-input-container {
    .chat-input {
      :deep(.el-textarea__inner) {
        background: var(--el-bg-color);
        border-color: rgba(255, 255, 255, 0.15);
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);

        &:hover {
          border-color: rgba(255, 255, 255, 0.25);
          box-shadow: 0 6px 20px rgba(0, 0, 0, 0.4);
        }

        &:focus {
          border-color: var(--el-color-primary);
          box-shadow: 0 6px 24px rgba(64, 158, 255, 0.3);
        }
      }
    }
  }

  .input-container {
    .chat-input-wrapper {
      .chat-input {
        :deep(.el-textarea__inner) {
          background: rgba(255, 255, 255, 0.05);
          border-color: rgba(255, 255, 255, 0.12);

          &:hover {
            border-color: rgba(255, 255, 255, 0.2);
            background: rgba(255, 255, 255, 0.07);
          }

          &:focus {
            border-color: var(--el-color-primary);
            background: rgba(255, 255, 255, 0.08);
          }
        }
      }

      .send-btn {
        background: var(--el-color-primary);

        &:hover:not(:disabled) {
          background: var(--el-color-primary-light-3);
        }

        &.abort-btn {
          background: var(--el-color-danger);

          &:hover {
            background: var(--el-color-danger-light-3);
          }
        }
      }
    }
  }
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

@keyframes bounce {
  0%, 80%, 100% {
    transform: scale(0);
  }
  40% {
    transform: scale(1);
  }
}

@keyframes pulse {
  0% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.3);
    opacity: 0.7;
  }
  100% {
    transform: scale(1);
    opacity: 1;
  }
}

@keyframes blink {
  0%, 50%, 100% {
    opacity: 1;
  }
  25%, 75% {
    opacity: 0;
  }
}

@keyframes shimmer {
  0% {
    transform: translateX(-100%);
  }
  100% {
    transform: translateX(100%);
  }
}

@keyframes rotate {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

// Chat history panel styles
.history-panel {
  height: 100%;
  display: flex;
  flex-direction: column;

  .search-box {
    margin-bottom: 16px;
  }

  .loading-state {
    flex: 1;
    padding: 20px;
  }

  .conversations-list {
    flex: 1;
    overflow-y: auto;

    .conversation-item {
      padding: 12px;
      margin-bottom: 8px;
      border-radius: 8px;
      background-color: var(--el-fill-color-lighter);
      cursor: pointer;
      transition: all 0.2s;

      &:hover {
        background-color: var(--el-fill-color-light);
        transform: translateX(-4px);
      }

      &.active {
        background-color: var(--el-color-primary-light-9);
        border-left: 3px solid var(--el-color-primary);
      }

      .conversation-header {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        margin-bottom: 8px;

        .conversation-title {
          flex: 1;
          display: flex;
          align-items: center;
          font-weight: 600;
          font-size: 14px;
          color: var(--el-text-color-primary);
          overflow: hidden;

          span {
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
          }
        }

        .el-button {
          margin-left: 8px;
          opacity: 0;
          transition: opacity 0.2s;
        }
      }

      &:hover .el-button {
        opacity: 1;
      }

      .conversation-meta {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-bottom: 8px;
        font-size: 12px;

        .conversation-time {
          color: var(--el-text-color-secondary);
        }
      }

      .conversation-preview {
        font-size: 13px;
        color: var(--el-text-color-regular);
        line-height: 1.4;
        overflow: hidden;
        text-overflow: ellipsis;
        display: -webkit-box;
        line-clamp: 2;
        -webkit-line-clamp: 2;
        -webkit-box-orient: vertical;
      }
    }
  }

  .empty-state {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
  }
}

// Responsive adjustments for smaller screens (e.g. 1280px)
@media (max-width: 1280px) {
  .chat-panel .chat-card {
    // Adjust message styling
    .message {
      margin-bottom: 16px;

      .message-avatar {
        width: 32px;
        height: 32px;
        margin-right: 12px;

        .avatar-icon {
          font-size: 18px;
        }
      }

      .message-content {
        .message-header {
          margin-bottom: 4px;

          .message-role {
            font-size: 13px;
          }
          .message-time {
            font-size: 11px;
          }
        }

        .message-text {
          font-size: 13px;
          padding: 8px 12px;

          // Scale down markdown content
          :deep(h1) { font-size: 1.4em; margin: 8px 0; }
          :deep(h2) { font-size: 1.25em; margin: 8px 0; }
          :deep(h3) { font-size: 1.1em; margin: 6px 0; }
          :deep(p) { margin: 6px 0; }
          :deep(pre) {
            margin: 10px 0;
            padding: 10px;
            font-size: 12px;
          }
          :deep(code) { font-size: 0.9em; }
        }
      }
    }

    // Input Container
    .input-container {
      padding: 12px 16px;

      .chat-input-wrapper {
        .chat-input {
          :deep(.el-textarea__inner) {
            font-size: 13px;
            padding: 10px 40px 10px 12px;
          }
        }

        .send-btn {
          width: 28px;
          height: 28px;
          right: 8px;

          .el-icon {
            font-size: 16px;
          }
        }
      }
    }

    // Integrated Input Container (Empty State)
    .integrated-input-container {
      .chat-input-wrapper {
        .chat-input {
          :deep(.el-textarea__inner) {
            font-size: 14px;
            padding: 12px 50px 12px 16px;
          }
        }

        .send-btn {
          width: 30px;
          height: 30px;
          right: 10px;

          .el-icon {
            font-size: 18px;
          }
        }
      }
    }

    // Empty State Compact Mode
    .messages-container {
      padding: 16px !important;

      .empty-state-wrapper {
         padding: 10px !important;
         // On small screens, use safe center or fallback to flex-start
         justify-content: safe center !important;
         @supports not (justify-content: safe center) {
           justify-content: flex-start !important;
           padding-top: 4vh !important;
         }
      }

      .empty-state {
        gap: 20px !important; // Reduce from 40px

        .example-queries-section {
          .queries-hint {
            font-size: 14px !important;
            margin-bottom: 12px !important;
          }

          .example-queries {
            gap: 8px !important;

            .query-item {
               padding: 10px 14px !important;

               .query-icon { font-size: 16px !important; }
               span { font-size: 13px !important; }
            }
          }
        }
      }
    }
  }
}
</style>


<!-- eslint-disable vue/multi-word-component-names -->
<template>
  <div class="poco-chat-page">
    <!-- Left Sidebar -->
    <div class="sidebar" :class="{ collapsed: !sidebarVisible }">
      <div class="sidebar-header">
        <div class="sidebar-title">
          <el-icon :size="22"><Connection /></el-icon>
          <span>PrimusClaw</span>
        </div>
        <el-button text class="collapse-btn" @click="sidebarVisible = false">
          <img :src="sidebarIcon" class="collapse-icon" alt="Collapse" />
        </el-button>
      </div>

      <!-- New Chat Button -->
      <div class="sidebar-new-chat">
        <el-button text class="new-chat-btn" @click="startNewSession">
          <el-icon><Edit /></el-icon>
          <span>New Chat</span>
        </el-button>
      </div>

      <!-- History Section -->
      <div class="sidebar-section-header" @click="historyCollapsed = !historyCollapsed">
        <div class="section-header-left">
          <el-icon class="section-icon" :class="{ collapsed: historyCollapsed }">
            <ArrowRight />
          </el-icon>
          <span class="section-title">History</span>
        </div>
      </div>

      <div v-if="loadingSessions && !historyCollapsed" class="sidebar-loading">
        <el-skeleton :rows="4" animated />
      </div>

      <div v-else-if="sessions.length > 0 && !historyCollapsed" class="sidebar-list">
        <div
          v-for="item in sessions"
          :key="item.session_id"
          class="sidebar-item"
          :class="{ active: currentSessionId === item.session_id }"
          @click="loadSession(item)"
        >
          <div class="sidebar-item-content">
            <el-icon class="item-icon"><ChatDotRound /></el-icon>
            <div class="item-info">
              <div class="item-title">{{ item.name || item.title || 'Untitled' }}</div>
              <div class="item-meta">
                <el-tag v-if="item.status" size="small" :type="item.status === 'completed' ? 'success' : 'info'" effect="plain">
                  {{ item.status }}
                </el-tag>
              </div>
            </div>
          </div>
          <el-icon class="sidebar-item-delete" @click.stop="handleDeleteSession(item.session_id)"><Delete /></el-icon>
        </div>
      </div>

      <div v-else-if="!historyCollapsed" class="sidebar-empty">
        <el-icon><ChatDotRound /></el-icon>
        <p>No sessions yet</p>
      </div>
    </div>

    <!-- Main Content -->
    <div class="main-content" :class="{ 'sidebar-collapsed': !sidebarVisible }">
      <!-- Top Bar -->
      <div class="topbar">
        <div class="topbar-left">
          <el-button v-if="!sidebarVisible" text class="expand-sidebar-btn" @click="sidebarVisible = true">
            <img :src="sidebarIcon" class="expand-icon" alt="Expand" />
          </el-button>
        </div>
        <div class="topbar-right">
          <el-button class="back-button" @click="goBack">
            <el-icon class="back-icon"><Back /></el-icon>
            <span>Back</span>
          </el-button>
        </div>
      </div>

      <!-- Chat Container -->
      <div class="chat-container" :class="{ 'has-messages': messages.length > 0 || loadingMessages }">
        <div class="messages-scroll" ref="messagesContainer">

          <!-- Loading Messages -->
          <div v-if="loadingMessages" class="messages-loading">
            <el-skeleton :rows="6" animated />
          </div>

          <!-- Welcome Screen -->
          <div v-else-if="messages.length === 0" class="welcome-screen">
            <!-- Quick Start Prompts -->
            <h2 class="quick-start-title">💡 Quick Start:</h2>
            <div class="quick-prompts">
              <div class="quick-prompt-item" @click="setInput('What skills are available?')">
                <span class="prompt-bullet">▸</span><span>What skills are available?</span>
              </div>
              <div class="quick-prompt-item" @click="setInput('Help me understand how to use this agent')">
                <span class="prompt-bullet">▸</span><span>Help me understand how to use this agent</span>
              </div>
              <div class="quick-prompt-item" @click="setInput('Show me an example task')">
                <span class="prompt-bullet">▸</span><span>Show me an example task</span>
              </div>
            </div>
          </div>

          <!-- Messages List -->
          <div v-else class="messages-list">
            <div
              v-for="(message, index) in messages"
              :key="index"
              class="message-wrapper"
              :class="message.role"
            >
              <!-- Assistant message group (merged segments) -->
              <div v-if="message.role === 'assistant'" class="assistant-message-group">
                <div class="message-avatar assistant-avatar">
                  <el-icon><Connection /></el-icon>
                </div>
                <div class="assistant-content-wrapper">
                  <div class="message-header">
                    <span class="message-sender">PrimusClaw</span>
                  </div>

                  <!-- Streaming placeholder (live chat) -->
                  <template v-if="!message.segments || message.segments.length === 0">
                    <div v-if="!message.content && loading && index === messages.length - 1" class="typing-indicator">
                      <span></span><span></span><span></span>
                    </div>
                    <div v-else-if="message.content" class="message-text" v-html="formatMessage(message.content)"></div>
                  </template>

                  <!-- Merged segments (history) -->
                  <template v-else>
                    <template v-for="(seg, si) in message.segments" :key="si">
                      <!-- Text segment -->
                      <div v-if="seg.type === 'text' && seg.text" class="message-text" v-html="formatMessage(seg.text)"></div>

                      <!-- Tool execution bar (expandable) -->
                      <div v-else-if="seg.type === 'tool-execution'" class="tool-execution-group">
                        <div class="tool-execution-bar" @click="seg.expanded = !seg.expanded">
                          <div class="tool-bar-left">
                            <el-icon class="tool-bar-icon"><Connection /></el-icon>
                            <span class="tool-bar-label">Tool Execution ({{ seg.toolCount }})</span>
                          </div>
                          <el-icon class="tool-bar-chevron" :class="{ expanded: seg.expanded }">
                            <ArrowRight />
                          </el-icon>
                        </div>

                        <!-- Expanded: individual tool calls -->
                        <div v-if="seg.expanded && seg.toolCalls" class="tool-calls-list">
                          <div
                            v-for="(tc, ti) in seg.toolCalls"
                            :key="ti"
                            class="tool-call-item"
                          >
                            <div class="tool-call-header" @click="tc.expanded = !tc.expanded">
                              <div class="tool-call-header-left">
                                <el-icon v-if="tc.status === 'start' || tc.status === 'running'" class="tool-call-status running is-loading"><Loading /></el-icon>
                                <el-icon v-else class="tool-call-status" :class="tc.isError ? 'error' : 'success'">
                                  <CircleCheck v-if="!tc.isError" />
                                  <CircleClose v-else />
                                </el-icon>
                                <span class="tool-call-name">{{ tc.name }}</span>
                                <span v-if="tc.brief" class="tool-call-brief">{{ tc.brief }}</span>
                              </div>
                              <el-icon class="tool-call-chevron" :class="{ expanded: tc.expanded }">
                                <ArrowRight />
                              </el-icon>
                            </div>

                            <div v-if="tc.expanded" class="tool-call-body">
                              <div v-if="tc.input && Object.keys(tc.input).length" class="tool-call-section">
                                <div class="tool-call-section-title">INPUT</div>
                                <pre class="tool-call-code">{{ JSON.stringify(tc.input, null, 4) }}</pre>
                              </div>
                              <div v-if="tc.output || tc.description" class="tool-call-section">
                                <div class="tool-call-section-title">OUTPUT</div>
                                <pre class="tool-call-code">{{ tc.output || tc.description }}</pre>
                              </div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </template>
                  </template>
                </div>
              </div>

              <!-- User message -->
              <div v-else class="message-row user">
                <div class="message-content">
                  <div class="message-text" v-html="formatMessage(message.content)"></div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Input Area -->
        <div class="input-section">
          <div class="input-wrapper" @click="focusInput">
            <textarea
              ref="inputRef"
              v-model="userInput"
              placeholder="Ask me anything..."
              @keydown.enter="handleEnterKey"
              :disabled="loading"
              class="message-input"
              rows="1"
            />
            <div class="bottom-controls" @click.stop>
              <div class="left-controls">
                <el-popover trigger="click" placement="top-start" :width="380" :teleported="false">
                  <template #reference>
                    <div class="tools-trigger" :class="{ 'has-selected': selectedToolIds.size > 0 }">
                      <el-icon><Connection /></el-icon>
                      <span>Tools</span>
                      <el-tag v-if="selectedToolIds.size > 0" size="small" effect="dark" round>{{ selectedToolIds.size }}</el-tag>
                    </div>
                  </template>
                  <div class="tools-popover">
                    <div class="tools-popover-header">
                      <span>MCP / Skills</span>
                      <span class="tools-popover-count">{{ selectedToolIds.size }} / {{ tools.length }}</span>
                    </div>
                    <div class="tools-popover-body">
                      <div v-if="loadingTools" style="padding: 16px;"><el-skeleton :rows="3" animated /></div>
                      <template v-else>
                        <div v-if="mcpTools.length > 0" class="tools-section">
                          <div class="tools-section-label">MCP</div>
                          <div v-for="tool in mcpTools" :key="tool.id" class="tool-item" :class="{ selected: isToolSelected(tool.id) }" @click="toggleTool(tool.id)">
                            <img v-if="tool.icon_url" :src="tool.icon_url" class="tool-item-icon" />
                            <el-icon v-else class="tool-item-icon-fallback"><Monitor /></el-icon>
                            <div class="tool-item-info">
                              <div class="tool-item-header">
                                <span class="tool-item-name">{{ tool.name }}</span>
                                <el-tag v-if="tool.author" size="small" effect="plain" type="info">{{ tool.author }}</el-tag>
                              </div>
                              <div v-if="tool.description" class="tool-item-desc">{{ tool.description }}</div>
                            </div>
                            <el-icon class="tool-item-check" :class="{ active: isToolSelected(tool.id) }"><CircleCheck /></el-icon>
                          </div>
                        </div>
                        <div v-if="skillTools.length > 0" class="tools-section">
                          <div class="tools-section-label">SKILLS</div>
                          <div v-for="tool in skillTools" :key="tool.id" class="tool-item" :class="{ selected: isToolSelected(tool.id) }" @click="toggleTool(tool.id)">
                            <img v-if="tool.icon_url" :src="tool.icon_url" class="tool-item-icon" />
                            <el-icon v-else class="tool-item-icon-fallback"><MagicStick /></el-icon>
                            <div class="tool-item-info">
                              <div class="tool-item-header">
                                <span class="tool-item-name">{{ tool.name }}</span>
                                <el-tag v-if="tool.author" size="small" effect="plain" type="info">{{ tool.author }}</el-tag>
                              </div>
                              <div v-if="tool.description" class="tool-item-desc">{{ tool.description }}</div>
                            </div>
                            <el-icon class="tool-item-check" :class="{ active: isToolSelected(tool.id) }"><CircleCheck /></el-icon>
                          </div>
                        </div>
                        <div v-if="tools.length === 0" style="padding: 24px; text-align: center; color: #64748b; font-size: 13px;">No tools available</div>
                      </template>
                    </div>
                  </div>
                </el-popover>
              </div>
              <div class="right-controls">
                <img v-if="loading" :src="stopIcon" class="stop-icon" @click="stopGeneration" alt="Stop" />
                <el-icon v-else class="send-icon" @click="sendMessage" :class="{ active: userInput.trim() }">
                  <Position />
                </el-icon>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ChatDotRound,
  Edit,
  Back,
  ArrowRight,
  Connection,
  Delete,
  Position,
  Monitor,
  MagicStick,
  CircleClose,
  CircleCheck,
  Loading,
} from '@element-plus/icons-vue'
import sidebarIcon from '@/assets/icons/sidebar.png'
import stopIcon from '@/assets/icons/stop.png'
import { marked } from 'marked'
import {
  getSessions,
  createSession,
  deleteSession,
  getSessionMessages,
  pocoChat,
  processHistoryEvents,
  type PocoSession,
  type PocoChatMessage,
  type ToolCallDetail,
} from '@/services/poco'
import { getTools, type Tool } from '@/services/tools'

// Router
const router = useRouter()
const route = useRoute()

// State
const sidebarVisible = ref(true)
const historyCollapsed = ref(false)
const userInput = ref('')
const messages = ref<PocoChatMessage[]>([])
const loading = ref(false)
const loadingMessages = ref(false)
const messagesContainer = ref<HTMLElement>()
const inputRef = ref<HTMLTextAreaElement>()

// Session state
const sessions = ref<PocoSession[]>([])
const loadingSessions = ref(false)
const currentSessionId = ref('')

// Tools state (skills + mcp from /tools/api/v1/tools)
const tools = ref<Tool[]>([])
const loadingTools = ref(false)
const skillTools = computed(() => tools.value.filter(t => t.type === 'skill'))
const mcpTools = computed(() => tools.value.filter(t => t.type === 'mcp'))
const selectedToolIds = ref<Set<number>>(new Set())

const isToolSelected = (id: number) => selectedToolIds.value.has(id)
const toggleTool = (id: number) => {
  const s = new Set(selectedToolIds.value)
  if (s.has(id)) s.delete(id); else s.add(id)
  selectedToolIds.value = s
}

// AbortController
let abortController: AbortController | null = null

// ========== Helpers ==========

const formatMessage = (content: string) => {
  return marked(content, { breaks: true })
}

const focusInput = () => { inputRef.value?.focus() }

const setInput = (text: string) => {
  userInput.value = text
  nextTick(() => inputRef.value?.focus())
}

const scrollToBottom = () => {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
  }
}

const adjustTextareaHeight = () => {
  if (inputRef.value) {
    inputRef.value.style.height = 'auto'
    inputRef.value.style.height = inputRef.value.scrollHeight + 'px'
  }
}

watch(userInput, () => { nextTick(() => adjustTextareaHeight()) })

// ========== API Calls ==========

const fetchSessions = async () => {
  loadingSessions.value = true
  try {
    const res = await getSessions()
    sessions.value = res.data || []
  } catch (error) {
    console.error('Failed to fetch sessions:', error)
  } finally {
    loadingSessions.value = false
  }
}

const fetchTools = async () => {
  loadingTools.value = true
  try {
    const res = await getTools({ offset: 0, limit: 50, order: 'desc' })
    tools.value = res.tools || []
  } catch (error) {
    console.error('Failed to fetch tools:', error)
  } finally {
    loadingTools.value = false
  }
}

// ========== Session ==========

const loadSession = async (session: PocoSession) => {
  currentSessionId.value = session.session_id
  loadingMessages.value = true

  try {
    const res = await getSessionMessages(session.session_id)
    if (res.data && res.data.length > 0) {
      messages.value = processHistoryEvents(res.data)
      await nextTick()
      scrollToBottom()
    } else {
      messages.value = []
    }
  } catch (error) {
    console.error('Failed to load session messages:', error)
    messages.value = []
    ElMessage.warning('Failed to load messages')
  } finally {
    loadingMessages.value = false
  }
}

const startNewSession = () => {
  currentSessionId.value = ''
  messages.value = []
  loadingMessages.value = false
  ElMessage.success('New chat started')
}

const handleDeleteSession = async (sessionId: string) => {
  try {
    await ElMessageBox.confirm('Are you sure you want to delete this session?', 'Delete Session', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
    await deleteSession(sessionId)
    if (currentSessionId.value === sessionId) startNewSession()
    await fetchSessions()
    ElMessage.success('Session deleted')
  } catch (error) {
    if (error !== 'cancel') {
      console.error('Failed to delete session:', error)
      ElMessage.error('Failed to delete session')
    }
  }
}

// ========== Chat ==========

const handleEnterKey = (event: KeyboardEvent) => {
  if (event.shiftKey) return
  event.preventDefault()
  sendMessage()
}

const autoScroll = () => {
  nextTick(() => {
    if (messagesContainer.value) {
      const c = messagesContainer.value
      if (c.scrollHeight - c.scrollTop - c.clientHeight < 200) c.scrollTop = c.scrollHeight
    }
  })
}

const sendMessage = async () => {
  const query = userInput.value.trim()
  if (!query || loading.value) return

  messages.value.push({ role: 'user', content: query })
  userInput.value = ''
  nextTick(() => { if (inputRef.value) inputRef.value.style.height = 'auto' })

  const assistantIndex = messages.value.length
  // Initialize with segments for rich real-time rendering (tool calls + text interleaved)
  messages.value.push({ role: 'assistant', content: '', segments: [] })

  await nextTick()
  scrollToBottom()

  loading.value = true
  abortController = new AbortController()

  // Create session if needed
  if (!currentSessionId.value) {
    try {
      const res = await createSession({
        name: query.slice(0, 50),
        agent_id: 'agent_default',
      })
      currentSessionId.value = res.data.session_id
      fetchSessions()
    } catch (error) {
      console.error('Failed to create session:', error)
      messages.value[assistantIndex].content = 'Failed to create session. Please try again.'
      messages.value[assistantIndex].segments = undefined
      loading.value = false
      return
    }
  }

  // --- Live segment builders ---
  const msg = () => messages.value[assistantIndex]

  const appendText = (content: string) => {
    const m = msg()
    m.content += content
    const segs = m.segments!
    const last = segs[segs.length - 1]
    if (last?.type === 'text') {
      last.text = (last.text || '') + content
    } else {
      segs.push({ type: 'text', text: content })
    }
    autoScroll()
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const handleToolUsed = (data: any) => {
    if (data.tool === 'suggestion') return
    const m = msg()
    const segs = m.segments!

    if (data.status === 'start' || data.status === 'running') {
      // Group consecutive tool calls into one tool-execution segment
      const last = segs[segs.length - 1]
      let toolSeg = last?.type === 'tool-execution' ? last : null
      if (!toolSeg) {
        toolSeg = { type: 'tool-execution' as const, toolCalls: [] as ToolCallDetail[], toolCount: 0, expanded: true }
        segs.push(toolSeg)
      }

      // Check if this actionId already exists (running → running update)
      const existing = toolSeg.toolCalls!.find(t => t.toolUseId === data.actionId)
      if (existing) {
        if (data.brief) existing.brief = data.brief
        if (data.description) existing.description = data.description
      } else {
        toolSeg.toolCalls!.push({
          toolUseId: data.actionId || '',
          name: data.tool || data.brief || 'Unknown',
          tool: data.tool || '',
          status: data.status,
          brief: data.brief || '',
          description: data.description || '',
          input: data.argumentsDetail || undefined,
          isError: false,
          expanded: false,
        })
        toolSeg.toolCount = toolSeg.toolCalls!.length
      }
    } else if (data.status === 'success' || data.status === 'error') {
      // Update existing tool call across all segments
      for (const seg of segs) {
        if (seg.type !== 'tool-execution') continue
        const tc = seg.toolCalls?.find(t => t.toolUseId === data.actionId)
        if (tc) {
          tc.status = data.status
          tc.isError = data.status === 'error'
          if (data.tool) { tc.name = data.tool; tc.tool = data.tool }
          if (data.brief) tc.brief = data.brief
          if (data.description) { tc.description = data.description; tc.output = data.description }
          break
        }
      }
    }
    autoScroll()
  }

  try {
    await pocoChat(
      { query, session_id: currentSessionId.value, tools: [...selectedToolIds.value] },
      appendText,
      (error: unknown) => {
        console.error('Chat error:', error)
        const m = msg()
        m.content = 'Sorry, an error occurred. Please try again.'
        m.segments = undefined
        loading.value = false
      },
      () => { loading.value = false },
      abortController.signal,
      { onToolUsed: handleToolUsed },
    )
  } catch (err) {
    console.error('Send message error:', err)
    const m = msg()
    m.content = 'Sorry, an error occurred. Please try again.'
    m.segments = undefined
    loading.value = false
  }
}

const stopGeneration = () => {
  if (abortController) {
    abortController.abort()
    loading.value = false
    ElMessage.info('Generation stopped')
  }
}

const goBack = () => { router.back() }

// Lifecycle
onMounted(async () => {
  await Promise.all([fetchTools(), fetchSessions()])

  // Auto-select tools from URL query: ?tools=howtocook,weather
  const toolsParam = route.query.tools as string
  if (toolsParam && tools.value.length > 0) {
    const names = toolsParam.split(',').map(n => n.trim().toLowerCase()).filter(Boolean)
    const matched = tools.value.filter(t => names.includes(t.name.toLowerCase()))
    if (matched.length > 0) {
      selectedToolIds.value = new Set(matched.map(t => t.id))
    }
  }
})

onUnmounted(() => {
  if (abortController) abortController.abort()
})

defineOptions({ name: 'PocoChatPage' })
</script>

<style scoped lang="scss">
@import './styles.scss';

/* sidebar delete icon */
.sidebar-item-delete {
  font-size: 22px;
  color: #64748b;
  flex-shrink: 0;
  opacity: 0;
  padding: 4px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s;
}
.sidebar-item:hover .sidebar-item-delete { opacity: 1; }
.sidebar-item-delete:hover { color: #f87171; background: rgba(248, 113, 113, 0.1); }

/* tool call running spinner */
.tool-call-status.running {
  font-size: 16px;
  color: var(--theme-primary);
  animation: spin 1s linear infinite;
}

/* tool call brief text */
.tool-call-brief {
  font-size: 12px;
  color: #94a3b8;
  margin-left: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Tools Trigger Button */
.tools-trigger {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 4px 12px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 500;
  color: #94a3b8;
  transition: all 0.15s;
  &:hover { background: rgba(255, 255, 255, 0.08); color: #e2e8f0; }
  &.has-selected { color: var(--theme-primary); }
  .el-icon { font-size: 16px; }
}

/* Tools Popover */
.tools-popover {
  margin: -12px;
}
.tools-popover-header {
  padding: 12px 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 14px;
  font-weight: 600;
  color: #e2e8f0;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}
.tools-popover-count { font-size: 12px; font-weight: 400; color: #64748b; }
.tools-popover-body {
  max-height: 360px;
  overflow-y: auto;
  &::-webkit-scrollbar { width: 5px; }
  &::-webkit-scrollbar-thumb { background: rgba(255, 255, 255, 0.15); border-radius: 3px; }
}
.tools-section { padding: 0 0 4px; }
.tools-section-label {
  padding: 10px 16px 6px;
  font-size: 11px;
  font-weight: 700;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.tool-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  transition: background 0.15s;
  cursor: pointer;
  user-select: none;
  &:hover { background: rgba(255, 255, 255, 0.04); }
  &.selected { background: rgba(20, 184, 166, 0.08); }
}
.tool-item-icon {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  object-fit: cover;
  flex-shrink: 0;
}
.tool-item-icon-fallback {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: rgba(20, 184, 166, 0.12);
  color: var(--theme-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  flex-shrink: 0;
}
.tool-item-info { flex: 1; min-width: 0; }
.tool-item-header { display: flex; align-items: center; gap: 6px; }
.tool-item-name {
  font-size: 13px;
  font-weight: 600;
  color: #e2e8f0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tool-item-desc {
  font-size: 11px;
  color: #94a3b8;
  margin-top: 1px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tool-item-check {
  font-size: 16px;
  color: #475569;
  flex-shrink: 0;
  transition: all 0.15s;
  &.active { color: var(--theme-primary); }
}

:root:not(.dark) {
  .tools-trigger {
    color: #64748b;
    &:hover { background: rgba(15, 23, 42, 0.06); color: #1e293b; }
  }
  .tools-popover-header { color: #1e293b; border-bottom-color: rgba(203, 213, 225, 0.5); }
  .tool-item:hover { background: rgba(15, 23, 42, 0.04); }
  .tool-item.selected { background: rgba(20, 184, 166, 0.06); }
  .tool-item-name { color: #1e293b; }
  .tool-item-desc { color: #64748b; }
  .tool-item-check { color: #cbd5e1; }
}
</style>

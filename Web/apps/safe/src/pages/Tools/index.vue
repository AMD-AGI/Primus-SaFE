<template>
  <div class="tools-container">
    <!-- Page title -->
    <div class="page-header">
      <el-text class="block textx-18 font-500" tag="b">Tools</el-text>
      <div class="subtitle text-gray-500 text-sm mt-1">
        Browse and manage AI skills and MCP tools
      </div>
    </div>

    <!-- Action bar -->
    <div class="flex flex-wrap items-center mt-4">
      <!-- Left: create button -->
      <div class="flex gap-2">
        <el-button
          type="primary"
          round
          :icon="Plus"
          @click="showAddDialog = true"
          class="mb-2 text-black"
        >
          Create Tool
        </el-button>
      </div>

      <!-- Right side filters -->
      <div class="flex flex-wrap items-center mt-2 mb-2 sm:mt-0 ml-auto gap-4">
        <!-- Search box -->
        <el-input
          v-model="searchQuery"
          placeholder="Search tools..."
          clearable
          style="width: 260px"
          class="mb-2"
          @input="handleSearchDebounced"
          @clear="handleSearchClear"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <!-- Type filter - Segmented -->
        <el-segmented
          v-model="filters.type"
          :options="typeSegOptions"
          @change="handleFilterChange"
          class="mb-2"
        />

      </div>
    </div>

    <!-- Content area -->
    <div class="content-area" v-loading="loading" :element-loading-text="'Loading tools...'">
      <!-- Tool card grid -->
      <div v-if="tools.length > 0" class="tools-grid">
        <div
          v-for="tool in tools"
          :key="tool.id"
          class="tool-card"
          :class="{ 'selected': isToolSelected(tool.id) }"
          @click="handleCardClick(tool)"
        >
        <!-- Window top bar -->
        <div class="card-window-header">
          <!-- Left: filename -->
          <div class="window-title">
            <span class="filename">{{ tool.name }}.md</span>
          </div>

          <!-- Right: Type tag + PRIVATE label + checkbox -->
          <div class="right-controls">
            <el-tag
              :type="tool.type === 'skill' ? 'primary' : 'success'"
              size="small"
              effect="light"
            >
              {{ tool.type.toUpperCase() }}
            </el-tag>
            <el-tag
              v-if="!tool.is_public"
              type="warning"
              size="small"
              effect="light"
            >
              PRIVATE
            </el-tag>
            <el-checkbox
              :model-value="isToolSelected(tool.id)"
              @change="toggleToolSelection(tool.id)"
              @click.stop
            />
          </div>
        </div>

        <!-- Content area -->
        <div class="card-body">
          <div class="card-body__top">
            <div class="tool-icon">
              <img v-if="tool.icon_url" :src="tool.icon_url" :alt="tool.name" class="tool-avatar-img" />
              <LetterAvatar v-else-if="tool.type === 'skill'" :name="tool.name" :size="40" />
              <el-icon v-else><Document /></el-icon>
            </div>
            <div class="tool-meta">
              <span class="tool-name">{{ tool.name }}</span>
              <span class="tool-author">{{ tool.author }}</span>
            </div>
          </div>
          <p class="tool-desc">{{ tool.description || 'No description available' }}</p>
        </div>

        <!-- Bottom info bar -->
        <div class="card-footer-bar">
          <div class="footer-left">
            <span class="footer-date">{{ formatDate(tool.created_at) }}</span>
          </div>
          <div class="footer-right">
            <el-tooltip content="Run Tool" placement="top">
              <button
                class="footer-icon-btn"
                @click.stop="handleRun(tool)"
              >
                <el-icon><VideoPlay /></el-icon>
              </button>
            </el-tooltip>
            <el-tooltip content="Download" placement="top">
              <button
                class="footer-icon-btn"
                @click.stop="handleDownload(tool)"
              >
                <el-icon><Download /></el-icon>
              </button>
            </el-tooltip>
            <el-tooltip :content="tool.is_liked ? 'Unlike' : 'Like'" placement="top">
              <button
                class="footer-icon-btn like-btn"
                :class="{ liked: tool.is_liked }"
                @click.stop="handleToggleLike(tool)"
              >
                <el-icon>
                  <svg
                    viewBox="0 0 24 24"
                    :fill="tool.is_liked ? 'currentColor' : 'none'"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"></path>
                  </svg>
                </el-icon>
              </button>
            </el-tooltip>
            <el-tooltip v-if="tool.type === 'mcp'" content="Clone" placement="top">
              <button
                class="footer-icon-btn"
                @click.stop="handleClone(tool)"
              >
                <el-icon><DocumentCopy /></el-icon>
              </button>
            </el-tooltip>
          </div>
        </div>
        </div>
      </div>

      <!-- Empty state -->
      <el-empty v-else-if="!loading && tools.length === 0" description="No tools found" :image-size="200">
        <template #image>
          <el-icon :size="100" color="#C0C4CC">
            <Box />
          </el-icon>
        </template>
      </el-empty>
    </div>

    <!-- Floating bottom action bar -->
    <transition name="slide-up" @after-leave="onBarAfterLeave">
      <div v-if="selectedTools.length" class="selection-bar">
        <div class="left">
          <span class="ml-2"
            >Selected {{ selectedTools.length }} item{{ selectedTools.length === 1 ? '' : 's' }}</span
          >
        </div>

        <div class="right">
          <el-button type="primary" plain @click="handleBatchRun">
            <el-icon><VideoPlay /></el-icon>
            Run Selected
          </el-button>
        </div>
      </div>
    </transition>

    <!-- Pagination - fixed at bottom -->
    <div v-if="pagination.total > 0" class="pagination-container">
      <el-pagination
        v-model:current-page="pagination.currentPage"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[12, 24, 48, 96]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="handlePageSizeChange"
        @current-change="handlePageChange"
      />
    </div>

    <!-- Create/Edit tool dialog -->
    <AddDialog
      v-model:visible="showAddDialog"
      :action="dialogAction"
      :tool-id="editToolId"
      :clone-data="cloneData"
      @success="fetchTools"
    />

    <!-- Detail dialog -->
    <DetailDialog
      v-model:visible="showDetailDialog"
      :tool-id="currentToolId"
      @success="fetchTools"
      @edit="handleEditTool"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { VideoPlay, Download, Box, Plus, Document, Search, DocumentCopy } from '@element-plus/icons-vue'
import { useDebounceFn } from '@vueuse/core'
import { watch } from 'vue'
import { getTools, searchTools, downloadTool, likeTool, unlikeTool, getTool, type Tool, type GetToolsParams } from '@/services/tools'
import AddDialog from './Components/AddDialog.vue'
import DetailDialog from './Components/DetailDialog.vue'
import LetterAvatar from '@/components/Base/LetterAvatar.vue'

const router = useRouter()

// Clone data type
interface CloneData {
  name: string
  description: string
  tags: string[]
  is_public: boolean
  config: Record<string, unknown>
}

// State
const loading = ref(false)
const tools = ref<Tool[]>([])
const showAddDialog = ref(false)
const showDetailDialog = ref(false)
const currentToolId = ref<number | null>(null)
const cloneData = ref<CloneData | null>(null)
const dialogAction = ref<'Create' | 'Edit'>('Create')
const editToolId = ref<number | undefined>(undefined)

// Batch selection
const selectedTools = ref<number[]>([])

// Type Filter options
const typeSegOptions = [
  { label: 'All Types', value: '' },
  { label: 'MCP', value: 'mcp' },
  { label: 'Skill', value: 'skill' },
  { label: 'My Tools', value: 'mine' },
]

// Filter criteria
const filters = reactive({
  type: '',
})

// Search related
const searchQuery = ref('')

// Pagination
const pagination = reactive({
  currentPage: 1,
  pageSize: 12,
  total: 0,
})

// Format date (date part only)
const formatDate = (dateStr: string): string => {
  return dateStr.split('T')[0]
}

// Fetch tool list
const fetchTools = async () => {
  loading.value = true
  try {
    // If search term exists, use search API
    if (searchQuery.value.trim()) {
      const searchType: 'skill' | 'mcp' | undefined =
        filters.type === 'skill' || filters.type === 'mcp' ? filters.type : undefined

      const searchParams = {
        q: searchQuery.value.trim(),
        mode: 'semantic' as const,
        limit: pagination.pageSize,
        type: searchType,
      }

      const res = await searchTools(searchParams)
      tools.value = res.tools || []
      pagination.total = res.total || 0
      pagination.currentPage = 1 // Reset to first page when searching
      return
    }

    // Otherwise use the regular list API
    const params: GetToolsParams = {
      offset: (pagination.currentPage - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      order: 'desc',
    }

    // Handle type filter
    if (filters.type === 'mine') {
      // My Tools: Add owner=me parameter
      (params as GetToolsParams & { owner?: string }).owner = 'me'
    } else if (filters.type === 'skill' || filters.type === 'mcp') {
      params.type = filters.type
    }

    const res = await getTools(params)
    tools.value = res.tools || []
    pagination.total = res.total || 0

  } catch (error) {
    ElMessage.error((error as Error).message || 'Failed to load tools')
  } finally {
    loading.value = false
  }
}

// Handle filter change
const handleFilterChange = () => {
  pagination.currentPage = 1
  fetchTools()
}

const handleSearchClear = () => {
  searchQuery.value = ''
  fetchTools()
}

const handleSearchDebounced = useDebounceFn(() => {
  if (searchQuery.value.trim()) {
    fetchTools()
  }
}, 500)

// Handle pagination change
const handlePageChange = () => {
  fetchTools()
}

const handlePageSizeChange = () => {
  pagination.currentPage = 1
  fetchTools()
}

// Click card — open detail
const handleCardClick = (tool: Tool) => {
  currentToolId.value = tool.id
  showDetailDialog.value = true
}

// Bottom action bar related
const hasBarSpace = ref(false)
function onBarAfterLeave() {
  hasBarSpace.value = false
}

const isToolSelected = (toolId: number) => {
  return selectedTools.value.includes(toolId)
}

const toggleToolSelection = (toolId: number) => {
  const index = selectedTools.value.indexOf(toolId)
  if (index > -1) {
    selectedTools.value.splice(index, 1)
  } else {
    selectedTools.value.push(toolId)
  }
}

// Single run - navigate to internal Poco Chat page
const handleRun = (tool: Tool) => {

  // const pocoUrl = `${location.origin}/poco?utm_source=safe#settings/tools/import?tools=${tool.name}`
  // window.open(pocoUrl, '_blank')

  router.push({ path: '/claw', query: { tools: tool.name } })
}

// Batch run - navigate to internal Poco Chat page
const handleBatchRun = () => {
  if (selectedTools.value.length === 0) {
    ElMessage.warning('Please select at least one tool')
    return
  }

  const selectedToolNames = tools.value
    .filter(t => selectedTools.value.includes(t.id))
    .map(t => t.name)
    .join(',')

  // const pocoUrl = `${location.origin}/poco?utm_source=safe#settings/tools/import?tools=${selectedToolNames}`
  // window.open(pocoUrl, '_blank')

  // ElMessage.success(`Opening ${selectedTools.value.length} tool(s) in Poco`)

  router.push({ path: '/claw', query: { tools: selectedToolNames } })
  selectedTools.value = [] // Clear selected items
}

// Download tool
const handleDownload = async (tool: Tool) => {
  try {
    const loadingMsg = ElMessage({
      message: `Downloading ${tool.name}...`,
      type: 'info',
      duration: 0,
    })

    await downloadTool(tool.id)

    loadingMsg.close()
    ElMessage.success(`${tool.name} downloaded successfully`)
  } catch (error) {
    ElMessage.error((error as Error).message || 'Failed to download tool')
  }
}

// Like/unlike
const handleToggleLike = async (tool: Tool) => {
  try {
    if (tool.is_liked) {
      await unlikeTool(tool.id)
      tool.is_liked = false
      tool.like_count = (tool.like_count || 1) - 1
    } else {
      await likeTool(tool.id)
      tool.is_liked = true
      tool.like_count = (tool.like_count || 0) + 1
    }
  } catch (error) {
    ElMessage.error((error as Error).message || 'Failed to toggle like')
  }
}

// Clone tool (MCP only)
const handleClone = async (tool: Tool) => {

  try {
    const loadingMsg = ElMessage({
      message: `Loading ${tool.name} for cloning...`,
      type: 'info',
      duration: 0,
    })

    // Fetch tool details
    const toolDetail = await getTool(tool.id)

    loadingMsg.close()

    // Set clone data
    cloneData.value = {
      name: toolDetail.name,
      description: toolDetail.description,
      tags: toolDetail.tags || [],
      is_public: toolDetail.is_public,
      config: toolDetail.config || {},
    }

    // Open create dialog
    showAddDialog.value = true
  } catch (error) {
    ElMessage.error((error as Error).message || 'Failed to load tool data')
  }
}

// Handle edit tool
const handleEditTool = (toolId: number) => {
  dialogAction.value = 'Edit'
  editToolId.value = toolId
  showAddDialog.value = true
}

// Watch dialog close and clear data
watch(showAddDialog, (newValue) => {
  if (!newValue) {
    cloneData.value = null
    dialogAction.value = 'Create'
    editToolId.value = undefined
  }
})

// Initialize
onMounted(() => {
  fetchTools()
})

defineOptions({
  name: 'ToolsPage',
})
</script>

<style scoped lang="scss">
.tools-container {
  padding: 0;
  display: flex;
  flex-direction: column;
  height: calc(100vh - 120px); // Subtract header and padding height

  .page-header {
    margin-bottom: 20px;
    flex-shrink: 0;
  }

  .ml-auto {
    margin-left: auto;
  }

  .mb-2 {
    margin-bottom: 8px;
  }

  .content-area {
    flex: 1;
    overflow-y: auto;
    min-height: 0;

    // Optimize scrollbar style
    &::-webkit-scrollbar {
      width: 8px;
    }

    &::-webkit-scrollbar-track {
      background: var(--el-fill-color-lighter);
      border-radius: 4px;
    }

    &::-webkit-scrollbar-thumb {
      background: var(--el-border-color);
      border-radius: 4px;

      &:hover {
        background: var(--el-border-color-darker);
      }
    }
  }

  .tools-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(420px, 1fr));
    gap: 20px;
    margin-top: 20px;
    padding-right: 4px; // Prevent checkbox overflow causing scrollbar
    grid-auto-rows: 1fr; // Make all cards equal height
  }

  .tool-card {
    border-radius: var(--safe-radius-xl);
    overflow: hidden;
    display: flex;
    flex-direction: column;
    height: 100%;
    cursor: pointer;
    position: relative;

    // Glass effect
    background:
      linear-gradient(
        180deg,
        color-mix(in oklab, var(--safe-card) 82%, transparent 18%) 0%,
        color-mix(in oklab, var(--safe-card) 72%, transparent 28%) 100%
      );
    backdrop-filter: blur(18px) saturate(130%);
    -webkit-backdrop-filter: blur(18px) saturate(130%);
    border: 1px solid color-mix(in oklab, var(--safe-border) 55%, transparent 45%);
    box-shadow:
      inset 0 0.5px 0 0 rgb(255 255 255 / 0.18),
      0 1px 3px rgb(0 0 0 / 0.03),
      0 4px 14px -4px rgb(0 0 0 / 0.06);

    transition:
      transform 0.22s ease,
      box-shadow 0.22s ease,
      border-color 0.22s ease;

    // Top highlight line
    &::before {
      content: '';
      position: absolute;
      top: 0; left: 0; right: 0;
      height: 2px;
      border-radius: var(--safe-radius-xl) var(--safe-radius-xl) 0 0;
      background: linear-gradient(90deg, transparent 5%, var(--safe-primary) 50%, transparent 95%);
      opacity: 0.18;
      pointer-events: none;
      transition: opacity 0.22s ease;
      z-index: 1;
    }

    // Selected
    &.selected {
      border-color: var(--safe-primary);
      background:
        linear-gradient(
          180deg,
          color-mix(in oklab, var(--safe-card) 78%, var(--safe-primary) 4%) 0%,
          color-mix(in oklab, var(--safe-card) 70%, var(--safe-primary) 6%) 100%
        );
      box-shadow:
        0 0 0 1px var(--safe-primary),
        0 0 16px -2px color-mix(in oklab, var(--safe-primary) 30%, transparent 70%),
        0 0 32px -4px color-mix(in oklab, var(--safe-primary) 15%, transparent 85%);
    }

    &:hover {
      transform: translateY(-3px);
      border-color: color-mix(in oklab, var(--safe-primary) 28%, var(--safe-border) 72%);
      box-shadow:
        inset 0 0.5px 0 0 rgb(255 255 255 / 0.22),
        0 2px 6px rgb(0 0 0 / 0.04),
        0 10px 24px -6px rgb(0 0 0 / 0.09);

      &::before { opacity: 0.35; }

      .card-body .tool-name {
        color: var(--safe-primary) !important;
      }
      .card-body .tool-icon {
        transform: scale(1.1) rotate(4deg);
      }
    }

    // Top bar
    .card-window-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 8px 18px;
      background: transparent;

      // Tag Custom style for better contrast
      :deep(.el-tag--primary.el-tag--plain) {
        background: var(--el-color-primary);
        border-color: var(--el-color-primary);
        color: #ffffff;

        &:hover {
          background: var(--el-color-primary-dark-2);
          border-color: var(--el-color-primary-dark-2);
        }
      }

      // Light tag style optimization
      :deep(.el-tag--primary.el-tag--light) {
        background: rgba(0, 229, 229, 0.12);
        border-color: rgba(0, 229, 229, 0.3);
        color: #00a3a3;
        font-weight: 500;
      }

      :deep(.el-tag--success.el-tag--light) {
        background: var(--el-color-success-light-7);
        border-color: var(--el-color-success-light-5);
        color: var(--el-color-success-dark-2);
      }

      :deep(.el-tag--warning.el-tag--light) {
        background: var(--el-color-warning-light-7);
        border-color: var(--el-color-warning-light-5);
        color: var(--el-color-warning-dark-2);
      }

      .window-title {
        display: flex;
        align-items: center;
        gap: 6px;
        flex: 1;

        .filename {
          font-size: 12px;
          font-weight: 500;
          color: var(--safe-muted);
        }
      }

      .right-controls {
        display: flex;
        align-items: center;
        gap: 8px;

        :deep(.el-checkbox) {
          .el-checkbox__inner {
            border-width: 2px;
            width: 18px;
            height: 18px;
          }
        }
      }

    }

    // Content area
    .card-body {
      padding: 12px 18px 20px;
      flex: 1;
      display: flex;
      flex-direction: column;
      gap: 10px;

      &__top {
        display: flex;
        align-items: center;
        gap: 10px;
      }

      .tool-icon {
        width: 40px;
        height: 40px;
        border-radius: 10px;
        overflow: hidden;
        display: flex;
        align-items: center;
        justify-content: center;
        flex-shrink: 0;
        background: color-mix(in oklab, var(--safe-card-2) 60%, transparent 40%);
        transition: transform 0.22s ease;

        .tool-avatar-img {
          width: 100%;
          height: 100%;
          object-fit: cover;
        }

        .el-icon { font-size: 22px; }

        :deep(.letter-avatar) { border-radius: 10px; }
        &:has(.letter-avatar) { background: transparent; }
      }

      .tool-meta {
        flex: 1;
        min-width: 0;
        display: flex;
        flex-direction: column;
        gap: 2px;
      }

      .tool-name {
        color: var(--safe-text);
        font-size: 16px;
        font-weight: 600;
        line-height: 1.3;
        transition: color 0.22s ease;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .tool-author {
        font-size: 12px;
        color: var(--safe-muted);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .tool-desc {
        margin: 0;
        color: var(--safe-muted);
        font-size: 13px;
        line-height: 1.55;
        display: -webkit-box;
        -webkit-line-clamp: 2;
        line-clamp: 2;
        -webkit-box-orient: vertical;
        overflow: hidden;
      }
    }

    // Bottom bar
    .card-footer-bar {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 8px 18px;
      background: transparent;
      margin-top: auto;
      border-top: 1px solid color-mix(in oklab, var(--safe-border) 40%, transparent 60%);

      .footer-left {
        .footer-date {
          font-size: 12px;
          color: var(--safe-muted);
          font-weight: 400;
          letter-spacing: 0.02em;
        }
      }

      .footer-right {
        display: flex;
        gap: 4px;
        align-items: center;

        .footer-icon-btn {
          padding: 5px;
          border: none;
          background: transparent;
          color: var(--safe-muted);
          cursor: pointer;
          border-radius: 4px;
          display: flex;
          align-items: center;
          justify-content: center;
          transition: all 0.2s;
          opacity: 0.7;

          .el-icon {
            font-size: 15px;
          }

          &:hover:not(:disabled) {
            background: color-mix(in oklab, var(--safe-primary) 10%, transparent 90%);
            color: var(--safe-primary);
            opacity: 1;
            transform: scale(1.1);
          }

          &:disabled {
            opacity: 0.3;
            cursor: not-allowed;
          }

          // Like button special style
          &.like-btn {
            &.liked {
              color: #f56c6c;
              opacity: 1;

              .el-icon {
                animation: heartBeat 0.3s ease-in-out;
              }
            }

            &:hover:not(:disabled) {
              color: #f56c6c;
            }
          }
        }
      }
    }
  }

  .pagination-container {
    padding: 8px 0;
    display: flex;
    justify-content: flex-start; // Align left
    flex-shrink: 0;
  }

  // Bottom action bar
  .selection-bar {
    position: sticky;
    bottom: 0;
    z-index: 10;
    height: 56px;
    padding: 0 16px;
    background: var(--el-bg-color);
    border-top: 1px solid var(--el-border-color);
    display: flex;
    align-items: center;
    justify-content: space-between;
    box-shadow: 0 -6px 12px rgba(0, 0, 0, 0.06);
    gap: 12px;
    flex-shrink: 0;

    .left {
      display: flex;
      align-items: center;

      .ml-2 {
        margin-left: 8px;
      }
    }

    .right {
      display: flex;
      gap: 8px;
      align-items: center;
    }
  }

  // Enter animation
  .slide-up-enter-active,
  .slide-up-leave-active {
    transition:
      transform 0.18s ease,
      opacity 0.18s ease;
  }
  .slide-up-enter-from,
  .slide-up-leave-to {
    transform: translateY(100%);
    opacity: 0;
  }
}

// Heartbeat animation
@keyframes heartBeat {
  0%, 100% { transform: scale(1); }
  50% { transform: scale(1.2); }
}

// Dark mode tweaks (main theme adapts via --safe-* variables)
.dark {
  .tool-card {
    box-shadow:
      inset 0 0.5px 0 0 rgb(255 255 255 / 0.06),
      0 1px 3px rgb(0 0 0 / 0.12),
      0 4px 14px -4px rgb(0 0 0 / 0.22);

    &:hover {
      box-shadow:
        inset 0 0.5px 0 0 rgb(255 255 255 / 0.08),
        0 2px 6px rgb(0 0 0 / 0.16),
        0 10px 24px -6px rgb(0 0 0 / 0.3);
    }
  }
}
</style>

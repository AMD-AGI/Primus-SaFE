<template>
  <el-dialog
    :model-value="visible"
    :title="toolDetail?.name || 'Tool Details'"
    width="700px"
    @close="handleClose"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div v-if="detailLoading" class="loading-container">
      <el-skeleton :rows="8" animated />
    </div>

    <div v-else-if="toolDetail">
      <!-- Detail display -->
      <div class="detail-view">
        <!-- Top: tags and likes -->
        <div class="detail-header">
          <div class="tags-row">
            <el-tag
              :type="toolDetail.type === 'skill' ? 'primary' : 'success'"
              :effect="isDark ? 'plain' : 'light'"
            >
              {{ toolDetail.type.toUpperCase() }}
            </el-tag>
            <el-tag v-if="toolDetail.is_public" type="info" effect="plain">
              Public
            </el-tag>
            <el-tag v-else type="warning" :effect="isDark ? 'plain' : 'light'">
              PRIVATE
            </el-tag>
          </div>

          <!-- Like button -->
          <button
            class="like-btn-header"
            :class="{ liked: toolDetail.is_liked }"
            @click="handleToggleLike"
            :disabled="likeLoading"
          >
            <el-icon>
              <svg
                viewBox="0 0 24 24"
                :fill="toolDetail.is_liked ? 'currentColor' : 'none'"
                stroke="currentColor"
                stroke-width="2"
              >
                <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"></path>
              </svg>
            </el-icon>
            <span class="like-count">{{ toolDetail.like_count || 0 }}</span>
          </button>
        </div>

        <!-- Basic info -->
        <div class="info-section">
          <div class="info-row">
            <span class="info-label">Author:</span>
            <span class="info-value">{{ toolDetail.author }}</span>
          </div>
          <div class="info-row">
            <span class="info-label">Downloads:</span>
            <span class="info-value">{{ toolDetail.download_count }}</span>
          </div>
        </div>

        <!-- Description -->
        <div class="section">
          <div class="section-title">Description</div>
          <p class="description-text">{{ toolDetail.description }}</p>
        </div>

        <!-- Tags -->
        <div v-if="toolDetail.tags && toolDetail.tags.length > 0" class="section">
          <div class="section-title">Tags</div>
          <div class="tags-list">
            <el-tag
              v-for="tag in toolDetail.tags"
              :key="tag"
              size="small"
              class="mr-2 mb-2"
              :effect="isDark ? 'plain' : 'light'"
            >
              {{ tag }}
            </el-tag>
          </div>
        </div>

        <!-- MCP Config -->
        <div v-if="toolDetail.type === 'mcp' && toolDetail.config" class="section">
          <div class="section-title">MCP Configuration</div>
          <pre class="config-json">{{ JSON.stringify(toolDetail.config, null, 2) }}</pre>
        </div>

        <!-- Skill Content Preview -->
        <div v-if="toolDetail.type === 'skill'" class="section">
          <div class="section-header-with-action">
            <div class="section-title">Content</div>
            <el-button
              size="small"
              @click="handlePreviewContent"
              :loading="contentLoading"
            >
              Preview Markdown
            </el-button>
          </div>
          <div v-if="skillContent" class="markdown-preview" v-html="renderedMarkdown"></div>
          <div v-else class="preview-placeholder">
            Click "Preview Markdown" to view the skill content
          </div>
        </div>

        <!-- Time info -->
        <div class="time-info">
          <span class="time-item">Created: {{ formatTimeStr(toolDetail.created_at) }}</span>
          <span class="time-item">Updated: {{ formatTimeStr(toolDetail.updated_at) }}</span>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <!-- Left side: Delete button -->
        <div class="footer-left-actions">
          <el-button
            v-if="canEdit"
            type="danger"
            plain
            @click="handleDelete"
            :loading="deleteLoading"
          >
            Delete
          </el-button>
        </div>

        <!-- Right side: main actions -->
        <div class="footer-right-actions">
          <el-button @click="handleClose">Close</el-button>
          <el-button
            v-if="canEdit"
            type="primary"
            @click="handleEdit"
          >
            Edit
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useDark } from '@vueuse/core'
import { getTool, likeTool, unlikeTool, deleteTool, getSkillContent, type ToolDetail } from '@/services/tools'
import { useUserStore } from '@/stores/user'
import { formatTimeStr } from '@/utils'
import { marked } from 'marked'

// Configure marked
marked.setOptions({
  breaks: true,
  gfm: true,
})

const isDark = useDark()
const userStore = useUserStore()

const props = defineProps<{
  visible: boolean
  toolId: number | null
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  success: []
  edit: [toolId: number]
}>()

const detailLoading = ref(false)
const likeLoading = ref(false)
const deleteLoading = ref(false)
const contentLoading = ref(false)
const toolDetail = ref<ToolDetail | null>(null)
const skillContent = ref<string>('')

// Whether can edit: system managers can edit all, regular users can only edit their own
const canEdit = computed(() => {
  if (!toolDetail.value) return false
  // System managers can edit all tools
  if (userStore.isManager) return true
  // Regular users can only edit tools they created
  return toolDetail.value.author === userStore.profile?.name
})

// Render markdown
const renderedMarkdown = computed(() => {
  if (!skillContent.value) return ''
  try {
    return marked.parse(skillContent.value)
  } catch (error) {
    console.error('Markdown parse error:', error)
    return '<p>Failed to render markdown</p>'
  }
})

// Preview skill content
const handlePreviewContent = async () => {
  if (!props.toolId || toolDetail.value?.type !== 'skill') return

  try {
    contentLoading.value = true
    skillContent.value = await getSkillContent(props.toolId)
  } catch (error) {
    console.error('Load skill content failed:', error)
    ElMessage.error('Failed to load skill content')
  } finally {
    contentLoading.value = false
  }
}

// Fetch details
const fetchDetail = async () => {
  if (!props.toolId) return

  detailLoading.value = true
  try {
    const detail = await getTool(props.toolId)
    toolDetail.value = detail
  } catch (error) {
    ElMessage.error((error as Error).message || 'Failed to load tool details')
    handleClose()
  } finally {
    detailLoading.value = false
  }
}

// Like/unlike
const handleToggleLike = async () => {
  if (!toolDetail.value || !props.toolId) return

  try {
    likeLoading.value = true

    if (toolDetail.value.is_liked) {
      await unlikeTool(props.toolId)
      toolDetail.value.is_liked = false
      toolDetail.value.like_count = (toolDetail.value.like_count || 1) - 1
      ElMessage.success('Unliked')
    } else {
      await likeTool(props.toolId)
      toolDetail.value.is_liked = true
      toolDetail.value.like_count = (toolDetail.value.like_count || 0) + 1
      ElMessage.success('Liked')
    }

    emit('success') // Refresh list
  } catch (error) {
    ElMessage.error((error as Error).message || 'Failed to toggle like')
  } finally {
    likeLoading.value = false
  }
}

// Edit tool
const handleEdit = () => {
  if (!props.toolId) return
  emit('edit', props.toolId)
  handleClose()
}

// Delete tool
const handleDelete = async () => {
  if (!toolDetail.value || !props.toolId) return

  try {
    await ElMessageBox.confirm(
      `Are you sure you want to delete "${toolDetail.value.name}"? This action cannot be undone.`,
      'Delete Tool',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning',
      }
    )

    deleteLoading.value = true
    await deleteTool(props.toolId)

    ElMessage.success('Tool deleted successfully')
    emit('success')
    handleClose()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error((error as Error).message || 'Failed to delete tool')
    }
  } finally {
    deleteLoading.value = false
  }
}

// Close dialog
const handleClose = () => {
  emit('update:visible', false)
  setTimeout(() => {
    toolDetail.value = null
    skillContent.value = ''
  }, 300)
}

// Watch visible and toolId changes
watch(
  () => [props.visible, props.toolId],
  ([newVisible, newToolId]) => {
    if (newVisible && newToolId) {
      fetchDetail()
    }
  },
  { immediate: true }
)

defineOptions({
  name: 'ToolDetailDialog',
})
</script>

<style scoped lang="scss">
.loading-container {
  padding: 40px;
}

.detail-view {
  .icon-section {
    display: flex;
    justify-content: center;
    margin-bottom: 20px;

    .tool-icon-large {
      width: 80px;
      height: 80px;
      border-radius: 8px;
      object-fit: cover;
      border: 1px solid var(--el-border-color-light);
      box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
    }
  }

  .detail-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 24px;
    padding-bottom: 16px;
    border-bottom: 2px solid var(--el-border-color-lighter);

    .tags-row {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
      align-items: center; // Vertically center aligned
    }

    .like-btn-header {
      display: flex;
      align-items: center;
      gap: 6px;
      padding: 6px 12px;
      border: 1px solid var(--el-border-color);
      background: var(--el-bg-color);
      color: var(--el-text-color-secondary);
      cursor: pointer;
      border-radius: 6px;
      transition: all 0.2s;
      font-size: 14px;
      flex-shrink: 0;

      .el-icon {
        font-size: 16px;
      }

      .like-count {
        font-weight: 600;
      }

      &:hover:not(:disabled) {
        border-color: #f56c6c;
        background: rgba(245, 108, 108, 0.1);
        color: #f56c6c;
        transform: scale(1.05);
      }

      &.liked {
        color: #f56c6c;
        border-color: #f56c6c;
        background: rgba(245, 108, 108, 0.1);

        .el-icon {
          animation: heartBeat 0.3s ease-in-out;
        }
      }

      &:disabled {
        opacity: 0.5;
        cursor: not-allowed;
      }
    }
  }

  .info-section {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 16px;
    margin-bottom: 24px;

    .info-row {
      display: flex;
      flex-direction: column;
      gap: 4px;

      .info-label {
        font-size: 12px;
        color: var(--el-text-color-secondary);
        font-weight: 500;
      }

      .info-value {
        font-size: 16px;
        color: var(--el-text-color-primary);
        font-weight: 600;
      }
    }
  }

  .section {
    margin-bottom: 24px;

    .section-title {
      font-size: 14px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      margin-bottom: 12px;
    }

    .description-text {
      font-size: 14px;
      line-height: 1.6;
      color: var(--el-text-color-regular);
      margin: 0;
    }

    .tags-list {
      display: flex;
      flex-wrap: wrap;
    }

    .section-header-with-action {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 12px;
    }

    .config-json {
      background: var(--el-fill-color-light);
      border: 1px solid var(--el-border-color-light);
      border-radius: 8px;
      padding: 16px;
      font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
      font-size: 13px;
      line-height: 1.6;
      color: var(--el-text-color-primary);
      overflow-x: auto;
      margin: 0;
      max-height: 400px;
      overflow-y: auto;

      &::-webkit-scrollbar {
        width: 8px;
        height: 8px;
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

    .markdown-preview {
      background: var(--el-fill-color-light);
      border: 1px solid var(--el-border-color-light);
      border-radius: 8px;
      padding: 20px;
      max-height: 500px;
      overflow-y: auto;
      line-height: 1.8;
      color: var(--el-text-color-primary);

      :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6) {
        margin-top: 1.5em;
        margin-bottom: 0.5em;
        font-weight: 600;
        line-height: 1.3;
        color: var(--el-text-color-primary);

        &:first-child {
          margin-top: 0;
        }
      }

      :deep(h1) {
        font-size: 1.8em;
        border-bottom: 1px solid var(--el-border-color-light);
        padding-bottom: 0.3em;
      }

      :deep(h2) {
        font-size: 1.5em;
        border-bottom: 1px solid var(--el-border-color-lighter);
        padding-bottom: 0.3em;
      }

      :deep(h3) { font-size: 1.25em; }
      :deep(h4) { font-size: 1.1em; }

      :deep(p) {
        margin: 0.8em 0;
      }

      :deep(code) {
        background: var(--el-fill-color);
        padding: 2px 6px;
        border-radius: 3px;
        font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', monospace;
        font-size: 0.9em;
        color: var(--el-color-primary);
      }

      :deep(pre) {
        background: var(--el-fill-color);
        border: 1px solid var(--el-border-color);
        border-radius: 6px;
        padding: 12px;
        overflow-x: auto;
        margin: 1em 0;

        code {
          background: transparent;
          padding: 0;
          color: var(--el-text-color-primary);
        }
      }

      :deep(ul), :deep(ol) {
        margin: 0.8em 0;
        padding-left: 2em;
      }

      :deep(li) {
        margin: 0.3em 0;
      }

      :deep(blockquote) {
        border-left: 4px solid var(--el-color-primary-light-5);
        padding-left: 1em;
        margin: 1em 0;
        color: var(--el-text-color-secondary);
      }

      :deep(a) {
        color: var(--el-color-primary);
        text-decoration: none;

        &:hover {
          text-decoration: underline;
        }
      }

      :deep(table) {
        border-collapse: collapse;
        width: 100%;
        margin: 1em 0;
      }

      :deep(th), :deep(td) {
        border: 1px solid var(--el-border-color);
        padding: 8px 12px;
        text-align: left;
      }

      :deep(th) {
        background: var(--el-fill-color);
        font-weight: 600;
      }

      :deep(img) {
        max-width: 100%;
        height: auto;
        border-radius: 4px;
        margin: 1em 0;
      }

      &::-webkit-scrollbar {
        width: 8px;
        height: 8px;
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

    .preview-placeholder {
      background: var(--el-fill-color-lighter);
      border: 1px dashed var(--el-border-color);
      border-radius: 8px;
      padding: 40px 20px;
      text-align: center;
      color: var(--el-text-color-secondary);
      font-size: 13px;
    }
  }

  .time-info {
    display: flex;
    gap: 24px;
    padding-top: 16px;
    border-top: 1px solid var(--el-border-color-lighter);
    font-size: 12px;
    color: var(--el-text-color-secondary);

    .time-item {
      display: flex;
      align-items: center;
    }
  }
}

// Heartbeat animation
@keyframes heartBeat {
  0%, 100% { transform: scale(1); }
  50% { transform: scale(1.3); }
}

.tag-input-el {
  width: 120px;
}

.button-new-tag {
  height: 24px;
}

.dialog-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;

  .footer-left-actions {
    flex: 1;
  }

  .footer-right-actions {
    display: flex;
    gap: 8px;
  }
}
</style>

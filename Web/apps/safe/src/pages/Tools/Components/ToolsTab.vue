<template>
  <div class="tools-tab">
    <!-- Action bar -->
    <div class="flex flex-wrap items-center mb-4 gap-2">
      <el-button type="primary" round :icon="Plus" @click="openCreate">
        Create Tool
      </el-button>
      <div class="ml-auto flex flex-wrap gap-3 items-center">
        <el-input
          v-model="searchName"
          placeholder="Search tools..."
          clearable
          style="width: 240px"
          @input="handleSearchDebounced"
          @clear="fetchList"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-segmented
          v-model="typeFilter"
          :options="typeOptions"
          @change="handleTypeChange"
        />
      </div>
    </div>

    <!-- Cards -->
    <div v-loading="loading" class="content-area">
      <div v-if="list.length > 0" class="tools-grid">
        <div
          v-for="item in list"
          :key="item.id"
          class="tool-card"
          @click="isEditable(item) ? openEdit(item) : undefined"
        >
          <div class="card-header">
            <div class="header-left">
              <el-tag size="small" :type="toolTagType(item.type)" effect="light">
                {{ item.type.toUpperCase() }}
              </el-tag>
              <span class="tool-name">{{ item.display_name || item.name }}</span>
            </div>
            <div class="header-tags">
              <el-tag v-if="item.status === 'inactive'" size="small" type="info" effect="light">
                Inactive
              </el-tag>
              <el-tag v-if="!item.is_public" size="small" type="warning" effect="light">
                Private
              </el-tag>
            </div>
          </div>

          <p class="tool-desc">{{ item.description || 'No description' }}</p>

          <div v-if="item.tags?.length" class="tool-tags">
            <el-tag
              v-for="tag in item.tags.slice(0, MAX_TAGS)"
              :key="tag"
              size="small"
              effect="plain"
              type="info"
            >{{ tag }}</el-tag>
            <span v-if="item.tags.length > MAX_TAGS" class="tag-more">
              +{{ item.tags.length - MAX_TAGS }}
            </span>
          </div>

          <div class="card-footer">
            <div class="footer-left">
              <el-tag size="small" effect="light" type="primary">v{{ item.version || '–' }}</el-tag>
              <span class="footer-author">{{ item.author || '–' }}</span>
              <span class="footer-date">{{ formatDate(item.updated_at || item.created_at) }}</span>
            </div>
            <div class="card-actions">
              <!-- AddDialog currently only knows how to edit mcp / skill; hide
                   the entry for hooks/rule until the dialog grows support. -->
              <el-button
                v-if="isEditable(item)"
                size="small"
                text
                :icon="Edit"
                @click.stop="openEdit(item)"
              >Edit</el-button>
              <el-button
                size="small"
                text
                type="danger"
                :icon="Delete"
                :loading="deletingId === item.id"
                @click.stop="handleDelete(item)"
              >Delete</el-button>
            </div>
          </div>
        </div>
      </div>
      <el-empty v-else-if="!loading" description="No tools found" />
    </div>

    <!-- Pagination -->
    <div v-if="pagination.total > 0" class="pagination-container">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[12, 24, 48]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next"
        @size-change="handlePageSizeChange"
        @current-change="fetchList"
      />
    </div>

    <!-- Create / Edit dialog (shared with PluginFormDialog) -->
    <AddDialog
      v-model:visible="showDialog"
      :action="editingId ? 'Edit' : 'Create'"
      :tool-id="editingId"
      @success="onSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search, Edit, Delete } from '@element-plus/icons-vue'
import { useDebounceFn } from '@vueuse/core'
import { getTools, deleteTool, type Tool, type ToolType } from '@/services/tools'
import AddDialog from './AddDialog.vue'

const loading = ref(false)
const list = ref<Tool[]>([])
const searchName = ref('')
// `typeFilter === 'all'` means no type constraint; anything else is a ToolType.
const typeFilter = ref<'all' | ToolType>('all')

const pagination = reactive({ page: 1, pageSize: 12, total: 0 })

const showDialog = ref(false)
const editingId = ref<number | undefined>()
const deletingId = ref<number | undefined>()

const typeOptions = [
  { label: 'All', value: 'all' },
  { label: 'Skill', value: 'skill' },
  { label: 'MCP', value: 'mcp' },
  { label: 'Hooks', value: 'hooks' },
  { label: 'Rule', value: 'rule' },
]

const MAX_TAGS = 4

const formatDate = (s: string) => (s || '').split(' ')[0]

// el-tag type union. `rule` falls back to info since there's no dedicated
// slot on Element Plus for it. Keep in sync with PluginDetailDialog.
type TagType = 'primary' | 'success' | 'info' | 'warning' | 'danger' | undefined
const toolTagType = (type: string): TagType => {
  const map: Record<string, TagType> = {
    skill: 'primary',
    mcp: 'success',
    hooks: 'warning',
    rule: 'info',
  }
  return map[type]
}

const fetchList = async () => {
  loading.value = true
  try {
    const res = await getTools({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      name: searchName.value.trim() || undefined,
      type: typeFilter.value === 'all' ? undefined : typeFilter.value,
      latest_per_name: true,
    })
    list.value = res.tools || []
    pagination.total = res.total || 0
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to load tools')
  } finally {
    loading.value = false
  }
}

const handleSearchDebounced = useDebounceFn(() => {
  pagination.page = 1
  fetchList()
}, 500)

const handleTypeChange = () => {
  pagination.page = 1
  fetchList()
}

const handlePageSizeChange = () => {
  pagination.page = 1
  fetchList()
}

const openCreate = () => {
  editingId.value = undefined
  showDialog.value = true
}

const isEditable = (tool: Tool) => tool.type === 'mcp' || tool.type === 'skill'

const openEdit = (tool: Tool) => {
  if (!isEditable(tool)) return
  editingId.value = tool.id
  showDialog.value = true
}

const onSaved = () => {
  showDialog.value = false
  fetchList()
}

const handleDelete = async (tool: Tool) => {
  try {
    await ElMessageBox.confirm(
      `Delete tool "${tool.display_name || tool.name}"? This cannot be undone.`,
      'Delete Tool',
      { confirmButtonText: 'Delete', cancelButtonText: 'Cancel', type: 'warning' },
    )
  } catch { return }

  deletingId.value = tool.id
  try {
    await deleteTool(tool.id)
    ElMessage.success('Tool deleted')
    fetchList()
  } catch (e) {
    ElMessage.error((e as Error).message || 'Delete failed')
  } finally {
    deletingId.value = undefined
  }
}

onMounted(fetchList)
</script>

<style scoped lang="scss">
.tools-tab {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.content-area {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding: 2px;
}

.tools-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 16px;
}

.tool-card {
  padding: 16px 20px;
  border-radius: var(--safe-radius-xl, 12px);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: color-mix(in oklab, var(--safe-card, var(--el-bg-color)) 82%, transparent 18%);
  border: 1px solid color-mix(in oklab, var(--safe-border, var(--el-border-color)) 55%, transparent 45%);
  transition: transform 0.2s, box-shadow 0.2s, border-color 0.2s;

  &:hover {
    transform: translateY(-2px);
    border-color: var(--safe-primary, var(--el-color-primary));
    box-shadow: 0 4px 16px -4px rgba(0, 0, 0, 0.08);

    .card-actions { opacity: 1; }
  }

  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    flex: 1;
  }

  .tool-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--safe-text, var(--el-text-color-primary));
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .header-tags {
    display: flex;
    gap: 6px;
    align-items: center;
    flex-shrink: 0;
  }

  .tool-desc {
    margin: 0;
    font-size: 13px;
    color: var(--safe-muted, var(--el-text-color-secondary));
    line-height: 1.5;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    min-height: 38px;
  }

  .tool-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    align-items: center;

    .tag-more {
      font-size: 12px;
      color: var(--safe-muted, var(--el-text-color-secondary));
    }
  }

  .card-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 12px;
    color: var(--safe-muted, var(--el-text-color-secondary));
    padding-top: 8px;
    border-top: 1px solid color-mix(in oklab, var(--safe-border, var(--el-border-color)) 40%, transparent 60%);
    margin-top: auto;
    gap: 8px;

    .footer-left {
      display: flex;
      align-items: center;
      gap: 8px;
      min-width: 0;
      overflow: hidden;
    }

    .footer-author,
    .footer-date {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  // Edit/Delete actions reveal on hover so the footer stays quiet at rest.
  .card-actions {
    display: flex;
    gap: 4px;
    opacity: 0;
    transition: opacity 0.2s;
    flex-shrink: 0;
  }
}

.pagination-container {
  padding: 8px 0;
  flex-shrink: 0;
}
</style>

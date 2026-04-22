<template>
  <div class="plugins-tab">
    <!-- Action bar -->
    <div class="flex flex-wrap items-center mb-4">
      <el-button type="primary" round :icon="Plus" @click="showCreate = true">
        Create Plugin
      </el-button>
      <div class="ml-auto flex gap-4 items-center">
        <el-input
          v-model="searchName"
          placeholder="Search plugins..."
          clearable
          style="width: 240px"
          @input="handleSearchDebounced"
          @clear="fetchList"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
      </div>
    </div>

    <!-- Cards -->
    <div v-loading="loading" class="content-area">
      <div v-if="list.length > 0" class="plugins-grid">
        <div
          v-for="item in list"
          :key="item.id"
          class="plugin-card"
          @click="handleDetail(item)"
        >
          <div class="card-header">
            <div class="card-header-left">
              <div class="plugin-icon">
                <img
                  v-if="item.icon_url && !failedIcons.has(item.id)"
                  :src="item.icon_url"
                  :alt="`${item.name} icon`"
                  @error="onIconError(item.id)"
                />
                <span v-else class="plugin-icon-fallback">{{ initialOf(item.name) }}</span>
              </div>
              <span class="plugin-name">{{ item.name }}</span>
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
          <p class="plugin-desc">{{ item.description || 'No description' }}</p>

          <!-- Tool chips: single-row, ellipsis, overflow fallback -->
          <div v-if="item.tools.length" class="tool-chips">
            <el-tooltip
              v-for="t in item.tools.slice(0, MAX_TOOL_CHIPS)"
              :key="t.id"
              placement="top"
              :show-after="300"
            >
              <template #content>
                <div style="max-width: 320px;">
                  <div style="font-weight: 600; margin-bottom: 4px;">
                    [{{ t.type.toUpperCase() }}] {{ t.name || `#${t.id}` }}
                    <span style="opacity: 0.7; font-weight: 400; margin-left: 4px;">v{{ t.version }}</span>
                  </div>
                  <div style="font-size: 12px; line-height: 1.5; white-space: pre-wrap;">
                    {{ t.description || 'No description' }}
                  </div>
                </div>
              </template>
              <span class="tool-chip" :class="`tool-chip--${t.type}`">
                <span class="tool-chip-type">{{ t.type.toUpperCase() }}</span>
                <span class="tool-chip-name">{{ t.name || `#${t.id}` }}</span>
              </span>
            </el-tooltip>
            <el-tooltip
              v-if="item.tools.length > MAX_TOOL_CHIPS"
              placement="top"
            >
              <template #content>
                <div style="max-width: 280px; font-size: 12px; line-height: 1.6;">
                  <div style="font-weight: 600; margin-bottom: 4px;">
                    +{{ item.tools.length - MAX_TOOL_CHIPS }} more tools
                  </div>
                  <div v-for="t in item.tools.slice(MAX_TOOL_CHIPS)" :key="t.id">
                    [{{ t.type.toUpperCase() }}] {{ t.name || `#${t.id}` }}
                  </div>
                </div>
              </template>
              <span class="tool-chip tool-chip--more">
                +{{ item.tools.length - MAX_TOOL_CHIPS }}
              </span>
            </el-tooltip>
          </div>

          <!-- Resources: muted inline text -->
          <div v-if="item.resources?.length" class="resource-line">
            <span class="resource-label">Resources</span>
            <span class="resource-names">
              {{ item.resources.map(r => r.name || `#${r.id}`).join(' · ') }}
            </span>
          </div>

          <div class="card-footer">
            <div class="footer-left">
              <el-tag size="small" effect="light" type="primary">v{{ item.version || '–' }}</el-tag>
              <span class="footer-author">{{ item.author || '–' }}</span>
              <el-tooltip
                v-if="item.updated_at && item.updated_at !== item.created_at"
                :content="`Updated ${formatDate(item.updated_at)}`"
                placement="top"
              >
                <span class="footer-date">{{ formatDate(item.updated_at) }}</span>
              </el-tooltip>
              <span v-else class="footer-date">{{ formatDate(item.created_at) }}</span>
            </div>
            <button class="run-btn" @click.stop="handleRun(item)">
              <el-icon><VideoPlay /></el-icon>
              Run
            </button>
          </div>
        </div>
      </div>
      <el-empty v-else-if="!loading" description="No plugins found" />
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

    <!-- Create / Edit dialog -->
    <PluginFormDialog
      v-model:visible="showCreate"
      :plugin-id="editId"
      @success="onFormSuccess"
      @close="editId = undefined"
    />

    <!-- Detail dialog -->
    <PluginDetailDialog
      v-model:visible="showDetail"
      :plugin-id="detailId"
      @edit="handleEdit"
      @deleted="fetchList"
      @run="handleRun"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Plus, Search, VideoPlay } from '@element-plus/icons-vue'
import { useDebounceFn } from '@vueuse/core'
import { getPlugins, type Plugin } from '@/services/tools'
import PluginFormDialog from './PluginFormDialog.vue'
import PluginDetailDialog from './PluginDetailDialog.vue'

const router = useRouter()

const loading = ref(false)
const list = ref<Plugin[]>([])
const searchName = ref('')
const showCreate = ref(false)
const showDetail = ref(false)
const editId = ref<number | undefined>()
const detailId = ref<number | null>(null)

const pagination = reactive({ page: 1, pageSize: 12, total: 0 })

const MAX_TOOL_CHIPS = 3

const formatDate = (s: string) => s.split(' ')[0]

const initialOf = (name: string) => (name || '?').trim().charAt(0).toUpperCase()

// Track icons that failed to load so we can fall back to the letter badge
// instead of showing a broken-image glyph (happens when icon_url is stale
// or S3 auth expires).
const failedIcons = reactive(new Set<number>())
const onIconError = (id: number) => failedIcons.add(id)

const toolTagType = (type: string) =>
  ({ skill: 'primary', mcp: 'success', hooks: 'warning', rule: '' } as Record<string, string>)[type] || ''

const fetchList = async () => {
  loading.value = true
  try {
    const res = await getPlugins({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      name: searchName.value.trim() || undefined,
      latest_per_name: true,
    })
    list.value = res.plugins || []
    pagination.total = res.total || 0
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to load plugins')
  } finally {
    loading.value = false
  }
}

const handleSearchDebounced = useDebounceFn(fetchList, 500)

const handlePageSizeChange = () => {
  pagination.page = 1
  fetchList()
}

const handleDetail = (item: Plugin) => {
  detailId.value = item.id
  showDetail.value = true
}

const handleEdit = (id: number) => {
  showDetail.value = false
  editId.value = id
  showCreate.value = true
}

const handleRun = (plugin: Plugin) => {
  const toolNames = plugin.tools.map(t => t.name || `tool-${t.id}`).filter(Boolean).join(',')
  if (!toolNames) {
    ElMessage.warning('This plugin has no tools to run')
    return
  }
  router.push({ path: '/claw', query: { tools: toolNames } })
}

const onFormSuccess = () => {
  editId.value = undefined
  fetchList()
}

onMounted(fetchList)
</script>

<style scoped lang="scss">
.plugins-tab {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.content-area {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
  padding: 2px;
}

.plugins-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 16px;
}

.plugin-card {
  padding: 16px 20px;
  border-radius: var(--safe-radius-xl, 12px);
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: color-mix(in oklab, var(--safe-card, var(--el-bg-color)) 82%, transparent 18%);
  border: 1px solid color-mix(in oklab, var(--safe-border, var(--el-border-color)) 55%, transparent 45%);
  transition: transform 0.2s, box-shadow 0.2s, border-color 0.2s;

  &:hover {
    transform: translateY(-2px);
    border-color: var(--safe-primary, var(--el-color-primary));
    box-shadow: 0 4px 16px -4px rgba(0, 0, 0, 0.08);
  }

  .card-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }

  .card-header-left {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    flex: 1;
  }

  // Compact 32px icon that sits flush-left of the plugin name. Fallback badge
  // uses the first letter, color-accented so different plugins stay visually
  // distinct even without a custom upload.
  .plugin-icon {
    width: 32px;
    height: 32px;
    border-radius: 8px;
    overflow: hidden;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: color-mix(in oklab, var(--safe-primary, var(--el-color-primary)) 10%, var(--el-fill-color));
    border: 1px solid var(--safe-border, var(--el-border-color-lighter));

    img {
      width: 100%;
      height: 100%;
      object-fit: cover;
      display: block;
    }

    .plugin-icon-fallback {
      font-size: 14px;
      font-weight: 600;
      color: var(--safe-primary, var(--el-color-primary));
      text-transform: uppercase;
      line-height: 1;
    }
  }

  .plugin-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--safe-text, var(--el-text-color-primary));
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .plugin-desc {
    margin: 0;
    font-size: 13px;
    color: var(--safe-muted, var(--el-text-color-secondary));
    line-height: 1.5;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .header-tags {
    display: flex;
    gap: 6px;
    align-items: center;
    flex-shrink: 0;
  }

  .tool-chips {
    display: flex;
    gap: 6px;
    flex-wrap: nowrap;
    overflow: hidden;
    min-width: 0;
  }

  .resource-line {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--safe-muted, var(--el-text-color-secondary));
    line-height: 1.4;
    overflow: hidden;

    .resource-label {
      font-weight: 500;
      opacity: 0.8;

      &::after { content: ':'; }
    }

    .resource-names {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .tool-chip {
    display: inline-flex;
    align-items: center;
    gap: 0;
    font-size: 12px;
    line-height: 20px;
    border-radius: 4px;
    overflow: hidden;
    border: 1px solid color-mix(in oklab, var(--safe-border, var(--el-border-color)) 60%, transparent 40%);
    background: var(--safe-card, var(--el-bg-color));
    min-width: 0;
    flex: 0 1 auto;

    .tool-chip-type {
      padding: 0 6px;
      font-weight: 600;
      font-size: 10px;
      letter-spacing: 0.3px;
      color: #fff;
      flex-shrink: 0;
    }

    .tool-chip-name {
      padding: 0 8px;
      color: var(--safe-text, var(--el-text-color-primary));
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      min-width: 24px;
    }

    &.tool-chip--mcp .tool-chip-type { background: var(--el-color-success); }
    &.tool-chip--skill .tool-chip-type { background: var(--el-color-primary); }
    &.tool-chip--hooks .tool-chip-type { background: var(--el-color-warning); }
    &.tool-chip--rule .tool-chip-type { background: var(--el-color-info); }

    &.tool-chip--more {
      padding: 0 10px;
      font-weight: 500;
      color: var(--safe-muted, var(--el-text-color-secondary));
      flex-shrink: 0;
      cursor: help;
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

    .footer-left {
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .run-btn {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 2px 10px;
      font-size: 12px;
      font-weight: 500;
      border-radius: 4px;
      border: 1px solid var(--el-border-color);
      background: transparent;
      color: var(--el-text-color-secondary);
      cursor: pointer;
      transition: all 0.2s;

      &:hover {
        color: var(--safe-primary, var(--el-color-primary));
        border-color: var(--safe-primary, var(--el-color-primary));
        background: rgba(0, 229, 229, 0.08);
      }
    }
  }
}


.pagination-container {
  padding: 8px 0;
  flex-shrink: 0;
}
</style>

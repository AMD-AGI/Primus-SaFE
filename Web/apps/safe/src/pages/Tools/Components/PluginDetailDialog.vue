<template>
  <el-dialog
    :model-value="visible"
    :title="detail?.name || 'Plugin Details'"
    width="680px"
    @close="handleClose"
    :close-on-click-modal="false"
  >
    <!-- Loading skeleton while the plugin detail is being fetched. -->
    <div v-if="loading" class="detail-loading">
      <el-skeleton :rows="6" animated />
    </div>

    <div v-else-if="detail" class="detail-view">
      <!-- Top info -->
      <div class="info-grid">
        <div class="info-item">
          <span class="label">Version</span>
          <span class="value">{{ detail.version || '–' }}</span>
        </div>
        <div class="info-item">
          <span class="label">Author</span>
          <span class="value">{{ detail.author || '–' }}</span>
        </div>
        <div class="info-item">
          <span class="label">Status</span>
          <el-tag size="small" :type="detail.status === 'active' ? 'success' : 'warning'" effect="light">
            {{ detail.status }}
          </el-tag>
        </div>
        <div class="info-item">
          <span class="label">Visibility</span>
          <el-tag size="small" :type="detail.is_public ? 'primary' : 'warning'" effect="light">
            {{ detail.is_public ? 'Public' : 'Private' }}
          </el-tag>
        </div>
      </div>

      <div v-if="detail.description" class="section">
        <div class="section-title">Description</div>
        <p class="desc">{{ detail.description }}</p>
      </div>

      <!-- Tools: header is lightweight (from plugin.tools), body lazy-loads
           the full tool detail via getTool(id) the first time it expands. -->
      <div v-if="detail.tools.length" class="section">
        <div class="section-title">Tools ({{ detail.tools.length }})</div>
        <div class="tool-list">
          <div
            v-for="t in detail.tools"
            :key="t.id"
            class="tool-item"
            :class="{ expanded: expandedTools.has(t.id) }"
          >
            <div class="tool-item-header" @click="toggleToolExpand(t)">
              <div class="tool-item-left">
                <el-tag size="small" :type="toolTagType(t.type)" effect="light">
                  {{ t.type.toUpperCase() }}
                </el-tag>
                <span class="tool-item-name">{{ t.name || `#${t.id}` }}</span>
                <span class="tool-item-ver">v{{ t.version }}</span>
              </div>
              <el-icon class="expand-icon">
                <ArrowDown />
              </el-icon>
            </div>
            <transition name="fade-slide">
              <div v-show="expandedTools.has(t.id)" class="tool-item-body">
                <div v-if="toolLoading[t.id]" class="tool-loading">
                  <el-skeleton :rows="2" animated />
                </div>
                <template v-else>
                  <!-- Display name (only when it's meaningfully different
                       from the bare `name` key we already show in the header) -->
                  <div
                    v-if="toolDetails[t.id]?.display_name && toolDetails[t.id].display_name !== t.name"
                    class="tool-item-display-name"
                  >{{ toolDetails[t.id].display_name }}</div>

                  <p
                    v-if="resolveToolField(t, 'description')"
                    class="tool-item-desc"
                  >{{ resolveToolField(t, 'description') }}</p>

                  <!-- Tags (only available from the tool detail endpoint) -->
                  <div v-if="toolDetails[t.id]?.tags?.length" class="tool-item-tags">
                    <el-tag
                      v-for="tag in toolDetails[t.id].tags"
                      :key="tag"
                      size="small"
                      effect="plain"
                      type="info"
                    >{{ tag }}</el-tag>
                  </div>

                  <!-- Meta chips: author / source link / privacy / dates.
                       Only rendered once the detail has landed so we avoid
                       partial noise when the plugin row alone is available. -->
                  <div v-if="toolDetails[t.id]" class="tool-item-meta">
                    <span v-if="toolDetails[t.id].author" class="meta-chip">
                      <el-icon><User /></el-icon>
                      {{ toolDetails[t.id].author }}
                    </span>
                    <a
                      v-if="toolDetails[t.id].tool_source_url"
                      class="meta-chip meta-link"
                      :href="toolDetails[t.id].tool_source_url!"
                      target="_blank"
                      rel="noopener"
                      @click.stop
                    >
                      <el-icon><Link /></el-icon>
                      {{ sourceLabel(toolDetails[t.id].tool_source) }}
                    </a>
                    <span v-else-if="toolDetails[t.id].tool_source" class="meta-chip">
                      <el-icon><Link /></el-icon>
                      {{ sourceLabel(toolDetails[t.id].tool_source) }}
                    </span>
                    <el-tag
                      v-if="!toolDetails[t.id].is_public"
                      type="warning"
                      size="small"
                      effect="light"
                    >Private</el-tag>
                    <el-tag
                      v-if="toolDetails[t.id].status === 'inactive'"
                      type="info"
                      size="small"
                      effect="light"
                    >Inactive</el-tag>
                    <el-tooltip
                      :content="`Created ${formatDate(toolDetails[t.id].created_at)}`"
                      placement="top"
                    >
                      <span class="meta-chip meta-date">
                        Updated {{ formatDate(toolDetails[t.id].updated_at) }}
                      </span>
                    </el-tooltip>
                  </div>

                  <!-- MCP servers -->
                  <div
                    v-if="t.type === 'mcp' && resolveToolConfig(t)?.mcpServers"
                    class="mcp-servers"
                  >
                    <div class="mcp-servers-title">MCP Servers</div>
                    <div
                      v-for="(srv, srvName) in resolveToolConfig(t)!.mcpServers"
                      :key="srvName"
                      class="mcp-server-item"
                    >
                      <code class="server-name">{{ srvName }}</code>
                      <code class="server-url">{{ (srv as any).url }}</code>
                    </div>
                  </div>

                  <!-- Skill artifact -->
                  <div
                    v-else-if="t.type === 'skill' && resolveToolConfig(t)?.s3_key"
                    class="skill-info"
                  >
                    <code>{{ resolveToolConfig(t)!.s3_key }}</code>
                  </div>
                </template>
              </div>
            </transition>
          </div>
        </div>
      </div>

      <!-- Resources -->
      <div v-if="detail.resources.length" class="section">
        <div class="section-title">Resources ({{ detail.resources.length }})</div>
        <div class="ref-tags">
          <el-tag
            v-for="r in detail.resources"
            :key="r.id"
            :type="r.type === 'gpu' ? 'success' : 'primary'"
            class="mr-2 mb-1"
            effect="light"
          >
            {{ r.name || r.type.toUpperCase() }} v{{ r.version }}
          </el-tag>
        </div>
      </div>

      <div class="time-info">
        <span>Created: {{ detail.created_at.split(' ')[0] }}</span>
        <span>Updated: {{ detail.updated_at.split(' ')[0] }}</span>
      </div>
    </div>

    <!-- Fetch failed. Keep the dialog open so the user can close it cleanly. -->
    <el-empty v-else-if="error" :description="error" />

    <template #footer>
      <div class="flex justify-between w-full">
        <el-button
          type="danger"
          plain
          :disabled="!detail"
          @click="handleDelete"
          :loading="deleting"
        >Delete</el-button>
        <div class="flex gap-2">
          <el-button @click="handleClose">Close</el-button>
          <el-button
            plain
            :disabled="!detail"
            @click="detail && (emit('edit', detail.id), handleClose())"
          >Edit</el-button>
          <el-button
            type="primary"
            :icon="VideoPlay"
            :disabled="!detail"
            @click="detail && (emit('run', detail), handleClose())"
          >Run</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, VideoPlay, User, Link } from '@element-plus/icons-vue'
import {
  deletePlugin,
  getPlugin,
  getTool,
  type Plugin,
  type PluginToolRef,
  type ToolDetail,
} from '@/services/tools'

const props = defineProps<{
  visible: boolean
  pluginId: number | null
}>()

const emit = defineEmits<{
  'update:visible': [val: boolean]
  edit: [id: number]
  deleted: []
  run: [plugin: Plugin]
}>()

const loading = ref(false)
const detail = ref<Plugin | null>(null)
const error = ref('')
const deleting = ref(false)

const expandedTools = ref(new Set<number>())
// Cache of detailed tool info keyed by tool id. Populated lazily the first
// time a tool row is expanded so the dialog doesn't N+1 up-front.
const toolDetails = ref<Record<number, ToolDetail>>({})
const toolLoading = ref<Record<number, boolean>>({})

// el-tag type union. `rule` falls back to `info` because Element Plus has
// no dedicated tag style for it.
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

// Prefer the detailed tool field when available, otherwise fall back to the
// lightweight data embedded in the plugin response.
const resolveToolField = (t: PluginToolRef, key: 'description') =>
  toolDetails.value[t.id]?.[key] ?? t[key]

const formatDate = (s?: string) => (s || '').split(' ')[0] || (s || '')

const sourceLabel = (src?: string) => {
  if (src === 'github') return 'GitHub'
  if (src === 'upload') return 'Upload'
  return src || 'Source'
}

const resolveToolConfig = (t: PluginToolRef): Record<string, any> | undefined =>
  toolDetails.value[t.id]?.config ?? (t as any).config

const resetState = () => {
  detail.value = null
  error.value = ''
  expandedTools.value = new Set()
  toolDetails.value = {}
  toolLoading.value = {}
}

const fetchDetail = async (id: number) => {
  loading.value = true
  error.value = ''
  try {
    detail.value = await getPlugin(id)
  } catch (e) {
    error.value = (e as Error).message || 'Failed to load plugin'
    detail.value = null
  } finally {
    loading.value = false
  }
}

const toggleToolExpand = async (t: PluginToolRef) => {
  const s = new Set(expandedTools.value)
  const wasOpen = s.has(t.id)
  if (wasOpen) s.delete(t.id)
  else s.add(t.id)
  expandedTools.value = s

  // Only fetch the richer tool detail on first expansion and only if we
  // don't already have it cached. Errors here shouldn't break the list, so
  // fall back silently to the lightweight data already on the plugin.
  if (!wasOpen && !toolDetails.value[t.id] && !toolLoading.value[t.id]) {
    toolLoading.value = { ...toolLoading.value, [t.id]: true }
    try {
      const full = await getTool(t.id)
      toolDetails.value = { ...toolDetails.value, [t.id]: full }
    } catch (e) {
      // Silent: user can still see the lightweight info; log for debugging.
      console.warn('[PluginDetailDialog] getTool failed', t.id, e)
    } finally {
      toolLoading.value = { ...toolLoading.value, [t.id]: false }
    }
  }
}

const handleDelete = async () => {
  if (!detail.value) return
  try {
    await ElMessageBox.confirm(
      `Delete plugin "${detail.value.name}"? This cannot be undone.`,
      'Delete Plugin',
      { confirmButtonText: 'Delete', type: 'warning' },
    )
    deleting.value = true
    await deletePlugin(detail.value.id)
    ElMessage.success('Plugin deleted')
    emit('deleted')
    handleClose()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error((e as Error).message || 'Delete failed')
  } finally {
    deleting.value = false
  }
}

const handleClose = () => {
  resetState()
  emit('update:visible', false)
}

// Kick off the fetch when the dialog opens with a valid id. Also refetch if
// the selected plugin changes while the dialog is open.
watch(
  () => [props.visible, props.pluginId] as const,
  ([vis, id]) => {
    if (vis && typeof id === 'number') {
      fetchDetail(id)
    } else if (!vis) {
      resetState()
    }
  },
  { immediate: true },
)
</script>

<style scoped lang="scss">
.detail-view {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.detail-loading {
  padding: 8px 4px;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;

  .info-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    .label { font-size: 12px; color: var(--el-text-color-secondary); font-weight: 500; }
    .value { font-size: 15px; font-weight: 600; color: var(--el-text-color-primary); }
  }
}

.section {
  .section-title { font-size: 14px; font-weight: 600; margin-bottom: 8px; }
  .desc { margin: 0; font-size: 14px; line-height: 1.6; color: var(--el-text-color-regular); }
}

.tool-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tool-item {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  overflow: hidden;

  .tool-item-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    cursor: pointer;
    &:hover { background: var(--el-fill-color-lighter); }
  }

  .tool-item-left {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .tool-item-name { font-weight: 500; font-size: 13px; }
  .tool-item-ver { font-size: 12px; color: var(--el-text-color-secondary); }

  .expand-icon {
    transition: transform 0.2s;
    color: var(--el-text-color-secondary);
  }

  &.expanded .expand-icon { transform: rotate(180deg); }

  .tool-item-body {
    padding: 0 12px 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .tool-item-desc {
    margin: 0;
    font-size: 12px;
    color: var(--el-text-color-regular);
    line-height: 1.5;
    white-space: pre-wrap;
  }

  .tool-item-tags {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .tool-item-display-name {
    font-size: 12px;
    font-style: italic;
    color: var(--el-text-color-secondary);
    margin-top: -2px;
  }

  .tool-item-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 12px;
    font-size: 12px;
    color: var(--el-text-color-secondary);

    .meta-chip {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      line-height: 1;

      .el-icon { font-size: 13px; }
    }

    .meta-link {
      color: var(--el-color-primary);
      text-decoration: none;

      &:hover { text-decoration: underline; }
    }

    .meta-date { color: var(--el-text-color-placeholder); }
  }

  .tool-loading {
    padding: 4px 0;
  }
}

.mcp-servers {
  .mcp-servers-title {
    font-size: 11px;
    font-weight: 600;
    color: var(--el-text-color-secondary);
    text-transform: uppercase;
    margin-bottom: 4px;
  }

  .mcp-server-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 4px 8px;
    background: var(--el-fill-color-lighter);
    border-radius: 4px;
    margin-bottom: 4px;

    .server-name { font-size: 12px; font-weight: 600; color: var(--el-text-color-primary); }
    .server-url { font-size: 11px; color: var(--el-text-color-secondary); word-break: break-all; }
  }
}

.skill-info code {
  font-size: 12px;
  padding: 4px 8px;
  background: var(--el-fill-color-lighter);
  border-radius: 4px;
  display: block;
  color: var(--el-text-color-secondary);
}

.time-info {
  display: flex;
  gap: 24px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.fade-slide-enter-active, .fade-slide-leave-active { transition: all 0.2s ease; }
.fade-slide-enter-from, .fade-slide-leave-to { opacity: 0; }
</style>

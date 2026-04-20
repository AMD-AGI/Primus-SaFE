<template>
  <el-dialog
    :model-value="visible"
    :title="plugin?.name || 'Plugin Details'"
    width="680px"
    @close="handleClose"
    :close-on-click-modal="false"
  >
    <div v-if="plugin" class="detail-view">
      <!-- Top info -->
      <div class="info-grid">
        <div class="info-item">
          <span class="label">Version</span>
          <span class="value">{{ plugin.version || '–' }}</span>
        </div>
        <div class="info-item">
          <span class="label">Author</span>
          <span class="value">{{ plugin.author || '–' }}</span>
        </div>
        <div class="info-item">
          <span class="label">Status</span>
          <el-tag size="small" :type="plugin.status === 'active' ? 'success' : 'warning'" effect="light">
            {{ plugin.status }}
          </el-tag>
        </div>
        <div class="info-item">
          <span class="label">Visibility</span>
          <el-tag size="small" :type="plugin.is_public ? 'primary' : 'warning'" effect="light">
            {{ plugin.is_public ? 'Public' : 'Private' }}
          </el-tag>
        </div>
      </div>

      <div v-if="plugin.description" class="section">
        <div class="section-title">Description</div>
        <p class="desc">{{ plugin.description }}</p>
      </div>

      <!-- Tools details -->
      <div v-if="plugin.tools.length" class="section">
        <div class="section-title">Tools ({{ plugin.tools.length }})</div>
        <div class="tool-list">
          <div
            v-for="t in plugin.tools"
            :key="t.id"
            class="tool-item"
            :class="{ expanded: expandedTools.has(t.id) }"
          >
            <div class="tool-item-header" @click="toggleToolExpand(t.id)">
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
                <p v-if="t.description" class="tool-item-desc">{{ t.description }}</p>
                <div v-if="t.config && t.type === 'mcp' && t.config.mcpServers" class="mcp-servers">
                  <div class="mcp-servers-title">MCP Servers</div>
                  <div v-for="(srv, srvName) in t.config.mcpServers" :key="srvName" class="mcp-server-item">
                    <code class="server-name">{{ srvName }}</code>
                    <code class="server-url">{{ (srv as any).url }}</code>
                  </div>
                </div>
                <div v-else-if="t.config && t.type === 'skill' && t.config.s3_key" class="skill-info">
                  <code>{{ t.config.s3_key }}</code>
                </div>
              </div>
            </transition>
          </div>
        </div>
      </div>

      <!-- Resources -->
      <div v-if="plugin.resources.length" class="section">
        <div class="section-title">Resources ({{ plugin.resources.length }})</div>
        <div class="ref-tags">
          <el-tag
            v-for="r in plugin.resources"
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
        <span>Created: {{ plugin.created_at.split(' ')[0] }}</span>
        <span>Updated: {{ plugin.updated_at.split(' ')[0] }}</span>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-between w-full">
        <el-button type="danger" plain @click="handleDelete" :loading="deleting">Delete</el-button>
        <div class="flex gap-2">
          <el-button @click="handleClose">Close</el-button>
          <el-button plain @click="emit('edit', plugin!.id); handleClose()">Edit</el-button>
          <el-button type="primary" :icon="VideoPlay" @click="emit('run', plugin!); handleClose()">
            Run
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, VideoPlay } from '@element-plus/icons-vue'
import { deletePlugin, type Plugin } from '@/services/tools'

const props = defineProps<{
  visible: boolean
  plugin: Plugin | null
}>()

const emit = defineEmits<{
  'update:visible': [val: boolean]
  edit: [id: number]
  deleted: []
  run: [plugin: Plugin]
}>()

const deleting = ref(false)
const expandedTools = ref(new Set<number>())

const toolTagType = (type: string) =>
  ({ skill: 'primary', mcp: 'success', hooks: 'warning', rule: '' } as Record<string, string>)[type] || ''

const toggleToolExpand = (id: number) => {
  const s = new Set(expandedTools.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  expandedTools.value = s
}

const handleDelete = async () => {
  if (!props.plugin) return
  try {
    await ElMessageBox.confirm(
      `Delete plugin "${props.plugin.name}"? This cannot be undone.`,
      'Delete Plugin',
      { confirmButtonText: 'Delete', type: 'warning' },
    )
    deleting.value = true
    await deletePlugin(props.plugin.id)
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
  expandedTools.value = new Set()
  emit('update:visible', false)
}
</script>

<style scoped lang="scss">
.detail-view {
  display: flex;
  flex-direction: column;
  gap: 20px;
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

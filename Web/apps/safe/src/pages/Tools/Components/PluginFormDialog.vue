<template>
  <el-dialog
    :model-value="visible"
    :title="isEdit ? 'Edit Plugin' : 'Create Plugin'"
    width="720px"
    @close="handleClose"
    :close-on-click-modal="false"
    destroy-on-close
    class="plugin-form-dialog"
  >
    <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
      <!-- Basic Info -->
      <div class="section-card">
        <div class="section-title">Basic Info</div>
        <el-form-item label="Name" prop="name">
          <el-input v-model="form.name" placeholder="e.g., my-plugin" :disabled="isEdit" />
        </el-form-item>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="Version">
              <el-input v-model="form.version" placeholder="1.0.0" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="Public">
              <el-switch v-model="form.is_public" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="Description">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="Plugin description" />
        </el-form-item>
      </div>

      <!-- Tools -->
      <div class="section-card">
        <div class="section-title">
          Tools
          <span class="section-hint">(at least one required)</span>
        </div>
        <div v-for="(t, i) in form.tools" :key="i" class="ref-row">
          <el-select
            v-model="t.id"
            filterable
            remote
            :remote-method="handleToolSearch"
            :loading="toolSearchLoading"
            placeholder="Search tools by name or tag..."
            style="flex: 1"
          >
            <el-option
              v-for="opt in toolOptions"
              :key="opt.id"
              :label="opt.name"
              :value="opt.id"
            >
              <div class="option-row">
                <div class="option-main">
                  <span class="option-name">{{ opt.name }}</span>
                  <span v-if="opt.tags?.length" class="option-tags">
                    <el-tag
                      v-for="tag in opt.tags.slice(0, 3)"
                      :key="tag"
                      size="small"
                      effect="plain"
                      round
                    >
                      {{ tag }}
                    </el-tag>
                    <span v-if="opt.tags.length > 3" class="option-tags-more">
                      +{{ opt.tags.length - 3 }}
                    </span>
                  </span>
                </div>
                <span class="option-meta">
                  <el-tag size="small" :type="toolTagType(opt.type)" effect="light">{{ opt.type }}</el-tag>
                  <span class="option-ver">v{{ opt.version }}</span>
                </span>
              </div>
            </el-option>
          </el-select>
          <el-button text type="danger" :icon="Delete" @click="form.tools.splice(i, 1)" />
        </div>
        <div class="add-btns">
          <div class="add-btn" @click="form.tools.push({ id: undefined })">
            <el-icon><Plus /></el-icon> Select Existing
          </div>
          <div class="add-btn" @click="openCreateTool">
            <el-icon><Plus /></el-icon> Create New
          </div>
        </div>
      </div>

      <!-- Resources -->
      <div class="section-card">
        <div class="section-title">
          Resources
          <span class="section-hint">(1–2 required, GPU + CPU recommended)</span>
        </div>
        <div v-for="(r, i) in form.resources" :key="i" class="ref-row">
          <el-tag
            v-if="selectedResourceType(r.id)"
            size="default"
            :type="selectedResourceType(r.id) === 'gpu' ? 'success' : 'primary'"
            effect="light"
            class="resource-type-tag"
          >
            {{ selectedResourceType(r.id)!.toUpperCase() }}
          </el-tag>
          <el-select
            v-model="r.id"
            filterable
            placeholder="Select a resource"
            style="flex: 1"
          >
            <el-option
              v-for="opt in availableResources"
              :key="opt.id"
              :label="opt.name"
              :value="opt.id"
            >
              <div class="option-row">
                <span>{{ opt.name }}</span>
                <span class="option-meta">
                  <el-tag size="small" :type="opt.type === 'gpu' ? 'success' : 'primary'" effect="light">{{ opt.type.toUpperCase() }}</el-tag>
                  <span class="option-ver">v{{ opt.version }}</span>
                </span>
              </div>
            </el-option>
          </el-select>
          <el-button
            text type="danger" :icon="Delete"
            @click="form.resources.splice(i, 1)"
          />
        </div>
        <div class="add-btn mt-2" @click="form.resources.push({ id: undefined })">
          <el-icon><Plus /></el-icon> Add Resource
        </div>
      </div>
    </el-form>

    <template #footer>
      <div class="flex justify-end gap-2">
        <el-button @click="handleClose">Cancel</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">
          {{ isEdit ? 'Save' : 'Create' }}
        </el-button>
      </div>
    </template>

    <!-- Nested: Create Tool (full AddDialog) -->
    <AddDialog
      v-model:visible="showAddTool"
      action="Create"
      @success="onToolCreated"
    />
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import { Plus, Delete } from '@element-plus/icons-vue'
import {
  upsertPlugin, updatePlugin, getPlugin,
  getTools, getResources,
  type Tool, type Resource,
} from '@/services/tools'
import AddDialog from './AddDialog.vue'

const props = defineProps<{
  visible: boolean
  pluginId?: number
}>()

const emit = defineEmits<{
  'update:visible': [val: boolean]
  success: []
  close: []
}>()

const isEdit = computed(() => !!props.pluginId)
const formRef = ref<FormInstance>()
const saving = ref(false)
const showAddTool = ref(false)

const allTools = ref<Tool[]>([])
const toolOptions = ref<Tool[]>([])
const toolSearchLoading = ref(false)
const availableResources = ref<Resource[]>([])

const form = reactive({
  name: '',
  description: '',
  version: '1.0.0',
  is_public: true,
  tools: [] as { id: number | undefined }[],
  resources: [] as { id: number | undefined }[],
})

const rules: FormRules = {
  name: [{ required: true, message: 'Name is required', trigger: 'blur' }],
}

const toolTagType = (type: string) =>
  ({ skill: 'primary', mcp: 'success', hooks: 'warning', rule: '' } as Record<string, string>)[type] || ''

const selectedResourceType = (id: number | undefined) => {
  if (id == null) return undefined
  return availableResources.value.find(r => r.id === id)?.type
}

const pickDefaultResources = () => {
  const firstGpu = availableResources.value.find(r => r.type === 'gpu')
  const firstCpu = availableResources.value.find(r => r.type === 'cpu')
  const defaults: { id: number | undefined }[] = []
  if (firstGpu) defaults.push({ id: firstGpu.id })
  if (firstCpu) defaults.push({ id: firstCpu.id })
  if (defaults.length === 0) defaults.push({ id: undefined })
  return defaults
}

const resetForm = () => {
  form.name = ''
  form.description = ''
  form.version = '1.0.0'
  form.is_public = true
  form.tools = []
  form.resources = pickDefaultResources()
}

const loadOptions = async () => {
  try {
    const [toolsRes, resourcesRes] = await Promise.all([
      getTools({ limit: 200, latest_per_name: true }),
      getResources({ limit: 200 }),
    ])
    allTools.value = toolsRes.tools || []
    toolOptions.value = allTools.value
    availableResources.value = resourcesRes.resources || []
  } catch { /* silent */ }
}

const handleToolSearch = async (query: string) => {
  const q = query.trim()
  if (!q) { toolOptions.value = allTools.value; return }
  toolSearchLoading.value = true
  try {
    // Search by name and tag in parallel, merge and dedupe
    const [byName, byTag] = await Promise.all([
      getTools({ limit: 100, name: q, latest_per_name: true }).catch(() => ({ tools: [] as Tool[] })),
      getTools({ limit: 100, tag: q, latest_per_name: true }).catch(() => ({ tools: [] as Tool[] })),
    ])
    const seen = new Set<number>()
    const merged: Tool[] = []
    for (const t of [...(byName.tools || []), ...(byTag.tools || [])]) {
      if (!seen.has(t.id)) { seen.add(t.id); merged.push(t) }
    }
    toolOptions.value = merged
  } catch {
    toolOptions.value = allTools.value
  } finally {
    toolSearchLoading.value = false
  }
}

const openCreateTool = () => { showAddTool.value = true }

const onToolCreated = async () => {
  const oldIds = new Set(allTools.value.map(t => t.id))
  await loadOptions()
  const newTool = allTools.value.find(t => !oldIds.has(t.id))
  if (!newTool) return

  const existingIdx = form.tools.findIndex(entry => {
    if (entry.id == null) return false
    const old = allTools.value.find(t => t.id === entry.id)
    return old && old.name === newTool.name
  })

  if (existingIdx >= 0) {
    form.tools[existingIdx].id = newTool.id
  } else {
    form.tools.push({ id: newTool.id })
  }
}

watch(() => props.visible, async (v) => {
  if (!v) return
  await loadOptions()
  if (props.pluginId) {
    try {
      saving.value = true
      const p = await getPlugin(props.pluginId)
      form.name = p.name
      form.description = p.description
      form.version = p.version
      form.is_public = p.is_public
      form.tools = (p.tools || []).map(t => ({ id: t.id }))
      form.resources = (p.resources || []).map(r => ({ id: r.id }))
      if (form.resources.length === 0) form.resources = pickDefaultResources()

      // Ensure plugin's tools are in the options list even if they're older versions
      for (const pt of (p.tools || [])) {
        if (pt.id && !allTools.value.some(t => t.id === pt.id)) {
          allTools.value.push({
            id: pt.id,
            type: pt.type,
            name: pt.name || `tool-${pt.id}`,
            version: pt.version,
            description: pt.description || '',
            display_name: '',
            tags: [],
            author: '',
            tool_source: 'upload',
            is_public: true,
            status: 'active',
            created_at: '',
            updated_at: '',
          } as Tool)
        }
      }
      toolOptions.value = allTools.value

      // Same for resources
      for (const pr of (p.resources || [])) {
        if (pr.id && !availableResources.value.some(r => r.id === pr.id)) {
          availableResources.value.push({
            id: pr.id,
            type: (pr.type as 'gpu' | 'cpu') || 'gpu',
            name: pr.name || `resource-${pr.id}`,
            version: pr.version,
            image: '',
            env: [],
            resources: {},
            timeout: 0,
            created_at: '',
            updated_at: '',
          } as Resource)
        }
      }
    } catch {
      ElMessage.error('Failed to load plugin')
      handleClose()
    } finally {
      saving.value = false
    }
  } else {
    resetForm()
  }
})

const handleSave = async () => {
  if (!formRef.value) return
  await formRef.value.validate()

  const selectedTools = form.tools.filter(t => t.id != null)
  if (selectedTools.length === 0) {
    ElMessage.warning('Add at least one tool')
    return
  }

  const selectedResources = form.resources.filter(r => r.id != null)
  if (selectedResources.length === 0) {
    ElMessage.warning('At least one resource is required')
    return
  }

  const resourceTypes = selectedResources.map(r => {
    const found = availableResources.value.find(o => o.id === r.id)
    return found?.type
  })
  const hasGpu = resourceTypes.includes('gpu')
  const hasCpu = resourceTypes.includes('cpu')

  if (selectedResources.length === 1 && !(hasGpu && hasCpu)) {
    const missing = hasGpu ? 'CPU' : 'GPU'
    try {
      await ElMessageBox.confirm(
        `Only a ${hasGpu ? 'GPU' : 'CPU'} resource is selected. No ${missing} resource. Continue?`,
        'Missing Resource', { confirmButtonText: 'Continue', cancelButtonText: 'Go Back', type: 'warning' },
      )
    } catch { return }
  }

  saving.value = true
  try {
    const toolRefs = selectedTools.map(t => {
      const found = allTools.value.find(o => o.id === t.id)
      return { id: t.id!, type: found?.type || 'mcp', version: found?.version || '1.0.0' }
    })

    const resourceRefs = selectedResources.map(r => {
      const found = availableResources.value.find(o => o.id === r.id)
      return { id: r.id!, type: found?.type || 'gpu', version: found?.version || '1.0.0' }
    })

    const payload = {
      name: form.name,
      description: form.description,
      version: form.version,
      tools: toolRefs,
      resources: resourceRefs,
      is_public: form.is_public,
    }

    if (isEdit.value && props.pluginId) {
      await updatePlugin(props.pluginId, payload)
      ElMessage.success('Plugin updated')
    } else {
      await upsertPlugin(payload)
      ElMessage.success('Plugin created')
    }
    emit('success')
    handleClose()
  } catch (e) {
    console.error('Save plugin failed:', e)
  } finally {
    saving.value = false
  }
}

const handleClose = () => {
  emit('update:visible', false)
  emit('close')
  setTimeout(resetForm, 300)
}
</script>

<style scoped lang="scss">
.section-card {
  margin-bottom: 20px;
  padding: 16px;
  border: 1px solid var(--safe-border, var(--el-border-color-lighter));
  border-radius: 8px;
  background: var(--safe-card-2, var(--el-fill-color-lighter));
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 12px;
  color: var(--safe-text, var(--el-text-color-primary));

  .section-hint {
    font-weight: 400;
    font-size: 12px;
    color: var(--safe-muted, var(--el-text-color-secondary));
    margin-left: 6px;
  }
}

.ref-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}

.add-btns {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.add-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  font-size: 13px;
  color: var(--safe-muted, var(--el-text-color-secondary));
  border: 1px dashed var(--safe-border, var(--el-border-color));
  border-radius: 6px;
  cursor: pointer;
  transition: color 0.2s, border-color 0.2s;
  width: fit-content;

  &:hover {
    color: var(--safe-primary, var(--el-color-primary));
    border-color: var(--safe-primary, var(--el-color-primary));
  }
}

.option-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  gap: 8px;
}
.option-main {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}
.option-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.option-tags {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-wrap: nowrap;
  overflow: hidden;
}
.option-tags-more {
  font-size: 11px;
  color: var(--safe-muted, var(--el-text-color-secondary));
}
.option-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}
.option-ver {
  font-size: 12px;
  color: var(--safe-muted, var(--el-text-color-secondary));
}

.resource-type-tag {
  min-width: 48px;
  justify-content: center;
  text-align: center;
  font-weight: 600;
  letter-spacing: 0.3px;
}
</style>

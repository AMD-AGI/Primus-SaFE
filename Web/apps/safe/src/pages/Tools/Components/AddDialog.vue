<template>
  <el-dialog
    :model-value="visible"
    :title="dialogTitle"
    width="700px"
    @close="handleClose"
    @open="onOpen"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <!-- Type selector (shown only in Create mode) -->
    <div v-if="!isEditMode" class="type-selector">
      <el-segmented v-model="toolType" :options="typeOptions" block />
    </div>

    <!-- MCP form -->
    <el-form
      v-if="toolType === 'mcp'"
      ref="mcpFormRef"
      :model="mcpForm"
      :rules="mcpRules"
      label-width="120px"
      class="mt-4"
    >
      <el-form-item label="Name" prop="name" required>
        <el-input v-model="mcpForm.name" placeholder="e.g., howtocook-mcp" :disabled="isEditMode" />
      </el-form-item>

      <el-form-item label="Description" prop="description" required>
        <el-input
          v-model="mcpForm.description"
          type="textarea"
          :rows="3"
          placeholder="Tool description"
        />
      </el-form-item>

      <el-form-item label="Tags" prop="tags">
        <div class="flex gap-2 flex-wrap items-center">
          <el-tag
            v-for="tag in mcpForm.tags"
            :key="tag"
            closable
            :disable-transitions="false"
            :effect="isDark ? 'plain' : 'light'"
            @close="removeTag(tag)"
          >
            {{ tag }}
          </el-tag>
          <el-input
            v-if="tagInputVisible"
            ref="tagInputRef"
            v-model="newTag"
            class="tag-input-el"
            size="small"
            @keyup.enter="handleTagInputConfirm"
            @blur="handleTagInputConfirm"
          />
          <el-button v-else class="button-new-tag" size="small" @click="showTagInput">
            + New Tag
          </el-button>
        </div>
      </el-form-item>

      <el-form-item label="Icon" prop="icon_url">
        <div class="icon-upload-area">
          <div v-if="mcpForm.icon_url" class="icon-preview">
            <img :src="mcpForm.icon_url" alt="Icon" />
            <el-button
              size="small"
              text
              type="danger"
              @click="mcpForm.icon_url = ''"
            >
              Remove
            </el-button>
          </div>
          <el-upload
            v-else
            :auto-upload="false"
            :show-file-list="false"
            :on-change="handleIconChange"
            accept="image/png,image/jpeg,image/jpg,image/svg+xml,image/webp"
            drag
          >
            <el-icon class="avatar-uploader-icon"><UploadFilled /></el-icon>
            <div class="el-upload__text">
              Drop icon here or <em>click to upload</em>
            </div>
            <template #tip>
              <div class="el-upload__tip">
                PNG, JPG, SVG, WEBP (max 2MB)
              </div>
            </template>
          </el-upload>
        </div>
      </el-form-item>

      <el-form-item label="" prop="configJson" required>
        <JsonEditor
          v-model="mcpForm.configJson"
          label="Config (JSON)"
          placeholder='{\n  "mcpServers": {\n    "server-name": {\n      "command": "...",\n      "args": [...],\n      "env": {...}\n    }\n  }\n}'
          @validate="handleJsonValidate"
        />
      </el-form-item>

      <el-form-item label="Public" prop="is_public">
        <el-switch v-model="mcpForm.is_public" />
      </el-form-item>

    </el-form>

    <!-- Skill metadata edit form (Edit mode only) -->
    <el-form
      v-if="toolType === 'skill' && isEditMode"
      ref="mcpFormRef"
      :model="mcpForm"
      :rules="skillMetaRules"
      label-width="120px"
      class="mt-4"
    >
      <el-form-item label="Name" prop="name">
        <el-input v-model="mcpForm.name" disabled />
      </el-form-item>

      <el-form-item label="Description" prop="description" required>
        <el-input
          v-model="mcpForm.description"
          type="textarea"
          :rows="3"
          placeholder="Tool description"
        />
      </el-form-item>

      <el-form-item label="Tags" prop="tags">
        <div class="flex gap-2 flex-wrap items-center">
          <el-tag
            v-for="tag in mcpForm.tags"
            :key="tag"
            closable
            :disable-transitions="false"
            :effect="isDark ? 'plain' : 'light'"
            @close="removeTag(tag)"
          >
            {{ tag }}
          </el-tag>
          <el-input
            v-if="tagInputVisible"
            ref="tagInputRef"
            v-model="newTag"
            class="tag-input-el"
            size="small"
            @keyup.enter="handleTagInputConfirm"
            @blur="handleTagInputConfirm"
          />
          <el-button v-else class="button-new-tag" size="small" @click="showTagInput">
            + New Tag
          </el-button>
        </div>
      </el-form-item>

      <el-form-item label="Icon" prop="icon_url">
        <div class="icon-upload-area">
          <div v-if="mcpForm.icon_url" class="icon-preview">
            <img :src="mcpForm.icon_url" alt="Icon" />
            <el-button
              size="small"
              text
              type="danger"
              @click="mcpForm.icon_url = ''"
            >
              Remove
            </el-button>
          </div>
          <div v-else class="skill-avatar-preview">
            <LetterAvatar :name="mcpForm.name || 'Skill'" :size="64" />
            <div class="upload-hint">
              <el-upload
                :auto-upload="false"
                :show-file-list="false"
                :on-change="handleIconChange"
                accept="image/png,image/jpeg,image/jpg,image/svg+xml,image/webp"
              >
                <el-button size="small">Upload Custom Icon</el-button>
              </el-upload>
              <div class="hint-text">Or use auto-generated avatar</div>
            </div>
          </div>
        </div>
      </el-form-item>

      <el-form-item label="Public" prop="is_public">
        <el-switch v-model="mcpForm.is_public" />
      </el-form-item>
    </el-form>

    <!-- Skills import form (Create mode only) -->
    <div v-if="toolType === 'skill' && !isEditMode" class="skills-section">
      <!-- No candidates found: upload interface -->
      <div v-if="!discoverResult">
        <div class="import-method-selector">
          <div class="selector-label">Import Method</div>
          <el-segmented v-model="importMethod" :options="importMethodOptions" block />
        </div>

        <!-- File upload -->
        <div v-if="importMethod === 'file'" class="upload-area">
          <el-upload
            ref="uploadRef"
            drag
            :auto-upload="false"
            :on-change="handleFileChange"
            :limit="1"
            accept=".md,.markdown,.zip"
          >
            <el-icon class="el-icon--upload"><UploadFilled /></el-icon>
            <div class="el-upload__text">
              Drop file here or <em>click to upload</em>
            </div>
            <template #tip>
              <div class="el-upload__tip">
                Support .md/.markdown (single skill) or .zip (multiple skills)
              </div>
            </template>
          </el-upload>
        </div>

        <!-- GitHub URL -->
        <div v-else class="github-input">
          <el-input
            v-model="githubUrl"
            placeholder="https://github.com/owner/repo"
            size="large"
          >
            <template #prepend>
              <el-icon><svg viewBox="0 0 24 24" fill="currentColor">
                <path d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.865 8.17 6.839 9.49.5.092.682-.217.682-.482 0-.237-.008-.866-.013-1.7-2.782.603-3.369-1.34-3.369-1.34-.454-1.156-1.11-1.463-1.11-1.463-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.092-.646.35-1.086.636-1.336-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.167 22 16.418 22 12c0-5.523-4.477-10-10-10z"/>
              </svg></el-icon>
            </template>
          </el-input>
        </div>
      </div>

      <!-- Candidates found: selection list -->
      <div v-else class="candidates-list">
        <div class="flex justify-between items-center mb-3">
          <div class="flex items-center gap-4">
            <span class="text-sm text-gray-600">
              Found {{ allCandidates.length }} skill(s)
            </span>
            <el-divider direction="vertical" />
            <span class="text-sm font-600" :class="selectedCount > 0 ? 'text-primary' : 'text-gray-500'">
              Selected: {{ selectedCount }}
            </span>
          </div>
          <el-button size="small" @click="resetDiscover" text>
            <el-icon><RefreshLeft /></el-icon>
            Start Over
          </el-button>
        </div>

        <div class="candidates-wrapper">
          <div
            v-for="(candidate, index) in paginatedCandidates"
            :key="index"
            class="candidate-item"
            :class="{
              selected: candidate.selected,
              disabled: isCandidateDisabled(candidate)
            }"
            @click="toggleCandidate(candidate)"
          >
            <div class="candidate-header">
              <div class="left-section">
                <el-checkbox
                  v-model="candidate.selected"
                  :disabled="isCandidateDisabled(candidate)"
                  @click.stop
                />
                <div class="candidate-info">
                  <div class="candidate-title">
                    <el-input
                      v-if="candidate.requires_name"
                      v-model="candidate.name_override"
                      placeholder="Enter skill name"
                      size="small"
                      :disabled="isCandidateDisabled(candidate)"
                      @click.stop
                    />
                    <span v-else class="skill-name">{{ candidate.skill_name }}</span>
                  </div>
                </div>
              </div>
              <el-tag
                v-if="isCandidateDisabled(candidate)"
                type="danger"
                size="small"
                effect="plain"
              >
                Cannot Overwrite
              </el-tag>
              <el-tag
                v-else-if="candidate.will_overwrite"
                type="warning"
                size="small"
                effect="plain"
              >
                Overwrite
              </el-tag>
              <el-tag v-else type="success" size="small" effect="plain">
                New
              </el-tag>
            </div>

            <div v-if="candidate.skill_description" class="candidate-description">
              {{ candidate.skill_description }}
            </div>

            <div class="candidate-path">
              <span class="path-label">Path:</span>
              <span class="path-value">{{ candidate.relative_path }}</span>
            </div>
          </div>
        </div>

        <!-- Frontend paginator -->
        <el-pagination
          v-if="allCandidates.length > candidatePagination.pageSize"
          v-model:current-page="candidatePagination.currentPage"
          v-model:page-size="candidatePagination.pageSize"
          :page-sizes="[10, 20, 50]"
          :total="allCandidates.length"
          layout="total, sizes, prev, pager, next"
          class="mt-4"
          small
        />
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleClose">Cancel</el-button>
        <el-button
          v-if="toolType === 'mcp' || (toolType === 'skill' && isEditMode)"
          type="primary"
          @click="handleCreateMCP"
          :loading="loading"
        >
          {{ isEditMode ? 'Save' : 'Create' }}
        </el-button>
        <el-button
          v-else-if="toolType === 'skill' && !discoverResult"
          type="primary"
          @click="handleDiscover"
          :loading="loading"
        >
          Next
        </el-button>
        <el-button
          v-else
          type="primary"
          @click="handleCommit"
          :loading="loading"
        >
          Confirm Import
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, nextTick, computed } from 'vue'
import { ElMessage, type FormInstance, type FormRules, type UploadInstance, type UploadFile, type InputInstance } from 'element-plus'
import { UploadFilled, RefreshLeft } from '@element-plus/icons-vue'
import { useDark } from '@vueuse/core'
import { createMCP, updateMCP, discoverSkills, commitSkills, uploadIcon, getTool, type SkillCandidate } from '@/services/tools'
import { useUserStore } from '@/stores/user'
import JsonEditor from '@/components/Base/JsonEditor.vue'
import LetterAvatar from '@/components/Base/LetterAvatar.vue'

const isDark = useDark()
const userStore = useUserStore()

interface CloneData {
  name: string
  description: string
  tags: string[]
  is_public: boolean
  config: Record<string, unknown>
}

const props = defineProps<{
  visible: boolean
  action?: 'Create' | 'Edit'
  toolId?: number
  cloneData?: CloneData | null
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
  success: []
}>()

// Whether in edit mode
const isEditMode = computed(() => props.action === 'Edit')

// Dialog title
const dialogTitle = computed(() => {
  if (isEditMode.value) {
    return 'Edit Tool'
  }
  return 'Add Tool'
})

// Extend candidate type to support user input and selection state
interface ExtendedSkillCandidate extends SkillCandidate {
  name_override?: string
  selected?: boolean
}

// Tool type
const toolType = ref<'mcp' | 'skill'>('mcp')
const typeOptions = [
  { label: 'MCP Server', value: 'mcp' },
  { label: 'Skills', value: 'skill' },
]

// MCP form
const mcpFormRef = ref<FormInstance>()
const mcpForm = reactive({
  name: '',
  description: '',
  tags: [] as string[],
  is_public: true,
  icon_url: '',
  configJson: '{\n  "mcpServers": {\n    \n  }\n}',
})

// Icon upload
const iconUploading = ref(false)
const iconFile = ref<File | null>(null)

// Tags input related
const tagInputVisible = ref(false)
const newTag = ref('')
const tagInputRef = ref<InputInstance>()

const showTagInput = () => {
  tagInputVisible.value = true
  nextTick(() => {
    tagInputRef.value?.focus()
  })
}

const handleTagInputConfirm = () => {
  const tag = newTag.value.trim()
  if (tag && !mcpForm.tags.includes(tag)) {
    mcpForm.tags.push(tag)
  }
  tagInputVisible.value = false
  newTag.value = ''
}

const removeTag = (tag: string) => {
  const index = mcpForm.tags.indexOf(tag)
  if (index > -1) {
    mcpForm.tags.splice(index, 1)
  }
}

// Handle icon upload
const handleIconChange = async (uploadFile: UploadFile) => {
  const file = uploadFile.raw
  if (!file) return

  // Validate file size
  if (file.size > 2 * 1024 * 1024) {
    ElMessage.error('Icon size must be less than 2MB')
    return
  }

  // Validate file type
  const validTypes = ['image/png', 'image/jpeg', 'image/jpg', 'image/svg+xml', 'image/webp']
  if (!validTypes.includes(file.type)) {
    ElMessage.error('Icon must be PNG, JPG, SVG or WEBP format')
    return
  }

  try {
    iconUploading.value = true
    const result = await uploadIcon(file)
    mcpForm.icon_url = result.icon_url
    ElMessage.success('Icon uploaded successfully')
  } catch (error) {
    console.error('Upload icon failed:', error)
  } finally {
    iconUploading.value = false
  }
}

const jsonError = ref('')

// JSON editor validation callback
const handleJsonValidate = (isValid: boolean, error?: string) => {
  if (isValid) {
    jsonError.value = ''
  } else {
    jsonError.value = error || 'Invalid JSON format'
  }
}

const mcpRules = computed<FormRules>(() => ({
  name: [{ required: !isEditMode.value, message: 'Please input name', trigger: 'blur' }],
  description: [{ required: true, message: 'Please input description', trigger: 'blur' }],
  configJson: [
    { required: true, message: 'Please input config JSON', trigger: 'blur' },
    {
      validator: (_, value, callback) => {
        if (jsonError.value) {
          callback(new Error(jsonError.value))
        } else {
          try {
            JSON.parse(value)
            callback()
          } catch {
            callback(new Error('Invalid JSON format'))
          }
        }
      },
      trigger: 'blur',
    },
  ],
}))

// Skill metadata edit rules
const skillMetaRules: FormRules = {
  description: [{ required: true, message: 'Please input description', trigger: 'blur' }],
}

// Skills import
const importMethod = ref<'file' | 'github'>('file')
const importMethodOptions = [
  { label: 'Upload File', value: 'file' },
  { label: 'GitHub URL', value: 'github' },
]

const uploadRef = ref<UploadInstance>()
const uploadedFile = ref<File | null>(null)
const githubUrl = ref('')
const discoverResult = ref<{ archive_key: string; candidates: ExtendedSkillCandidate[] } | null>(null)
const allCandidates = ref<ExtendedSkillCandidate[]>([])

// Frontend pagination
const candidatePagination = reactive({
  currentPage: 1,
  pageSize: 10,
})

// Calculate candidates for current page
const paginatedCandidates = computed(() => {
  const start = (candidatePagination.currentPage - 1) * candidatePagination.pageSize
  const end = start + candidatePagination.pageSize
  return allCandidates.value.slice(start, end)
})

// Calculate selected count
const selectedCount = computed(() => {
  return allCandidates.value.filter(c => c.selected).length
})

const handleFileChange = (file: UploadFile) => {
  uploadedFile.value = file.raw as File
}

const resetDiscover = () => {
  discoverResult.value = null
  allCandidates.value = []
  uploadedFile.value = null
  githubUrl.value = ''
  newTag.value = ''
  tagInputVisible.value = false
  candidatePagination.currentPage = 1
  candidatePagination.pageSize = 10
  uploadRef.value?.clearFiles()
}

// Loading state
const loading = ref(false)

// Save tool (MCP create/edit or skill metadata edit)
const handleCreateMCP = async () => {
  if (!mcpFormRef.value) return

  try {
    await mcpFormRef.value.validate()
    loading.value = true

    if (toolType.value === 'mcp') {
      // MCP create or edit
      const config = JSON.parse(mcpForm.configJson)

      const data = {
        name: mcpForm.name,
        description: mcpForm.description,
        config,
        tags: mcpForm.tags.length > 0 ? mcpForm.tags : undefined,
        is_public: mcpForm.is_public,
        icon_url: mcpForm.icon_url || undefined,
      }

      if (isEditMode.value && props.toolId) {
        // Edit mode
        await updateMCP(props.toolId, data)
        ElMessage.success('Tool updated successfully')
      } else {
        // Create mode
        await createMCP(data)
        ElMessage.success('MCP created successfully')
      }
    } else if (toolType.value === 'skill' && isEditMode.value && props.toolId) {
      // Skill metadata edit
      await updateMCP(props.toolId, {
        description: mcpForm.description,
        tags: mcpForm.tags.length > 0 ? mcpForm.tags : undefined,
        is_public: mcpForm.is_public,
        icon_url: mcpForm.icon_url || undefined,
      })
      ElMessage.success('Skill metadata updated successfully')
    }

    emit('success')
    handleClose()
  } catch (error) {
    console.error('Save failed:', error)
  } finally {
    loading.value = false
  }
}

// Toggle candidate selection state
// Check if candidate is disabled (regular users cannot overwrite others' skills)
const isCandidateDisabled = (candidate: ExtendedSkillCandidate) => {
  // Managers can bypass this restriction
  if (userStore.isManager) return false
  // Regular users: disabled if owned_by_other is true
  return candidate.owned_by_other === true
}

const toggleCandidate = (candidate: ExtendedSkillCandidate) => {
  // If disabled, do not allow toggling
  if (isCandidateDisabled(candidate)) return
  candidate.selected = !candidate.selected
}

// Discover Skills
const handleDiscover = async () => {
  const formData = new FormData()

  if (importMethod.value === 'file') {
    if (!uploadedFile.value) {
      ElMessage.warning('Please upload a file')
      return
    }
    formData.append('file', uploadedFile.value)
  } else {
    if (!githubUrl.value) {
      ElMessage.warning('Please input GitHub URL')
      return
    }
    formData.append('github_url', githubUrl.value)
  }

  try {
    loading.value = true
    const result = await discoverSkills(formData)

    // Save all candidates (initialized as selected)
    allCandidates.value = result.candidates.map(c => ({ ...c, selected: true }))

    discoverResult.value = {
      archive_key: result.archive_key,
      candidates: allCandidates.value
    }

    // Reset pagination to first page
    candidatePagination.currentPage = 1

    if (result.candidates.length === 0) {
      ElMessage.warning('No skills found in the file/repository')
    }
  } catch (error) {
    console.error('Discover skills failed:', error)
  } finally {
    loading.value = false
  }
}

// Commit Skills
const handleCommit = async () => {
  if (!discoverResult.value) return

  // Filter selected from all candidates
  const selectedCandidates = allCandidates.value.filter(c => c.selected)

  if (selectedCandidates.length === 0) {
    ElMessage.warning('Please select at least one skill to import')
    return
  }

  // Validate all candidates requiring a name have been filled in
  const invalidCandidates = selectedCandidates.filter(
    c => c.requires_name && !c.name_override
  )

  if (invalidCandidates.length > 0) {
    ElMessage.warning('Please provide names for all selected skills that require it')
    return
  }

  try {
    loading.value = true

    const selections = selectedCandidates.map(c => ({
      relative_path: c.relative_path,
      name_override: c.name_override,
    }))

    const result = await commitSkills({
      archive_key: discoverResult.value.archive_key,
      selections,
    })

    const successCount = result.items.filter(i => i.status === 'success').length
    const failCount = result.items.filter(i => i.status === 'failed').length

    if (failCount === 0) {
      ElMessage.success(`Successfully imported ${successCount} skill(s)`)
    } else {
      ElMessage.warning(`Imported ${successCount} skill(s), ${failCount} failed`)
    }

    emit('success')
    handleClose()
  } catch (error) {
    console.error('Commit skills failed:', error)
  } finally {
    loading.value = false
  }
}

// Reset form data
const resetForm = () => {
  mcpFormRef.value?.resetFields()
  mcpForm.name = ''
  mcpForm.description = ''
  mcpForm.tags = []
  mcpForm.is_public = true
  mcpForm.icon_url = ''
  mcpForm.configJson = '{\n  "mcpServers": {\n    \n  }\n}'
  toolType.value = 'mcp'
  newTag.value = ''
  tagInputVisible.value = false
  iconUploading.value = false
  iconFile.value = null
  resetDiscover()
}

// Initialize on dialog open
const onOpen = async () => {
  // Edit mode: load tool details
  if (isEditMode.value && props.toolId) {
    try {
      loading.value = true
      const toolDetail = await getTool(props.toolId)

      toolType.value = toolDetail.type

      if (toolDetail.type === 'mcp') {
        mcpForm.name = toolDetail.name
        mcpForm.description = toolDetail.description
        mcpForm.tags = [...(toolDetail.tags || [])]
        mcpForm.is_public = toolDetail.is_public
        mcpForm.icon_url = toolDetail.icon_url || ''
        mcpForm.configJson = JSON.stringify(toolDetail.config || {}, null, 2)
      } else if (toolDetail.type === 'skill') {
        // Skill edit mode - can only edit metadata
        mcpForm.name = toolDetail.name
        mcpForm.description = toolDetail.description
        mcpForm.tags = [...(toolDetail.tags || [])]
        mcpForm.is_public = toolDetail.is_public
        mcpForm.icon_url = toolDetail.icon_url || ''
      }
    } catch (error) {
      console.error('Load tool failed:', error)
      ElMessage.error('Failed to load tool details')
      handleClose()
    } finally {
      loading.value = false
    }
  }
  // Clone mode: fill clone data
  else if (props.cloneData && typeof props.cloneData === 'object') {
    // Set tool type to MCP
    toolType.value = 'mcp'

    // Fill form data
    mcpForm.name = props.cloneData.name + '_copy'
    mcpForm.description = props.cloneData.description
    mcpForm.tags = [...props.cloneData.tags]
    mcpForm.is_public = props.cloneData.is_public
    mcpForm.icon_url = ''
    mcpForm.configJson = JSON.stringify(props.cloneData.config, null, 2)
  } else {
    // Create mode: reset form
    resetForm()
  }
}

const handleClose = () => {
  emit('update:visible', false)
  // Reset form
  setTimeout(() => {
    resetForm()
  }, 300)
}

defineOptions({
  name: 'AddToolDialog',
})
</script>

<style scoped lang="scss">
.type-selector {
  margin-bottom: 24px;
}

.skills-section {
  margin-top: 24px;
}

.import-method-selector {
  margin-bottom: 24px;

  .selector-label {
    font-size: 14px;
    font-weight: 500;
    color: var(--el-text-color-primary);
    margin-bottom: 12px;
  }
}

.tag-input-el {
  width: 120px;
}

.button-new-tag {
  height: 24px;
}

.icon-upload-area {
  width: 100%;

  .icon-preview {
    display: flex;
    align-items: center;
    gap: 12px;

    img {
      width: 64px;
      height: 64px;
      border-radius: 6px;
      object-fit: cover;
      border: 1px solid var(--el-border-color);
    }
  }

  .skill-avatar-preview {
    display: flex;
    align-items: center;
    gap: 16px;

    .upload-hint {
      display: flex;
      flex-direction: column;
      gap: 8px;

      .hint-text {
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }
  }

  :deep(.el-upload) {
    width: 100%;

    .el-upload-dragger {
      padding: 20px;
      width: 100%;
    }
  }

  .avatar-uploader-icon {
    font-size: 28px;
    color: var(--el-text-color-secondary);
    margin-bottom: 8px;
  }
}

.upload-area {
  margin: 20px 0;
}

.github-input {
  margin: 20px 0;
}

.candidates-list {
  margin-top: 20px;

  .candidates-wrapper {
    max-height: 500px;
    overflow-y: auto;

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

    .candidate-item {
      padding: 16px;
      margin-bottom: 12px;
      border: 1px solid var(--el-border-color-light);
      border-radius: 8px;
      background: var(--el-bg-color);
      cursor: pointer;
      transition: all 0.2s;

      &:last-child {
        margin-bottom: 0;
      }

      &:hover {
        border-color: var(--el-border-color);
        background: var(--el-fill-color-lighter);
      }

      &.selected {
        border-color: var(--el-color-primary);
        background: var(--el-color-primary-light-9);
      }

      &.disabled {
        opacity: 0.6;
        cursor: not-allowed;
        background: var(--el-fill-color-lighter);

        &:hover {
          border-color: var(--el-border-color-light);
          background: var(--el-fill-color-lighter);
        }

        .skill-name {
          color: var(--el-text-color-disabled);
        }
      }

      .candidate-header {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        margin-bottom: 8px;

        .left-section {
          display: flex;
          align-items: flex-start;
          gap: 12px;
          flex: 1;
          min-width: 0;

          .candidate-info {
            flex: 1;
            min-width: 0;

            .candidate-title {
              .skill-name {
                font-size: 15px;
                font-weight: 600;
                color: var(--el-text-color-primary);
              }

              :deep(.el-input) {
                width: 100%;
              }
            }
          }
        }
      }

      .candidate-description {
        font-size: 13px;
        color: var(--el-text-color-regular);
        line-height: 1.5;
        margin-bottom: 8px;
        padding-left: 32px;
      }

      .candidate-path {
        font-size: 12px;
        color: var(--el-text-color-secondary);
        padding-left: 32px;
        word-break: break-all;

        .path-label {
          font-weight: 500;
          margin-right: 4px;
        }

        .path-value {
          font-family: 'Monaco', 'Menlo', 'Consolas', monospace;
        }
      }
    }
  }
}

// Dark mode optimization
.dark {
  .candidates-wrapper {
    .candidate-item {
      background: var(--el-bg-color-overlay);
      border-color: var(--el-border-color);

      &:hover {
        background: var(--el-fill-color);
        border-color: var(--el-border-color-darker);
      }

      &.selected {
        background: rgba(64, 158, 255, 0.15);
        border-color: var(--el-color-primary);
      }
    }
  }
}
</style>

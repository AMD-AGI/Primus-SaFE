<template>
  <el-dialog
    v-model="dialogVisible"
    :title="mode === 'create' ? 'New Answer' : 'Edit Answer'"
    width="850px"
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <el-form
      ref="formRef"
      :model="formData"
      :rules="formRules"
      label-width="auto"
      class="qa-edit-form"
    >
      <el-form-item label="Answer" prop="answer">
        <div class="w-full">
          <div ref="vditorRef" class="vditor-host" :class="{ 'vditor--dark': isDark }"></div>
        </div>
      </el-form-item>

      <!-- Questions: up to 10 variants (primary = first) -->
      <el-form-item label="Questions" prop="questions">
        <div class="w-full" v-loading="generatingQuestions">
          <div
            v-for="(q, idx) in formData.questions"
            :key="idx"
            class="question-row"
            @dragover.prevent
            @drop="onQDrop(idx)"
          >
            <div class="left">
              <div class="drag-zone" draggable="true" @dragstart="onQDragStart(idx)">
                <div class="drag-handle">::</div>
                <el-tag v-if="idx === 0" type="success" effect="plain">Primary</el-tag>
                <el-tag v-else type="info" effect="plain">Variant</el-tag>
              </div>
              <el-input
                v-model="formData.questions[idx].question"
                placeholder="Enter question"
                maxlength="500"
                show-word-limit
                class="flex-1"
              />
            </div>
            <div class="right">
              <template v-if="idx === 0">
                <el-button
                  size="small"
                  type="primary"
                  :loading="generatingQuestions"
                  :disabled="!canGenerateQuestions"
                  @click="handleGenerateQuestions"
                >
                  Generate
                </el-button>
              </template>
              <template v-else>
                <el-tooltip content="Remove" placement="top">
                  <el-button
                    type="danger"
                    plain
                    size="small"
                    :disabled="formData.questions.length <= 1"
                    @click="removeQuestion(idx)"
                  >
                    Remove
                  </el-button>
                </el-tooltip>
              </template>
            </div>
          </div>

          <div class="mt-2 flex items-center gap-2">
            <el-button
              size="small"
              :disabled="formData.questions.length >= MAX_QUESTION_VARIANTS"
              @click="addQuestion"
            >
              Add question
            </el-button>
            <span class="text-gray-500 text-sm">
              {{ formData.questions.length }}/{{ MAX_QUESTION_VARIANTS }}
            </span>
          </div>
        </div>
      </el-form-item>

      <el-form-item label="Priority" prop="priority">
        <el-select v-model="formData.priority" placeholder="Select priority">
          <el-option label="Low" value="low" />
          <el-option label="Medium" value="medium" />
          <el-option label="High" value="high" />
        </el-select>
      </el-form-item>
      <el-form-item label="Status" prop="is_active">
        <el-switch v-model="formData.is_active" active-text="Active" inactive-text="Inactive" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">
        {{ mode === 'create' ? 'Create' : 'Save' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useDark } from '@vueuse/core'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage, ElMessageBox } from 'element-plus'
import Vditor from 'vditor'
import 'vditor/dist/index.css'
import {
  createQAItem,
  generateQAQuestions,
  updateQAItem,
  uploadQAFile,
  uploadQAImage,
} from '@/services/chatbot'
import {
  FILE_EXTENSIONS,
  IMAGE_MIME_TYPES,
  MAX_FILE_SIZE,
  MAX_IMAGE_SIZE,
  GENERATE_QUESTION_COUNT,
  MAX_QUESTION_VARIANTS,
  MIN_ANSWER_LENGTH,
} from './qaEditDialogConstants'

interface Props {
  modelValue: boolean
  mode: 'create' | 'edit'
  collectionId?: number
  itemData?: {
    id: number
    questions: Array<{ id?: number; question: string }>
    answer: string
    priority: 'low' | 'medium' | 'high'
    is_active: boolean
  } | null
}

interface Emits {
  (e: 'update:modelValue', value: boolean): void
  (e: 'success'): void
}

const props = withDefaults(defineProps<Props>(), {
  mode: 'create',
  collectionId: undefined,
  itemData: null,
})

const emit = defineEmits<Emits>()

const dialogVisible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const isDark = useDark()
const editorTheme = computed(() => (isDark.value ? 'dark' : 'classic'))

const formRef = ref<FormInstance>()
const submitting = ref(false)
const generatingQuestions = ref(false)
const generateQuestionCount = GENERATE_QUESTION_COUNT
const canGenerateQuestions = computed(() => {
  const primary = (formData.value.questions?.[0]?.question ?? '').trim()
  return primary.length > 0
})

const formData = ref({
  // create mode
  questions: [{ question: '' }] as Array<{ id?: number; question: string }>,
  answer_markdown: '',
  priority: 'medium' as 'low' | 'medium' | 'high',
  is_active: true,
})

const formRules: FormRules = {
  questions: [
    {
      validator: (_rule, value: Array<{ id?: number; question?: string }>, callback) => {
        const arr = Array.isArray(value) ? value : []
        const cleaned = arr.map((item) => (item?.question ?? '').trim()).filter(Boolean)
        if (cleaned.length === 0) return callback(new Error('Please enter at least 1 question'))
        if (cleaned.length > MAX_QUESTION_VARIANTS)
          return callback(new Error(`Up to ${MAX_QUESTION_VARIANTS} questions`))
        return callback()
      },
      trigger: 'blur',
    },
  ],
  answer: [
    {
      validator: (_rule, _value, callback) => {
        const payload = getAnswerPayload()
        if (!payload.trim()) return callback(new Error('Please enter answer'))
        return callback()
      },
      trigger: 'blur',
    },
  ],
  priority: [{ required: true, message: 'Please select priority', trigger: 'change' }],
}

const vditorRef = ref<HTMLElement | null>(null)
const vditorInstance = ref<Vditor | null>(null)
const syncingFromEditor = ref(false)
const lastEditorRange = ref<Range | null>(null)
const toolbarEl = ref<HTMLElement | null>(null)
const uploadPlaceholderToken = ref<string | null>(null)

function extractImageUrlFromText(text: string) {
  const trimmed = text.trim()
  if (!trimmed) return ''
  const markdownMatch = trimmed.match(/!\[[^\]]*]\((https?:\/\/[^)]+)\)/i)
  if (markdownMatch?.[1]) return markdownMatch[1]
  const urlMatch = trimmed.match(/https?:\/\/\S+/i)
  return urlMatch ? urlMatch[0] : ''
}

function extractImageUrlFromHtml(html: string) {
  if (!html) return ''
  try {
    const doc = new DOMParser().parseFromString(html, 'text/html')
    const img = doc.querySelector('img')
    if (img?.getAttribute('src')) return img.getAttribute('src') || ''
    const link = doc.querySelector('a[href]')
    const href = link?.getAttribute('href') || ''
    return href
  } catch {
    return ''
  }
}

function normalizeImageUrl(url: string) {
  try {
    const parsed = new URL(url)
    if (parsed.hostname === 'github.com') {
      const parts = parsed.pathname.split('/').filter(Boolean)
      if (parts.length >= 5 && (parts[2] === 'raw' || parts[2] === 'blob')) {
        const [owner, repo, , branch, ...rest] = parts
        return `https://raw.githubusercontent.com/${owner}/${repo}/${branch}/${rest.join('/')}`
      }
    }
  } catch {
    // ignore
  }
  return url
}

function isImageUrl(url: string) {
  return /\.(png|jpe?g|gif|webp)(\?.*)?$/i.test(url)
}

async function replaceImageUrlInContent(originalUrls: string[], newUrl: string) {
  const candidates = originalUrls.filter(Boolean)
  if (candidates.length === 0 || !newUrl) return
  const maxAttempts = 10
  const delayMs = 80
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    const current = formData.value.answer_markdown || ''
    const matched = candidates.some((url) => current.includes(url))
    if (matched) {
      let next = current
      for (const url of candidates) {
        const escaped = url.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
        next = next.replace(new RegExp(escaped, 'g'), newUrl)
      }
      formData.value.answer_markdown = next
      return
    }
    await new Promise((resolve) => setTimeout(resolve, delayMs))
  }
}

function getFileNameFromUrl(url: string, mimeType?: string) {
  try {
    const parsed = new URL(url)
    const name = parsed.pathname.split('/').pop()
    if (name) return name
  } catch {
    // ignore
  }
  const ext = mimeType?.split('/')[1] || 'png'
  return `pasted-image-${Date.now()}.${ext}`
}

async function uploadImageFileAndGetUrl(file: File) {
  if (!beforeImageUpload(file)) return ''
  const desc = await promptImageDescription()
  if (!desc) return ''
  const res = await uploadQAImage(file, desc)
  if ((res as { success?: boolean }).success === false) {
    ElMessage.error('Upload failed')
    return ''
  }
  const url = extractUploadUrl(res)
  if (!url) {
    ElMessage.error('Upload failed: no url returned')
    return ''
  }
  return url
}

async function handlePasteImageUrl(event: ClipboardEvent) {
  const text = event.clipboardData?.getData('text/plain') || ''
  const html = event.clipboardData?.getData('text/html') || ''
  let url = extractImageUrlFromText(text)
  if (!url && html) url = extractImageUrlFromHtml(html)
  if (!url) return
  const originalUrl = url
  url = normalizeImageUrl(url)
  if (url.toLowerCase().includes('amd.com')) return
  // Skip private GitHub images (they have CORS restrictions)
  if (url.includes('private-user-images.githubusercontent.com')) {
    ElMessage.warning(
      'Private GitHub image detected. Cannot upload due to access restrictions. The link may expire in a few minutes.',
    )
    return
  }
  if (!isImageUrl(url)) return
  try {
    let response = await fetch(url, { redirect: 'follow' })
    if (response.status >= 300 && response.status < 400) {
      const location = response.headers.get('location')
      if (location) {
        const nextUrl = new URL(location, url).toString()
        response = await fetch(nextUrl, { redirect: 'follow' })
      }
    }
    if (!response.ok) throw new Error('fetch failed')
    const blob = await response.blob()
    const fileName = getFileNameFromUrl(url, blob.type)
    const file = new File([blob], fileName, { type: blob.type || 'image/png' })
    const uploadedUrl = await uploadImageFileAndGetUrl(file)
    if (uploadedUrl) {
      await replaceImageUrlInContent([originalUrl, url], uploadedUrl)
    }
  } catch (error) {
    console.error('Failed to upload pasted image:', error)
    ElMessage.error('Paste upload failed, keep original url')
  }
}

function storeEditorSelection() {
  const selection = window.getSelection()
  if (!selection || selection.rangeCount === 0) return
  const range = selection.getRangeAt(0)
  if (!vditorRef.value || !vditorRef.value.contains(range.commonAncestorContainer)) return
  lastEditorRange.value = range.cloneRange()
}

function restoreEditorSelection() {
  if (!lastEditorRange.value) return
  const selection = window.getSelection()
  if (!selection) return
  selection.removeAllRanges()
  selection.addRange(lastEditorRange.value)
}

function insertUploadPlaceholderAtCursor() {
  if (!vditorInstance.value) return ''
  const token = `[[UPLOAD_${Date.now()}_${Math.random().toString(36).slice(2, 8)}]]`
  uploadPlaceholderToken.value = token
  restoreEditorSelection()
  vditorInstance.value.focus()
  vditorInstance.value.insertValue(token)
  return token
}

function replaceUploadPlaceholder(snippet: string) {
  const token = uploadPlaceholderToken.value
  if (!token) return false
  const current = formData.value.answer_markdown || ''
  if (!current.includes(token)) return false
  formData.value.answer_markdown = current.replace(token, `${snippet}\n`)
  uploadPlaceholderToken.value = null
  return true
}

function clearUploadPlaceholder() {
  const token = uploadPlaceholderToken.value
  if (!token) return
  const current = formData.value.answer_markdown || ''
  if (current.includes(token)) {
    formData.value.answer_markdown = current.replace(token, '')
  }
  uploadPlaceholderToken.value = null
}

function bindEditorSelectionListeners() {
  const host = vditorRef.value
  if (!host) return
  host.addEventListener('mouseup', storeEditorSelection)
  host.addEventListener('keyup', storeEditorSelection)
  host.addEventListener('focusin', storeEditorSelection)
  host.addEventListener('paste', handlePasteImageUrl, true)
  const toolbar = host.querySelector('.vditor-toolbar') as HTMLElement | null
  if (toolbar) {
    toolbar.addEventListener('mousedown', storeEditorSelection, true)
    toolbarEl.value = toolbar
  }
}

function unbindEditorSelectionListeners() {
  const host = vditorRef.value
  if (!host) return
  host.removeEventListener('mouseup', storeEditorSelection)
  host.removeEventListener('keyup', storeEditorSelection)
  host.removeEventListener('focusin', storeEditorSelection)
  host.removeEventListener('paste', handlePasteImageUrl, true)
  if (toolbarEl.value) {
    toolbarEl.value.removeEventListener('mousedown', storeEditorSelection, true)
    toolbarEl.value = null
  }
}

function initVditor() {
  if (vditorInstance.value || !vditorRef.value) return
  vditorInstance.value = new Vditor(vditorRef.value, {
    lang: 'en_US',
    mode: 'ir',
    theme: editorTheme.value,
    height: 400,
    placeholder: 'Enter Markdown',
    cache: { enable: false },
    toolbar: [
      'headings',
      'bold',
      'italic',
      'strike',
      '|',
      'list',
      'ordered-list',
      'check',
      '|',
      'quote',
      'line',
      'code',
      'inline-code',
      'table',
      'upload',
      'link',
      '|',
      'undo',
      'redo',
      '|',
      'fullscreen',
    ],
    upload: {
      accept: getUploadAccept(),
      multiple: true,
      handler: async (files) => {
        await handleVditorUpload(files)
        return null
      },
    },
    counter: { enable: true, max: 50000, type: 'markdown' },
    value: formData.value.answer_markdown,
    input: (value) => {
      syncingFromEditor.value = true
      formData.value.answer_markdown = value
      nextTick(() => {
        syncingFromEditor.value = false
      })
    },
  })
  bindEditorSelectionListeners()
}

function destroyVditor() {
  unbindEditorSelectionListeners()
  vditorInstance.value?.destroy()
  vditorInstance.value = null
}

onBeforeUnmount(() => {
  destroyVditor()
})

watch(
  () => dialogVisible.value,
  (visible) => {
    if (!visible) {
      destroyVditor()
      return
    }
    nextTick(() => {
      initVditor()
    })
  },
  { immediate: true },
)

watch(isDark, (value) => {
  vditorInstance.value?.setTheme(value ? 'dark' : 'classic')
})

watch(
  () => formData.value.answer_markdown,
  (value) => {
    if (syncingFromEditor.value) return
    const instance = vditorInstance.value
    if (!instance) return
    if (value !== instance.getValue()) {
      instance.setValue(value)
    }
  },
)

// Watch itemData changes
watch(
  () => props.itemData,
  (newData) => {
    if (newData && props.mode === 'edit') {
      formData.value = {
        questions:
          newData.questions && newData.questions.length > 0
            ? newData.questions
            : [{ question: '' }],
        answer_markdown: newData.answer || '',
        priority: newData.priority,
        is_active: newData.is_active,
      }
    }
  },
  { immediate: true },
)

// Reset form when dialog opens in create mode
watch(
  () => props.modelValue,
  (visible) => {
    if (visible && props.mode === 'create') {
      formData.value = {
        questions: [{ question: '' }],
        answer_markdown: '',
        priority: 'medium',
        is_active: true,
      }
      formRef.value?.clearValidate()
    }
  },
)

function getAnswerPayload(): string {
  return formData.value.answer_markdown || ''
}

function addQuestion() {
  if (formData.value.questions.length >= MAX_QUESTION_VARIANTS) return
  formData.value.questions.push({ question: '' })
}

function removeQuestion(idx: number) {
  if (formData.value.questions.length <= 1) return
  formData.value.questions.splice(idx, 1)
}

async function handleGenerateQuestions() {
  const primary = (formData.value.questions?.[0]?.question ?? '').trim()
  if (!primary) {
    ElMessage.warning('Please enter a primary question before generating')
    return
  }
  const answer = getAnswerPayload().trim()
  if (answer.length < MIN_ANSWER_LENGTH) {
    ElMessage.warning(
      `Answer must be at least ${MIN_ANSWER_LENGTH} characters to generate questions`,
    )
    return
  }
  generatingQuestions.value = true
  try {
    const res = await generateQAQuestions({
      answer,
      max_questions: generateQuestionCount,
      primary_question: primary,
    })
    const questions = Array.isArray(res.questions) ? res.questions : []
    if (questions.length === 0) {
      ElMessage.warning('No questions generated')
      return
    }
    formData.value.questions = questions
      .slice(0, MAX_QUESTION_VARIANTS)
      .map((question) => ({ question }))
  } catch (error) {
    console.error('Failed to generate questions:', error)
    ElMessage.error('Failed to generate questions')
  } finally {
    generatingQuestions.value = false
  }
}

// drag & drop reorder for questions
const qDragIndex = ref<number | null>(null)
function onQDragStart(idx: number) {
  qDragIndex.value = idx
}
function onQDrop(targetIdx: number) {
  if (qDragIndex.value == null) return
  const from = qDragIndex.value
  const to = targetIdx
  qDragIndex.value = null
  if (from === to) return
  const moved = formData.value.questions.splice(from, 1)[0]
  formData.value.questions.splice(to, 0, moved)
}

function getFileExt(name: string) {
  const idx = name.lastIndexOf('.')
  return idx >= 0 ? name.slice(idx + 1).toLowerCase() : ''
}

function isAllowedImage(file: File) {
  return IMAGE_MIME_TYPES.includes(file.type)
}

function isAllowedDoc(file: File) {
  const ext = getFileExt(file.name)
  return FILE_EXTENSIONS.includes(ext) ? true : false
}

function beforeImageUpload(file: File) {
  const okType = isAllowedImage(file)
  const okSize = file.size <= MAX_IMAGE_SIZE
  if (!okType) ElMessage.warning('Only jpg/png/gif/webp are allowed')
  if (!okSize) ElMessage.warning('Max file size is 10MB')
  return okType && okSize
}

const handleSubmit = async () => {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
  } catch {
    return
  }

  submitting.value = true
  try {
    if (props.mode === 'create') {
      if (!props.collectionId) {
        ElMessage.error('Collection ID is required')
        return
      }
      const questions = formData.value.questions
        .map((item) => (item?.question ?? '').trim())
        .filter(Boolean)
        .slice(0, 10)
      await createQAItem({
        collection_id: props.collectionId,
        answer: getAnswerPayload(),
        answer_type: 'markdown',
        questions,
        primary_question_index: 0,
        priority: formData.value.priority,
        is_active: formData.value.is_active,
      })
      ElMessage.success('Created successfully')
    } else {
      if (!props.itemData?.id) {
        ElMessage.error('Item ID is required')
        return
      }
      const questionPayload = formData.value.questions
        .map((item, idx) => ({
          id: item.id,
          question: (item.question ?? '').trim(),
          is_primary: idx === 0,
        }))
        .filter((item) => item.question)
      await updateQAItem(props.itemData.id, {
        answer: getAnswerPayload(),
        answer_type: 'markdown',
        questions: questionPayload,
        priority: formData.value.priority,
        is_active: formData.value.is_active,
      })
      ElMessage.success('Updated successfully')
    }
    emit('success')
    handleClose()
  } catch (error) {
    console.error('Failed to submit:', error)
    ElMessage.error(props.mode === 'create' ? 'Failed to create' : 'Failed to update')
  } finally {
    submitting.value = false
  }
}

function appendMarkdownSnippet(snippet: string) {
  const instance = vditorInstance.value
  const current = formData.value.answer_markdown || ''
  if (instance) {
    const insertText = `${snippet}\n`
    restoreEditorSelection()
    instance.focus()
    instance.insertValue(insertText)
    return
  }
  const next =
    current && !current.endsWith('\n') ? `${current}\n${snippet}` : `${current}${snippet}`
  formData.value.answer_markdown = `${next}\n`
}

function extractUploadUrl(raw: unknown): string {
  if (!raw || typeof raw !== 'object') return ''
  const record = raw as Record<string, unknown>
  return (
    (record.url as string) ||
    (record.path as string) ||
    ((record.data as Record<string, unknown> | undefined)?.url as string) ||
    ((record.data as Record<string, unknown> | undefined)?.path as string) ||
    ''
  )
}

function getUploadAccept() {
  const docExts = FILE_EXTENSIONS.map((ext) => `.${ext}`).join(',')
  return `image/*,${docExts}`
}

async function handleVditorUpload(files: File[] | FileList) {
  for (const file of Array.from(files)) {
    await uploadMarkdownAsset(file)
  }
}

async function promptImageDescription() {
  try {
    const { value } = (await ElMessageBox.prompt(
      'Please enter image description',
      'Image Description',
      {
        confirmButtonText: 'OK',
        cancelButtonText: 'Cancel',
        inputPlaceholder: 'e.g. System architecture diagram',
        inputValidator: (val) => (val && String(val).trim() ? true : 'Description is required'),
      },
    )) as { value: string }
    return String(value).trim()
  } catch {
    return ''
  }
}

async function uploadMarkdownAsset(file: File) {
  try {
    insertUploadPlaceholderAtCursor()
    if (isAllowedImage(file)) {
      if (!beforeImageUpload(file)) return
      const desc = await promptImageDescription()
      if (!desc) {
        clearUploadPlaceholder()
        return
      }
      const res = await uploadQAImage(file, desc)
      if ((res as { success?: boolean }).success === false) {
        ElMessage.error('Upload failed')
        clearUploadPlaceholder()
        return
      }
      const url = extractUploadUrl(res)
      if (!url) {
        ElMessage.error('Upload failed: no url returned')
        clearUploadPlaceholder()
        return
      }
      if (!replaceUploadPlaceholder(`![${desc}](${url})`)) {
        appendMarkdownSnippet(`![${desc}](${url})`)
      }
      ElMessage.success('Uploaded')
      return
    }

    const okType = isAllowedDoc(file)
    const okSize = file.size <= MAX_FILE_SIZE
    if (!okType) ElMessage.warning('Only PDF/Word/Excel/PPT/TXT/CSV/JSON/ZIP are allowed')
    if (!okSize) ElMessage.warning('Max file size is 10MB')
    if (!okType || !okSize) return

    const desc = file.name
    const res = await uploadQAFile(file, desc)
    if ((res as { success?: boolean }).success === false) {
      ElMessage.error('Upload failed')
      clearUploadPlaceholder()
      return
    }
    const url = extractUploadUrl(res)
    if (!url) {
      ElMessage.error('Upload failed: no url returned')
      clearUploadPlaceholder()
      return
    }
    if (!replaceUploadPlaceholder(`[${desc}](${url})`)) {
      appendMarkdownSnippet(`[${desc}](${url})`)
    }
    ElMessage.success('Uploaded')
  } catch (e) {
    console.error(e)
    ElMessage.error('Upload failed')
    clearUploadPlaceholder()
  }
}

const handleClose = () => {
  emit('update:modelValue', false)
}
</script>

<style scoped lang="scss">
.qa-edit-form {
  padding: 20px 0;
}

.vditor-host {
  width: 100%;
}

.vditor-host :deep(.vditor) {
  border-radius: 6px;
}

:global(html.dark) .vditor-host :deep(.vditor) {
  color: var(--el-text-color-primary);
  --textarea-text-color: var(--el-text-color-primary);
}

:global(html.dark) .vditor-host :deep(.vditor-ir),
:global(html.dark) .vditor-host :deep(.vditor-wysiwyg),
:global(html.dark) .vditor-host :deep(.vditor-reset) {
  color: var(--el-text-color-primary) !important;
}

:global(.vditor--dark .vditor-reset) {
  color: var(--textarea-text-color) !important;
}

.question-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--safe-border);
  background: var(--safe-card-2);
  border-radius: 8px;
  margin-bottom: 8px;

  .left {
    display: flex;
    align-items: center;
    gap: 10px;
    flex: 1;
    min-width: 0;
  }

  .right {
    flex-shrink: 0;
  }

  .drag-handle {
    cursor: grab;
    user-select: none;
    padding: 2px 4px;
    border-radius: 4px;
    color: var(--safe-muted);
    border: 1px dashed var(--safe-border);
  }

  .drag-zone {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    cursor: grab;
    user-select: none;
  }
}
</style>

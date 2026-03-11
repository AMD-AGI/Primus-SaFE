<template>
  <el-dialog
    v-model="dialogVisible"
    title="Manage question variants"
    width="720px"
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <div v-loading="loading">
      <el-alert
        type="info"
        show-icon
        :closable="false"
        title="Drag to reorder. The first question will be saved as the primary question. Click Save to apply changes."
        class="mb-3"
      />

      <div class="mb-3 flex items-center gap-2">
        <el-input
          v-model="newQuestion"
          placeholder="Add a new question variant (max 5)"
          clearable
          maxlength="500"
          show-word-limit
          @keyup.enter="handleAdd"
        />
        <el-button type="primary" :disabled="!canAdd" :loading="adding" @click="handleAdd">
          Add
        </el-button>
      </div>

      <el-empty v-if="!loading && questions.length === 0" description="No questions" class="py-6" />

      <div v-else class="variants-list">
        <div
          v-for="(q, idx) in questions"
          :key="q.id ? q.id : `new-${idx}`"
          class="variant-row"
          draggable="true"
          @dragstart="onDragStart(idx)"
          @dragover.prevent
          @drop="onDrop(idx)"
        >
          <div class="left">
            <el-tag v-if="idx === 0" type="success" effect="plain">Primary</el-tag>
            <el-tag v-else type="info" effect="plain">Variant</el-tag>
            <div class="text">
              <span class="idx">{{ idx + 1 }}.</span>
              <span class="q">{{ q.question }}</span>
            </div>
          </div>

          <div class="right">
            <el-tooltip content="Delete" placement="top">
              <el-button
                size="small"
                type="danger"
                plain
                :disabled="questions.length <= 1"
                @click="handleDelete(q)"
              >
                Delete
              </el-button>
            </el-tooltip>
          </div>
        </div>
      </div>

      <div class="mt-3 text-sm text-gray-500">{{ questions.length }}/5 variants</div>
    </div>

    <template #footer>
      <el-button @click="handleClose">Cancel</el-button>
      <el-button type="primary" :loading="saving" @click="handleSave">Save</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getQAAnswerDetail, updateQAItem, type QAQuestionVariant } from '@/services/chatbot'

interface Props {
  modelValue: boolean
  answerId?: number | null
}

interface Emits {
  (e: 'update:modelValue', value: boolean): void
  (e: 'success'): void
}

const props = withDefaults(defineProps<Props>(), {
  answerId: null,
})
const emit = defineEmits<Emits>()

const dialogVisible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const loading = ref(false)
const adding = ref(false)
const saving = ref(false)
type EditableQuestion = {
  id?: number
  question: string
  is_primary: boolean
}
const questions = ref<EditableQuestion[]>([])
const newQuestion = ref('')
const answerMeta = ref<{
  answer: string
  answer_type: 'plaintext' | 'markdown' | 'richtext'
  priority: 'low' | 'medium' | 'high'
  is_active: boolean
} | null>(null)

const canAdd = computed(() => {
  return (
    !!props.answerId && questions.value.length < 5 && !!newQuestion.value.trim() && !adding.value
  )
})

async function loadDetail() {
  if (!props.answerId) return
  loading.value = true
  try {
    const resp = await getQAAnswerDetail(props.answerId)
    const data = resp.data
    answerMeta.value = {
      answer: data.answer.answer,
      answer_type: data.answer.answer_type ?? 'markdown',
      priority: data.answer.priority ?? 'medium',
      is_active: data.answer.is_active ?? true,
    }
    questions.value = Array.isArray(data?.questions)
      ? (data.questions as QAQuestionVariant[]).map((q) => ({
          id: q.id,
          question: q.question,
          is_primary: !!q.is_primary,
        }))
      : []
    // ensure primary first in UI
    const primaryIdx = questions.value.findIndex((q) => q.is_primary)
    if (primaryIdx > 0) {
      const [p] = questions.value.splice(primaryIdx, 1)
      questions.value.unshift(p)
    }
  } catch (e) {
    console.error(e)
    ElMessage.error('Failed to load variants')
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (v) => {
    if (v) {
      newQuestion.value = ''
      loadDetail()
    }
  },
)

const handleAdd = async () => {
  if (!canAdd.value || !props.answerId) return
  adding.value = true
  try {
    // Add new question: no id, added locally and submitted via update
    questions.value.push({
      question: newQuestion.value.trim(),
      is_primary: false,
    })
    newQuestion.value = ''
    ElMessage.success('Added')
  } catch (e) {
    console.error(e)
    ElMessage.error('Failed to add')
  } finally {
    adding.value = false
  }
}

const handleDelete = async (q: EditableQuestion) => {
  if (!props.answerId) return
  await ElMessageBox.confirm('Delete this question variant?', 'Confirm', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
  try {
    const idx = questions.value.findIndex((x) => x.id === q.id && x.question === q.question)
    if (idx >= 0) questions.value.splice(idx, 1)
    ElMessage.success('Deleted')
  } catch (e) {
    console.error(e)
    ElMessage.error('Failed to delete')
  }
}

async function handleSave() {
  await persistOrderAndPrimary()
}

async function persistOrderAndPrimary() {
  if (!props.answerId) return
  if (!answerMeta.value) {
    await loadDetail()
    if (!answerMeta.value) return
  }
  // Payload: questions array; first item is_primary=true, new questions have no id
  const payloadQuestions = questions.value.map((q, idx) => ({
    ...(q.id ? { id: q.id } : {}),
    question: q.question,
    is_primary: idx === 0,
  }))
  try {
    saving.value = true
    await updateQAItem(props.answerId, {
      ...answerMeta.value,
      questions: payloadQuestions,
    })
    ElMessage.success('Updated')
    await loadDetail()
    emit('success')
  } catch (e) {
    console.error(e)
    ElMessage.error('Failed to update')
  } finally {
    saving.value = false
  }
}

// ===== Drag & Drop reorder =====
const dragIndex = ref<number | null>(null)

function onDragStart(idx: number) {
  dragIndex.value = idx
}

async function onDrop(targetIdx: number) {
  if (dragIndex.value == null) return
  const from = dragIndex.value
  const to = targetIdx
  dragIndex.value = null
  if (from === to) return

  const moved = questions.value.splice(from, 1)[0]
  questions.value.splice(to, 0, moved)
}

function handleClose() {
  emit('update:modelValue', false)
}
</script>

<style scoped lang="scss">
.variants-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.variant-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--safe-border);
  background: var(--safe-card-2);
  border-radius: 8px;
  cursor: grab;

  .left {
    display: flex;
    align-items: center;
    gap: 10px;
    min-width: 0;
    flex: 1;

    .text {
      min-width: 0;
      display: flex;
      gap: 6px;
      align-items: baseline;
      .idx {
        color: var(--safe-muted);
        font-size: 12px;
        flex-shrink: 0;
      }
      .q {
        color: var(--safe-text);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }
  }

  .right {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
  }
}
</style>

<template>
  <el-dialog
    v-model="dialogVisible"
    title="QA Base Details"
    width="700px"
    append-to-body
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <div v-loading="loading" class="qa-detail-content">
      <div v-if="data" class="qa-detail">
        <div class="detail-section">
          <div class="section-label">
            <el-icon><QuestionFilled /></el-icon>
            <span>Question</span>
          </div>
          <div class="section-content">{{ getPrimaryQuestion(data) }}</div>
        </div>
        <div class="detail-section">
          <div class="section-label">
            <el-icon><ChatDotRound /></el-icon>
            <span>Answer</span>
          </div>
          <div
            class="section-content markdown-preview"
            v-html="getAnswerHtml(data)"
            @click="handleImageClick"
          ></div>
        </div>
        <div v-if="data.answer?.source" class="detail-section">
          <div class="section-label">
            <el-icon><Document /></el-icon>
            <span>Source</span>
          </div>
          <div class="section-content">{{ data.answer?.source }}</div>
        </div>
        <div class="detail-meta">
          <el-tag
            size="small"
            :type="
              data.answer?.priority === 'high'
                ? 'danger'
                : data.answer?.priority === 'medium'
                  ? 'warning'
                  : 'primary'
            "
          >
            Priority: {{ data.answer?.priority ?? '-' }}
          </el-tag>
          <el-tag v-if="data.answer?.is_active" size="small" type="success">Active</el-tag>
          <span class="meta-time">
            Created at: {{ data.answer?.created_at ? formatTimeStr(data.answer.created_at) : '-' }}
          </span>
        </div>
      </div>
    </div>
    <ImagePreviewOverlay
      :visible="imagePreviewVisible"
      :url="imagePreviewUrl"
      @close="closeImagePreview"
    />
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { QuestionFilled, ChatDotRound, Document } from '@element-plus/icons-vue'
import { marked } from 'marked'
import type { QAAnswerDetailData } from '@/services/chatbot'
import { formatTimeStr } from '@/utils'
import ImagePreviewOverlay from '@/components/Base/ImagePreviewOverlay.vue'
import { useImagePreview } from '@/composables/useImagePreview'

interface Props {
  modelValue: boolean
  loading?: boolean
  data: QAAnswerDetailData | null
}

interface Emits {
  (e: 'update:modelValue', value: boolean): void
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
})

const emit = defineEmits<Emits>()

const dialogVisible = computed({
  get: () => props.modelValue,
  set: (value) => emit('update:modelValue', value),
})

const { imagePreviewVisible, imagePreviewUrl, handleImageClick, closeImagePreview } =
  useImagePreview()

const getPrimaryQuestion = (item: QAAnswerDetailData) => {
  const primary = item.questions?.find((question) => question.is_primary)
  return primary?.question ?? item.questions?.[0]?.question ?? ''
}

const getAnswerHtml = (item: QAAnswerDetailData) => {
  const text = item.answer?.answer ?? ''
  return marked.parse(text || '')
}

const handleClose = () => {
  emit('update:modelValue', false)
}
</script>

<style scoped lang="scss">
.qa-detail-content {
  min-height: 200px;

  .qa-detail {
    display: flex;
    flex-direction: column;
    gap: 20px;

    .detail-section {
      .section-label {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-bottom: 10px;
        font-size: 14px;
        font-weight: 600;
        color: #3b82f6;

        .el-icon {
          font-size: 16px;
        }
      }

      .section-content {
        padding: 12px 16px;
        background: rgba(248, 249, 250, 0.8);
        border: 1px solid #e2e8f0;
        border-radius: 8px;
        font-size: 14px;
        line-height: 1.8;
        color: #334155;
        white-space: pre-wrap;
        word-wrap: break-word;

        :deep(img) {
          display: block;
          max-width: 100%;
          height: auto;
          max-height: 360px;
          border-radius: 8px;
          object-fit: contain;
          cursor: zoom-in;
        }

        :deep(pre) {
          display: block;
          margin: 12px 0;
          padding: 12px;
          background: #1e293b !important;
          border-radius: 6px;
          overflow-x: auto;
          max-width: 100%;

          code {
            display: block;
            font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
            font-size: 13px;
            line-height: 1.6;
            color: #e2e8f0;
            white-space: pre;
            word-wrap: normal;
            overflow-wrap: normal;
          }
        }

        :deep(code:not(pre code)) {
          padding: 2px 6px;
          background: #e2e8f0;
          border-radius: 4px;
          font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
          font-size: 13px;
          color: #dc2626;
        }
      }
    }

    .detail-meta {
      display: flex;
      align-items: center;
      gap: 12px;
      padding-top: 16px;
      border-top: 1px solid #e2e8f0;
      flex-wrap: wrap;

      .meta-time {
        font-size: 13px;
        color: #64748b;
        margin-left: auto;
      }
    }
  }
}

// Dark mode
.dark {
  .qa-detail-content {
    .qa-detail {
      .detail-section {
        .section-label {
          color: #60a5fa;
        }

        .section-content {
          background: rgba(30, 41, 59, 0.6);
          border-color: #334155;
          color: #cbd5e1;

          :deep(pre) {
            background: #0f172a !important;

            code {
              color: #e2e8f0;
            }
          }

          :deep(code:not(pre code)) {
            background: #334155;
            color: #fb7185;
          }
        }
      }

      .detail-meta {
        border-top-color: #334155;

        .meta-time {
          color: #94a3b8;
        }
      }
    }
  }
}
</style>

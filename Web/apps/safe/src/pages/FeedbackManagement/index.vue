<template>
  <div class="feedback-management-container">
    <!-- Header -->
    <el-text class="block textx-18 font-500 mb-4" tag="b">Answer Feedback</el-text>

    <!-- Stats Cards -->
    <div class="stats-section" v-loading="statsLoading">
      <div
        class="stat-card clickable"
        :class="{ active: activeStatCard === 'total' }"
        @click="handleStatCardClick('total')"
      >
        <div class="stat-icon total">
          <el-icon><MessageBox /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.total }}</div>
          <div class="stat-label">Total</div>
        </div>
      </div>
      <div
        class="stat-card clickable"
        :class="{ active: activeStatCard === 'pending' }"
        @click="handleStatCardClick('pending')"
      >
        <div class="stat-icon pending">
          <el-icon><Clock /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.pending }}</div>
          <div class="stat-label">Pending</div>
        </div>
      </div>
      <div
        class="stat-card clickable"
        :class="{ active: activeStatCard === 'resolved' }"
        @click="handleStatCardClick('resolved')"
      >
        <div class="stat-icon resolved">
          <el-icon><Check /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.resolved }}</div>
          <div class="stat-label">Resolved</div>
        </div>
      </div>
      <div
        class="stat-card clickable"
        :class="{ active: activeStatCard === 'upvotes' }"
        @click="handleStatCardClick('upvotes')"
      >
        <div class="stat-icon upvotes">
          <el-icon><Like /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.upvotes }}</div>
          <div class="stat-label">Upvotes</div>
        </div>
      </div>
      <div
        class="stat-card clickable"
        :class="{ active: activeStatCard === 'downvotes' }"
        @click="handleStatCardClick('downvotes')"
      >
        <div class="stat-icon downvotes">
          <el-icon><DisLike /></el-icon>
        </div>
        <div class="stat-content">
          <div class="stat-value">{{ stats.downvotes }}</div>
          <div class="stat-label">Downvotes</div>
        </div>
      </div>
    </div>

    <!-- Filters and Card List -->
    <!-- <el-card class="mt-4 safe-card" shadow="never"> -->
    <div class="filters-bar">
      <div class="filters-left">
        <el-select
          v-model="filters.status"
          placeholder="Status"
          clearable
          @change="handleFilterChange"
          style="width: 150px"
        >
          <el-option label="Pending" value="pending" />
          <el-option label="Resolved" value="resolved" />
          <el-option label="Ignored" value="ignored" />
        </el-select>
        <el-select
          v-model="filters.vote_type"
          placeholder="Type"
          clearable
          @change="handleFilterChange"
          style="width: 150px"
        >
          <el-option label="Upvote" value="up" />
          <el-option label="Downvote" value="down" />
        </el-select>
      </div>
    </div>

    <!-- Feedback Cards -->
    <div v-loading="tableLoading" class="feedback-cards-container">
      <div v-if="feedbackList.length === 0 && !tableLoading" class="empty-state">
        <el-icon class="empty-icon"><MessageBox /></el-icon>
        <p class="empty-text">No feedback found</p>
      </div>

      <div v-for="feedback in feedbackList" :key="feedback.id" class="feedback-card">
        <!-- Card Header -->
        <div class="feedback-card-header">
          <div class="feedback-card-header-left">
            <span class="feedback-id">#{{ feedback.id }}</span>
            <div class="feedback-user">
              <el-icon><User /></el-icon>
              <span>{{ feedback.user_name }}</span>
            </div>
            <el-tag :type="feedback.vote_type === 'up' ? 'success' : 'danger'" size="small">
              <el-icon style="margin-right: 4px">
                <Like v-if="feedback.vote_type === 'up'" />
                <DisLike v-else />
              </el-icon>
              {{ feedback.vote_type === 'up' ? 'Upvote' : 'Downvote' }}
            </el-tag>
            <el-tag
              :type="
                feedback.status === 'pending'
                  ? 'warning'
                  : feedback.status === 'resolved'
                    ? 'success'
                    : 'info'
              "
              size="small"
            >
              {{
                feedback.status === 'pending'
                  ? 'Pending'
                  : feedback.status === 'resolved'
                    ? 'Resolved'
                    : 'Ignored'
              }}
            </el-tag>
          </div>
          <div class="feedback-card-header-right">
            <span class="feedback-time">{{ formatTimeStr(feedback.created_at) }}</span>
          </div>
        </div>

        <!-- Card Content -->
        <div class="feedback-card-content">
          <div class="feedback-section">
            <div class="section-label">
              <el-icon><QuestionFilled /></el-icon>
              <span>Question</span>
            </div>
            <div class="section-text">{{ feedback.query }}</div>
          </div>

          <div class="feedback-section">
            <div class="section-label">
              <el-icon><ChatDotRound /></el-icon>
              <span>Answer</span>
            </div>
            <div class="section-text">{{ feedback.answer }}</div>
          </div>

          <div v-if="feedback.reason" class="feedback-section">
            <div class="section-label">
              <el-icon><InfoFilled /></el-icon>
              <span>Reason</span>
            </div>
            <div class="section-text">{{ feedback.reason }}</div>
          </div>
        </div>

        <!-- Knowledge Base Sources -->
        <div
          v-if="feedback.source_refs && feedback.source_refs.length > 0"
          class="sources-section"
          @click.stop
        >
          <div class="sources-title">
            <el-icon class="sources-icon"><Collection /></el-icon>
            <span>Knowledge Base Sources ({{ feedback.source_refs.length }})</span>
          </div>
          <div class="sources-list">
            <div
              v-for="(source, sourceIndex) in feedback.source_refs"
              :key="sourceIndex"
              class="source-item"
              @click="handleViewSourceDetail(source.item_id)"
            >
              <div class="source-header">
                <span class="source-collection">{{ source.collection_name || 'QA Items' }}</span>
                <span v-if="source.similarity" class="source-similarity"
                  >{{ (source.similarity * 100).toFixed(1) }}%</span
                >
              </div>
              <div class="source-question">
                {{ source.question || `Item #${source.item_id}` }}
              </div>
            </div>
          </div>
        </div>

        <!-- Card Actions (only shown in pending state) -->
        <div v-if="feedback.status === 'pending'" class="feedback-card-actions" @click.stop>
          <el-button type="primary" size="small" @click="handleResolve(feedback)">
            Resolve
          </el-button>
          <el-button type="info" size="small" @click="handleIgnore(feedback)"> Ignore </el-button>
        </div>
      </div>
    </div>

    <!-- Pagination -->
    <div class="pagination-wrapper">
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.page_size"
        :page-sizes="[10, 20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="handleSizeChange"
        @current-change="handlePageChange"
      />
    </div>
    <!-- </el-card> -->

    <!-- Resolve Dialog -->
    <el-dialog
      v-model="resolveDialogVisible"
      :title="resolveAction === 'resolved' ? 'Resolve Feedback' : 'Ignore Feedback'"
      width="500px"
      :close-on-click-modal="false"
    >
      <el-form :model="resolveForm" label-width="100px">
        <el-form-item label="Note">
          <el-input
            v-model="resolveForm.note"
            type="textarea"
            :rows="4"
            :placeholder="
              resolveAction === 'resolved'
                ? 'Enter resolution note (optional)'
                : 'Enter ignore reason (optional)'
            "
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="resolveDialogVisible = false">Cancel</el-button>
          <el-button type="primary" @click="confirmResolve" :loading="resolving">
            Confirm
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- Edit QA Item Dialog -->
    <QAEditDialog
      v-model="qaEditDialogVisible"
      mode="edit"
      :item-data="qaEditData"
      @success="handleQAEditSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import {
  MessageBox,
  Clock,
  Check,
  Select as Like,
  CloseBold as DisLike,
  User,
  QuestionFilled,
  ChatDotRound,
  InfoFilled,
  Collection,
} from '@element-plus/icons-vue'
import {
  getFeedbackList,
  getFeedbackStats,
  resolveFeedback,
  getQAItemDetail,
  type FeedbackData,
} from '@/services/chatbot'
import { formatTimeStr } from '@/utils'
import QAEditDialog from '@/pages/QABase/Components/QAEditDialog.vue'

// Stats
const statsLoading = ref(false)
const stats = ref({
  total: 0,
  pending: 0,
  resolved: 0,
  ignored: 0,
  upvotes: 0,
  downvotes: 0,
})

// Table
const tableLoading = ref(false)
const feedbackList = ref<FeedbackData[]>([])
const filters = ref({
  status: 'pending' as 'pending' | 'resolved' | 'ignored' | undefined,
  vote_type: undefined as 'up' | 'down' | undefined,
})

// Active stat card tracking
const activeStatCard = ref<'total' | 'pending' | 'resolved' | 'upvotes' | 'downvotes' | null>(
  'pending',
)

// Pagination
const pagination = ref({
  page: 1,
  page_size: 20,
  total: 0,
  total_pages: 0,
})

// Current Feedback
const currentFeedback = ref<FeedbackData | null>(null)

// Resolve Dialog
const resolveDialogVisible = ref(false)
const resolveAction = ref<'resolved' | 'ignored'>('resolved')
const resolveForm = ref({
  note: '',
})
const resolving = ref(false)

// QA Edit Dialog
const qaEditDialogVisible = ref(false)
const qaEditData = ref<{
  id: number
  questions: Array<{ id?: number; question: string }>
  answer: string
  priority: 'low' | 'medium' | 'high'
  is_active: boolean
} | null>(null)

// Fetch stats
const fetchStats = async () => {
  statsLoading.value = true
  try {
    const response = await getFeedbackStats()
    stats.value = response.data
  } catch (error) {
    console.error('Failed to fetch stats:', error)
    ElMessage.error('Failed to fetch stats')
  } finally {
    statsLoading.value = false
  }
}

// Fetch feedback list
const fetchFeedbackList = async () => {
  tableLoading.value = true
  try {
    const response = await getFeedbackList({
      status: filters.value.status,
      vote_type: filters.value.vote_type,
      page: pagination.value.page,
      page_size: pagination.value.page_size,
    })
    feedbackList.value = response.data.items
    pagination.value = response.data.pagination
  } catch (error) {
    console.error('Failed to fetch feedback list:', error)
    ElMessage.error('Failed to fetch feedback list')
  } finally {
    tableLoading.value = false
  }
}

// Handle stat card click
const handleStatCardClick = (type: 'total' | 'pending' | 'resolved' | 'upvotes' | 'downvotes') => {
  activeStatCard.value = type
  if (type === 'total') {
    filters.value.status = undefined
    filters.value.vote_type = undefined
  } else if (type === 'upvotes') {
    filters.value.status = undefined
    filters.value.vote_type = 'up'
  } else if (type === 'downvotes') {
    filters.value.status = undefined
    filters.value.vote_type = 'down'
  } else {
    filters.value.status = type as 'pending' | 'resolved'
    filters.value.vote_type = undefined
  }
  pagination.value.page = 1
  fetchFeedbackList()
}

// Handle filter change
const handleFilterChange = () => {
  activeStatCard.value = null
  pagination.value.page = 1
  fetchFeedbackList()
}

// Handle page change
const handlePageChange = (page: number) => {
  pagination.value.page = page
  fetchFeedbackList()
}

// Handle page size change
const handleSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  fetchFeedbackList()
}

// Handle resolve
const handleResolve = (row: FeedbackData) => {
  currentFeedback.value = row
  resolveAction.value = 'resolved'
  resolveForm.value.note = ''
  resolveDialogVisible.value = true
}

// Handle ignore
const handleIgnore = (row: FeedbackData) => {
  currentFeedback.value = row
  resolveAction.value = 'ignored'
  resolveForm.value.note = ''
  resolveDialogVisible.value = true
}

// Confirm resolve
const confirmResolve = async () => {
  if (!currentFeedback.value) return

  resolving.value = true
  try {
    await resolveFeedback(currentFeedback.value.id, {
      status: resolveAction.value,
      note: resolveForm.value.note || undefined,
    })
    ElMessage.success(
      resolveAction.value === 'resolved' ? 'Resolved successfully' : 'Ignored successfully',
    )
    resolveDialogVisible.value = false
    await fetchFeedbackList()
    await fetchStats()
  } catch (error: unknown) {
    console.error('Failed to resolve feedback:', error)
    const err = error as { message?: string }
    ElMessage.error('Operation failed: ' + (err?.message || 'Unknown error'))
  } finally {
    resolving.value = false
  }
}

// View and edit source detail
const handleViewSourceDetail = async (itemId?: number) => {
  if (!itemId) {
    ElMessage.warning('Source item ID not available')
    return
  }

  try {
    const res = await getQAItemDetail(itemId)
    const questions: Array<{ id?: number; question: string; is_primary?: boolean }> =
      res.questions && res.questions.length > 0
        ? res.questions.map((question) => ({
            id: question.id,
            question: question.question,
            is_primary: question.is_primary,
          }))
        : [{ id: undefined, question: '' }]
    const primaryIndex = questions.findIndex((q) => q.is_primary)
    if (primaryIndex > 0) {
      const [primary] = questions.splice(primaryIndex, 1)
      questions.unshift(primary)
    }
    qaEditData.value = {
      id: res.answer.id,
      questions: questions.map((q) => ({ id: q.id, question: q.question })),
      answer: res.answer.answer,
      priority: res.answer.priority as 'low' | 'medium' | 'high',
      is_active: res.answer.is_active ?? true,
    }
    qaEditDialogVisible.value = true
  } catch (error) {
    console.error('Failed to load QA detail:', error)
    ElMessage.error('Failed to load details: ' + (error as Error).message)
  }
}

// Handle QA edit success
const handleQAEditSuccess = () => {
  // No need to reload anything, just close the dialog
  qaEditDialogVisible.value = false
}

// Mounted
onMounted(async () => {
  await Promise.all([fetchStats(), fetchFeedbackList()])
})
</script>

<style scoped lang="scss">
.feedback-management-container {
  padding: 0;
  min-height: calc(100vh - 66px);
  background: linear-gradient(135deg, rgba(249, 250, 251, 0.5) 0%, rgba(243, 244, 246, 0.5) 100%);
}

.textx-18 {
  font-size: 18px;
}

.font-500 {
  font-weight: 500;
}

.block {
  display: block;
}

.stats-section {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
  margin-bottom: 16px;
}

.stat-card {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  padding: 20px;
  display: flex;
  align-items: center;
  gap: 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  transition: all 0.3s ease;

  &.clickable {
    cursor: pointer;

    &:hover {
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
      border-color: var(--el-border-color-light);

      .stat-icon {
        transform: rotate(5deg) scale(1.05);
      }
    }

    &:active {
      transform: scale(0.98);
    }

    &.active {
      border-color: var(--el-color-primary);
      box-shadow: 0 4px 16px rgba(64, 158, 255, 0.2);
      background: linear-gradient(
        135deg,
        rgba(64, 158, 255, 0.05) 0%,
        rgba(64, 158, 255, 0.02) 100%
      );

      .stat-icon {
        transform: scale(1.08);
      }
    }
  }

  .stat-icon {
    width: 48px;
    height: 48px;
    border-radius: 12px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 22px;
    transition: transform 0.3s ease;

    &.total {
      background: linear-gradient(135deg, #e8eaf6 0%, #dce1f5 100%);
      color: #5c6bc0;
    }

    &.pending {
      background: linear-gradient(135deg, #fff3e0 0%, #ffe0b2 100%);
      color: #fb8c00;
    }

    &.resolved {
      background: linear-gradient(135deg, #e0f2f1 0%, #b2dfdb 100%);
      color: #26a69a;
    }

    &.upvotes {
      background: linear-gradient(135deg, #e8f5e9 0%, #c8e6c9 100%);
      color: #66bb6a;
    }

    &.downvotes {
      background: linear-gradient(135deg, #fce4ec 0%, #f8bbd0 100%);
      color: #ec407a;
    }
  }

  .stat-content {
    flex: 1;

    .stat-value {
      font-size: 28px;
      font-weight: 700;
      color: var(--el-text-color-primary);
      margin-bottom: 4px;
    }

    .stat-label {
      font-size: 13px;
      color: var(--el-text-color-secondary);
    }
  }
}

.filters-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0 0;

  .filters-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }
}

.feedback-cards-container {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
  margin-top: 16px;
  min-height: 200px;
}

.empty-state {
  grid-column: 1 / -1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 80px 20px;
  color: #94a3b8;
  background: rgba(255, 255, 255, 0.4);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 12px;

  .empty-icon {
    font-size: 64px;
    color: #cbd5e1;
    margin-bottom: 16px;
  }

  .empty-text {
    font-size: 16px;
    margin: 0;
  }
}

.feedback-card {
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(10px);
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 12px;
  padding: 18px;
  cursor: pointer;
  transition: all 0.3s ease;
  box-shadow:
    0 2px 8px rgba(0, 0, 0, 0.04),
    0 1px 2px rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
  height: 100%;

  &:hover {
    background: rgba(255, 255, 255, 0.85);
    transform: translateY(-3px);
    box-shadow:
      0 8px 24px rgba(0, 0, 0, 0.08),
      0 4px 8px rgba(0, 0, 0, 0.06);
    border-color: rgba(59, 130, 246, 0.3);
  }

  .feedback-card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;
    padding-bottom: 14px;
    border-bottom: 2px solid var(--el-border-color-lighter);
    flex-shrink: 0;

    .feedback-card-header-left {
      display: flex;
      align-items: center;
      gap: 10px;
      flex-wrap: wrap;

      .feedback-id {
        font-weight: 700;
        font-size: 15px;
        color: var(--el-color-primary);
      }

      .feedback-user {
        display: flex;
        align-items: center;
        gap: 4px;
        font-size: 13px;
        color: var(--el-text-color-regular);
        padding: 2px 8px;
        background: var(--el-fill-color-light);
        border-radius: 6px;

        .el-icon {
          font-size: 14px;
          color: var(--el-color-primary);
        }
      }
    }

    .feedback-card-header-right {
      flex-shrink: 0;

      .feedback-time {
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }
  }

  .feedback-card-content {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin-bottom: 0;

    .feedback-section {
      .section-label {
        display: flex;
        align-items: center;
        gap: 5px;
        margin-bottom: 6px;
        font-size: 12px;
        font-weight: 600;
        color: var(--el-text-color-regular);

        .el-icon {
          font-size: 14px;
          color: var(--el-color-primary);
        }
      }

      .section-text {
        font-size: 13px;
        line-height: 1.5;
        color: var(--el-text-color-primary);
        padding: 10px 12px;
        background: var(--el-fill-color-light);
        border-radius: 6px;
        white-space: pre-wrap;
        word-wrap: break-word;
        max-height: 100px;
        overflow-y: auto;

        &::-webkit-scrollbar {
          width: 4px;
        }

        &::-webkit-scrollbar-track {
          background: transparent;
        }

        &::-webkit-scrollbar-thumb {
          background: var(--el-border-color);
          border-radius: 2px;

          &:hover {
            background: var(--el-border-color-darker);
          }
        }
      }
    }
  }

  .sources-section {
    margin-top: 12px;
    margin-bottom: 0;
    padding: 12px;
    background: var(--el-fill-color-lighter);
    border-radius: 8px;
    font-size: 12px;

    .sources-title {
      display: flex;
      align-items: center;
      gap: 6px;
      margin-bottom: 10px;
      color: var(--el-text-color-regular);
      font-weight: 600;
      font-size: 13px;

      .sources-icon {
        font-size: 16px;
        color: var(--el-color-primary);
      }
    }

    .sources-list {
      display: flex;
      flex-direction: row;
      flex-wrap: wrap;
      gap: 8px;

      .source-item {
        flex: 0 1 auto;
        min-width: 200px;
        max-width: calc(33.333% - 6px);
        padding: 10px 12px;
        background: var(--el-bg-color);
        border: 1px solid var(--el-border-color);
        border-radius: 8px;
        transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
        cursor: pointer;

        &:hover {
          background: linear-gradient(135deg, #f3e8ff 0%, #faf5ff 100%);
          border-color: var(--el-color-primary);
          transform: translateY(-2px);
          box-shadow: 0 4px 12px rgba(var(--el-color-primary-rgb), 0.15);
        }

        .source-header {
          display: flex;
          align-items: center;
          justify-content: space-between;
          margin-bottom: 5px;

          .source-collection {
            font-weight: 600;
            color: var(--el-color-primary);
            font-size: 11px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
          }

          .source-similarity {
            font-size: 11px;
            color: var(--el-text-color-secondary);
            font-weight: 600;
            background: var(--el-fill-color-light);
            padding: 2px 6px;
            border-radius: 4px;
          }
        }

        .source-question {
          color: var(--el-text-color-regular);
          line-height: 1.5;
          font-size: 12px;
          overflow: hidden;
          text-overflow: ellipsis;
          display: -webkit-box;
          -webkit-line-clamp: 2;
          line-clamp: 2;
          -webkit-box-orient: vertical;
        }
      }
    }
  }

  .feedback-card-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-top: 14px;
    margin-top: auto;
    border-top: 1px solid var(--el-border-color-light);
    flex-shrink: 0;
  }
}

.pagination-wrapper {
  margin-top: 16px;
  display: flex;
  justify-content: flex-start;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

// Dark mode
.dark {
  .feedback-management-container {
    background: linear-gradient(135deg, rgba(15, 23, 42, 0.3) 0%, rgba(30, 41, 59, 0.3) 100%);
  }

  .stat-card {
    background: rgba(30, 41, 59, 0.6);
    border-color: rgba(71, 85, 105, 0.3);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);

    &.clickable {
      &:hover {
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
        border-color: rgba(71, 85, 105, 0.5);
      }

      &:active {
        transform: scale(0.98);
      }

      &.active {
        border-color: var(--el-color-primary);
        box-shadow: 0 4px 16px rgba(64, 158, 255, 0.3);
        background: rgba(64, 158, 255, 0.1);
      }
    }

    .stat-icon {
      &.total {
        background: linear-gradient(
          135deg,
          rgba(92, 107, 192, 0.2) 0%,
          rgba(92, 107, 192, 0.15) 100%
        );
        color: #9fa8da;
      }

      &.pending {
        background: linear-gradient(
          135deg,
          rgba(251, 140, 0, 0.2) 0%,
          rgba(251, 140, 0, 0.15) 100%
        );
        color: #ffb74d;
      }

      &.resolved {
        background: linear-gradient(
          135deg,
          rgba(38, 166, 154, 0.2) 0%,
          rgba(38, 166, 154, 0.15) 100%
        );
        color: #4db6ac;
      }

      &.upvotes {
        background: linear-gradient(
          135deg,
          rgba(102, 187, 106, 0.2) 0%,
          rgba(102, 187, 106, 0.15) 100%
        );
        color: #81c784;
      }

      &.downvotes {
        background: linear-gradient(
          135deg,
          rgba(236, 64, 122, 0.2) 0%,
          rgba(236, 64, 122, 0.15) 100%
        );
        color: #f48fb1;
      }
    }

    .stat-value {
      color: var(--el-text-color-primary);
    }

    .stat-label {
      color: var(--el-text-color-secondary);
    }
  }

  .feedback-card {
    background: rgba(20, 20, 24, 0.5);
    backdrop-filter: blur(10px);
    border-color: rgba(60, 60, 67, 0.3);

    &:hover {
      background: rgba(20, 20, 24, 0.7);
      border-color: rgba(80, 80, 87, 0.5);
    }

    .section-text {
      background: rgba(0, 0, 0, 0.3);
      color: var(--el-text-color-primary);
    }

    .sources-section {
      background: rgba(0, 0, 0, 0.2);

      .source-item {
        background: rgba(15, 15, 18, 0.6);
        border-color: rgba(60, 60, 67, 0.3);

        &:hover {
          background: rgba(30, 30, 35, 0.7);
          border-color: rgba(100, 100, 110, 0.5);
          box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
        }

        .source-collection {
          color: #9ca3af;
        }

        .source-similarity {
          background: rgba(0, 0, 0, 0.3);
          color: #94a3b8;
        }

        .source-question {
          color: #cbd5e1;
        }
      }
    }
  }

  .empty-state {
    background: rgba(20, 20, 24, 0.4);
    border-color: rgba(60, 60, 67, 0.3);
    color: #94a3b8;
  }
}

// Responsive
@media (max-width: 1200px) {
  .feedback-cards-container {
    grid-template-columns: 1fr;
    gap: 16px;
  }
}

@media (max-width: 1024px) {
  .feedback-card {
    .sources-section {
      .sources-list {
        .source-item {
          max-width: calc(50% - 4px);
        }
      }
    }
  }
}

@media (max-width: 768px) {
  .stats-section {
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  }

  .feedback-card {
    padding: 14px;

    .feedback-card-header {
      flex-direction: column;
      align-items: flex-start;
      gap: 8px;
      margin-bottom: 12px;
      padding-bottom: 10px;

      .feedback-card-header-left {
        width: 100%;
        gap: 6px;
      }

      .feedback-card-header-right {
        width: 100%;
      }
    }

    .feedback-card-content {
      .feedback-section {
        .section-text {
          max-height: 80px;
        }
      }
    }

    .sources-section {
      .sources-list {
        .source-item {
          max-width: 100%;
          min-width: 100%;
        }
      }
    }

    .feedback-card-actions {
      flex-wrap: wrap;
    }
  }

  .filters-bar {
    flex-direction: column;
    align-items: stretch;

    .filters-left {
      flex-wrap: wrap;
    }
  }
}
</style>

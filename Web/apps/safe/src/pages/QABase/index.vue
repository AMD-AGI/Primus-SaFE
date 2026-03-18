<template>
  <div class="knowledge-base-container">
    <!-- Header -->
    <div class="page-header">
      <el-text class="block textx-18 font-500" tag="b">QA Base</el-text>
    </div>

    <!-- Main Layout: Left (Collections) + Right (Items) -->
    <div class="main-layout">
      <!-- Left Panel: Collections List -->
      <div class="left-panel">
        <div class="panel-header">
          <div class="flex items-center gap-2">
            <el-icon :size="18"><Collection /></el-icon>
            <span class="font-500">QA Collections</span>
          </div>
          <el-button
            type="primary"
            round
            size="small"
            :icon="Plus"
            @click="handleCreateCollection"
            class="text-black"
          >
            New Collection
          </el-button>
        </div>

        <div
          v-loading="loading"
          class="collections-list"
          ref="collectionsListRef"
          @scroll="(e) => handleScrollLoadMore(e, loadMoreCollections)"
        >
          <div
            v-for="collection in collections"
            :key="collection.id"
            :class="['collection-item', { active: selectedCollection?.id === collection.id }]"
            @click="selectCollection(collection)"
          >
            <div class="flex items-center justify-between">
              <span class="collection-name">{{ collection.name }}</span>
              <div class="collection-actions">
                <el-tooltip content="Edit" placement="top">
                  <el-button
                    text
                    :icon="Edit"
                    size="small"
                    @click.stop="handleEditCollection(collection)"
                  />
                </el-tooltip>
                <el-tooltip content="Delete" placement="top">
                  <el-button
                    text
                    :icon="Delete"
                    size="small"
                    @click.stop="handleDeleteCollection(collection)"
                  />
                </el-tooltip>
              </div>
            </div>
            <div class="collection-meta">
              <span class="item-count">{{ collection.item_count || 0 }} items</span>
              <el-icon v-if="collection.is_active" class="check-icon" :size="14"><Check /></el-icon>
            </div>
          </div>

          <!-- Loading more collections -->
          <div v-if="loadingMoreCollections" class="loading-more">
            <el-icon class="is-loading"><Loading /></el-icon>
            <span>Loading more...</span>
          </div>
          <!-- No more collections -->
          <div v-if="collectionHasNoMore && collections.length > 0" class="no-more">
            No more collections
          </div>

          <el-empty v-if="!loading && collections.length === 0" description="No collections yet" />
        </div>
      </div>

      <!-- Right Panel: QA Items -->
      <div class="right-panel">
        <div class="panel-header">
          <div class="flex items-center gap-2">
            <el-icon :size="18"><Document /></el-icon>
            <span class="font-500">QA Items</span>
          </div>

          <!-- Right side: Search + Create Button -->
          <div class="header-right">
            <!-- Vector Search Bar -->
            <div v-if="selectedCollection" class="search-bar-inline">
              <el-input
                v-model="searchQuery"
                placeholder="Vector search..."
                clearable
                @clear="handleClearSearch"
                class="search-input"
              >
                <template #prefix>
                  <el-icon v-if="searching" class="is-loading"><Loading /></el-icon>
                  <el-icon v-else><Search /></el-icon>
                </template>
              </el-input>
              <div class="search-controls-inline">
                <el-tooltip content="Minimum Similarity" placement="top">
                  <div class="slider-wrapper-inline">
                    <el-slider v-model="searchMinSimilarity" :min="0" :max="100" :step="5" />
                    <span class="control-value">{{ searchMinSimilarity }}%</span>
                  </div>
                </el-tooltip>
              </div>
            </div>

            <el-button
              v-if="selectedCollection"
              type="primary"
              round
              size="small"
              :icon="Plus"
              class="text-black"
              @click="handleCreateItem"
            >
              New Item
            </el-button>
          </div>
        </div>

        <div v-if="!selectedCollection" class="empty-state">
          <el-empty description="Select a collection to view items" />
        </div>

        <div v-else class="items-container">
          <div v-loading="itemsLoading || searching" class="items-list" ref="itemsListRef">
            <!-- Search Results Header -->
            <div v-if="hasSearched && searchResults.length > 0" class="search-results-header">
              <el-icon :size="16"><Star /></el-icon>
              <span>Found {{ searchResults.length }} results</span>
            </div>

            <!-- Display Search Results or Regular Items -->
            <div
              v-for="item in hasSearched ? searchResults : items"
              :key="getItemKey(item)"
              class="item-card"
              :class="{ 'search-result': hasSearched }"
            >
              <div class="item-header">
                <div class="item-header-left">
                  <!-- Similarity Badge (only for search results) -->
                  <div v-if="hasSearched" class="item-similarity">
                    <el-icon><Star /></el-icon>
                    <span>{{ getSimilarityPercent(item) }}%</span>
                  </div>
                  <el-tag
                    v-if="!hasSearched"
                    size="small"
                    :type="
                      getPriority(item) === 'high'
                        ? 'danger'
                        : getPriority(item) === 'medium'
                          ? 'warning'
                          : 'primary'
                    "
                    :effect="isDark ? 'plain' : 'light'"
                  >
                    {{ getPriority(item) }}
                  </el-tag>
                  <el-tag v-else size="small" :effect="isDark ? 'plain' : 'light'">
                    {{ getCollectionName(item) }}
                  </el-tag>
                </div>
                <div class="item-actions">
                  <el-tooltip content="Edit" placement="top">
                    <el-button text :icon="Edit" size="small" @click="handleEditClick(item)" />
                  </el-tooltip>
                  <el-tooltip content="Delete" placement="top">
                    <el-button text :icon="Delete" size="small" @click="handleDeleteClick(item)" />
                  </el-tooltip>
                </div>
              </div>
              <div class="item-question">
                <el-text tag="b" class="font-500">Q:</el-text>
                {{ getPrimaryQuestion(item) }}
              </div>
              <div class="item-answer">
                <el-text tag="b" class="font-500">A:</el-text>
                {{ formatAnswerForList(item) }}
              </div>
              <div v-if="getSource(item)" class="item-source">
                <el-icon :size="12"><Document /></el-icon>
                <span>{{ getSource(item) }}</span>
              </div>
              <div v-if="!hasSearched" class="item-footer">
                <span class="item-meta">
                  {{
                    getCreatedAt(item)
                      ? new Date(getCreatedAt(item) as string).toLocaleDateString()
                      : '-'
                  }}
                </span>
                <el-icon v-if="isActive(item)" class="check-icon" :size="14"><Check /></el-icon>
                <el-tag v-else size="small" type="info" :effect="isDark ? 'plain' : 'light'"
                  >Inactive</el-tag
                >
              </div>
            </div>

            <el-empty
              v-if="!itemsLoading && !searching && !hasSearched && items.length === 0"
              description="No items yet"
            />
            <el-empty
              v-if="!searching && hasSearched && searchResults.length === 0"
              description="No results found"
            />
          </div>

          <!-- Pagination -->
          <el-pagination
            v-if="!hasSearched && items.length > 0"
            class="items-pagination"
            :current-page="itemPagination.page"
            :page-size="itemPagination.pageSize"
            :total="itemPagination.total"
            @current-change="handleItemPageChange"
            @size-change="handleItemPageSizeChange"
            layout="total, sizes, prev, pager, next"
            :page-sizes="[10, 20, 50, 100]"
          />
        </div>
      </div>
    </div>

    <!-- Create/Edit Collection Dialog -->
    <el-dialog
      v-model="collectionDialogVisible"
      :title="dialogMode === 'create' ? 'New Collection' : 'Edit Collection'"
      width="600px"
      :close-on-click-modal="false"
    >
      <el-form
        ref="formRef"
        :model="formData"
        :rules="formRules"
        label-width="120px"
        label-position="left"
      >
        <el-form-item label="Name" prop="name">
          <el-input
            v-model="formData.name"
            placeholder="Enter collection name"
            maxlength="100"
            show-word-limit
          />
        </el-form-item>
        <el-form-item label="Description" prop="description">
          <el-input
            v-model="formData.description"
            type="textarea"
            :rows="4"
            placeholder="Enter description (optional)"
            maxlength="500"
            show-word-limit
          />
        </el-form-item>
        <el-form-item label="Status" prop="is_active">
          <el-switch v-model="formData.is_active" active-text="Active" inactive-text="Inactive" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="collectionDialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmitCollection">
          Confirm
        </el-button>
      </template>
    </el-dialog>

    <!-- Create/Edit Item Dialog -->
    <QAEditDialog
      v-model="itemDialogVisible"
      :mode="itemDialogMode"
      :collection-id="currentCollectionId"
      :item-data="itemFormData"
      @success="handleItemDialogSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox, type FormInstance, type FormRules } from 'element-plus'
import {
  Plus,
  Collection,
  Edit,
  Delete,
  Check,
  Document,
  Search,
  Star,
  Loading,
} from '@element-plus/icons-vue'
import { useDark } from '@vueuse/core'
import {
  getQACollectionList,
  createQACollection,
  updateQACollection,
  deleteQACollection,
  getQAItemList,
  getQAItemDetail,
  deleteQAItem,
  searchQAItems,
  type QACollectionListItem,
  type CreateQACollectionRequest,
  type UpdateQACollectionRequest,
  type QAItemData,
  type SearchQAItemResult,
} from '@/services/chatbot'
import QAEditDialog from '@/pages/QABase/Components/QAEditDialog.vue'

const isDark = useDark()
const loading = ref(false)
const collections = ref<QACollectionListItem[]>([])
const selectedCollection = ref<QACollectionListItem | null>(null)

// Collections pagination state
const collectionsListRef = ref<HTMLElement>()
const loadingMoreCollections = ref(false)
const collectionCurrentPage = ref(1)
const collectionPageSize = ref(20)
const collectionHasNoMore = ref(false)

const collectionDialogVisible = ref(false)
const dialogMode = ref<'create' | 'edit'>('create')
const formRef = ref<FormInstance>()
const submitting = ref(false)
const editingCollectionId = ref<number | null>(null)
const formData = ref<CreateQACollectionRequest>({
  name: '',
  description: '',
  is_active: true,
})

const formRules: FormRules = {
  name: [{ required: true, message: 'Please enter collection name', trigger: 'blur' }],
}

// QA Items state
const itemsLoading = ref(false)
const items = ref<QAItemData[]>([])
const itemDialogVisible = ref(false)

// Items pagination state
const itemsListRef = ref<HTMLElement>()
const itemPagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0,
})
const itemDialogMode = ref<'create' | 'edit'>('create')
const currentCollectionId = ref<number | undefined>(undefined)
const itemFormData = ref<{
  id: number
  questions: Array<{ id?: number; question: string }>
  answer: string
  priority: 'low' | 'medium' | 'high'
  is_active: boolean
} | null>(null)

// Vector Search state
const searchQuery = ref('')
const searchMinSimilarity = ref(30)
const searchLimit = ref(10)
const searching = ref(false)
const searchResults = ref<SearchQAItemResult[]>([])
const hasSearched = ref(false)

function isSearchResult(item: QAItemData | SearchQAItemResult): item is SearchQAItemResult {
  return 'answer_id' in item
}

function getItemKey(item: QAItemData | SearchQAItemResult) {
  return isSearchResult(item) ? item.answer_id : item.answer.id
}

function getPriority(item: QAItemData | SearchQAItemResult) {
  return isSearchResult(item) ? undefined : item.answer?.priority
}

function getSource(item: QAItemData | SearchQAItemResult) {
  return isSearchResult(item) ? '' : item.answer?.source
}

function getCreatedAt(item: QAItemData | SearchQAItemResult) {
  return isSearchResult(item) ? '' : item.answer?.created_at
}

function isActive(item: QAItemData | SearchQAItemResult) {
  return !isSearchResult(item) && !!item.answer?.is_active
}

function getCollectionName(item: QAItemData | SearchQAItemResult) {
  return isSearchResult(item) ? item.collection_name : ''
}

function getSimilarityPercent(item: QAItemData | SearchQAItemResult) {
  return isSearchResult(item) ? (item.similarity * 100).toFixed(1) : '0.0'
}

function getPrimaryQuestion(item: QAItemData | SearchQAItemResult) {
  if (isSearchResult(item)) {
    return item.question || ''
  }
  const primary = item.questions?.find((question) => question.is_primary)
  return primary?.question ?? item.questions?.[0]?.question ?? ''
}

function getAnswerText(item: QAItemData | SearchQAItemResult) {
  if (isSearchResult(item)) {
    return item.answer ?? ''
  }
  return item.answer?.answer ?? ''
}

/**
 * Load QA collections list
 */
async function loadCollections(reset = true) {
  if (reset) {
    loading.value = true
    collectionCurrentPage.value = 1
    collectionHasNoMore.value = false
  } else {
    loadingMoreCollections.value = true
  }

  try {
    const response = await getQACollectionList({
      page: collectionCurrentPage.value,
      page_size: collectionPageSize.value,
    })
    if (response.success) {
      if (reset) {
        collections.value = response.data.items
        // Auto select first collection if none selected
        if (collections.value.length > 0 && !selectedCollection.value) {
          selectedCollection.value = collections.value[0]
          await loadItems() // Load items for the first collection
        }
      } else {
        collections.value = [...collections.value, ...response.data.items]
      }

      // Check if there are more pages
      const { page, page_size, total } = response.data.pagination
      collectionHasNoMore.value = page * page_size >= total
    } else {
      ElMessage.error('Failed to load collections')
    }
  } catch (error) {
    console.error('Failed to load collections:', error)
    ElMessage.error('Failed to load collections: ' + (error as Error).message)
  } finally {
    loading.value = false
    loadingMoreCollections.value = false
  }
}

/**
 * Load more collections
 */
async function loadMoreCollections() {
  if (loadingMoreCollections.value || collectionHasNoMore.value) {
    return
  }

  collectionCurrentPage.value += 1
  await loadCollections(false)
}

/**
 * Generic scroll handler - load more when scrolled to bottom
 */
function handleScrollLoadMore(event: Event, loadMoreFn: () => void) {
  const target = event.target as HTMLElement
  const scrollTop = target.scrollTop
  const scrollHeight = target.scrollHeight
  const clientHeight = target.clientHeight

  // Load more when scrolled to bottom (with 50px threshold)
  if (scrollTop + clientHeight >= scrollHeight - 50) {
    loadMoreFn()
  }
}

/**
 * Select a collection
 */
function selectCollection(collection: QACollectionListItem) {
  selectedCollection.value = collection
  // Clear search results when switching collections
  searchQuery.value = ''
  searchResults.value = []
  hasSearched.value = false
  // Reset pagination
  itemPagination.page = 1
  loadItems()
}

/**
 * Open create collection dialog
 */
function handleCreateCollection() {
  dialogMode.value = 'create'
  formData.value = {
    name: '',
    description: '',
    is_active: true,
  }
  collectionDialogVisible.value = true
}

/**
 * Open edit collection dialog
 */
function handleEditCollection(collection: QACollectionListItem) {
  dialogMode.value = 'edit'
  editingCollectionId.value = collection.id
  formData.value = {
    name: collection.name,
    description: collection.description,
    is_active: collection.is_active,
  }
  collectionDialogVisible.value = true
}

/**
 * Delete collection
 */
function handleDeleteCollection(collection: QACollectionListItem) {
  ElMessageBox.confirm(
    `Are you sure you want to delete collection "${collection.name}"?`,
    'Confirm',
    {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    },
  )
    .then(async () => {
      try {
        const response = await deleteQACollection(collection.id)
        if (response.success) {
          ElMessage.success('Collection deleted successfully')
          if (selectedCollection.value?.id === collection.id) {
            selectedCollection.value = null
            items.value = []
          }
          await loadCollections()
        } else {
          ElMessage.error('Failed to delete collection')
        }
      } catch (error) {
        console.error('Failed to delete collection:', error)
        ElMessage.error('Failed to delete collection: ' + (error as Error).message)
      }
    })
    .catch(() => {
      // User cancelled
    })
}

/**
 * Submit collection form
 */
async function handleSubmitCollection() {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (!valid) return

    submitting.value = true
    try {
      if (dialogMode.value === 'create') {
        const response = await createQACollection(formData.value)
        if (response.success) {
          ElMessage.success('Collection created successfully')
          collectionDialogVisible.value = false
          await loadCollections()
        } else {
          ElMessage.error('Failed to create collection')
        }
      } else {
        // Edit mode
        if (!editingCollectionId.value) {
          ElMessage.error('Invalid collection ID')
          return
        }
        const updateData: UpdateQACollectionRequest = formData.value
        const response = await updateQACollection(editingCollectionId.value, updateData)
        if (response.success) {
          ElMessage.success('Collection updated successfully')
          collectionDialogVisible.value = false
          await loadCollections()
        } else {
          ElMessage.error('Failed to update collection')
        }
      }
    } catch (error) {
      console.error('Failed to submit:', error)
      ElMessage.error('Operation failed: ' + (error as Error).message)
    } finally {
      submitting.value = false
    }
  })
}

// ========== QA Items Management ==========

/**
 * Load QA items for selected collection
 */
async function loadItems() {
  if (!selectedCollection.value) return

  itemsLoading.value = true

  try {
    const res = await getQAItemList({
      collection_id: selectedCollection.value.id,
      page: itemPagination.page,
      page_size: itemPagination.pageSize,
    })

    const payload = (res as { data?: typeof res }).data ?? res
    const newItems = payload.items ?? []
    items.value = newItems
    // total is at the top level of the response object
    itemPagination.total = payload.total ?? newItems.length
  } catch (error) {
    console.error('Failed to load items:', error)
  } finally {
    itemsLoading.value = false
  }
}

/**
 * Handle item page change
 */
function handleItemPageChange(newPage: number) {
  itemPagination.page = newPage
  loadItems()
}

/**
 * Handle item page size change
 */
function handleItemPageSizeChange(newSize: number) {
  itemPagination.pageSize = newSize
  itemPagination.page = 1
  loadItems()
}

/**
 * Open create item dialog
 */
function handleCreateItem() {
  if (!selectedCollection.value) return

  itemDialogMode.value = 'create'
  currentCollectionId.value = selectedCollection.value.id
  itemFormData.value = null
  itemDialogVisible.value = true
}

/**
 * Open edit item dialog
 */
function handleEditItem(item: QAItemData) {
  itemDialogMode.value = 'edit'
  currentCollectionId.value = item.answer?.collection_id
  const questionList: Array<{ id?: number; question: string; is_primary?: boolean }> =
    item.questions.map((question) => ({
      id: question.id,
      question: question.question,
      is_primary: question.is_primary,
    }))
  if (questionList.length === 0) {
    questionList.push({
      id: undefined,
      question: getPrimaryQuestion(item),
      is_primary: true,
    })
  }
  const primaryIndex = questionList.findIndex((q) => q.is_primary)
  if (primaryIndex > 0) {
    const [primary] = questionList.splice(primaryIndex, 1)
    questionList.unshift(primary)
  }
  itemFormData.value = {
    id: item.answer?.id ?? 0,
    questions: questionList.map((q) => ({ id: q.id, question: q.question })),
    answer: getAnswerText(item),
    priority: item.answer?.priority ?? 'medium',
    is_active: item.answer?.is_active ?? true,
  }
  itemDialogVisible.value = true
}

/**
 * Open edit dialog from search result
 */
async function handleEditSearchItem(item: SearchQAItemResult) {
  try {
    const res = await getQAItemDetail(item.answer_id)
    const detail = (res as { data?: QAItemData }).data ?? (res as QAItemData)
    handleEditItem(detail)
  } catch (error) {
    console.error('Failed to load item detail:', error)
    ElMessage.error('Failed to load item detail: ' + (error as Error).message)
  }
}

function handleEditClick(item: QAItemData | SearchQAItemResult) {
  if (isSearchResult(item)) {
    handleEditSearchItem(item)
  } else {
    handleEditItem(item)
  }
}

/**
 * Delete item
 */
function handleDeleteItem(item: QAItemData) {
  ElMessageBox.confirm(`Are you sure you want to delete this Q&A item?`, 'Confirm', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      try {
        const response = await deleteQAItem(item.answer.id)
        if (response.success) {
          ElMessage.success('Item deleted successfully')
          await loadItems()
          await loadCollections() // Refresh to update item count
        } else {
          ElMessage.error('Failed to delete item')
        }
      } catch (error) {
        console.error('Failed to delete item:', error)
        ElMessage.error('Failed to delete item: ' + (error as Error).message)
      }
    })
    .catch(() => {
      // User cancelled
    })
}

/**
 * Delete item from search result
 */
function handleDeleteSearchItem(item: SearchQAItemResult) {
  ElMessageBox.confirm(`Are you sure you want to delete this Q&A item?`, 'Confirm', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  })
    .then(async () => {
      try {
        const response = await deleteQAItem(item.answer_id)
        if (response.success) {
          ElMessage.success('Item deleted successfully')
          if (hasSearched.value) {
            await handleSearch()
          } else {
            await loadItems()
          }
          await loadCollections() // Refresh to update item count
        } else {
          ElMessage.error('Failed to delete item')
        }
      } catch (error) {
        console.error('Failed to delete item:', error)
        ElMessage.error('Failed to delete item: ' + (error as Error).message)
      }
    })
    .catch(() => {
      // User cancelled
    })
}

function handleDeleteClick(item: QAItemData | SearchQAItemResult) {
  if (isSearchResult(item)) {
    handleDeleteSearchItem(item)
  } else {
    handleDeleteItem(item)
  }
}

/**
 * Handle item dialog success
 */
async function handleItemDialogSuccess() {
  if (hasSearched.value) {
    await handleSearch()
  } else {
    await loadItems()
  }
  await loadCollections() // Refresh to update item count
}

type RichTextBlock = {
  type?: string
  content?: string
  items?: string[]
  url?: string
}

type RichTextDoc = {
  blocks?: RichTextBlock[]
}

function formatAnswerForList(item: QAItemData | SearchQAItemResult) {
  const answerType = isSearchResult(item) ? item.answer_type : item.answer?.answer_type
  const answerText = isSearchResult(item) ? (item.answer ?? '') : (item.answer?.answer ?? '')
  if (answerType !== 'richtext') return answerText
  try {
    const doc = JSON.parse(answerText || '') as RichTextDoc
    const blocks = Array.isArray(doc?.blocks) ? doc.blocks : []
    const texts: string[] = []
    for (const b of blocks) {
      if (b?.type === 'paragraph' || b?.type === 'heading') {
        if (typeof b.content === 'string') texts.push(b.content)
      } else if (b?.type === 'list' && Array.isArray(b.items)) {
        texts.push(b.items.join(' '))
      } else if (b?.type === 'code' && typeof b.content === 'string') {
        texts.push(b.content)
      } else if (b?.type === 'image' && typeof b.url === 'string') {
        texts.push('[image]')
      }
      if (texts.join(' ').length > 200) break
    }
    return texts.join(' ').slice(0, 200) || '[rich text]'
  } catch {
    return '[rich text]'
  }
}

// ========== Vector Search ==========

/**
 * Execute vector search
 */
async function handleSearch() {
  if (!searchQuery.value.trim()) {
    // Clear results if query is empty
    handleClearSearch()
    return
  }

  searching.value = true
  hasSearched.value = true
  try {
    const res = await searchQAItems({
      query: searchQuery.value,
      limit: searchLimit.value,
      min_similarity: searchMinSimilarity.value / 100,
      collection_id: selectedCollection.value?.id,
    })

    searchResults.value = res.results
  } catch (error) {
    console.error('Search failed:', error)
    ElMessage.error('Search failed: ' + (error as Error).message)
  } finally {
    searching.value = false
  }
}

/**
 * Clear search and return to normal list
 */
function handleClearSearch() {
  searchQuery.value = ''
  searchResults.value = []
  hasSearched.value = false
  searchMinSimilarity.value = 30
  searchLimit.value = 10
  // Reload normal items
  if (selectedCollection.value) {
    loadItems()
  }
}

// Watch search query and auto search with debounce
let searchTimeout: number | null = null
watch([searchQuery, searchMinSimilarity, searchLimit], () => {
  if (searchTimeout) {
    clearTimeout(searchTimeout)
  }

  if (!searchQuery.value.trim()) {
    if (hasSearched.value || searchResults.value.length > 0) {
      handleClearSearch()
    }
    return
  }

  // Debounce search for 800ms
  searchTimeout = window.setTimeout(() => {
    if (searchQuery.value.trim()) {
      handleSearch()
    }
  }, 800)
})

onMounted(() => {
  loadCollections()
})
</script>

<style scoped lang="scss">
.knowledge-base-container {
  height: 100%;
  display: flex;
  flex-direction: column;

  .page-header {
    padding: 16px 20px;
  }

  .main-layout {
    display: flex;
    gap: 16px;
    flex: 1;
    overflow: hidden;
    padding: 0 20px 20px;
  }

  .left-panel,
  .right-panel {
    background-color: var(--safe-card);
    border: 1px solid var(--safe-border);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .left-panel {
    width: 360px;
    flex-shrink: 0;

    .panel-header {
      padding: 16px 20px;
      gap: 12px;
    }
  }

  .right-panel {
    flex: 1;
    min-width: 0;
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 20px 24px;
    border-bottom: 1px solid var(--safe-border);
    gap: 24px;
    flex-wrap: wrap;

    .header-right {
      display: flex;
      align-items: center;
      gap: 16px;
      flex: 1;
      justify-content: flex-end;
      min-width: 300px;
    }

    @media (max-width: 1400px) {
      gap: 16px;

      .header-right {
        flex-basis: 100%;
        justify-content: flex-start;
      }
    }
  }

  .collections-list {
    flex: 1;
    overflow-y: auto;
    padding: 8px;
  }

  .items-container {
    flex: 1;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .items-list {
    flex: 1;
    overflow-y: auto;
    padding: 8px;
  }

  .items-pagination {
    padding: 16px 20px;
    border-top: 1px solid var(--safe-border);
    display: flex;
    justify-content: flex-start;
  }

  .collection-item {
    padding: 12px;
    margin-bottom: 4px;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s;
    border: 1px solid transparent;

    &:hover {
      background-color: var(--safe-card-2);
    }

    &.active {
      background-color: var(--safe-primary-plain-bg);
      border-color: var(--safe-primary-plain-border);

      .collection-name {
        color: var(--safe-primary);
        font-weight: 500;
      }
    }

    .collection-name {
      font-size: 14px;
      color: var(--safe-text);
    }

    .collection-actions {
      display: none;
      gap: 4px;
    }

    &:hover .collection-actions {
      display: flex;
    }

    .collection-meta {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-top: 6px;
      font-size: 12px;
      color: var(--safe-muted);

      .check-icon {
        color: var(--safe-green);
      }
    }
  }

  .empty-state {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
  }

  .item-card {
    padding: 16px;
    margin-bottom: 12px;
    background-color: var(--safe-card-2);
    border: 1px solid var(--safe-border);
    border-radius: 8px;
    transition: all 0.2s;

    &:hover {
      box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
      border-color: var(--safe-primary-plain-border);
    }

    .item-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 12px;

      .item-actions {
        display: none;
        gap: 4px;
      }
    }

    &:hover .item-actions {
      display: flex;
    }

    .item-question {
      margin-bottom: 8px;
      font-size: 14px;
      color: var(--safe-text);
      line-height: 1.6;

      .el-text {
        margin-right: 8px;
        color: var(--safe-primary);
      }
    }

    .item-answer {
      margin-bottom: 8px;
      font-size: 13px;
      color: var(--safe-muted);
      line-height: 1.6;

      .el-text {
        margin-right: 8px;
        color: var(--safe-primary);
      }
    }

    .item-source {
      display: flex;
      align-items: center;
      gap: 4px;
      margin-bottom: 8px;
      font-size: 12px;
      color: var(--safe-muted);
    }

    .item-footer {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding-top: 8px;
      border-top: 1px solid var(--safe-border);

      .item-meta {
        font-size: 12px;
        color: var(--safe-muted);
      }

      .check-icon {
        color: var(--safe-green);
      }
    }
  }

  // Vector Search Bar Inline
  .search-bar-inline {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;

    .search-input {
      width: 280px;
      min-width: 200px;
    }

    .search-controls-inline {
      display: flex;
      align-items: center;
      gap: 12px;
      flex-wrap: wrap;

      .slider-wrapper-inline {
        display: flex;
        align-items: center;
        gap: 8px;
        min-width: 140px;

        .el-slider {
          flex: 1;
          min-width: 80px;
        }

        .control-value {
          font-size: 13px;
          font-weight: 500;
          color: var(--safe-primary);
          min-width: 38px;
          text-align: right;
        }
      }

      .el-input-number {
        width: 90px;
      }
    }

    @media (max-width: 1200px) {
      gap: 8px;

      .search-input {
        width: 220px;
      }

      .search-controls-inline {
        gap: 8px;

        .slider-wrapper-inline {
          min-width: 120px;
        }

        .el-input-number {
          width: 80px;
        }
      }
    }

    @media (max-width: 768px) {
      flex-direction: column;
      align-items: stretch;
      width: 100%;

      .search-input {
        width: 100%;
      }

      .search-controls-inline {
        width: 100%;
        justify-content: space-between;
      }
    }
  }

  .search-results-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px 16px;
    margin-bottom: 12px;
    background: var(--safe-primary-plain-bg);
    border: 1px solid var(--safe-primary-plain-border);
    border-radius: 8px;
    font-size: 14px;
    font-weight: 500;
    color: var(--safe-primary);

    .el-icon {
      color: var(--safe-primary);
    }
  }

  .item-card.search-result {
    border-color: var(--safe-primary-plain-border);
    background: linear-gradient(to right, rgba(59, 130, 246, 0.02), transparent);

    &:hover {
      border-color: var(--safe-primary);
    }
  }

  .item-header-left {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .item-similarity {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 10px;
    background: var(--safe-primary-plain-bg);
    border: 1px solid var(--safe-primary-plain-border);
    border-radius: 12px;
    font-size: 12px;
    font-weight: 600;
    color: var(--safe-primary);

    .el-icon {
      color: var(--safe-primary);
      font-size: 14px;
    }
  }

  // Common action buttons styles (for edit/delete buttons)
  .collection-actions,
  .item-actions {
    .el-button {
      transition: all 0.2s;

      &:hover {
        transform: scale(1.1);
      }

      // Edit button (first child) - primary color on hover
      &:first-child:hover {
        color: var(--safe-primary);
      }

      // Delete button (second child) - danger color on hover
      &:last-child:hover {
        color: #f56c6c !important;
      }
    }
  }

  // Dialog styles
  // :deep(.el-dialog__body) {
  //   padding: 14px 32px;
  // }

  // Loading more and no more indicators (for collections)
  .loading-more {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 12px;
    color: var(--safe-muted);
    font-size: 13px;

    .el-icon {
      font-size: 16px;
    }
  }

  .no-more {
    text-align: center;
    padding: 12px;
    color: var(--safe-muted);
    font-size: 12px;
  }
}
</style>

<template>
  <div class="resources-tab">
    <!-- Action bar -->
    <div class="flex flex-wrap items-center mb-4">
      <el-button type="primary" round :icon="Plus" @click="showCreate = true">
        Create Resource
      </el-button>
      <div class="ml-auto flex gap-4 items-center">
        <el-segmented v-model="typeFilter" :options="typeOptions" @change="handleFilterChange" />
      </div>
    </div>

    <!-- Cards -->
    <div v-loading="loading" class="content-area">
      <div v-if="list.length > 0" class="resources-grid">
        <div
          v-for="item in list"
          :key="item.id"
          class="resource-card"
          @click="handleDetail(item)"
        >
          <div class="card-header">
            <span class="resource-name">{{ item.name }}</span>
            <el-tag size="small" :type="item.type === 'gpu' ? 'success' : 'primary'" effect="light">
              {{ item.type.toUpperCase() }}
            </el-tag>
          </div>
          <div class="card-specs">
            <span v-if="item.resources.gpu" class="spec-item">GPU: {{ item.resources.gpu }}</span>
            <span v-if="item.resources.cpu" class="spec-item">CPU: {{ item.resources.cpu }}</span>
            <span v-if="item.resources.memory" class="spec-item">Mem: {{ item.resources.memory }}</span>
          </div>
          <div class="card-image" v-if="item.image">
            <span class="image-label">Image:</span>
            <span class="image-value">{{ item.image }}</span>
          </div>
          <div class="card-footer">
            <span class="footer-version">v{{ item.version || '–' }}</span>
            <span class="footer-date">{{ formatDate(item.created_at) }}</span>
          </div>
        </div>
      </div>
      <el-empty v-else-if="!loading" description="No resources found" />
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
    <ResourceFormDialog
      v-model:visible="showCreate"
      :resource-id="editId"
      @success="onFormSuccess"
      @close="editId = undefined"
    />

    <!-- Detail dialog -->
    <ResourceDetailDialog
      v-model:visible="showDetail"
      :resource="detailItem"
      @edit="handleEdit"
      @deleted="fetchList"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { getResources, type Resource } from '@/services/tools'
import ResourceFormDialog from './ResourceFormDialog.vue'
import ResourceDetailDialog from './ResourceDetailDialog.vue'

const loading = ref(false)
const list = ref<Resource[]>([])
const showCreate = ref(false)
const showDetail = ref(false)
const editId = ref<number | undefined>()
const detailItem = ref<Resource | null>(null)
const typeFilter = ref('')

const typeOptions = [
  { label: 'All', value: '' },
  { label: 'GPU', value: 'gpu' },
  { label: 'CPU', value: 'cpu' },
]

const pagination = reactive({ page: 1, pageSize: 12, total: 0 })

const formatDate = (s: string) => s.split(' ')[0]

const fetchList = async () => {
  loading.value = true
  try {
    const res = await getResources({
      offset: (pagination.page - 1) * pagination.pageSize,
      limit: pagination.pageSize,
      type: (typeFilter.value as 'gpu' | 'cpu') || undefined,
    })
    list.value = res.resources || []
    pagination.total = res.total || 0
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to load resources')
  } finally {
    loading.value = false
  }
}

const handleFilterChange = () => {
  pagination.page = 1
  fetchList()
}

const handlePageSizeChange = () => {
  pagination.page = 1
  fetchList()
}

const handleDetail = (item: Resource) => {
  detailItem.value = item
  showDetail.value = true
}

const handleEdit = (id: number) => {
  showDetail.value = false
  editId.value = id
  showCreate.value = true
}

const onFormSuccess = () => {
  editId.value = undefined
  fetchList()
}

onMounted(fetchList)
</script>

<style scoped lang="scss">
.resources-tab {
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

.resources-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 16px;
}

.resource-card {
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
  }

  .resource-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--safe-text, var(--el-text-color-primary));
  }

  .card-specs {
    display: flex;
    gap: 12px;
    font-size: 13px;

    .spec-item {
      color: var(--safe-primary, var(--el-color-primary));
      font-weight: 500;
    }
  }

  .card-image {
    font-size: 12px;
    color: var(--safe-muted, var(--el-text-color-secondary));
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;

    .image-label { font-weight: 500; margin-right: 4px; }
    .image-value { font-family: 'Monaco', 'Consolas', monospace; }
  }

  .card-footer {
    display: flex;
    justify-content: space-between;
    font-size: 12px;
    color: var(--safe-muted, var(--el-text-color-secondary));
    padding-top: 8px;
    border-top: 1px solid color-mix(in oklab, var(--safe-border, var(--el-border-color)) 40%, transparent 60%);
    margin-top: auto;

    .footer-version {
      color: var(--safe-primary, var(--el-color-primary));
      font-weight: 500;
    }
  }
}

.pagination-container {
  padding: 8px 0;
  flex-shrink: 0;
}
</style>

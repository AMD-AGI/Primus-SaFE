<template>
  <el-dialog
    :model-value="visible"
    @update:model-value="$emit('update:visible', $event)"
    title="Dataset Detail"
    width="900px"
    :close-on-click-modal="false"
    destroy-on-close
  >
    <div v-loading="loading" :element-loading-text="$loadingText" class="p-y-3 p-x-5">
      <template v-if="detail">
        <!-- Basic information -->
        <div class="flex items-center m-b-4">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Basic Information</span>
        </div>

        <el-descriptions :column="2" border class="m-b-6">
          <el-descriptions-item label="Name">
            {{ detail.displayName }}
          </el-descriptions-item>
          <el-descriptions-item label="Dataset ID">
            {{ detail.datasetId }}
          </el-descriptions-item>
          <el-descriptions-item label="Source">
            <el-tag
              v-if="detail.source === 'huggingface'"
              type="warning"
              :effect="isDark ? 'plain' : 'light'"
            >
              HuggingFace
            </el-tag>
            <el-tag
              v-else-if="detail.source === 'upload'"
              type="info"
              :effect="isDark ? 'plain' : 'light'"
            >
              Upload
            </el-tag>
            <span v-else>-</span>
          </el-descriptions-item>
          <el-descriptions-item label="Source URL" v-if="detail.source === 'huggingface'">
            <el-link
              v-if="detail.sourceUrl"
              :href="detail.sourceUrl"
              target="_blank"
              type="primary"
              :underline="false"
            >
              {{ detail.sourceUrl }}
              <el-icon class="ml-1"><Link /></el-icon>
            </el-link>
            <span v-else>-</span>
          </el-descriptions-item>
          <el-descriptions-item label="Type">
            <el-tag v-if="detail.datasetType" :effect="isDark ? 'plain' : 'light'">{{
              detail.datasetType
            }}</el-tag>
            <span v-else>-</span>
          </el-descriptions-item>
          <el-descriptions-item label="Status">
            <el-tag
              v-if="detail.status"
              :type="
                detail.status === 'Ready' ? 'success' : detail.status === 'Failed' ? 'danger' : ''
              "
            >
              {{ detail.status }}
            </el-tag>
            <span v-else>-</span>
          </el-descriptions-item>
          <el-descriptions-item label="Workspace">
            {{ detail.workspaceName || detail.workspace || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="Size">
            {{ detail.totalSizeStr || '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="Creation Time">
            {{
              detail.creationTime ? dayjs(detail.creationTime).format('YYYY-MM-DD HH:mm:ss') : '-'
            }}
          </el-descriptions-item>
          <el-descriptions-item label="Update Time">
            {{ detail.updateTime ? dayjs(detail.updateTime).format('YYYY-MM-DD HH:mm:ss') : '-' }}
          </el-descriptions-item>
          <el-descriptions-item label="Description" :span="2">
            {{ detail.description || '-' }}
          </el-descriptions-item>
        </el-descriptions>

        <!-- File list -->
        <div class="flex items-center m-b-4 m-t-6">
          <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
          <span class="textx-15 font-medium">Files</span>
          <span class="text-gray-400 text-[12px] ml-2"
            >({{ detail.files?.length || 0 }} files)</span
          >
        </div>

        <el-table :data="detail.files" border max-height="400">
          <el-table-column prop="fileName" label="File Name" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">
              {{ row.fileName }}
            </template>
          </el-table-column>
          <el-table-column prop="sizeStr" label="Size" width="120">
            <template #default="{ row }">
              {{ row.sizeStr || '-' }}
            </template>
          </el-table-column>
          <el-table-column label="Actions" width="100" align="center">
            <template #default="{ row }">
              <el-button
                type="primary"
                link
                size="small"
                @click="handlePreviewFile(row)"
                :loading="previewingFile === row.filePath"
              >
                Preview
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="$emit('update:visible', false)">Close</el-button>
      </div>
    </template>
  </el-dialog>

  <!-- File preview dialog -->
  <el-dialog
    v-model="showPreviewDialog"
    :title="`Preview: ${previewFileName}`"
    width="85%"
    top="5vh"
    destroy-on-close
    :close-on-click-modal="false"
  >
    <div v-loading="loadingPreview" :element-loading-text="$loadingText">
      <el-scrollbar v-if="previewContent" max-height="75vh">
        <pre class="preview-content">{{ previewContent }}</pre>
      </el-scrollbar>
      <el-empty v-else description="No content available" />
    </div>
    <template #footer>
      <div class="flex justify-between items-center">
        <el-text type="info" size="small">
          {{ previewContent ? `${previewContent.split('\n').length} lines` : '' }}
        </el-text>
        <el-button @click="showPreviewDialog = false">Close</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Link } from '@element-plus/icons-vue'
import { getDatasetDetail, previewDatasetFile } from '@/services/dataset'
import type { DatasetDetail, DatasetFile } from '@/services/dataset/type'
import dayjs from 'dayjs'
import { useDark } from '@vueuse/core'

const props = defineProps<{
  visible: boolean
  datasetId: string
}>()

defineEmits(['update:visible'])

const isDark = useDark()
const loading = ref(false)
const detail = ref<DatasetDetail | null>(null)
const showPreviewDialog = ref(false)
const loadingPreview = ref(false)
const previewContent = ref('')
const previewFileName = ref('')
const previewingFile = ref('')

const fetchDetail = async () => {
  if (!props.datasetId) return

  try {
    loading.value = true
    detail.value = await getDatasetDetail(props.datasetId)
  } catch (error) {
    console.error('Failed to fetch dataset detail:', error)
    ElMessage.error('Failed to load dataset detail')
  } finally {
    loading.value = false
  }
}

const formatContent = (content: string): string => {
  // Try to parse and format JSON
  try {
    const parsed = JSON.parse(content)
    return JSON.stringify(parsed, null, 2)
  } catch {
    // If not JSON, return original content
    return content
  }
}

const handlePreviewFile = async (file: DatasetFile) => {
  if (!props.datasetId || !file.filePath) return

  try {
    previewingFile.value = file.filePath
    loadingPreview.value = true
    showPreviewDialog.value = true

    const response = await previewDatasetFile(props.datasetId, file.filePath)

    // Extract fileName and content
    previewFileName.value = response.fileName || file.fileName
    previewContent.value = formatContent(response.content)
  } catch (error) {
    console.error('Failed to preview file:', error)
    ElMessage.error('Failed to preview file')
    showPreviewDialog.value = false
  } finally {
    loadingPreview.value = false
    previewingFile.value = ''
  }
}

watch(
  () => props.visible,
  (val) => {
    if (val) {
      fetchDetail()
    } else {
      detail.value = null
      previewContent.value = ''
      showPreviewDialog.value = false
    }
  },
)
</script>

<style scoped>
.preview-content {
  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'Courier New', monospace;
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-wrap: break-word;
  margin: 0;
  padding: 20px;
  background-color: var(--el-fill-color-light);
  border-radius: 6px;
  color: var(--el-text-color-primary);
  tab-size: 2;
  -moz-tab-size: 2;
}

/* JSON syntax highlighting effect (differentiated by color) */
:deep(.el-scrollbar__view) {
  min-height: 100px;
}
</style>

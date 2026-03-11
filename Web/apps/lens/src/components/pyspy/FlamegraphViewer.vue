<template>
  <el-dialog
    v-model="dialogVisible"
    :title="dialogTitle"
    :width="isFullscreen ? '100%' : '90%'"
    :fullscreen="isFullscreen"
    :close-on-click-modal="false"
    destroy-on-close
    class="flamegraph-dialog"
  >
    <!-- Toolbar -->
    <div class="flamegraph-toolbar">
      <div class="toolbar-left">
        <el-input
          v-model="searchText"
          placeholder="Search functions..."
          :prefix-icon="Search"
          clearable
          size="default"
          style="width: 300px"
          @input="handleSearch"
        />
        <el-tag v-if="matchCount > 0" type="success" size="small">
          {{ matchCount }} matches
        </el-tag>
      </div>
      <div class="toolbar-right">
        <el-tooltip content="Toggle Fullscreen" placement="top">
          <el-button :icon="isFullscreen ? Minus : FullScreen" @click="toggleFullscreen" />
        </el-tooltip>
        <el-tooltip content="Reset View" placement="top">
          <el-button :icon="RefreshRight" @click="resetView" />
        </el-tooltip>
      </div>
    </div>

    <div v-loading="loading" class="flamegraph-container" :class="{ 'is-fullscreen': isFullscreen }">
      <div 
        v-if="!loading && svgContent" 
        ref="svgContainerRef"
        class="flamegraph-content" 
        v-html="processedSvgContent"
      ></div>
      
      <el-empty
        v-else-if="!loading && !svgContent"
        description="Failed to load flamegraph"
      >
        <el-button type="primary" @click="loadFlamegraph">Retry</el-button>
      </el-empty>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <div class="footer-left">
          <el-tag type="info" size="small">Task: {{ taskId }}</el-tag>
          <el-tag type="info" size="small">Format: {{ format }}</el-tag>
        </div>
        <el-space>
          <el-button @click="dialogVisible = false">Close</el-button>
          <el-button 
            v-if="format === 'speedscope'" 
            type="success" 
            :icon="Link"
            @click="openInSpeedscope"
          >
            Open in Speedscope
          </el-button>
          <el-button type="primary" :icon="Download" @click="downloadFile">
            Download
          </el-button>
        </el-space>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Download, Search, FullScreen, Minus, RefreshRight, Link } from '@element-plus/icons-vue'
import { getPySpyFileContent } from '@/services/pyspy'

interface Props {
  visible: boolean
  taskId: string
  format: 'flamegraph' | 'speedscope'
  cluster?: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

const loading = ref(false)
const svgContent = ref('')
const searchText = ref('')
const matchCount = ref(0)
const isFullscreen = ref(false)
const svgContainerRef = ref<HTMLElement | null>(null)

const dialogVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

const dialogTitle = computed(() => {
  return `Flamegraph Viewer - ${props.taskId}`
})

// Process SVG content to highlight search matches
const processedSvgContent = computed(() => {
  if (!svgContent.value || !searchText.value) {
    return svgContent.value
  }
  
  // Simple search highlighting for SVG text elements
  const search = searchText.value.toLowerCase()
  let count = 0
  
  // Replace matching text in SVG with highlighted version
  const processed = svgContent.value.replace(
    /(<text[^>]*>)([^<]+)(<\/text>)/gi,
    (match, openTag, text, closeTag) => {
      if (text.toLowerCase().includes(search)) {
        count++
        // Add highlight class to matching text
        return openTag.replace('<text', '<text class="search-match"') + text + closeTag
      }
      return match
    }
  )
  
  matchCount.value = count
  return processed
})

const getFilename = () => {
  if (props.format === 'flamegraph') return 'profile.svg'
  if (props.format === 'speedscope') return 'profile.json'
  return 'profile.txt'
}

const loadFlamegraph = async () => {
  if (!props.taskId) return

  loading.value = true
  searchText.value = ''
  matchCount.value = 0
  
  try {
    const filename = getFilename()
    const content: any = await getPySpyFileContent(
      props.taskId,
      filename,
      props.cluster
    )
    svgContent.value = content
  } catch (error) {
    ElMessage.error('Failed to load flamegraph')
    console.error('Load flamegraph error:', error)
  } finally {
    loading.value = false
  }
}

const handleSearch = () => {
  // Search is handled reactively via processedSvgContent
  // This function can be used for debouncing if needed
}

const toggleFullscreen = () => {
  isFullscreen.value = !isFullscreen.value
}

const resetView = () => {
  searchText.value = ''
  matchCount.value = 0
  
  // Reset SVG view if it has zoom/pan
  if (svgContainerRef.value) {
    const svg = svgContainerRef.value.querySelector('svg')
    if (svg) {
      svg.style.transform = ''
    }
  }
}

const downloadFile = () => {
  const baseUrl = import.meta.env.BASE_URL || ''
  const filename = getFilename()
  let url = `${baseUrl}v1/pyspy/file/${props.taskId}/${filename}`
  
  if (props.cluster) {
    url += `?cluster=${encodeURIComponent(props.cluster)}`
  }
  
  const a = document.createElement('a')
  a.href = url
  a.download = `${props.taskId}-${filename}`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

const openInSpeedscope = () => {
  // For speedscope format, we can either:
  // 1. Open speedscope.app with the file URL
  // 2. Download and let user import manually
  
  // Option 1: Direct link (requires CORS to be configured)
  // const baseUrl = window.location.origin
  // const fileUrl = `${baseUrl}/v1/pyspy/file/${props.taskId}/${getFilename()}`
  // const speedscopeUrl = `https://www.speedscope.app/#profileURL=${encodeURIComponent(fileUrl)}`
  // window.open(speedscopeUrl, '_blank')
  
  // Option 2: Download file and show instructions
  ElMessage.info('Download the JSON file and import it at speedscope.app')
  downloadFile()
  window.open('https://www.speedscope.app/', '_blank')
}

watch(() => props.visible, (newVal) => {
  if (newVal && props.taskId) {
    loadFlamegraph()
  }
  if (!newVal) {
    isFullscreen.value = false
  }
})
</script>

<style scoped lang="scss">
.flamegraph-dialog {
  :deep(.el-dialog__body) {
    padding: 0 20px 20px 20px;
    max-height: 70vh;
    overflow: auto;
  }
  
  :deep(.el-dialog__header) {
    padding-bottom: 10px;
    border-bottom: 1px solid var(--el-border-color-lighter);
  }
  
  &:deep(.el-dialog.is-fullscreen) {
    .el-dialog__body {
      max-height: calc(100vh - 150px);
    }
  }
}

.flamegraph-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 0;
  margin-bottom: 12px;
  border-bottom: 1px solid var(--el-border-color-lighter);
  
  .toolbar-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  
  .toolbar-right {
    display: flex;
    gap: 8px;
  }
}

.flamegraph-container {
  min-height: 400px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--el-fill-color-light);
  border-radius: 8px;
  overflow: auto;
  
  &.is-fullscreen {
    min-height: calc(100vh - 250px);
  }
}

.flamegraph-content {
  width: 100%;
  padding: 16px;
  
  :deep(svg) {
    width: 100%;
    height: auto;
    
    // Highlight search matches
    .search-match {
      fill: #ff6b6b !important;
      font-weight: bold;
    }
  }
}

.dialog-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  
  .footer-left {
    display: flex;
    gap: 8px;
  }
}
</style>

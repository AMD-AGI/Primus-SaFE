<template>
  <div ref="wrapperRef" class="node-detail-wrapper" :style="{ width: containerWidth }">
    <el-card class="safe-card" shadow="never">
      <el-table :data="nodes" v-loading="loading" size="small" :max-height="360">
        <el-table-column label="Node" prop="node" min-width="250" show-overflow-tooltip />
        <el-table-column label="Status" prop="status" width="100">
          <template #default="{ row }">
            <el-tag :type="statusTagType(row.status)" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Reason" prop="reason" min-width="300" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.reason || '-' }}
          </template>
        </el-table-column>
      </el-table>
      <div class="mt-2 text-xs text-gray-400">
        Total: {{ nodes.length }} nodes
        | Ready: {{ readyCount }}
        | Failed: {{ failedCount }}
      </div>
    </el-card>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { getPrewarmNodes } from '@/services'
import { ElMessage } from 'element-plus'

interface PrewarmNode {
  node: string
  status: string
  reason?: string
}

const props = defineProps<{ jobName: string }>()

const loading = ref(false)
const nodes = ref<PrewarmNode[]>([])
const wrapperRef = ref<HTMLElement>()
const containerWidth = ref('100%')

const readyCount = computed(() => nodes.value.filter(n => n.status === 'Ready').length)
const failedCount = computed(() => nodes.value.filter(n => n.status === 'Failed').length)

const statusTagType = (status: string) => {
  const map: Record<string, string> = {
    Ready: 'success',
    Running: 'warning',
    Pending: 'info',
    Failed: 'danger',
  }
  return map[status] || 'info'
}

let resizeObserver: ResizeObserver | null = null

const calcWidth = () => {
  const scrollWrap = wrapperRef.value?.closest('.el-scrollbar__wrap')
  if (scrollWrap) {
    containerWidth.value = `${scrollWrap.clientWidth}px`
  }
}

onMounted(async () => {
  await nextTick()
  calcWidth()

  const scrollWrap = wrapperRef.value?.closest('.el-scrollbar__wrap')
  if (scrollWrap) {
    resizeObserver = new ResizeObserver(calcWidth)
    resizeObserver.observe(scrollWrap)
  }

  loading.value = true
  try {
    const res = await getPrewarmNodes(props.jobName)
    nodes.value = res.nodes || []
  } catch (e) {
    ElMessage.error((e as Error).message || 'Failed to fetch node details')
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
})
</script>

<style scoped>
.node-detail-wrapper {
  padding: 12px 20px;
  box-sizing: border-box;
}
</style>

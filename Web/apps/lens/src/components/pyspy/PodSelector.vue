<template>
  <div class="pod-selector-inline">
    <div class="selector-label">
      <span class="label-text">Select Pod:</span>
      <el-tag size="small" type="info">
        {{ runningCount }} running / {{ totalCount }} total
      </el-tag>
    </div>
    
    <el-select
      v-model="selectedPodUid"
      @change="handleSelect"
      placeholder="Choose a pod to profile"
      size="large"
      filterable
      class="pod-select"
    >
      <el-option
        v-for="pod in pods"
        :key="pod.uid"
        :value="pod.uid"
        :label="pod.name"
        :disabled="pod.status !== 'Running'"
      >
        <div class="option-content">
          <span class="pod-name">{{ pod.name }}</span>
          <div class="pod-meta">
            <el-tag
              :type="pod.status === 'Running' ? 'success' : 'info'"
              size="small"
            >
              {{ pod.status }}
            </el-tag>
            <span class="node-name">{{ pod.nodeName }}</span>
          </div>
        </div>
      </el-option>
    </el-select>

    <el-empty
      v-if="pods.length === 0"
      description="No pods available"
      :image-size="60"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'

interface PodInfo {
  uid: string
  name: string
  namespace: string
  nodeName: string
  status: 'Running' | 'Pending' | 'Succeeded' | 'Failed' | 'Unknown'
  ip?: string
  gpuAllocated?: number
}

interface Props {
  pods: PodInfo[]
  selectedPod: PodInfo | null
}

const props = defineProps<Props>()
const emit = defineEmits<{
  select: [pod: PodInfo]
}>()

const selectedPodUid = ref<string>('')

const runningCount = computed(() => 
  props.pods.filter(p => p.status === 'Running').length
)

const totalCount = computed(() => props.pods.length)

watch(() => props.selectedPod, (newPod) => {
  selectedPodUid.value = newPod?.uid || ''
}, { immediate: true })

const handleSelect = (uid: string) => {
  const pod = props.pods.find(p => p.uid === uid)
  if (pod) {
    emit('select', pod)
  }
}
</script>

<style scoped lang="scss">
.pod-selector-inline {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;

  .selector-label {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-shrink: 0;

    .label-text {
      font-size: 15px;
      font-weight: 500;
      color: var(--el-text-color-primary);
    }
  }

  .pod-select {
    flex: 1;
    max-width: 600px;
  }

  .option-content {
    display: flex;
    justify-content: space-between;
    align-items: center;
    width: 100%;
    pointer-events: none; // Allow click to pass through to option

    .pod-name {
      font-weight: 500;
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .pod-meta {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-shrink: 0;

      .node-name {
        font-size: 12px;
        color: var(--el-text-color-secondary);
      }
    }
  }
}
</style>

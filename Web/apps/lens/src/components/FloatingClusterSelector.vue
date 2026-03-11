<template>
  <div
    class="floating-cluster-wrapper"
    :class="{ dragging: isDragging }"
    :style="{ top: position.y + 'px', left: position.x + 'px' }"
    ref="wrapperRef"
  >
    <!-- Minimized state - circular icon -->
    <transition name="fade">
      <div
        v-if="isMinimized"
        class="cluster-toggle"
        @mousedown="startDrag"
        @click="handleToggleClick"
        @contextmenu.prevent="resetPosition"
      >
        <i class="toggle-icon" i="ep-data-board" />
      </div>
    </transition>

    <!-- Expanded state - full dropdown -->
    <transition name="slide">
      <div v-if="!isMinimized" class="floating-cluster-selector">
        <div
          class="selector-header"
          @mousedown="startDrag"
        >
          <span class="header-text">Select Cluster</span>
          <el-button
            link
            size="small"
            @click="toggleMinimize"
            class="close-btn"
          >
            <i i="ep-close" />
          </el-button>
        </div>
        <el-select
          v-model="selectedCluster"
          placeholder="Choose a cluster"
          size="default"
          @change="handleClusterChange"
          class="cluster-select"
          :teleported="true"
          popper-class="cluster-select-dropdown"
          style="width: 230px;margin-top: 10px;"
        >
          <el-option
            v-for="cluster in clusterOptions"
            :key="cluster"
            :label="cluster"
            :value="cluster"
          />
        </el-select>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed, watch } from 'vue'
import { useGlobalCluster } from '@/composables/useGlobalCluster'
import { useClusterSync } from '@/composables/useClusterSync'
import { getClusters } from '@/services/gpu-aggregation'
import { ElMessage } from 'element-plus'
import { useUserStore } from '@/stores/user'
import dayjs from 'dayjs'

// Global cluster state
const { selectedCluster, clusterOptions, setCluster, setClusters, initCluster } = useGlobalCluster()
const { updateUrlWithCluster } = useClusterSync()
const userStore = useUserStore()

// Local state
const isMinimized = ref(true) // initially minimized (circular icon)
const wrapperRef = ref<HTMLElement>()

// Dragging state
const position = reactive({ x: 20, y: 100 })
const isDragging = ref(false)
const dragStart = reactive({ x: 0, y: 0 })
const clickTime = ref(0)

// Handle cluster change
const handleClusterChange = (value: string) => {
  setCluster(value)
  // Sync cluster parameter in URL
  updateUrlWithCluster(value)
  ElMessage.success(`Cluster changed to: ${value}`)
}

// Toggle minimize state
const toggleMinimize = () => {
  isMinimized.value = !isMinimized.value
}

// Dragging methods
const startDrag = (e: MouseEvent) => {
  isDragging.value = true
  dragStart.x = e.clientX - position.x
  dragStart.y = e.clientY - position.y
  clickTime.value = Date.now()

  document.addEventListener('mousemove', onDrag)
  document.addEventListener('mouseup', endDrag)
  e.preventDefault()
}

const onDrag = (e: MouseEvent) => {
  if (!isDragging.value) return

  const newX = e.clientX - dragStart.x
  const newY = e.clientY - dragStart.y

  // Keep within viewport bounds with buffer
  const buffer = 80 // Keep at least 80px visible (enough to grab and drag back)
  const elementWidth = wrapperRef.value?.offsetWidth || 300
  const elementHeight = wrapperRef.value?.offsetHeight || 150

  // Calculate bounds - ensure at least buffer pixels remain visible
  const maxX = window.innerWidth - buffer
  const maxY = window.innerHeight - buffer
  const minX = buffer - elementWidth
  const minY = Math.max(0, buffer - elementHeight) // Don't allow negative Y (above viewport)

  position.x = Math.max(minX, Math.min(newX, maxX))
  position.y = Math.max(minY, Math.min(newY, maxY))
}

const endDrag = () => {
  isDragging.value = false
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', endDrag)

  // Don't save position since we always reset on refresh
  // localStorage.setItem('clusterSelectorPosition', JSON.stringify(position))
}

const handleToggleClick = () => {
  // Only toggle state on quick click, not during drag
  const clickDuration = Date.now() - clickTime.value
  if (clickDuration < 200 && !isDragging.value) {
    toggleMinimize()
  }
}

// Ensure selector stays within viewport
const ensureInViewport = () => {
  const buffer = 80
  const elementWidth = wrapperRef.value?.offsetWidth || 300
  const elementHeight = wrapperRef.value?.offsetHeight || 150
  const maxX = window.innerWidth - buffer
  const maxY = window.innerHeight - buffer
  const minX = buffer - elementWidth
  const minY = Math.max(0, buffer - elementHeight)

  position.x = Math.max(minX, Math.min(position.x, maxX))
  position.y = Math.max(minY, Math.min(position.y, maxY))
}

// Reset position to default (right-click)
const resetPosition = () => {
  position.x = 20
  position.y = 100
  localStorage.removeItem('clusterSelectorPosition')
  ElMessage.success('Position reset to default')
}

// Load clusters
const loadClusters = async () => {
  try {
    const params = {
      startTime: dayjs().subtract(1, 'hour').toISOString(),
      endTime: dayjs().toISOString()
    }
    const clusters = await getClusters(params)
    setClusters(clusters)
  } catch (error) {
    console.error('Failed to load clusters:', error)
  }
}

onMounted(async () => {
  initCluster()

  // Wait for user auth state to be determined before loading clusters
  // If user is logged in, load immediately
  // If not logged in or login in progress, wait
  await userStore.ensureSessionOnce()

  if (userStore.isLogin) {
    loadClusters()
  } else {
    // Watch login state changes
    const unwatch = watch(() => userStore.isLogin, (isLogin) => {
      if (isLogin) {
        loadClusters()
        unwatch() // only load once
      }
    })
  }

  // Always reset to default position on refresh
  position.x = 20
  position.y = 100
  localStorage.removeItem('clusterSelectorPosition')

  // Old code for loading saved position (now disabled)
  // const savedPosition = localStorage.getItem('clusterSelectorPosition')
  // if (savedPosition) {
  //   try {
  //     const pos = JSON.parse(savedPosition)
  //     // Ensure position is within viewport
  //     const maxX = window.innerWidth - 300 // Approximate width
  //     const maxY = window.innerHeight - 150 // Approximate height
  //     position.x = Math.max(0, Math.min(pos.x, maxX))
  //     position.y = Math.max(0, Math.min(pos.y, maxY))
  //   } catch (e) {
  //     // Use default position
  //   }
  // }

  // Add window resize listener to keep selector in viewport
  window.addEventListener('resize', ensureInViewport)
})

// Cleanup
onUnmounted(() => {
  window.removeEventListener('resize', ensureInViewport)
})
</script>

<style scoped lang="scss">
.floating-cluster-wrapper {
  position: fixed;
  top: 100px;
  left: 20px;
  z-index: 99;

  &.dragging {
    .cluster-toggle,
    .floating-cluster-selector {
      transition: none;
      opacity: 0.9;
    }
  }
}

// Minimized state - circular icon
.cluster-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(12px);
  border-radius: 50%;
  box-shadow:
    0 2px 8px rgba(0, 0, 0, 0.08),
    0 8px 16px rgba(0, 0, 0, 0.04),
    0 0 0 1px rgba(0, 0, 0, 0.03);
  border: 1px solid rgba(0, 0, 0, 0.06);
  cursor: pointer;
  transition: all 0.2s ease;
  user-select: none;

  &:hover {
    box-shadow:
      0 4px 12px rgba(0, 0, 0, 0.1),
      0 12px 24px rgba(0, 0, 0, 0.05),
      0 0 0 1px rgba(0, 0, 0, 0.05);
    transform: translateY(-1px);
  }

  &:active {
    transform: scale(0.98);
  }

  .toggle-icon {
    font-size: 20px;
    color: var(--el-color-primary);
  }
}

// Expanded state - full dropdown
.floating-cluster-selector {
  width: 250px;
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(16px);
  border-radius: 12px;
  box-shadow:
    0 4px 16px rgba(0, 0, 0, 0.08),
    0 12px 32px rgba(0, 0, 0, 0.06),
    0 0 0 1px rgba(0, 0, 0, 0.04);
  border: 1px solid rgba(0, 0, 0, 0.08);
  overflow: visible;

  .selector-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 12px 10px 12px 16px;
    background: rgba(0, 0, 0, 0.02);
    cursor: pointer;
    user-select: none;
    border-bottom: 1px solid rgba(0, 0, 0, 0.04);
    border-radius: 12px 12px 0 0;

    &:hover {
      background: rgba(0, 0, 0, 0.03);
    }

    &:active {
      background: rgba(0, 0, 0, 0.04);
    }

    .header-text {
      font-size: 14px;
      font-weight: 600;
      color: var(--el-text-color-primary);
    }

    .close-btn {
      padding: 4px;
      color: var(--el-text-color-secondary);

      &:hover {
        color: var(--el-text-color-primary);
      }
    }
  }

  .cluster-select {
    padding: 10px;
    padding-top: 0;

    :deep(.el-select) {
      width: 100%;
    }

    :deep(.el-input__wrapper) {
      background: rgba(0, 0, 0, 0.03);
      border: 1px solid rgba(0, 0, 0, 0.06);
      padding: 0 12px;

      &:hover {
        border-color: var(--el-color-primary-light-5);
      }

      &.is-focus {
        border-color: var(--el-color-primary);
      }
    }

    :deep(.el-input__inner) {
      font-weight: 500;
      font-size: 14px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    :deep(.el-input__suffix) {
      margin-right: -4px;
    }
  }
}

// Dark mode
.dark {
  .cluster-toggle {
    background: rgba(30, 30, 30, 0.95);
    border-color: rgba(255, 255, 255, 0.1);
    box-shadow:
      0 2px 8px rgba(0, 0, 0, 0.2),
      0 8px 16px rgba(0, 0, 0, 0.1),
      0 0 0 1px rgba(255, 255, 255, 0.05);

    &:hover {
      box-shadow:
        0 4px 12px rgba(0, 0, 0, 0.25),
        0 12px 24px rgba(0, 0, 0, 0.15),
        0 0 0 1px rgba(255, 255, 255, 0.08);
    }
  }

  .floating-cluster-selector {
    background: rgba(30, 30, 30, 0.95);
    border-color: rgba(255, 255, 255, 0.1);
    box-shadow:
      0 4px 16px rgba(0, 0, 0, 0.3),
      0 12px 32px rgba(0, 0, 0, 0.2),
      0 0 0 1px rgba(255, 255, 255, 0.05);

    .selector-header {
      background: rgba(255, 255, 255, 0.02);
      border-bottom-color: rgba(255, 255, 255, 0.06);

      &:hover {
        background: rgba(255, 255, 255, 0.03);
      }
    }

    .cluster-select {
      :deep(.el-input__wrapper) {
        background: rgba(255, 255, 255, 0.03);
        border-color: rgba(255, 255, 255, 0.08);
      }
    }
  }
}

// Transitions
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.slide-enter-active,
.slide-leave-active {
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
}

.slide-enter-from {
  opacity: 0;
  transform: translateY(-10px) scale(0.95);
}

.slide-leave-to {
  opacity: 0;
  transform: translateY(-10px) scale(0.95);
}
</style>

<style lang="scss">
// Global styles for dropdown (non-scoped)
.cluster-select-dropdown {
  min-width: 220px !important; // Ensure minimum width for content

  .el-select-dropdown__item {
    font-size: 14px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    padding: 0 20px; // Add some padding
  }
}
</style>

<template>
  <div
    class="floating-github-wrapper"
    :class="{ dragging: isDragging, expanded: isHovered }"
    :style="{ top: position.y + 'px', left: position.x + 'px' }"
    ref="wrapperRef"
  >
    <div
      class="github-button"
      @mousedown="startDrag"
      @mouseenter="isHovered = true"
      @mouseleave="isHovered = false"
      @click="handleClick"
      @contextmenu.prevent="resetPosition"
    >
      <el-icon class="star-icon"><Star /></el-icon>
      <div class="content">
        <svg class="github-icon" viewBox="0 0 24 24" width="16" height="16">
          <path
            fill="currentColor"
            d="M12 2C6.477 2 2 6.477 2 12c0 4.419 2.865 8.17 6.839 9.49.5.092.682-.217.682-.482 0-.237-.009-.866-.013-1.7-2.782.603-3.369-1.34-3.369-1.34-.454-1.154-1.11-1.462-1.11-1.462-.908-.619.069-.607.069-.607 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.088 2.91.832.092-.646.35-1.088.636-1.338-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.646 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 7.708c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.376.203 2.393.1 2.646.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.167 22 16.417 22 12c0-5.523-4.477-10-10-10z"
          />
        </svg>
        <span class="button-text">Star us on GitHub</span>
      </div>

      <!-- Fireworks particles -->
      <div class="fireworks" v-if="showFireworks">
        <span v-for="i in 8" :key="i" class="particle" :style="`--i: ${i}`"></span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Star } from '@element-plus/icons-vue'

// Local state
const wrapperRef = ref<HTMLElement>()
const isHovered = ref(false)
const showFireworks = ref(false)

// Dragging state - position in top-right
const position = reactive({ x: window.innerWidth - 80, y: 60 })
const isDragging = ref(false)
const dragStart = reactive({ x: 0, y: 0 })
const clickTime = ref(0)

// Handle click to open GitHub
const handleClick = () => {
  // Only open if it was a quick click, not a drag
  const clickDuration = Date.now() - clickTime.value
  if (clickDuration < 200 && !isDragging.value) {
    // Trigger fireworks animation
    showFireworks.value = true
    setTimeout(() => {
      showFireworks.value = false
    }, 800)

    // Open GitHub after a short delay
    setTimeout(() => {
      window.open('https://github.com/AMD-AGI/Primus-SaFE', '_blank')
    }, 300)
  }
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
  const buffer = 80
  const elementWidth = wrapperRef.value?.offsetWidth || 200
  const elementHeight = wrapperRef.value?.offsetHeight || 40

  const maxX = window.innerWidth - buffer
  const maxY = window.innerHeight - buffer
  const minX = buffer - elementWidth
  const minY = Math.max(0, buffer - elementHeight)

  position.x = Math.max(minX, Math.min(newX, maxX))
  position.y = Math.max(minY, Math.min(newY, maxY))
}

const endDrag = () => {
  isDragging.value = false
  document.removeEventListener('mousemove', onDrag)
  document.removeEventListener('mouseup', endDrag)

  // Save position to localStorage
  localStorage.setItem('githubButtonPosition', JSON.stringify(position))
}

// Ensure button stays within viewport
const ensureInViewport = () => {
  const buffer = 80
  const elementWidth = wrapperRef.value?.offsetWidth || 200
  const elementHeight = wrapperRef.value?.offsetHeight || 40
  const maxX = window.innerWidth - buffer
  const maxY = window.innerHeight - buffer
  const minX = buffer - elementWidth
  const minY = Math.max(0, buffer - elementHeight)

  position.x = Math.max(minX, Math.min(position.x, maxX))
  position.y = Math.max(minY, Math.min(position.y, maxY))
}

// Reset position to default (right-click)
const resetPosition = () => {
  position.x = window.innerWidth - 80
  position.y = 60
  localStorage.removeItem('githubButtonPosition')
  ElMessage.success('Position reset to default')
}

onMounted(() => {
  // Load saved position with bounds checking
  const savedPosition = localStorage.getItem('githubButtonPosition')
  if (savedPosition) {
    try {
      const pos = JSON.parse(savedPosition)
      const maxX = window.innerWidth - 200
      const maxY = window.innerHeight - 40
      position.x = Math.max(0, Math.min(pos.x, maxX))
      position.y = Math.max(0, Math.min(pos.y, maxY))
    } catch (_e) {
      // Use default position
    }
  }

  // Add window resize listener to keep button in viewport
  window.addEventListener('resize', ensureInViewport)
})

onUnmounted(() => {
  window.removeEventListener('resize', ensureInViewport)
})
</script>

<style scoped lang="scss">
.floating-github-wrapper {
  position: fixed;
  z-index: 99;
  user-select: none;
}

.github-button {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 10px;
  background: rgba(36, 41, 46, 0.95);
  color: #fff;
  border-radius: 50%;
  box-shadow:
    0 2px 8px rgba(0, 0, 0, 0.15),
    0 0 0 1px rgba(255, 255, 255, 0.05);
  cursor: pointer;
  transition: all 0.25s ease;
  font-size: 12px;
  font-weight: 500;
  user-select: none;
  width: 40px;
  height: 40px;
  min-width: 40px;
  backdrop-filter: blur(8px);

  .star-icon {
    color: #ffd700;
    font-size: 18px;
    transition: all 0.25s ease;
    flex-shrink: 0;

    :deep(svg) {
      filter: drop-shadow(0 0 2px rgba(255, 215, 0, 0.3));
    }
  }

  .content {
    display: flex;
    align-items: center;
    gap: 6px;
    overflow: hidden;
    width: 0;
    opacity: 0;
    transition: all 0.25s ease;
  }

  .github-icon {
    flex-shrink: 0;
    opacity: 0.95;
  }

  .button-text {
    white-space: nowrap;
    flex-shrink: 0;
    opacity: 0.95;
  }

  &:hover {
    background: rgba(46, 160, 67, 0.9);
    box-shadow:
      0 3px 10px rgba(0, 0, 0, 0.2),
      0 0 20px rgba(46, 160, 67, 0.2);
    transform: translateY(-1px);

    .star-icon {
      transform: rotate(10deg) scale(1.05);
      color: #ffd33d;

      :deep(svg) {
        filter: drop-shadow(0 0 3px rgba(255, 215, 0, 0.5));
      }
    }
  }

  &:active {
    transform: scale(0.96);
  }
}

// Expanded state
.floating-github-wrapper.expanded {
  .github-button {
    width: auto;
    border-radius: 20px;
    padding: 8px 12px;
    gap: 8px;

    .content {
      width: auto;
      opacity: 1;
      margin-left: 2px;
    }
  }
}

// Dragging state
.floating-github-wrapper.dragging {
  .github-button {
    transition: none !important;
    opacity: 0.7;
    cursor: grabbing;
    transform: scale(0.92);
  }
}

// Dark mode adjustments
.dark {
  .github-button {
    background: rgba(22, 27, 34, 0.95);
    box-shadow:
      0 2px 8px rgba(0, 0, 0, 0.3),
      0 0 0 1px rgba(255, 255, 255, 0.08);

    &:hover {
      background: rgba(46, 160, 67, 0.85);
      box-shadow:
        0 3px 10px rgba(0, 0, 0, 0.25),
        0 0 25px rgba(46, 160, 67, 0.25);
    }
  }
}

// Fireworks animation
.fireworks {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  pointer-events: none;
  z-index: 100;

  .particle {
    position: absolute;
    display: block;
    width: 4px;
    height: 4px;
    border-radius: 50%;
    opacity: 0;

    &:nth-child(1) {
      background: #ff6b6b;
      animation: firework-1 0.8s ease-out forwards;
    }
    &:nth-child(2) {
      background: #ffd700;
      animation: firework-2 0.8s ease-out forwards;
    }
    &:nth-child(3) {
      background: #51cf66;
      animation: firework-3 0.8s ease-out forwards;
    }
    &:nth-child(4) {
      background: #339af0;
      animation: firework-4 0.8s ease-out forwards;
    }
    &:nth-child(5) {
      background: #ff922b;
      animation: firework-5 0.8s ease-out forwards;
    }
    &:nth-child(6) {
      background: #f06292;
      animation: firework-6 0.8s ease-out forwards;
    }
    &:nth-child(7) {
      background: #845ef7;
      animation: firework-7 0.8s ease-out forwards;
    }
    &:nth-child(8) {
      background: #20c997;
      animation: firework-8 0.8s ease-out forwards;
    }
  }
}

@keyframes firework-1 {
  0% {
    transform: translate(0, 0) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
    transform: translate(30px, 0) scale(1.2);
  }
  100% {
    opacity: 0;
    transform: translate(50px, 0) scale(0.3);
  }
}

@keyframes firework-2 {
  0% {
    transform: translate(0, 0) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
    transform: translate(21px, -21px) scale(1.2);
  }
  100% {
    opacity: 0;
    transform: translate(35px, -35px) scale(0.3);
  }
}

@keyframes firework-3 {
  0% {
    transform: translate(0, 0) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
    transform: translate(0, -30px) scale(1.2);
  }
  100% {
    opacity: 0;
    transform: translate(0, -50px) scale(0.3);
  }
}

@keyframes firework-4 {
  0% {
    transform: translate(0, 0) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
    transform: translate(-21px, -21px) scale(1.2);
  }
  100% {
    opacity: 0;
    transform: translate(-35px, -35px) scale(0.3);
  }
}

@keyframes firework-5 {
  0% {
    transform: translate(0, 0) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
    transform: translate(-30px, 0) scale(1.2);
  }
  100% {
    opacity: 0;
    transform: translate(-50px, 0) scale(0.3);
  }
}

@keyframes firework-6 {
  0% {
    transform: translate(0, 0) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
    transform: translate(-21px, 21px) scale(1.2);
  }
  100% {
    opacity: 0;
    transform: translate(-35px, 35px) scale(0.3);
  }
}

@keyframes firework-7 {
  0% {
    transform: translate(0, 0) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
    transform: translate(0, 30px) scale(1.2);
  }
  100% {
    opacity: 0;
    transform: translate(0, 50px) scale(0.3);
  }
}

@keyframes firework-8 {
  0% {
    transform: translate(0, 0) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
    transform: translate(21px, 21px) scale(1.2);
  }
  100% {
    opacity: 0;
    transform: translate(35px, 35px) scale(0.3);
  }
}

// Responsive adjustments
@media (max-width: 768px) {
  .github-button {
    width: 36px;
    height: 36px;
    min-width: 36px;
    padding: 8px;

    .star-icon {
      font-size: 18px;
    }

    .button-text {
      font-size: 11px;
    }

    .github-icon {
      width: 14px;
      height: 14px;
    }
  }

  .floating-github-wrapper.expanded {
    .github-button {
      padding: 8px 12px;
      gap: 6px;

      .content {
        margin-left: 2px;
      }
    }
  }
}
</style>

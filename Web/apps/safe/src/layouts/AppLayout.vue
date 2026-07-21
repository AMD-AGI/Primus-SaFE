<template>
  <div class="h-full w-full overflow-hidden flex flex-col">
    <div class="flex flex-1 overflow-hidden">
      <div
        class="sidebar-wrap shrink-0 relative"
        :class="{ resizing }"
        :style="{ width: sidebar.effectiveWidth + 'px' }"
      >
        <BaseMenu class="w-full h-full" />
        <div
          v-if="!sidebar.collapsed"
          class="sidebar-resizer"
          :class="{ resizing }"
          @pointerdown="startResize"
        />
      </div>

      <main class="flex-1 overflow-y-auto p-6 main-content-with-glow">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import BaseMenu from '@/components/layout/BaseMenu.vue'
import { useSidebarStore } from '@/stores/sidebar'

const sidebar = useSidebarStore()

const resizing = ref(false)
function startResize(e: PointerEvent) {
  e.preventDefault()
  resizing.value = true
  const startX = e.clientX
  const startWidth = sidebar.width
  const onMove = (ev: PointerEvent) => sidebar.setWidth(startWidth + (ev.clientX - startX))
  const onUp = () => {
    resizing.value = false
    window.removeEventListener('pointermove', onMove)
    window.removeEventListener('pointerup', onUp)
    document.body.style.userSelect = ''
    document.body.style.cursor = ''
  }
  document.body.style.userSelect = 'none'
  document.body.style.cursor = 'col-resize'
  window.addEventListener('pointermove', onMove)
  window.addEventListener('pointerup', onUp)
}
</script>

<style scoped lang="scss">
.sidebar-wrap {
  transition: width 0.18s ease;

  &.resizing {
    transition: none;
  }
}

.sidebar-resizer {
  position: absolute;
  top: 0;
  right: -3px;
  width: 6px;
  height: 100%;
  cursor: col-resize;
  z-index: 20;
  touch-action: none;

  &::after {
    content: '';
    position: absolute;
    top: 0;
    right: 3px;
    width: 2px;
    height: 100%;
    background: transparent;
    transition: background 0.15s ease;
  }

  &:hover::after,
  &.resizing::after {
    background: var(--safe-primary);
  }
}

.main-content-with-glow {
  position: relative;
  background:
    radial-gradient(circle at 20% 30%, rgba(139, 92, 246, 0.08) 0%, transparent 50%),
    radial-gradient(circle at 80% 70%, rgba(59, 130, 246, 0.06) 0%, transparent 50%),
    radial-gradient(circle at 40% 80%, rgba(6, 182, 212, 0.05) 0%, transparent 50%),
    linear-gradient(135deg, #f6f8fb 0%, #eef2f7 50%, #e8ecf1 100%);
  background-attachment: fixed;
}
</style>

<style lang="scss">
// Dark mode (unscoped)
html.dark .main-content-with-glow {
  background:
    radial-gradient(circle at 20% 30%, rgba(139, 92, 246, 0.12) 0%, transparent 50%),
    radial-gradient(circle at 80% 70%, rgba(59, 130, 246, 0.1) 0%, transparent 50%),
    radial-gradient(circle at 40% 80%, rgba(6, 182, 212, 0.08) 0%, transparent 50%),
    linear-gradient(135deg, #0a0e14 0%, #0f1419 50%, #141921 100%);
  background-attachment: fixed;
}
</style>

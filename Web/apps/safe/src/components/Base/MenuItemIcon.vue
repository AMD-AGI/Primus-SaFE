<!-- src/components/MenuItemIcon3.vue -->
<template>
  <img
    :src="src"
    :class="['menu-icon', iconClass]"
    :width="size"
    :height="size"
    aria-hidden="true"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useDark } from '@vueuse/core'

const props = withDefaults(
  defineProps<{
    /** Menu index (matches <el-menu-item index="...">) */
    index: string
    /** Inactive - light mode icon */
    light: string
    /** Inactive - dark mode icon */
    dark: string
    /** Active state (shared for light/dark) */
    active: string
    /** Icon size, default 16 */
    size?: number
    /** Route matching strategy: 'prefix' (/a matches /a/x) or 'exact', default 'prefix' */
    match?: 'prefix' | 'exact'
  }>(),
  {
    size: 16,
    match: 'prefix',
  },
)

const route = useRoute()
const isDark = useDark()

const isActive = computed(() => {
  if (props.match === 'exact') return route.path === props.index
  return route.path === props.index || route.path.startsWith(props.index + '/')
})

const src = computed(() => {
  if (isActive.value) return props.active
  return isDark.value ? props.dark : props.light
})

// Check if it's a torchft icon (needs CSS filter for color change)
const isTorchftIcon = computed(() => {
  return (
    props.light.includes('torchft') ||
    props.dark.includes('torchft') ||
    props.active.includes('torchft')
  )
})

const iconClass = computed(() => {
  if (!isTorchftIcon.value) return ''

  if (isActive.value) return 'torchft-icon-active'
  return isDark.value ? 'torchft-icon-dark' : 'torchft-icon-light'
})
</script>

<style scoped>
.menu-icon {
  display: inline-block;
  margin-right: 8px;
}

/* TorchFT icon color conversion (orange -> target color) */
.torchft-icon-light {
  /* Orange -> dark gray #2c2c2c */
  filter: brightness(0.3) saturate(0);
}

.torchft-icon-dark {
  /* Orange -> white #ffffff */
  filter: brightness(0) invert(1);
}

.torchft-icon-active {
  /* Orange -> cyan #00e5e5 */
  filter: brightness(1.3) saturate(3) hue-rotate(180deg);
}
</style>

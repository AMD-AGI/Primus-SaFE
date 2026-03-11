<template>
  <div 
    class="letter-avatar" 
    :style="{ 
      backgroundColor: bgColor,
      width: size + 'px',
      height: size + 'px',
      fontSize: fontSize + 'px'
    }"
  >
    {{ letter }}
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  name: string
  size?: number
}>(), {
  size: 40,
})

// Predefined color palette (low saturation)
const colors = [
  '#E8A87C', // Soft orange
  '#A69ADB', // Soft purple
  '#D4C28A', // Soft gold
  '#7EBAC0', // Soft cyan
  '#8BADD6', // Soft blue
  '#9AC69B', // Soft green
  '#D98CB3', // Soft pink
  '#D9A87C', // Soft amber
  '#7EBAC0', // Soft teal
  '#A88FC7', // Soft deep purple
  '#D98C7C', // Soft coral
  '#8B9ED6', // Soft indigo
]

const letter = computed(() => {
  const firstChar = props.name.trim()[0]
  return firstChar ? firstChar.toUpperCase() : '?'
})

const bgColor = computed(() => {
  // Generate a stable color based on the first character of the name
  const charCode = props.name.charCodeAt(0) || 0
  const index = charCode % colors.length
  return colors[index]
})

const fontSize = computed(() => {
  return Math.floor(props.size * 0.5)
})
</script>

<style scoped lang="scss">
.letter-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  color: #fff;
  font-weight: 600;
  user-select: none;
  flex-shrink: 0;
  text-transform: uppercase;
}
</style>

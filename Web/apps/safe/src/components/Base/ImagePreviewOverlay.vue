<template>
  <div v-if="visible" class="image-preview-overlay" @click.self="emit('close')">
    <button class="image-preview-close" type="button" @click="emit('close')">×</button>
    <img :src="url" class="image-preview-img" alt="Preview" />
  </div>
</template>

<script setup lang="ts">
interface Props {
  visible: boolean
  url: string
}

interface Emits {
  (e: 'close'): void
}

defineProps<Props>()
const emit = defineEmits<Emits>()
</script>

<style scoped lang="scss">
.image-preview-overlay {
  position: fixed;
  inset: 0;
  z-index: 3000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.85);
  padding: 24px;
}

.image-preview-img {
  max-width: min(95vw, 1200px);
  max-height: 90vh;
  width: auto;
  height: auto;
  object-fit: contain;
  border-radius: 10px;
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.35);
}

.image-preview-close {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 40px;
  height: 40px;
  border-radius: 999px;
  border: none;
  background: rgba(255, 255, 255, 0.9);
  color: #0f172a;
  font-size: 24px;
  line-height: 1;
  cursor: pointer;
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.25);
}

.image-preview-close:hover {
  background: #fff;
}
</style>

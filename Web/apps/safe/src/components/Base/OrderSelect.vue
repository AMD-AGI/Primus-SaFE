<template>
  <span class="order-select" @click.stop>
    <div class="arrow-container">
      <i
        class="arrow up"
        :class="{ active: modelValue === 'asc' }"
        @click.stop="emitOrder('asc')"
      />
      <i
        class="arrow down"
        :class="{ active: modelValue === 'desc' }"
        @click.stop="emitOrder('desc')"
      />
    </div>
  </span>
</template>

<script setup lang="ts">
const props = defineProps<{
  prop: string
  modelValue: 'asc' | 'desc' | ''
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: 'asc' | 'desc' | ''): void
  (e: 'sortChange', prop: string, order: 'asc' | 'desc' | ''): void
}>()

const emitOrder = (target: 'asc' | 'desc') => {
  const newOrder = props.modelValue === target ? '' : target
  emit('update:modelValue', newOrder)
  emit('sortChange', props.prop, newOrder)
}
</script>

<style scoped>
.arrow-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.arrow {
  display: inline-block;
  width: 0;
  height: 0;
  border: 5px solid transparent;
  cursor: pointer;
}
.up {
  border-bottom-color: #aaa;
}
.down {
  border-top-color: #aaa;
}
.active.up {
  border-bottom-color: var(--el-color-primary);
}
.active.down {
  border-top-color: var(--el-color-primary);
}
</style>

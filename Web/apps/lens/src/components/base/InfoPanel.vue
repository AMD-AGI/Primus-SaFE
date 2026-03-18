<template>
  <div :class="['grid gap-x-12 gap-y-4', `grid-cols-${columns}`]">
    <template v-for="(item, index) in items" :key="item.label">
      <div class="flex flex-col items-start pb-2">
        <span class="text-sm text-gray-400 mb-2">{{ item.label }}</span>
        <span class="text-base dark:text-white">{{ item.value }}</span>
      </div>

      <!-- ifRowEnd -->
      <template v-if="(index + 1) % 2 === 0">
        <div :class="[`col-span-${columns}`, 'white-border']"></div>
      </template>
    </template>
  </div>

</template>

<script setup lang="ts">
    import { defineProps } from 'vue';
    const props = withDefaults(defineProps<{
        items: { label: string; value: string }[],
        columns?: number,
    }>(), {
        columns: 2,
    })

</script>

<style>
.white-border {
    border-bottom: 1px solid var(--border-color);
}
</style>
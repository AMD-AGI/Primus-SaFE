<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path d="M12 2.5c-3.3 4.3-6.5 8.1-6.5 11.8a6.5 6.5 0 0 0 13 0c0-3.7-3.2-7.5-6.5-11.8z" />
    <path d="M9 14.5a3 3 0 0 0 3 3" />
  </svg>
</template>

<script setup lang="ts">
/** The Post Training droplet of the menu, drawn in the current text color. */
withDefaults(defineProps<{ size?: number }>(), { size: 16 })
</script>

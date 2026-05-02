<template>
  <el-tag :type="tagType" effect="light">{{ status }}</el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { OptimizationStatusTagType } from '@/services/model-optimization/type'

const props = defineProps<{ status: string }>()

const tagType = computed(() => OptimizationStatusTagType[props.status] ?? 'info')
</script>

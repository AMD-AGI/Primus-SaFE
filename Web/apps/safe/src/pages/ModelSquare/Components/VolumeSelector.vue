<template>
  <el-form-item v-if="writableVolumes.length > 0" label="Target Volume">
    <el-select
      :model-value="modelValue"
      placeholder="Select target volume"
      class="w-full"
      @update:model-value="$emit('update:modelValue', $event)"
    >
      <el-option
        v-for="v in writableVolumes"
        :key="v.mountPath"
        :label="`${v.mountPath} (${v.type}, ${v.accessMode})`"
        :value="v.mountPath"
      />
    </el-select>
    <div class="tip">ReadOnlyMany volumes are hidden. Data will be written to the selected volume.</div>
  </el-form-item>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { WorkspaceVolume } from '@/services/base/type'

const props = defineProps<{
  modelValue: string
  volumes: WorkspaceVolume[]
}>()

defineEmits<{ 'update:modelValue': [val: string] }>()

const writableVolumes = computed(() =>
  props.volumes.filter((v) => v.accessMode !== 'ReadOnlyMany'),
)
</script>

<style scoped>
.w-full { width: 100%; }
.tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}
</style>

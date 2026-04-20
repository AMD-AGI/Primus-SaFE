<template>
  <div class="plugins-container">
    <!-- Page title -->
    <div class="page-header">
      <el-text class="block textx-18 font-500" tag="b">Plugins</el-text>
      <div class="subtitle text-gray-500 text-sm mt-1">
        Browse and manage AI plugins & resources
      </div>
    </div>

    <!-- Tab bar -->
    <div class="tab-bar">
      <span
        v-for="t in tabOptions"
        :key="t.value"
        class="tab-item"
        :class="{ active: activeTab === t.value }"
        @click="activeTab = t.value"
      >{{ t.label }}</span>
    </div>

    <!-- Plugins tab -->
    <PluginsTab v-if="activeTab === 'plugins'" />

    <!-- Resources tab -->
    <ResourcesTab v-if="activeTab === 'resources'" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import PluginsTab from './Components/PluginsTab.vue'
import ResourcesTab from './Components/ResourcesTab.vue'

const activeTab = ref('plugins')
const tabOptions = [
  { label: 'Plugins', value: 'plugins' },
  { label: 'Resources', value: 'resources' },
]

defineOptions({
  name: 'PluginsPage',
})
</script>

<style scoped lang="scss">
.plugins-container {
  padding: 0;
  display: flex;
  flex-direction: column;
  height: calc(100vh - 120px);

  .page-header {
    margin-bottom: 12px;
    flex-shrink: 0;
  }

  .tab-bar {
    display: flex;
    gap: 24px;
    margin-bottom: 16px;
    border-bottom: 1px solid var(--safe-border, var(--el-border-color-light));
    flex-shrink: 0;

    .tab-item {
      padding: 8px 0;
      font-size: 14px;
      font-weight: 500;
      color: var(--safe-muted, var(--el-text-color-secondary));
      cursor: pointer;
      border-bottom: 2px solid transparent;
      transition: color 0.2s, border-color 0.2s;
      margin-bottom: -1px;

      &:hover {
        color: var(--safe-text, var(--el-text-color-primary));
      }

      &.active {
        color: var(--safe-primary, var(--el-color-primary));
        border-bottom-color: var(--safe-primary, var(--el-color-primary));
      }
    }
  }
}
</style>

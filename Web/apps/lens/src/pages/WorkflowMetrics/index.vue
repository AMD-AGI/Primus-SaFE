<template>
  <div class="workflow-metrics-page">
    <!-- Tab Navigation -->
    <div class="tab-nav">
      <router-link
        v-for="tab in tabs"
        :key="tab.path"
        :to="tab.path"
        class="tab-item"
        :class="{ active: isActiveTab(tab.path) }"
      >
        <el-icon :size="16" class="tab-icon"><component :is="tab.icon" /></el-icon>
        <span>{{ tab.label }}</span>
      </router-link>
    </div>

    <!-- Router View for Child Components -->
    <router-view />
  </div>
</template>

<script setup lang="ts">
import { useRoute } from 'vue-router'
import { Setting, List, DataAnalysis } from '@element-plus/icons-vue'

const route = useRoute()

const tabs = [
  {
    path: '/workflow-metrics/configs',
    label: 'Configs',
    icon: Setting
  },
  {
    path: '/workflow-metrics/runs',
    label: 'Runs',
    icon: List
  },
  {
    path: '/workflow-metrics/explorer',
    label: 'Explorer',
    icon: DataAnalysis
  }
]

const isActiveTab = (path: string) => {
  return route.path.startsWith(path)
}
</script>

<style scoped lang="scss">
.workflow-metrics-page {
  position: relative;
  padding: 0 20px;
  
  // Decorative background elements
  &::before {
    content: '';
    position: absolute;
    top: -50px;
    right: 15%;
    width: 450px;
    height: 450px;
    background: radial-gradient(circle, rgba(64, 158, 255, 0.07) 0%, transparent 70%);
    border-radius: 50%;
    pointer-events: none;
    z-index: 0;
  }
  
  &::after {
    content: '';
    position: absolute;
    bottom: 50px;
    left: 10%;
    width: 400px;
    height: 400px;
    background: radial-gradient(circle, rgba(103, 194, 58, 0.06) 0%, transparent 70%);
    border-radius: 50%;
    pointer-events: none;
    z-index: 0;
  }

  .tab-nav {
    display: flex;
    gap: 8px;
    margin-bottom: 24px;
    padding: 4px;
    background: var(--el-bg-color-overlay);
    border-radius: 12px;
    border: 1px solid var(--el-border-color-light);
    width: fit-content;

    .tab-item {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 10px 20px;
      border-radius: 8px;
      font-size: 14px;
      font-weight: 500;
      color: var(--el-text-color-secondary);
      text-decoration: none;
      transition: all 0.2s ease;

      @media (min-width: 1920px) {
        font-size: 15px;
        padding: 12px 24px;
      }

      .tab-icon {
        font-size: 16px;

        @media (min-width: 1920px) {
          font-size: 18px;
        }
      }

      &:hover {
        color: var(--el-color-primary);
        background: var(--el-fill-color-light);
      }

      &.active {
        color: #fff;
        background: var(--el-color-primary);
        box-shadow: 0 2px 8px rgba(64, 158, 255, 0.3);

        .tab-icon {
          color: #fff;
        }
      }
    }
  }
}
</style>


<script setup lang="ts">
import { computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { 
  DataAnalysis, 
  Bell, 
  Operation, 
  Document, 
  MuteNotification,
  Notification,
  Tickets,
  Message
} from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

// Tab configuration
const tabs = [
  { name: 'overview', label: 'Overview', icon: DataAnalysis, path: '/alerts/overview' },
  { name: 'events', label: 'Events', icon: Bell, path: '/alerts/events' },
  { name: 'rules', label: 'Rules', icon: Operation, path: '/alerts/rules/metric', 
    children: [
      { name: 'metric', label: 'Metric Rules', path: '/alerts/rules/metric' },
      { name: 'log', label: 'Log Rules', path: '/alerts/rules/log' },
    ]
  },
  { name: 'templates', label: 'Templates', icon: Document, path: '/alerts/rules/templates' },
  { name: 'silences', label: 'Silences', icon: MuteNotification, path: '/alerts/silences' },
  { name: 'channels', label: 'Channels', icon: Message, path: '/alerts/channels' },
  { name: 'advices', label: 'AI Advices', icon: Notification, path: '/alerts/advices' },
]

// Current active tab
const activeTab = computed(() => {
  const path = route.path
  if (path.startsWith('/alerts/rules')) return 'rules'
  if (path.startsWith('/alerts/events')) return 'events'
  if (path.startsWith('/alerts/silences')) return 'silences'
  if (path.startsWith('/alerts/channels')) return 'channels'
  if (path.startsWith('/alerts/advices')) return 'advices'
  return 'overview'
})

// Current active sub-tab for rules
const activeRuleTab = computed(() => {
  const path = route.path
  if (path.includes('/rules/log')) return 'log'
  if (path.includes('/rules/templates')) return 'templates'
  return 'metric'
})

function handleTabChange(tabName: string) {
  const tab = tabs.find(t => t.name === tabName)
  if (tab) {
    router.push(tab.path)
  }
}

function handleRuleTabChange(tabName: string) {
  if (tabName === 'metric') {
    router.push('/alerts/rules/metric')
  } else if (tabName === 'log') {
    router.push('/alerts/rules/log')
  }
}
</script>

<template>
  <div class="alerts-layout">
    <!-- Page Header with Tab Navigation -->
    <div class="alerts-header">
      <div class="header-title">
        <el-icon class="title-icon" :size="28"><Tickets /></el-icon>
        <h1>Alert Center</h1>
      </div>
      
      <el-tabs 
        :model-value="activeTab" 
        class="alerts-tabs"
        @tab-change="handleTabChange"
      >
        <el-tab-pane 
          v-for="tab in tabs" 
          :key="tab.name"
          :name="tab.name"
          :lazy="true"
        >
          <template #label>
            <span class="tab-label">
              <el-icon><component :is="tab.icon" /></el-icon>
              {{ tab.label }}
            </span>
          </template>
        </el-tab-pane>
      </el-tabs>
    </div>

    <!-- Sub-tabs for Rules -->
    <div v-if="activeTab === 'rules'" class="rules-sub-tabs">
      <el-radio-group 
        :model-value="activeRuleTab" 
        size="default"
        @change="handleRuleTabChange"
      >
        <el-radio-button value="metric">Metric Alert Rules</el-radio-button>
        <el-radio-button value="log">Log Alert Rules</el-radio-button>
      </el-radio-group>
    </div>

    <!-- Content Area -->
    <div class="alerts-content">
      <router-view v-slot="{ Component }">
        <keep-alive :include="['AlertOverview', 'AlertEventsList']">
          <component :is="Component" />
        </keep-alive>
      </router-view>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.alerts-layout {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 24px;
  background: var(--el-bg-color-page);
}

.alerts-header {
  margin-bottom: 20px;
  
  .header-title {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 16px;
    
    .title-icon {
      color: var(--el-color-primary);
    }
    
    h1 {
      margin: 0;
      font-size: 24px;
      font-weight: 600;
      color: var(--el-text-color-primary);
    }
  }
}

.alerts-tabs {
  :deep(.el-tabs__header) {
    margin: 0;
    border-bottom: 1px solid var(--el-border-color-lighter);
  }
  
  :deep(.el-tabs__nav-wrap::after) {
    display: none;
  }
  
  :deep(.el-tabs__item) {
    padding: 0 24px;
    height: 48px;
    line-height: 48px;
    font-weight: 500;
    
    &.is-active {
      color: var(--el-color-primary);
    }
    
    &:hover {
      color: var(--el-color-primary);
    }
  }
  
  :deep(.el-tabs__active-bar) {
    height: 3px;
    border-radius: 2px;
  }
  
  :deep(.el-tabs__content) {
    display: none;
  }
}

.tab-label {
  display: flex;
  align-items: center;
  gap: 8px;
  
  .el-icon {
    font-size: 16px;
  }
}

.rules-sub-tabs {
  margin-bottom: 20px;
  padding: 12px 16px;
  background: var(--el-fill-color-lighter);
  border-radius: 8px;
  
  :deep(.el-radio-group) {
    display: flex;
    gap: 8px;
  }
  
  :deep(.el-radio-button__inner) {
    border-radius: 6px !important;
    border: none !important;
    box-shadow: none !important;
    padding: 8px 16px;
    background: transparent;
    
    &:hover {
      color: var(--el-color-primary);
    }
  }
  
  :deep(.el-radio-button__original-radio:checked + .el-radio-button__inner) {
    background: var(--el-color-primary-light-9);
    color: var(--el-color-primary);
  }
}

.alerts-content {
  flex: 1;
  overflow: auto;
  background: var(--el-bg-color);
  border-radius: 12px;
  padding: 24px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.04);
}

// Dark mode adjustments
.dark {
  .alerts-content {
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.2);
  }
  
  .rules-sub-tabs {
    background: var(--el-fill-color);
  }
}
</style>

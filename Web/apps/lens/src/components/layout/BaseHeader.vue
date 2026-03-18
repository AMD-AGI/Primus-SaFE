<template>
  <el-menu
    ref="menuRef"
    class="el-menu-demo flex items-center"
    mode="horizontal"
    :ellipsis="false"
    :default-active="route.path"
    @select="handleMenuSelect"
  >
    <!-- Logo -->
    <div class="logo-wrapper">
      <img class="app-logo" :src="isDark ? `${baseUrl}logo_w.png` : `${baseUrl}logo_b.png`" alt="Logo" />
    </div>

    <!-- Theme Toggle Button -->
    <button class="theme-toggle-btn" @click="toggleDark()">
      <el-tooltip :content="isDark ? 'Switch to Light Mode' : 'Switch to Dark Mode'" placement="bottom">
        <el-icon :size="14" class="theme-icon">
          <Moon v-if="isDark" />
          <Sunny v-else />
        </el-icon>
      </el-tooltip>
    </button>

    <!-- Divider -->
    <div class="menu-divider"></div>

    <!-- Menu items container -->
    <div ref="menuItemsRef" class="menu-items-container">
      <el-menu-item
        v-show="visibleMenuItems.includes('cluster')"
        index="/statistics/cluster"
        data-menu-key="cluster"
      >
        <span class="menu-item-text">Cluster</span>
      </el-menu-item>
      <el-menu-item
        v-show="visibleMenuItems.includes('namespace')"
        index="/statistics/namespace"
        data-menu-key="namespace"
      >
        <span class="menu-item-text">Namespace</span>
      </el-menu-item>
      <el-menu-item
        v-show="visibleMenuItems.includes('workload')"
        index="/statistics/workload"
        data-menu-key="workload"
      >
        <span class="menu-item-text">Workload</span>
      </el-menu-item>
      <el-menu-item
        v-show="visibleMenuItems.includes('label')"
        index="/statistics/label"
        data-menu-key="label"
      >
        <span class="menu-item-text">Tags</span>
      </el-menu-item>
      <el-menu-item
        v-show="visibleMenuItems.includes('reports')"
        index="/weekly-reports"
        data-menu-key="reports"
      >
        <span class="menu-item-text">Reports</span>
      </el-menu-item>
      <el-menu-item
        v-show="visibleMenuItems.includes('github')"
        index="/github-workflow"
        data-menu-key="github"
      >
        <span class="menu-item-text">Github Workflow</span>
      </el-menu-item>
      <el-menu-item
        v-show="visibleMenuItems.includes('agent')"
        index="/agent"
        data-menu-key="agent"
      >
        <span class="menu-item-with-tag">
          Agent
          <el-tag size="small" type="warning" class="beta-tag">Beta</el-tag>
        </span>
      </el-menu-item>
      <el-menu-item
        v-show="visibleMenuItems.includes('alerts')"
        index="/alerts"
        data-menu-key="alerts"
      >
        <span class="menu-item-with-tag">
          Alerts
          <el-tag size="small" type="danger" class="beta-tag">New</el-tag>
        </span>
      </el-menu-item>
    </div>

    <!-- More dropdown for overflow items -->
    <el-dropdown
      v-if="overflowMenuItems.length > 0"
      class="more-dropdown"
      trigger="click"
      popper-class="header-more-dropdown-menu"
      @command="handleMenuSelect"
    >
      <el-menu-item class="more-menu-item">
        <span class="menu-item-text">More</span>
        <el-icon class="more-icon"><ArrowDown /></el-icon>
      </el-menu-item>
      <template #dropdown>
        <el-dropdown-menu class="header-dropdown-content">
          <el-dropdown-item
            v-for="item in overflowMenuItems"
            :key="item.key"
            :command="item.index"
            :class="{ 'is-active': route.path === item.index }"
          >
            <span v-if="item.label === 'Agent' || item.label === 'Alerts'" class="menu-item-with-tag">
              {{ item.label }}
              <el-tag
                size="small"
                :type="item.label === 'Agent' ? 'warning' : 'danger'"
                class="beta-tag"
              >
                {{ item.label === 'Agent' ? 'Beta' : 'New' }}
              </el-tag>
            </span>
            <span v-else>{{ item.label }}</span>
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>

    <div class="flex-1" />

    <!-- Lens entry - production only -->
    <el-link link @click="goToSaFE" type="primary" class="lens-link">
      <span class="lens-link-text">Go to SaFE</span>
      <el-icon class="lens-link-arrow"><Right /></el-icon>
    </el-link>

    <!-- Icon buttons group -->
    <el-menu-item class="ml-auto icon-btn github-btn" @click="goToChatbot">
      <el-tooltip content="Visit AI Chatbot" placement="bottom">
        <img
          :src="SparklesIcon"
          alt="Chatbot"
          class="sparkles-icon"
          style="width: 20px; height: 20px;"
        />
      </el-tooltip>
    </el-menu-item>

    <el-menu-item class="icon-btn grafana-btn" h="full" @click="goGrafana">
      <el-tooltip content="jump to grafana" placement="top">
        <button
          class="w-full cursor-pointer border-none bg-transparent"
          style="height: var(--ep-menu-item-height)"
        >
          <img
            :src="GrafanaIcon"
            alt="Grafana"
            class="inline-block w-5 h-5"
          />
        </button>
      </el-tooltip>
    </el-menu-item>

    <!-- User Info Dropdown - placed at the rightmost position -->
    <el-dropdown v-if="userStore.userId" class="user-dropdown" trigger="click" popper-class="header-user-dropdown-menu">
      <div class="user-info">
        <el-avatar :size="32" class="user-avatar" :style="getAvatarStyle()">
          <span class="avatar-text">{{ getAvatarText() }}</span>
        </el-avatar>
        <span class="user-name">{{ userStore.profile?.name || userStore.userId || 'User' }}</span>
        <el-icon class="dropdown-arrow"><ArrowDown /></el-icon>
      </div>
        <template #dropdown>
          <el-dropdown-menu class="header-dropdown-content">
            <el-dropdown-item disabled>
              <div class="user-detail">
                <div class="detail-name">{{ userStore.profile?.name || 'User' }}</div>
                <div class="detail-email">{{ userStore.profile?.email || userStore.userId }}</div>
              </div>
            </el-dropdown-item>
            <el-dropdown-item @click="goToUserManagement">
              <el-icon><User /></el-icon>
              <span>User Management</span>
            </el-dropdown-item>
            <el-dropdown-item @click="goToJobHistory">
              <el-icon><Clock /></el-icon>
              <span>Job Execution History</span>
            </el-dropdown-item>
            <el-dropdown-item @click="goToDetectionStatus">
              <el-icon><DataAnalysis /></el-icon>
              <span>Detection Status</span>
            </el-dropdown-item>
            <el-dropdown-item @click="goToSystemConfig">
              <el-icon><Setting /></el-icon>
              <span>System Config</span>
            </el-dropdown-item>
            <el-dropdown-item @click="goToReleaseManagement">
              <el-icon><Upload /></el-icon>
              <span>Release Management</span>
            </el-dropdown-item>
            <el-dropdown-item @click="goToClusterManagement">
              <el-icon><Grid /></el-icon>
              <span>Cluster Management</span>
            </el-dropdown-item>
            <el-dropdown-item divided @click="handleLogout">
              <el-icon><SwitchButton /></el-icon>
              <span>Logout</span>
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
    </el-dropdown>
  </el-menu>
</template>

<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { Right, User, SwitchButton, ArrowDown, Clock, Moon, Sunny, DataAnalysis, Setting, Upload, Grid } from '@element-plus/icons-vue'
import { toggleDark, isDark } from '@/composables'
  import { useUserStore } from '@/stores/user'
import SparklesIcon from '@/assets/icons/sparkles.png'
  import { ElMessage } from 'element-plus'
  import { onMounted, watch, ref, onUnmounted, nextTick } from 'vue'
  import GrafanaIcon from '@/assets/icons/grafana.png'
  import { useClusterSync } from '@/composables/useClusterSync'

  const isProd = import.meta.env.PROD
  const baseUrl = import.meta.env.BASE_URL || '/'
  const route = useRoute()
  const router = useRouter()
  const { navigateWithCluster } = useClusterSync()
  const userStore = useUserStore()

  // Menu items configuration
  const menuItems = [
    { key: 'cluster', index: '/statistics/cluster', label: 'Cluster' },
    { key: 'namespace', index: '/statistics/namespace', label: 'Namespace' },
    { key: 'workload', index: '/statistics/workload', label: 'Workload' },
    { key: 'label', index: '/statistics/label', label: 'Tags' },
    { key: 'reports', index: '/weekly-reports', label: 'Reports' },
    { key: 'github', index: '/github-workflow', label: 'Github Workflow' },
    { key: 'agent', index: '/agent', label: 'Agent' },
    // { key: 'alerts', index: '/alerts', label: 'Alerts' },
  ]

  // Responsive menu state
  const menuRef = ref()
  const menuItemsRef = ref()
  const visibleMenuItems = ref<string[]>(menuItems.map(item => item.key))
  const overflowMenuItems = ref<typeof menuItems>([])

  // Calculate which items should be visible and which should overflow
  const calculateMenuLayout = () => {
    if (!menuRef.value || !menuItemsRef.value) return

    nextTick(() => {
      const menuElement = menuRef.value.$el as HTMLElement
      const menuItemsElement = menuItemsRef.value as HTMLElement

      if (!menuElement || !menuItemsElement) return

      // Temporarily show all items to measure their widths
      visibleMenuItems.value = menuItems.map(item => item.key)

      nextTick(() => {
        // Get total available width
        const menuWidth = menuElement.offsetWidth

        // Calculate fixed elements width (logo, theme button, divider, spacer, right side items)
        const logoWrapper = menuElement.querySelector('.logo-wrapper') as HTMLElement
        const themeToggle = menuElement.querySelector('.theme-toggle-btn') as HTMLElement
        const menuDivider = menuElement.querySelector('.menu-divider') as HTMLElement
        const lensLink = menuElement.querySelector('.lens-link') as HTMLElement
        const iconButtons = menuElement.querySelectorAll('.icon-btn')
        const userDropdown = menuElement.querySelector('.user-dropdown') as HTMLElement

        let fixedWidth = 0
        if (logoWrapper) fixedWidth += logoWrapper.offsetWidth + 20 // Add margin
        if (themeToggle) fixedWidth += themeToggle.offsetWidth + 10
        if (menuDivider) fixedWidth += menuDivider.offsetWidth + 32 // Divider margins
        if (lensLink) fixedWidth += lensLink.offsetWidth + 16
        if (userDropdown) fixedWidth += userDropdown.offsetWidth + 12
        iconButtons.forEach(btn => {
          fixedWidth += (btn as HTMLElement).offsetWidth
        })

        // Reserve extra space for "More" button and safety margin
        const moreButtonWidth = 120 // Estimated width for "More" button
        const safetyMargin = 50
        fixedWidth += moreButtonWidth + safetyMargin

        // Available width for menu items
        const availableWidth = menuWidth - fixedWidth

        // Get all menu item elements with their actual widths
        const menuItemElements = menuItemsElement.querySelectorAll('[data-menu-key]') as NodeListOf<HTMLElement>

        let usedWidth = 0
        const visible: string[] = []
        const overflow: typeof menuItems = []

        // Iterate through menu items in order
        menuItems.forEach(menuItem => {
          const element = Array.from(menuItemElements).find(
            el => el.getAttribute('data-menu-key') === menuItem.key
          ) as HTMLElement

          if (!element) return

          const itemWidth = element.offsetWidth

          // Check if this item fits (with some padding)
          if (usedWidth + itemWidth <= availableWidth) {
            visible.push(menuItem.key)
            usedWidth += itemWidth
          } else {
            overflow.push(menuItem)
          }
        })

        // If nothing overflows, we don't need the "More" button, recalculate
        if (overflow.length === 0 && visible.length < menuItems.length) {
          visibleMenuItems.value = menuItems.map(item => item.key)
          overflowMenuItems.value = []
        } else {
          visibleMenuItems.value = visible
          overflowMenuItems.value = overflow
        }
      })
    })
  }

  // Debounce helper
  let resizeTimer: number | null = null
  const debouncedCalculate = () => {
    if (resizeTimer) clearTimeout(resizeTimer)
    resizeTimer = window.setTimeout(() => {
      calculateMenuLayout()
    }, 150)
  }

  onMounted(() => {
    // Initial calculation
    setTimeout(() => {
      calculateMenuLayout()
    }, 300)

    // Listen to window resize
    window.addEventListener('resize', debouncedCalculate)
  })

  onUnmounted(() => {
    window.removeEventListener('resize', debouncedCalculate)
    if (resizeTimer) clearTimeout(resizeTimer)
  })

  const handleMenuSelect = (index: string) => {
    // Use navigateWithCluster to ensure cluster parameter is preserved
    navigateWithCluster(index)
  }

  const goGrafana = () => {
    const baseUrl = import.meta.env.BASE_URL || '/'
    const grafanaPath = `${baseUrl}grafana`.replace('//', '/')
    window.open(`${window.location.origin}${grafanaPath}`, '_blank')
  }

  const goToManagement = () => {
    navigateWithCluster('/management')
  }

const goToSaFE = () => {
  window.open(window.location.origin, '_blank')
}

const goToChatbot = () => {
  window.open(`${window.location.origin}/chatbot`, '_blank')
}

const goToUserManagement = () => {
  window.open(`${window.location.origin}/usermanage`, '_blank')
}

const goToJobHistory = () => {
  router.push('/management')
}

const goToDetectionStatus = () => {
  router.push('/management/detection-status')
}

const goToSystemConfig = () => {
  router.push('/management/system-config')
}

const goToReleaseManagement = () => {
  router.push('/management/releases')
}

const goToClusterManagement = () => {
  router.push('/management/clusters')
}

  const handleLogout = async () => {
    try {
      await userStore.logout()
      ElMessage.success('Logged out successfully')
    } catch (error) {
      console.error('Logout error:', error)
      // Even if backend logout fails, clear local state
      userStore.$reset()
    }
    // Use window.location.href to force a full page refresh and redirect to login page.
    // Cannot use router.push because non-persisted Pinia state like _initPromise
    // will not be fully reset, causing ensureSessionOnce() in Login onMounted to behave abnormally,
    // and auto-SSO won't trigger. Full page refresh ensures all state is rehydrated from localStorage cleanly.
    window.location.href = `${baseUrl}login`
  }


  // Get avatar display text (first letters of username)
  const getAvatarText = () => {
    const name = userStore.profile?.name || userStore.userId || 'User'
    // If English name, take first two letters
    if (/^[a-zA-Z]/.test(name)) {
      return name.substring(0, 2).toUpperCase()
    }
    // If Chinese name, take last two characters
    if (/[\u4e00-\u9fa5]/.test(name)) {
      return name.length > 2 ? name.substring(name.length - 2) : name
    }
    // Otherwise take first two characters
    return name.substring(0, 2).toUpperCase()
  }

  // Generate unique gradient color based on username
  const getAvatarStyle = () => {
    const name = userStore.profile?.name || userStore.userId || 'User'
    const gradients = [
      'linear-gradient(135deg, #667eea 0%, #764ba2 100%)', // purple
      'linear-gradient(135deg, #f093fb 0%, #f5576c 100%)', // pink
      'linear-gradient(135deg, #4facfe 0%, #00f2fe 100%)', // blue
      'linear-gradient(135deg, #43e97b 0%, #38f9d7 100%)', // green
      'linear-gradient(135deg, #fa709a 0%, #fee140 100%)', // orange-pink
      'linear-gradient(135deg, #30cfd0 0%, #330867 100%)', // deep blue
      'linear-gradient(135deg, #a8edea 0%, #fed6e3 100%)', // light pink-blue
      'linear-gradient(135deg, #ff9a9e 0%, #fecfef 100%)', // light pink
    ]

    // Generate a stable index based on the name
    let hash = 0
    for (let i = 0; i < name.length; i++) {
      hash = name.charCodeAt(i) + ((hash << 5) - hash)
    }
    const index = Math.abs(hash) % gradients.length

    return {
      background: gradients[index]
    }
  }

</script>

<style scoped lang="scss">
.el-menu-demo {
  backdrop-filter: blur(20px) saturate(180%);
  -webkit-backdrop-filter: blur(20px) saturate(180%);
  background: rgba(255, 255, 255, 0.85);
  border-bottom: 1px solid rgba(255, 255, 255, 0.18);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
  position: sticky;
  top: 0;
  z-index: 100;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  height: 72px; // moderate height

  &:hover {
    background: rgba(255, 255, 255, 0.95);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
  }

  // Optimize menu item transitions
  :deep(.el-menu-item) {
    transition-property: color, font-weight !important;
    transition-duration: 0.15s !important;
    transition-timing-function: ease-out !important;
    height: 72px; // match menu height
    line-height: 72px; // vertical centering
  }
}

.dark .el-menu-demo {
  background: rgba(30, 30, 30, 0.8);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);

  &:hover {
    background: rgba(30, 30, 30, 0.9);
  }
}

.el-menu-item {
  font-weight: 500;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  border-radius: 10px;
  margin: 0 8px; // More spacing between items
  padding: 0 32px; // More horizontal padding
  min-width: 110px; // Wider minimum width
  position: relative;
  background: transparent;
  will-change: transform;
  overflow: hidden;

  // Subtle indicator line
  &::after {
    content: '';
    position: absolute;
    bottom: 0;
    left: 50%;
    width: 0;
    height: 2px;
    background: var(--el-color-primary);
    transform: translateX(-50%);
    transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  }


  &:hover {
    background: rgba(64, 158, 255, 0.05);
    transform: translateY(-1px);

    &::after {
      width: 30px;
      opacity: 0.5;
    }
  }

  &.is-active {
    font-weight: 600;
    color: var(--el-color-primary);
    background: linear-gradient(
      135deg,
      rgba(64, 158, 255, 0.1) 0%,
      rgba(103, 194, 255, 0.15) 50%,
      rgba(64, 158, 255, 0.1) 100%
    );
    backdrop-filter: blur(16px) saturate(180%);
    box-shadow:
      0 2px 12px rgba(64, 158, 255, 0.15),
      inset 0 1px 0 rgba(255, 255, 255, 0.2);

    &::after {
      width: 100%;
      opacity: 1;
    }
  }
}


.dark .el-menu-item {
  &:hover {
    background: rgba(255, 255, 255, 0.05);
  }

  &.is-active {
    background: linear-gradient(
      135deg,
      rgba(64, 158, 255, 0.15) 0%,
      rgba(103, 194, 255, 0.2) 50%,
      rgba(64, 158, 255, 0.15) 100%
    );
  }
}

// Chatbot icon button
.github-btn {
  .sparkles-icon {
    // Blue effect: using more effective filter combination
    filter: brightness(0) saturate(100%) invert(42%) sepia(93%) saturate(1352%) hue-rotate(201deg) brightness(103%) contrast(97%) drop-shadow(0 2px 4px rgba(59, 130, 246, 0.2));
    transition: all 0.3s ease;
    vertical-align: middle;
    display: inline-block;
  }

  &:hover .sparkles-icon {
    filter: brightness(0) saturate(100%) invert(42%) sepia(93%) saturate(1352%) hue-rotate(201deg) brightness(120%) contrast(97%) drop-shadow(0 4px 8px rgba(59, 130, 246, 0.4));
    transform: scale(1.15) rotate(5deg);
  }
}

.dark .github-btn {
  .sparkles-icon {
    // Blue effect in dark mode - slightly increased brightness
    filter: brightness(0) saturate(100%) invert(42%) sepia(93%) saturate(1352%) hue-rotate(201deg) brightness(110%) contrast(97%) drop-shadow(0 2px 4px rgba(59, 130, 246, 0.3));
  }

  &:hover .sparkles-icon {
    filter: brightness(0) saturate(100%) invert(42%) sepia(93%) saturate(1352%) hue-rotate(201deg) brightness(130%) contrast(97%) drop-shadow(0 4px 8px rgba(59, 130, 246, 0.5));
    transform: scale(1.15) rotate(5deg);
  }
}

.lens-link {
  margin-right: 16px;
  font-weight: 500;
  padding: 8px 16px;
  border-radius: 8px;
  background: rgba(64, 158, 255, 0.1);
  backdrop-filter: blur(10px);
  transition: all 0.3s ease;
  display: inline-flex;
  align-items: center;
  white-space: nowrap;

  .lens-link-text {
    margin-right: 4px;
  }

  .lens-link-arrow {
    font-size: 14px;
    display: none; // hidden by default

    @media (max-width: 1280px) {
      display: inline-flex; // show arrow on laptop screens
    }
  }

  &:hover {
    background: rgba(64, 158, 255, 0.2);
    transform: translateY(-1px);

    .lens-link-arrow {
      transform: translateX(2px); // arrow moves right on hover
    }
  }

  @media (max-width: 1280px) {
    padding: 6px 10px;
    font-size: 13px;
    margin-right: 8px;

    .lens-link-text {
      // Show only "SaFE" on laptop screens
      &::after {
        content: 'SaFE';
      }
      font-size: 0; // hide original text
      margin-right: 4px;

      &::after {
        font-size: 13px;
      }
    }
  }
}

.menu-divider {
  width: 1px;
  height: 36px; // Adjusted to match new height
  background: rgba(0, 0, 0, 0.1);
  margin: 0 16px; // Slightly more spacing
  align-self: center;
}

.dark .menu-divider {
  background: rgba(255, 255, 255, 0.1);
}

.logo-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 10px 0 20px;
  height: 100%;

  .app-logo {
    height: 34px; // Adjusted for better proportion with 72px height
    width: auto;
    max-width: 150px;
    object-fit: contain;
  }

  @media (max-width: 768px) {
    padding: 0 12px;

    .app-logo {
      height: 28px;
      max-width: 120px;
    }
  }

}

// Theme toggle button - simple icon button
.theme-toggle-btn {
  width: 36px;
  height: 36px;
  border-radius: 6px;
  background: transparent;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s ease;
  // margin-right: 12px;

  &:hover {
    background: var(--el-fill-color-light);
  }

  &:active {
    transform: scale(0.95);
  }

  .theme-icon {
    color: var(--el-text-color-regular);
    transition: color 0.2s ease;
  }

  &:hover .theme-icon {
    color: var(--el-color-primary);
  }
}

.dark .theme-toggle-btn {
  &:hover {
    background: rgba(255, 255, 255, 0.1);
  }
}

.dark .logo-cluster-wrapper {
  .cluster-select-compact {
    :deep(.el-input__wrapper) {
      background: rgba(255, 255, 255, 0.03);

      &:hover {
        background: rgba(255, 255, 255, 0.06);
      }

      &.is-focus {
        background: rgba(255, 255, 255, 0.1);
        box-shadow: 0 0 0 1px rgba(64, 158, 255, 0.25);
      }
    }
  }
}

.cluster-selector-left {
  display: flex;
  align-items: center;
  height: var(--ep-menu-item-height);

  .cluster-select {
    min-width: 140px;
    max-width: 200px;

    :deep(.el-input__wrapper) {
      background: transparent;
      border: none;
      box-shadow: none;
      padding: 0 6px;

      &:hover {
        background: rgba(64, 158, 255, 0.06);
      }

      &.is-focus {
        background: rgba(64, 158, 255, 0.08);
      }
    }

    :deep(.el-input__inner) {
      font-size: 13px;
      color: var(--el-text-color-primary);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      padding: 0 4px;
    }

    :deep(.el-input__suffix) {
      margin-left: 2px;
    }
  }
}

.dark .cluster-selector-left {
  .cluster-select {
    :deep(.el-input__wrapper) {
      &:hover {
        background: rgba(255, 255, 255, 0.06);
      }

      &.is-focus {
        background: rgba(255, 255, 255, 0.08);
      }
    }
  }
}

// Menu item with beta tag
.menu-item-with-tag {
  display: flex;
  align-items: center;
  gap: 8px; // Increased from 6px

  .beta-tag {
    margin-left: 4px; // Additional spacing
    font-size: 11px; // Smaller than menu text
    padding: 0 8px;
    height: 18px;
    line-height: 18px;
    border-radius: 3px;
    background: rgba(64, 158, 255, 0.12); // Low saturation blue background
    border: 1px solid rgba(64, 158, 255, 0.2);
    color: var(--el-color-primary); // Blue text
    font-weight: 500;
    letter-spacing: 0.3px;
    transform: none; // Remove scale
    box-shadow: none; // Remove shadow for subtlety

    :deep(.el-tag__content) {
      line-height: 18px;
    }
  }
}

.dark .menu-item-with-tag {
  .beta-tag {
    background: rgba(64, 158, 255, 0.15);
    border-color: rgba(64, 158, 255, 0.25);
    color: #67c3ff; // Slightly brighter blue for dark mode
  }
}

// Menu items container - prevent shrinking
.menu-items-container {
  display: flex;
  flex-shrink: 0; // Prevent container from shrinking
  align-items: center;

  .el-menu-item {
    flex-shrink: 0; // Prevent individual items from shrinking
  }
}

// More dropdown styles - match other menu items
.more-dropdown {
  flex-shrink: 0;
  margin: 0;

  .more-menu-item {
    display: flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
    font-weight: 500;
    padding: 0 32px; // Match other menu items
    min-width: 110px; // Match other menu items

    .menu-item-text {
      font-size: 14px;
    }

    .more-icon {
      font-size: 14px;
      margin-left: 4px;
      transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
      color: var(--el-text-color-secondary);
    }

    // Apply same hover effect as other menu items
    &:hover .more-icon {
      color: var(--el-color-primary);
    }
  }

  // Rotate icon when dropdown is active
  &:hover .more-icon {
    transform: rotate(180deg);
  }

  // Match the active state styling
  :deep(.el-menu-item) {
    position: relative;

    &::after {
      content: '';
      position: absolute;
      bottom: 0;
      left: 50%;
      width: 0;
      height: 2px;
      background: var(--el-color-primary);
      transform: translateX(-50%);
      transition: width 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    }

    &:hover::after {
      width: 30px;
      opacity: 0.5;
    }
  }
}

// Note: Dropdown menu styles moved to the non-scoped style block at the bottom of the file
// because Element Plus dropdowns are teleported to body, so scoped styles won't apply

// Icon buttons spacing
.icon-btn {
  min-width: 48px !important;
  padding: 0 12px !important;

  &.grafana-btn {
    margin-left: auto !important;
  }

  &.settings-btn {
    margin-left: 0 !important;
  }
}

// Optimize dark mode transition - only for theme icon
.theme-icon.i-ep-moon,
.theme-icon.i-ep-sunny {
  &::before {
    transition: all 0.3s ease;
  }
}

// User dropdown styles
// User dropdown styles
.user-dropdown {
  margin-left: 12px;
  margin-right: 0;
  height: 100%;
  display: flex;
  align-items: center;

  .user-info {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 12px 6px 6px;
    border-radius: 20px;
    cursor: pointer;
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    height: 40px;
    background: rgba(255, 255, 255, 0.5);
    backdrop-filter: blur(10px);

    &:hover {
      background: rgba(64, 158, 255, 0.12);
      box-shadow: 0 2px 8px rgba(64, 158, 255, 0.15);
      transform: translateY(-1px);
    }


    .user-avatar {
      background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
      border: 2px solid rgba(255, 255, 255, 0.9);
      box-shadow: 0 2px 8px rgba(102, 126, 234, 0.3);
      transition: all 0.3s ease;

      .avatar-text {
        font-size: 12px;
        font-weight: 600;
        color: white;
        letter-spacing: 0.5px;
        text-shadow: 0 1px 2px rgba(0, 0, 0, 0.2);
      }
    }

    .user-name {
      font-size: 14px;
      font-weight: 500;
      color: var(--el-text-color-primary);
      max-width: 120px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .dropdown-arrow {
      margin-left: 4px;
      font-size: 12px;
      color: var(--el-text-color-secondary);
      transition: transform 0.3s ease;
    }
  }
}

// Dark mode adaptation for user dropdown
.dark .user-dropdown {
  .user-info {
    background: rgba(30, 30, 30, 0.5);

    &:hover {
      background: rgba(64, 158, 255, 0.15);
    }

    .user-avatar {
      background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
      border-color: rgba(255, 255, 255, 0.2);

      .avatar-text {
        color: #1a1a1a;
        font-weight: 700;
      }
    }
  }
}

// User detail styles moved to the non-scoped style block at the bottom of the file

// Medium screen responsive styles
@media (max-width: 1024px) {
  // User dropdown responsive
  .user-dropdown {
    margin-right: 8px;

    .user-info {
      padding: 4px 8px;

      .user-name {
        max-width: 80px;
        font-size: 13px;
      }
    }
  }

  // Adjust SaFE link for smaller screens


  // Compress icon buttons spacing
  .el-menu-item.ml-auto {
    margin-left: 0 !important;
    padding: 0 8px;
    min-width: auto;
  }
}

// Medium screen responsive styles (1280px and below)
@media (max-width: 1280px) {
  .el-menu-demo {
    .el-menu-item {
      padding: 0 20px;
      min-width: 80px !important; // force override default value
      margin: 0 6px;

      &.ml-auto {
        min-width: auto; // icon buttons don't need min-width
        padding: 0 10px;
      }
    }
  }
}

// Small screen responsive styles
@media (max-width: 768px) {
  .el-menu-demo {
    height: 60px;
    padding: 0 8px;

    .el-menu-item {
      height: 60px;
      line-height: 60px;
      padding: 0 16px;
      min-width: auto !important; // force override default min-width
      margin: 0 4px;
      font-size: 13px;

      // Icon buttons get minimal padding
      &.ml-auto {
        padding: 0 6px;
        margin: 0 2px;
      }

    }

    .logo-wrapper {
      padding: 0 12px;

      .app-logo {
        height: 28px;
      }
    }

    .menu-divider {
      height: 28px;
      margin: 0 8px;
    }

    // Compact SaFE link


    // Compact beta tag
    .menu-item-with-tag {
      .beta-tag {
        font-size: 10px;
        padding: 0 6px;
        height: 16px;
        line-height: 16px;
        margin-left: 2px;
      }
    }
  }
}
</style>

<style lang="scss">
// Global styles - for dropdown menus (teleported to body)
.header-more-dropdown-menu,
.header-user-dropdown-menu {
  margin-top: 12px !important;
  border-radius: 12px !important;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12) !important;
  backdrop-filter: blur(20px) saturate(180%) !important;
  -webkit-backdrop-filter: blur(20px) saturate(180%) !important;
  background: rgba(255, 255, 255, 0.95) !important;
  border: 1px solid rgba(0, 0, 0, 0.06) !important;
  padding: 8px !important;
  min-width: 180px !important;
  overflow-x: hidden !important;

  .el-dropdown-menu__item {
    padding: 12px 16px !important;
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1) !important;
    border-radius: 8px !important;
    margin: 4px 0 !important;
    font-size: 14px !important;
    font-weight: 500 !important;
    height: auto !important;
    min-height: 40px !important;
    display: flex !important;
    align-items: center !important;
    white-space: nowrap !important;
    overflow-x: hidden !important;

    &.is-active {
      color: var(--el-color-primary) !important;
      background: linear-gradient(
        135deg,
        rgba(64, 158, 255, 0.1) 0%,
        rgba(103, 194, 255, 0.15) 50%,
        rgba(64, 158, 255, 0.1) 100%
      ) !important;
      backdrop-filter: blur(16px) saturate(180%) !important;
      font-weight: 600 !important;
      box-shadow: 0 2px 8px rgba(64, 158, 255, 0.15) !important;
    }

    &:hover {
      background: rgba(64, 158, 255, 0.08) !important;
      color: var(--el-color-primary) !important;
      transform: translateX(4px) !important;
    }

    .menu-item-with-tag {
      display: flex !important;
      align-items: center !important;
      gap: 8px !important;
      width: 100% !important;
      overflow: hidden !important;

      > span:first-child {
        overflow: hidden !important;
        text-overflow: ellipsis !important;
        white-space: nowrap !important;
      }

      .beta-tag {
        margin-left: 4px !important;
        font-size: 11px !important;
        padding: 0 8px !important;
        height: 18px !important;
        line-height: 18px !important;
        border-radius: 3px !important;
        flex-shrink: 0 !important;
      }
    }

    // User detail styles
    .user-detail {
      padding: 4px 0 !important;
      width: 100% !important;
      overflow: hidden !important;

      .detail-name {
        font-size: 14px !important;
        font-weight: 600 !important;
        color: var(--el-text-color-primary) !important;
        margin-bottom: 4px !important;
        overflow: hidden !important;
        text-overflow: ellipsis !important;
        white-space: nowrap !important;
      }

      .detail-email {
        font-size: 12px !important;
        color: var(--el-text-color-secondary) !important;
        overflow: hidden !important;
        text-overflow: ellipsis !important;
        white-space: nowrap !important;
      }
    }

    // Icon styles
    .el-icon {
      font-size: 16px !important;
      margin-right: 8px !important;
      flex-shrink: 0 !important;
    }

    > span {
      font-size: 14px !important;
      overflow: hidden !important;
      text-overflow: ellipsis !important;
      white-space: nowrap !important;
    }
  }
}

// User dropdown menu set to a wider width
.header-user-dropdown-menu {
  min-width: 200px !important;
}

// Dark mode
.dark {
  .header-more-dropdown-menu,
  .header-user-dropdown-menu {
    background: rgba(30, 30, 30, 0.95) !important;
    border-color: rgba(255, 255, 255, 0.1) !important;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4) !important;

    .el-dropdown-menu__item {
      &.is-active {
        background: linear-gradient(
          135deg,
          rgba(64, 158, 255, 0.15) 0%,
          rgba(103, 194, 255, 0.2) 50%,
          rgba(64, 158, 255, 0.15) 100%
        ) !important;
      }

      &:hover {
        background: rgba(64, 158, 255, 0.12) !important;
      }
    }
  }
}
</style>

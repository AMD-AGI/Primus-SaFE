<template>
  <aside class="h-full flex flex-col" style="border-right: 1px solid var(--el-menu-border-color)">
    <!-- Title -->
    <div class="flex items-center justify-between mt-8 mb-4 px-4">
      <img class="app-logo" :src="isDark ? '/logo_w.png' : '/logo_b.png'" alt="Primus SaFE" />
      <button
        class="theme-toggle"
        :title="isDark ? 'Switch to light mode' : 'Switch to dark mode'"
        @click="toggleDark()"
      >
        <el-icon :size="18">
          <component :is="isDark ? Moon : Sunny" />
        </el-icon>
      </button>
    </div>
    <!-- Search (pinned above the scroll area) -->
    <div class="menu-search px-5 pt-1 pb-2">
      <el-input
        v-model="searchQuery"
        clearable
        placeholder="Search menu"
        :prefix-icon="Search"
      />
    </div>
    <!-- Middle: menu tree + search-results overlay -->
    <div class="menu-body flex-1 relative overflow-hidden">
      <!-- Menu tree stays mounted & displayed so el-menu keeps its layout width -->
      <div class="menu-scroll absolute inset-0 overflow-y-auto">
      <el-menu
        ref="menuRef"
        class="ws-menu relative flex-1 overflow-y-auto p-2"
        :ellipsis="false"
        :default-active="route.path"
        router
        active-text-color="var(--safe-primary)"
        :default-openeds="defaultOpeneds"
        style="border: none"
        @open="handleMenuOpen"
      >
        <el-menu-item-group title="Workspace">
          <div class="p-x-3 p-y-2">
            <WorkspaceSelect />
          </div>
        </el-menu-item-group>
        <!-- MenuBody -->
        <el-menu-item index="/">
          <el-icon><House /></el-icon>Homepage
        </el-menu-item>
        <el-menu-item index="/userquickstart" data-tour="menu-userquickstart">
          <MenuItemIcon
            index="/userquickstart"
            :light="menuIcons.quickstart.light"
            :dark="menuIcons.quickstart.dark"
            :active="menuIcons.quickstart.active"
            :size="16"
            match="prefix"
          />Quick Start
        </el-menu-item>
        <el-sub-menu index="workloads" class="nav-section workload-menu" v-if="workloadMenuItems.length">
          <template #title>
            <el-icon class="section-icon"><Cpu /></el-icon>
            <span class="nav-section-label">Workloads</span>
          </template>
          <MenuItem v-for="item in workloadMenuItems" :key="item.index" :item="item" />
        </el-sub-menu>

        <el-sub-menu index="artifacts" class="nav-section">
          <template #title>
            <el-icon class="section-icon"><FolderOpened /></el-icon>
            <span class="nav-section-label">Artifacts</span>
          </template>
          <el-menu-item index="/preflight/ws">
            <MenuItemIcon
              index="/preflight/ws"
              :light="menuIcons.preflight.light"
              :dark="menuIcons.preflight.dark"
              :active="menuIcons.preflight.active"
              :size="16"
              match="prefix"
            />Bench
          </el-menu-item>
          <el-menu-item index="/download">
            <MenuItemIcon
              index="/download"
              :light="menuIcons.download.light"
              :dark="menuIcons.download.dark"
              :active="menuIcons.download.active"
              :size="16"
              match="prefix"
            />Datasync
          </el-menu-item>
          <el-menu-item index="/images" data-tour="menu-images">
            <MenuItemIcon
              index="/images"
              :light="menuIcons.images.light"
              :dark="menuIcons.images.dark"
              :active="menuIcons.images.active"
              :size="16"
              match="prefix"
            />Images
          </el-menu-item>
          <el-menu-item index="/secrets" data-tour="menu-secrets">
            <MenuItemIcon
              index="/secrets"
              :light="menuIcons.secrets.light"
              :dark="menuIcons.secrets.dark"
              :active="menuIcons.secrets.active"
              :size="16"
              match="prefix"
            />Secrets
          </el-menu-item>
          <el-menu-item index="/manageapikeys">
            <MenuItemIcon
              index="/manageapikeys"
              :light="menuIcons.apikey.light"
              :dark="menuIcons.apikey.dark"
              :active="menuIcons.apikey.active"
              :size="16"
              match="prefix"
            />API Keys
          </el-menu-item>
        </el-sub-menu>

        <!-- Model Lab Menu -->
        <el-sub-menu index="model-lab" class="nav-section" v-if="hasManagerAccess">
          <template #title>
            <el-icon class="section-icon"><MagicStick /></el-icon>
            <span class="nav-section-label">Model Lab</span>
          </template>
          <MenuItem v-for="item in modelLabMenuItems" :key="item.index" :item="item" />
        </el-sub-menu>

        <!-- AI Agent Menu -->
        <el-sub-menu index="ai-agent" class="nav-section" v-if="aiAgentMenuItems.length">
          <template #title>
            <el-icon class="section-icon"><ChatDotRound /></el-icon>
            <span class="nav-section-label">AI Agent</span>
          </template>
          <MenuItem v-for="item in aiAgentMenuItems" :key="item.index" :item="item" />
        </el-sub-menu>

        <!-- Agent Infra Menu -->
        <el-sub-menu index="agent-infra" class="nav-section" v-if="agentInfraMenuItems.length">
          <template #title>
            <el-icon class="section-icon"><Connection /></el-icon>
            <span class="nav-section-label">Agent Infra</span>
          </template>
          <MenuItem v-for="item in agentInfraMenuItems" :key="item.index" :item="item" />
        </el-sub-menu>

        <!-- System Menu -->
        <el-sub-menu index="1" class="nav-section" v-if="systemMenuItems.length">
          <template #title>
            <el-icon class="section-icon"><Setting /></el-icon>
            <span class="nav-section-label">System</span>
          </template>
          <MenuItem v-for="item in systemMenuItems" :key="item.index" :item="item" />

          <el-sub-menu index="/addontemplate" class="addon-catalog-sub" v-if="hasManagerAccess">
            <template #title>
              <MenuItemIcon
                index="/addontemplate"
                :light="menuIcons.addon.light"
                :dark="menuIcons.addon.dark"
                :active="menuIcons.addon.active"
                :size="16"
                match="prefix"
              />
              <span style="color: var(--el-menu-text-color)">Catalog</span>
            </template>

            <!-- Helm template -->
            <el-menu-item index="/addontemplate?type=helm"> Helm </el-menu-item>

            <!-- Node template -->
            <el-menu-item index="/addontemplate?type=default"> Node </el-menu-item>
          </el-sub-menu>
        </el-sub-menu>

        <!-- Extra menu items for workspace-admin -->
        <template v-if="canSeeWorkspaceAndUserMenu && !hasManagerAccess">
          <MenuItem v-for="item in workspaceAdminMenuItems" :key="item.index" :item="item" />
        </template>
      </el-menu>
      </div>
      <!-- Search results overlay (covers the tree while typing) -->
      <div
        v-if="isSearching"
        class="menu-search-results absolute inset-0 overflow-y-auto px-2 py-2"
      >
        <button
          v-for="r in filteredResults"
          :key="r.path"
          type="button"
          class="menu-search-item"
          :class="{ 'is-active': r.path === route.fullPath }"
          @click="goResult(r.path)"
        >
          <span class="menu-search-item__name">{{ r.name }}</span>
          <span v-if="r.section" class="menu-search-item__section">{{ r.section }}</span>
        </button>
        <div v-if="!filteredResults.length" class="menu-search-empty">No matches</div>
      </div>
    </div>

    <!-- User info section -->
    <div
      class="user-footer px-2 py-3 border-t border-[var(--el-border-color)] bg-[var(--safe-bg)] z-10"
    >
      <UserInfo />
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, shallowRef, watch, watchEffect, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import WorkspaceSelect from '../Base/WorkspaceSelect.vue'
import UserInfo from '../Base/UserInfo.vue'
import {
  House,
  Cpu,
  FolderOpened,
  MagicStick,
  ChatDotRound,
  Connection,
  Setting,
  Search,
  Sunny,
  Moon,
} from '@element-plus/icons-vue'

import { toggleDark } from '@/composables'
import MenuItemIcon from '@/components/Base/MenuItemIcon.vue'
import MenuItem from './MenuItem.vue'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import { useDark } from '@vueuse/core'
import { menuIcons } from './menuIcons'
import type { ScopesKeys } from '@/services/base/type'

const isDark = useDark()
const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const wsStore = useWorkspaceStore()
const hasManagerAccess = computed(() => userStore.hasManagerAccess)
const menuRef = ref()

// Elegant permission check function
const hasWorkloadScope = (scope: ScopesKeys) => {
  return computed(() => (wsStore.currentScopes ?? []).includes(scope))
}

// Workload permissions
const workloadPermissions = {
  canTrain: hasWorkloadScope('Train'),
  canAuthoring: hasWorkloadScope('Authoring'),
  canCICD: hasWorkloadScope('CICD'),
  canInfer: hasWorkloadScope('Infer'),
  canRay: hasWorkloadScope('Ray'),
  canSandbox: hasWorkloadScope('Sandbox'),
}

// Destructure specific permission variables (maintain backward compatibility)
const { canTrain, canAuthoring, canCICD, canInfer, canRay, canSandbox } = workloadPermissions

// workspaceMenu and usersMenu:
// Visible to system-admin, system-admin-readonly, or current workspace-admin
const canSeeWorkspaceAndUserMenu = computed(() => {
  return hasManagerAccess.value || wsStore.isCurrentWorkspaceAdmin()
})

// Menu config type definition
interface MenuItem {
  index: string
  name: string
  icon?: {
    light: string
    dark: string
    active: string
  }
  elIcon?: Component // Element Plus icon component
  canAccess?: boolean
  tooltip?: string
  dataTour?: string
  children?: MenuItem[]
}

// Workload menu config - use shallowRef to avoid deep reactivity
const workloadMenuItems = shallowRef<MenuItem[]>([])
const workloadGroupIndexes = ['workloads-infer', 'workloads-training']
const workloadGroupRoutes = [
  { group: 'workloads-infer', paths: ['/infer', '/infera'] },
  {
    group: 'workloads-training',
    paths: ['/training', '/torchft', '/rayjob', '/monarch'],
  },
]
const activeWorkloadGroup = computed(
  () => workloadGroupRoutes.find(({ paths }) => paths.some((path) => route.path.startsWith(path)))?.group,
)
const defaultOpeneds = computed(() => [
  'workloads',
  'artifacts',
  ...(activeWorkloadGroup.value ? [activeWorkloadGroup.value] : []),
])

const syncOpenWorkloadGroup = async () => {
  await nextTick()
  const currentGroup = activeWorkloadGroup.value
  workloadGroupIndexes.forEach((index) => {
    if (index === currentGroup) {
      menuRef.value?.open(index)
      return
    }
    menuRef.value?.close(index)
  })
}

const handleMenuOpen = (index: string) => {
  if (!workloadGroupIndexes.includes(index)) return
  workloadGroupIndexes.forEach((groupIndex) => {
    if (groupIndex !== index) menuRef.value?.close(groupIndex)
  })
}

watch(() => route.path, syncOpenWorkloadGroup, { immediate: true })

// Only update menu config when permissions change
watchEffect(() => {
  const inferChildren: MenuItem[] = [
    {
      index: '/infer',
      name: 'Deployment',
      canAccess: canInfer.value,
      tooltip: 'Deployment has been disabled by the administrator.',
      icon: menuIcons.deployment,
    },
    {
      index: '/infera',
      name: 'Infera',
      canAccess: canInfer.value,
      tooltip: 'Infera has been disabled by the administrator.',
      icon: menuIcons.infera,
    },
  ].filter((item) => item.canAccess !== false)

  const trainingChildren: MenuItem[] = [
    {
      index: '/training',
      name: 'PyTorch',
      canAccess: canTrain.value,
      tooltip: 'PyTorch has been disabled by the administrator.',
      icon: menuIcons.pytorch,
      dataTour: 'menu-training',
    },
    {
      index: '/torchft',
      name: 'TorchFT',
      canAccess: canTrain.value,
      tooltip: 'TorchFT has been disabled by the administrator.',
      icon: menuIcons.torchft,
    },
    {
      index: '/rayjob',
      name: 'RayJob',
      canAccess: canRay.value,
      tooltip: 'RayJob has been disabled by the administrator.',
      icon: menuIcons.rayjob,
    },
    {
      index: '/monarch',
      name: 'Monarch',
      canAccess: canTrain.value,
      tooltip: 'Monarch has been disabled by the administrator.',
      icon: menuIcons.monarch,
    },
  ].filter((item) => item.canAccess !== false)

  const allWorkloadItems: MenuItem[] = [
    ...(inferChildren.length
      ? [
          {
            index: 'workloads-infer',
            name: 'Infer',
            icon: menuIcons.infer,
            children: inferChildren,
          },
        ]
      : []),
    ...(trainingChildren.length
      ? [
          {
            index: 'workloads-training',
            name: 'Training',
            icon: menuIcons.training,
            children: trainingChildren,
          },
        ]
      : []),
    {
      index: '/authoring',
      name: 'Authoring',
      canAccess: canAuthoring.value,
      tooltip: 'Authoring has been disabled by the administrator.',
      icon: menuIcons.authoring,
      dataTour: 'menu-authoring',
    },
    {
      index: '/sandbox-workload',
      name: 'Sandbox',
      canAccess: canSandbox.value,
      tooltip: 'Sandbox has been disabled by the administrator.',
      icon: menuIcons.sandbox,
    },
    {
      index: '/cicd',
      name: 'CICD',
      canAccess: canCICD.value,
      tooltip: 'CICD has been disabled by the administrator.',
      icon: menuIcons.cicd,
    },
  ]

  // Filter out menu items without permission, simply hide them
  workloadMenuItems.value = allWorkloadItems.filter((item) => item.canAccess !== false)
})

// Model Lab submenu config - static
const modelLabMenuItems: MenuItem[] = [
  {
    index: '/model-square',
    name: 'Model Square',
    icon: menuIcons.modelSquare,
  },
  {
    index: '/posttrain',
    name: 'Post Train',
    icon: menuIcons.training,
  },
  {
    index: '/playground-agent',
    name: 'Playground',
    icon: menuIcons.playground,
  },
  {
    index: '/dataset',
    name: 'Dataset',
    icon: menuIcons.dataset,
  },
  {
    index: '/evaluation',
    name: 'Evaluation',
    icon: menuIcons.evaluation,
  },
  {
    index: '/model-optimization',
    name: 'Optimization',
    icon: menuIcons.modelOptimization,
  },
]

// AI Agent submenu config - dynamic (partially open to regular users)
const aiAgentMenuItems = shallowRef<MenuItem[]>([])

// Watch permission changes, dynamically update AI Agent menu
watchEffect(() => {
  const allAiAgentItems = [
    {
      index: '/qabase',
      name: 'QA Base',
      icon: menuIcons.qabase,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/feedback-management',
      name: 'Feedback',
      icon: menuIcons.usermanage,
      canAccess: hasManagerAccess.value, // Admin only
    },
  ]

  // Filter out menu items without permission
  aiAgentMenuItems.value = allAiAgentItems.filter((item) => item.canAccess !== false)
})

// Agent Infra submenu config - dynamic
const agentInfraMenuItems = shallowRef<MenuItem[]>([])

watchEffect(() => {
  const allAgentInfraItems = [
    // {
    //   index: '/tools',
    //   name: 'Plugins',
    //   icon: menuIcons.tools,
    //   canAccess: true,
    // },
    {
      index: '/sandbox',
      name: 'Sandbox',
      icon: menuIcons.sandbox,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/litellm-gateway',
      name: 'LLM Gateway',
      icon: menuIcons.llmGateway,
      canAccess: true, // Open to all users
    },
    {
      index: '/a2a',
      name: 'A2A Protocol',
      icon: menuIcons.a2a,
      canAccess: true, // Open to all users
    },
  ]

  agentInfraMenuItems.value = allAgentInfraItems.filter((item) => item.canAccess !== false)
})

// System submenu config - dynamic (partially open to regular users)
const systemMenuItems = shallowRef<MenuItem[]>([])

// Watch permission changes, dynamically update System menu
watchEffect(() => {
  const allSystemItems = [
    {
      index: '/quickstart',
      name: 'Quick Start',
      icon: menuIcons.quickstart,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/nodeflavor',
      name: 'Flavors',
      dataTour: 'menu-nodeflavors',
      icon: menuIcons.flavors,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/nodes',
      name: 'Nodes',
      dataTour: 'menu-nodes',
      icon: menuIcons.node,
      canAccess: true, // Open to all users
    },
    {
      index: '/clusters',
      name: 'Clusters',
      dataTour: 'menu-clusters',
      icon: menuIcons.cluster,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/workspace',
      name: 'Workspaces',
      dataTour: 'menu-workspace',
      icon: menuIcons.queue,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/usermanage',
      name: 'Users',
      icon: menuIcons.usermanage,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/user-group',
      name: 'User Group',
      icon: menuIcons.usermanage,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/fault',
      name: 'Faults',
      icon: menuIcons.fault,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/preflight',
      name: 'Bench',
      icon: menuIcons.diagnoser,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/registries',
      name: 'Registries',
      icon: menuIcons.registry,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/addons',
      name: 'Addons',
      icon: menuIcons.addons,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/deploy',
      name: 'Deploy',
      icon: menuIcons.deploy,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/auditlogs',
      name: 'Audit Logs',
      icon: menuIcons.fault,
      canAccess: hasManagerAccess.value, // Admin only
    },
    {
      index: '/workload-manage',
      name: 'Workloads',
      icon: menuIcons.training,
      canAccess: hasManagerAccess.value, // Admin only
    },
  ]

  // Filter out menu items without permission
  systemMenuItems.value = allSystemItems.filter((item) => item.canAccess !== false)
})

// Extra menu items for workspace-admin - static (Nodes already in System menu)
const workspaceAdminMenuItems: MenuItem[] = [
  {
    index: '/workspace',
    name: 'Workspaces',
    icon: menuIcons.queue,
  },
  {
    index: '/usermanage',
    name: 'Users',
    icon: menuIcons.usermanage,
  },
]

// --- Menu search (real-time filter) ---
const searchQuery = ref('')
const isSearching = computed(() => searchQuery.value.trim().length > 0)

interface FlatNavItem {
  name: string
  path: string
  section: string
}

// Hardcoded template entries not covered by the dynamic *MenuItems arrays.
const staticNavItems: FlatNavItem[] = [
  { name: 'Homepage', path: '/', section: '' },
  { name: 'Quick Start', path: '/userquickstart', section: '' },
  { name: 'Bench', path: '/preflight/ws', section: 'Artifacts' },
  { name: 'Datasync', path: '/download', section: 'Artifacts' },
  { name: 'Images', path: '/images', section: 'Artifacts' },
  { name: 'Secrets', path: '/secrets', section: 'Artifacts' },
  { name: 'API Keys', path: '/manageapikeys', section: 'Artifacts' },
  { name: 'Catalog · Helm', path: '/addontemplate?type=helm', section: 'System' },
  { name: 'Catalog · Node', path: '/addontemplate?type=default', section: 'System' },
]

const flattenNav = (items: MenuItem[], section: string): FlatNavItem[] => {
  const out: FlatNavItem[] = []
  for (const item of items) {
    if (item.children?.length) {
      out.push(...flattenNav(item.children, section))
    } else if (item.index?.startsWith('/')) {
      out.push({ name: item.name, path: item.index, section })
    }
  }
  return out
}

const searchIndex = computed<FlatNavItem[]>(() => [
  ...staticNavItems,
  ...flattenNav(workloadMenuItems.value, 'Workloads'),
  ...(hasManagerAccess.value ? flattenNav(modelLabMenuItems, 'Model Lab') : []),
  ...flattenNav(aiAgentMenuItems.value, 'AI Agent'),
  ...flattenNav(agentInfraMenuItems.value, 'Agent Infra'),
  ...flattenNav(systemMenuItems.value, 'System'),
  ...(canSeeWorkspaceAndUserMenu.value && !hasManagerAccess.value
    ? flattenNav(workspaceAdminMenuItems, '')
    : []),
])

const filteredResults = computed<FlatNavItem[]>(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return []
  const seen = new Set<string>()
  return searchIndex.value.filter((item) => {
    if (seen.has(item.path)) return false
    const hit = item.name.toLowerCase().includes(q) || item.section.toLowerCase().includes(q)
    if (hit) seen.add(item.path)
    return hit
  })
})

const goResult = (path: string) => {
  searchQuery.value = ''
  if (path !== route.fullPath) router.push(path)
}
</script>
<style scoped>
/* Static section labels (Workspace / Workloads / Artifacts / ...) — passive,
   uppercase and muted so they read as dividers, not clickable nav items. */
.el-menu :deep(.el-menu-item-group__title) {
  color: var(--safe-primary) !important;
  font-weight: 600 !important;
  font-size: 12px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  padding: 6px 20px 4px;
  line-height: 1.4;
  height: auto;
  margin-top: 6px;
}

/* When nested sub-menu is active, span turns blue */
.el-menu :deep(.el-sub-menu .el-sub-menu.is-active > .el-sub-menu__title span) {
  color: var(--safe-primary) !important;
}

.el-menu :deep(.workload-menu .menu-branch.is-active > .el-sub-menu__title span) {
  color: var(--el-menu-text-color) !important;
}

/* Nested sub-menu icon should not be blue when inactive */
.el-menu :deep(.el-sub-menu .el-sub-menu__title .menu-icon) {
  filter: none !important;
}
.menu-icon {
  width: 18px;
  height: 18px;
  margin-right: 6px;
  vertical-align: middle;
}
.menu-scroll {
  overscroll-behavior: contain;
}

/* --- Menu search --- */
/* Sidebar search: self-contained soft-filled look (independent of global inputs). */
.menu-search :deep(.el-input__wrapper) {
  background: color-mix(in srgb, var(--el-text-color-secondary) 8%, transparent) !important;
  border: 1px solid transparent !important;
  border-radius: 8px;
  box-shadow: none !important;
}
.menu-search :deep(.el-input__wrapper:hover) {
  background: color-mix(in srgb, var(--el-text-color-secondary) 12%, transparent) !important;
}
.menu-search :deep(.el-input__wrapper.is-focus) {
  border-color: transparent !important;
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--safe-primary) 20%, transparent) !important;
}

.menu-search-results {
  display: flex;
  flex-direction: column;
  gap: 2px;
  z-index: 5;
  background: var(--safe-bg);
}
.menu-search-item {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  padding: 7px 12px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--el-menu-text-color);
  font-size: 14px;
  text-align: left;
  cursor: pointer;
}
.menu-search-item:hover {
  background: color-mix(in srgb, var(--safe-primary) 8%, transparent);
}
.menu-search-item.is-active {
  color: var(--safe-primary);
  background: color-mix(in srgb, var(--safe-primary) 12%, transparent);
}
.menu-search-item__name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.menu-search-item__section {
  flex-shrink: 0;
  font-size: 11px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--el-text-color-secondary);
}
.menu-search-empty {
  padding: 12px;
  text-align: center;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
.app-logo {
  height: 50px;
  width: auto;
  object-fit: contain;
  transition:
    filter 0.3s ease,
    opacity 0.3s ease;
}
.app-logo {
  transition: opacity 0.25s ease-in-out;
}
.dark .app-logo {
  opacity: 0.9;
}
.theme-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: var(--el-text-color-regular);
  cursor: pointer;
  transition:
    background 0.16s ease,
    color 0.16s ease;
}
.theme-toggle:hover {
  background: color-mix(in srgb, var(--el-text-color-secondary) 10%, transparent);
  color: var(--safe-primary);
}
</style>
<style>
/* Menu active rounded rectangle */
.ws-menu .el-menu-item,
.ws-menu .el-sub-menu .el-menu-item {
  margin: 4px 8px;
  border-radius: 8px;
}
/* Also add border-radius and spacing to first-level sub-menu titles */
.ws-menu > .el-sub-menu > .el-sub-menu__title {
  margin: 4px 8px;
  border-radius: 8px;
}

/* Top-level section headers: passive uppercase dividers, clearly distinct
   from the icon-bearing nav items they group, but still collapsible.
   Kept compact (short row, no big highlight) so the collapsed menu stays dense. */
.ws-menu > .el-sub-menu.nav-section > .el-sub-menu__title {
  height: 40px !important;
  line-height: 40px !important;
  margin: 4px 8px !important;
  padding: 0 12px !important;
  color: var(--safe-primary) !important;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  border-radius: 8px;
}
/* Section headers are text-only dividers — hide the leading icon. */
.ws-menu > .el-sub-menu.nav-section > .el-sub-menu__title .section-icon {
  display: none !important;
}
/* No big card background in any state — a section label is not a nav target. */
.ws-menu > .el-sub-menu.nav-section > .el-sub-menu__title,
.ws-menu > .el-sub-menu.nav-section.is-active > .el-sub-menu__title,
.ws-menu > .el-sub-menu.nav-section.is-opened > .el-sub-menu__title {
  background: transparent !important;
}
.ws-menu > .el-sub-menu.nav-section > .el-sub-menu__title:hover {
  background: color-mix(in srgb, var(--el-text-color-secondary) 8%, transparent) !important;
}
.ws-menu > .el-sub-menu.nav-section > .el-sub-menu__title .nav-section-label {
  color: inherit !important;
}
.ws-menu > .el-sub-menu.nav-section > .el-sub-menu__title .el-sub-menu__icon-arrow {
  right: 10px;
  font-size: 12px;
  color: color-mix(in srgb, var(--safe-primary) 65%, transparent);
}

/* Nested sub-menu titles (e.g. Catalog) also get rounded corners and spacing */
.ws-menu .el-sub-menu .el-sub-menu__title {
  margin: 4px 8px;
  border-radius: 8px;
}

/* Workload type groups stay lighter than route leaves. */
.ws-menu .workload-menu .menu-branch > .el-sub-menu__title {
  margin: 4px 8px;
  padding-left: 20px !important;
  color: var(--el-menu-text-color);
  border-radius: 8px;
}

.ws-menu .workload-menu .menu-branch.is-opened > .el-sub-menu__title {
  background: transparent;
}

.ws-menu .workload-menu .menu-branch.is-active > .el-sub-menu__title {
  color: var(--el-menu-text-color);
  background: transparent;
}

.ws-menu .workload-menu .menu-branch.is-active.is-opened > .el-sub-menu__title {
  background: transparent;
}

.ws-menu .workload-menu .menu-branch.is-opened > .el-sub-menu__title .el-sub-menu__icon-arrow {
  color: var(--safe-primary);
}

.ws-menu .workload-menu .menu-branch > .el-menu {
  position: relative;
  margin: 2px 0 10px;
}

/* Nested items under a first-level branch stay text-only (dot/indent, no icon). */
.ws-menu .workload-menu .menu-branch > .el-menu .menu-icon {
  display: none !important;
}

.ws-menu .workload-menu .menu-branch > .el-menu::before {
  content: '';
  position: absolute;
  top: 6px;
  bottom: 6px;
  left: 36px;
  width: 1px;
  background: color-mix(in srgb, var(--el-text-color-secondary) 10%, transparent);
}

.ws-menu .workload-menu .menu-branch > .el-menu > .el-menu-item {
  height: 40px;
  line-height: 40px;
  margin: 4px 10px 4px 44px;
  padding-left: 18px !important;
  border-radius: 8px;
  position: relative;
}

.ws-menu .workload-menu .menu-branch > .el-menu > .el-menu-item.is-active {
  color: var(--safe-primary);
  background: color-mix(in srgb, var(--safe-primary) 14%, transparent);
}

.ws-menu .workload-menu .menu-branch > .el-menu > .el-menu-item.is-active::before {
  content: '';
  position: absolute;
  left: 8px;
  top: 10px;
  bottom: 10px;
  width: 2px;
  border-radius: 999px;
  background: var(--safe-primary);
}
</style>

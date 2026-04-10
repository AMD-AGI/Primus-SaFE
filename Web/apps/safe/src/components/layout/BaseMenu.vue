<template>
  <aside class="h-full flex flex-col" style="border-right: 1px solid var(--el-menu-border-color)">
    <!-- Title -->
    <div class="flex items-center mt-8 mb-4 px-2">
      <div class="flex items-center mt-4 mb-2 px-2">
        <img class="app-logo" :src="isDark ? '/logo_w.png' : '/logo_b.png'" alt="Primus SaFE" />
        <button
          class="ml-5 cursor-pointer border-none bg-transparent flex items-center justify-center"
          @click="toggleDark()"
        >
          <i class="i-ep-sunny dark:i-ep-moon inline-block w-5 h-5 text-current align-middle" />
        </button>
      </div>
    </div>
    <!-- Middle menu items scroll area -->
    <div class="menu-scroll flex-1 overflow-y-auto">
      <el-menu
        class="ws-menu relative flex-1 overflow-y-auto p-2"
        :ellipsis="false"
        :default-active="route.path"
        router
        active-text-color="var(--safe-primary)"
        :default-openeds="['workloads', 'artifacts']"
        style="border: none"
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
        <el-sub-menu index="workloads" v-if="workloadMenuItems.length">
          <template #title>
            <span style="color: var(--safe-primary)">Workloads</span>
          </template>
          <MenuItem v-for="item in workloadMenuItems" :key="item.index" :item="item" />
        </el-sub-menu>

        <el-sub-menu index="artifacts">
          <template #title>
            <span style="color: var(--safe-primary)">Artifacts</span>
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
        <el-sub-menu index="model-lab" v-if="hasManagerAccess">
          <template #title>
            <span style="color: var(--safe-primary)">Model Lab</span>
          </template>
          <MenuItem v-for="item in modelLabMenuItems" :key="item.index" :item="item" />
        </el-sub-menu>

        <!-- AI Agent Menu -->
        <el-sub-menu index="ai-agent" v-if="aiAgentMenuItems.length">
          <template #title>
            <span style="color: var(--safe-primary)">AI Agent</span>
          </template>
          <MenuItem v-for="item in aiAgentMenuItems" :key="item.index" :item="item" />
        </el-sub-menu>

        <!-- Agent Infra Menu -->
        <el-sub-menu index="agent-infra" v-if="agentInfraMenuItems.length">
          <template #title>
            <span style="color: var(--safe-primary)">Agent Infra</span>
          </template>
          <MenuItem v-for="item in agentInfraMenuItems" :key="item.index" :item="item" />
        </el-sub-menu>

        <!-- System Menu -->
        <el-sub-menu index="1" v-if="systemMenuItems.length">
          <template #title>
            <span style="color: var(--safe-primary)">System</span>
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

    <!-- User info section -->
    <div
      class="user-footer px-2 py-3 border-t border-[var(--el-border-color)] bg-[var(--safe-bg)] z-10"
    >
      <UserInfo />
    </div>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import WorkspaceSelect from '../Base/WorkspaceSelect.vue'
import UserInfo from '../Base/UserInfo.vue'
import { House } from '@element-plus/icons-vue'

import { type Component, shallowRef, watchEffect } from 'vue'
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
const userStore = useUserStore()
const wsStore = useWorkspaceStore()
const hasManagerAccess = computed(() => userStore.hasManagerAccess)

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

// Only update menu config when permissions change
watchEffect(() => {
  const allWorkloadItems = [
    {
      index: '/training',
      name: 'Training',
      canAccess: canTrain.value,
      tooltip: 'Training has been disabled by the administrator.',
      icon: menuIcons.training,
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
    {
      index: '/authoring',
      name: 'Authoring',
      canAccess: canAuthoring.value,
      tooltip: 'Authoring has been disabled by the administrator.',
      icon: menuIcons.authoring,
      dataTour: 'menu-authoring',
    },
    {
      index: '/cicd',
      name: 'CICD',
      canAccess: canCICD.value,
      tooltip: 'CICD has been disabled by the administrator.',
      icon: menuIcons.cicd,
    },
    {
      index: '/infer',
      name: 'Infer',
      canAccess: canInfer.value,
      tooltip: 'Infer has been disabled by the administrator.',
      icon: menuIcons.infer,
    },
    {
      index: '/sandbox-workload',
      name: 'Sandbox',
      canAccess: canSandbox.value,
      tooltip: 'Sandbox has been disabled by the administrator.',
      icon: menuIcons.playground,
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
]

// AI Agent submenu config - dynamic (partially open to regular users)
const aiAgentMenuItems = shallowRef<MenuItem[]>([])

// Watch permission changes, dynamically update AI Agent menu
watchEffect(() => {
  const allAiAgentItems = [
    {
      index: '/chatbot',
      name: 'Chatbot',
      icon: menuIcons.chatbot,
      canAccess: true, // Open to all users
    },
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
    {
      index: '/tools',
      name: 'Tools',
      icon: menuIcons.tools,
      canAccess: true, // Open to all users
    },
    {
      index: '/sandbox',
      name: 'Sandbox',
      icon: menuIcons.playground,
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
</script>
<style scoped>
.el-menu :deep(.el-menu-item-group__title) {
  color: var(--safe-primary) !important;
  font-weight: 400 !important;
  font-size: 15px;
  padding: 7px 20px;
  line-height: 40px;
  height: 56px;
}

/* When nested sub-menu is active, span turns blue */
.el-menu :deep(.el-sub-menu .el-sub-menu.is-active > .el-sub-menu__title span) {
  color: var(--safe-primary) !important;
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

/* Nested sub-menu titles (e.g. Catalog) also get rounded corners and spacing */
.ws-menu .el-sub-menu .el-sub-menu__title {
  margin: 4px 8px;
  border-radius: 8px;
}
</style>

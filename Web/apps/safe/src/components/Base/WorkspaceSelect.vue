<template>
  <el-select
    :model-value="store.currentWorkspaceId"
    class="ws-select w-full"
    popper-class="ws-select-popper"
    @visible-change="(v: any) => v && refetch()"
    @update:model-value="onWorkspaceChange"
    :teleported="false"
  >
    <!-- Small color block on the left of selected area -->
    <template #prefix>
      <span
        v-if="selected"
        class="ws-dot"
        :style="dotStyle(selected.workspaceName)"
        aria-hidden="true"
      />
    </template>

    <!-- Options: color block + text for each item -->
    <el-option
      v-for="ws in store.items"
      :key="ws.workspaceId"
      :label="ws.workspaceName"
      :value="ws.workspaceId"
    >
      <div class="ws-option">
        <span class="ws-dot" :style="dotStyle(ws.workspaceName)" aria-hidden="true" />
        <span class="ws-label">{{ ws.workspaceName }}</span>
      </div>
    </el-option>
  </el-select>
</template>

<script setup lang="ts">
import { computed, nextTick, watch } from 'vue'
import { useDark } from '@vueuse/core'
import { useWorkspaceStore } from '@/stores/workspace'
import { useRoute, useRouter } from 'vue-router'
import type { ScopesKeys } from '@/services/base/type'

const route = useRoute()
const router = useRouter()

const store = useWorkspaceStore()
const isDark = useDark()

const getRequiredScopeByPath = (path: string): ScopesKeys | undefined => {
  // Detail pages also validate by basePath
  const basePath = path.replace(/\/detail(?:\/.*)?$/, '')
  const rules: Array<[string, ScopesKeys]> = [
    ['/training', 'Train'],
    ['/torchft', 'Train'],
    ['/infer', 'Infer'],
    ['/authoring', 'Authoring'],
    ['/cicd', 'CICD'],
    ['/rayjob', 'Ray'],
  ]
  return rules.find(([prefix]) => basePath.startsWith(prefix))?.[1]
}

const getFirstAllowedWorkloadPath = (scopes: ScopesKeys[]): string | undefined => {
  // When multiple permissions exist, prefer more commonly used entry
  if (scopes.includes('Train')) return '/training'
  if (scopes.includes('Infer')) return '/infer'
  if (scopes.includes('Authoring')) return '/authoring'
  if (scopes.includes('CICD')) return '/cicd'
  if (scopes.includes('Ray')) return '/rayjob'
  return undefined
}

const onWorkspaceChange = async (val: string | undefined) => {
  const nextId = val ?? ''
  // Ensure workspace switch + new scopes fetch completes before route validation/redirect
  await store.setCurrentWorkspace(nextId)
  await nextTick()

  const required = getRequiredScopeByPath(route.path)
  if (required) {
    const scopes = (store.currentScopes ?? []) as ScopesKeys[]
    if (!scopes.includes(required)) {
      const fallback = getFirstAllowedWorkloadPath(scopes)
      await router.replace({ path: fallback ?? '/' })
      return
    }
  }

  const basePath = route.path.replace(/\/detail(?:\/.*)?$/, '')
  if (basePath === route.path) return
  router.replace({ path: basePath })
}

const selected = computed(
  () => store.items?.find((i) => i.workspaceId === store.currentWorkspaceId) || '',
)

const refetch = () => store.fetchWorkspace(true)

// After switching workspace, auto-redirect if current page has no permission in new workspace
watch(
  () => [store.currentWorkspaceId, store.items, route.path] as const,
  async () => {
    if (!store.isFetched) return
    if (!store.currentWorkspaceId) return

    const required = getRequiredScopeByPath(route.path)
    if (!required) return

    const scopes = (store.currentScopes ?? []) as ScopesKeys[]
    if (scopes.includes(required)) return

    const fallback = getFirstAllowedWorkloadPath(scopes)
    await router.replace({ path: fallback ?? '/' })
  },
  { flush: 'post' },
)

// Generate stable color (HSL) from name
function colorFromName(name: string) {
  let h = 5381
  for (let i = 0; i < name.length; i++) h = (h << 5) + h + name.charCodeAt(i)
  const hue = ((h % 360) + 360) % 360
  const sat = 68
  const light = isDark.value ? 62 : 45
  return `hsl(${hue} ${sat}% ${light}%)`
}

const dotStyle = (label: string) => ({
  backgroundColor: colorFromName(label),
  borderColor: isDark.value ? 'rgba(255,255,255,.22)' : 'rgba(0,0,0,.12)',
})
</script>

<style>
.ws-select .el-select__wrapper {
  box-shadow: none;
  width: 90%;
  margin: 0 auto;
  /* Align overall style with design */
  border-radius: 10px;
  background: var(--safe-card);
  border: 1px solid var(--safe-border);
}
.ws-select .el-select__wrapper span {
  font-size: 14px;
}
.ws-select .el-select__wrapper.is-hovering:not(.is-focused) {
  box-shadow: none !important;
}
.ws-select .el-select__placeholder {
  font-weight: 500;
}

/* Color block and option layout */
.ws-dot {
  display: inline-block;
  width: 12px;
  height: 12px;
  border-radius: 4px;
  border: 1px solid transparent;
  margin-right: 8px;
}
.ws-option {
  display: flex;
  align-items: center;
}
.ws-option .ws-dot {
  margin-right: 10px;
}
.ws-label {
  line-height: 1;
}
.ws-select-popper .el-select-dropdown__item {
  display: flex;
  align-items: center;
  line-height: 1; /* Prevent misalignment from line-height */
}

/* Dropdown popup adapts to safe theme (dark/light auto-switch) */
.ws-select-popper {
  --el-bg-color-overlay: var(--safe-card);
  --el-text-color-regular: var(--safe-text);
  --el-border-color-light: var(--safe-border);
}
</style>

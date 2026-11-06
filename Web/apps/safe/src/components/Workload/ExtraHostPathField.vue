<template>
  <el-form-item
    v-if="shown"
    label="extra mounts"
    :prop="prop"
    :rules="rules"
    class="kv-full"
    data-testid="extra-hostpath-field"
  >
    <el-select
      v-model="model"
      multiple
      clearable
      filterable
      allow-create
      default-first-option
      collapse-tags
      collapse-tags-tooltip
      :max-collapse-tags="4"
      :reserve-keyword="false"
      :disabled="disabled"
      :placeholder="placeholder"
      no-data-text="Type a directory and press Enter"
      style="width: 100%"
      @change="onChange"
    />
    <div class="extra-hostpath-hint">
      <div>
        Extra host directories to mount, each at the same path in the container. Each must be a
        directory under a workspace volume<template v-if="rootsText"> ({{ rootsText }})</template>,
        not the volume itself and not its <code>users</code> directory.
      </div>
      <div v-if="ownDirsText">
        Keep private data in <code>{{ ownDirsText }}</code
        >: it is mounted automatically and only you can see it.
      </div>
    </div>
  </el-form-item>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import type { FormItemRule } from 'element-plus'
import { getWorkspaceDetail } from '@/services/workspace/index'
import { useUserStore } from '@/stores/user'
import {
  checkHostPathEntry,
  declaredHostPathRoots,
  normalizeHostPaths,
  ownUserDirs,
  type HostPathRoot,
  type HostPathVolumeLike,
  type WorkloadScope,
} from '@/utils/workloadHostPath'

const props = withDefaults(
  defineProps<{
    /** Workspace whose volumes the entries are checked against. */
    workspaceId?: string
    /** Scope of the workload kind the form creates. */
    scope: WorkloadScope
    /** Form model key the field validates. */
    prop?: string
    disabled?: boolean
    /** Accept entries under no workspace volume (holders of the workload/privileged grant). */
    allowUndeclared?: boolean
  }>(),
  { prop: 'hostpath', disabled: false, allowUndeclared: false, workspaceId: '' },
)

const model = defineModel<string[]>({ default: () => [] })

const userStore = useUserStore()
const roots = ref<HostPathRoot[]>([])
// False until the workspace volumes are known; entries are then left to the apiserver.
const loaded = ref(false)

const loadVolumes = async (wsId: string) => {
  loaded.value = false
  roots.value = []
  if (!wsId) return
  try {
    const res = await getWorkspaceDetail(wsId)
    if (wsId !== props.workspaceId) return
    roots.value = declaredHostPathRoots(res?.volumes as HostPathVolumeLike[], props.scope)
    loaded.value = true
  } catch {
    loaded.value = false
  }
}

watch(
  () => [props.workspaceId, props.scope] as const,
  ([wsId]) => loadVolumes(wsId),
  { immediate: true },
)

// Shown where a volume restricts workloads to their user directory, or when entries exist
// (a clone or resume of a workload that had them).
const shown = computed(() => roots.value.some((r) => r.userDirs) || (model.value?.length ?? 0) > 0)

const rootsText = computed(() =>
  roots.value.map((r) => (r.userDirs ? `${r.path}/<dir>` : `${r.path}/...`)).join(', '),
)
const ownDirsText = computed(() => ownUserDirs(roots.value, userStore.userId).join(', '))
const placeholder = computed(() => {
  const r = roots.value.find((x) => x.userDirs) ?? roots.value[0]
  return r ? `e.g. ${r.path}/<dir> (Enter to add)` : 'Enter a directory (Enter to add)'
})

const onChange = (val: string[]) => {
  model.value = normalizeHostPaths(val)
}

const rules = computed<FormItemRule[]>(() => [
  {
    trigger: 'change',
    validator: (_rule: unknown, value: unknown, callback: (err?: Error) => void) => {
      if (!loaded.value) return callback()
      for (const entry of (value as string[] | undefined) ?? []) {
        const msg = checkHostPathEntry(entry, roots.value, {
          allowUndeclared: props.allowUndeclared,
        })
        if (msg) return callback(new Error(msg))
      }
      callback()
    },
  },
])
</script>

<style scoped>
.extra-hostpath-hint {
  width: 100%;
  margin-top: 4px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--el-text-color-secondary);
}
.extra-hostpath-hint code {
  font-size: 12px;
}
</style>

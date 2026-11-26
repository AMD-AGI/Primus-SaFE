<template>
  <el-dialog
    :model-value="visible"
    :title="`Effective workspaces: ${user?.name || user?.id || ''}`"
    width="640px"
    destroy-on-close
    @update:model-value="(v: boolean) => emit('update:visible', v)"
    @open="onOpen"
  >
    <el-table :data="rows" v-loading="loading" size="default" max-height="420">
      <el-table-column label="Workspace" min-width="200">
        <template #default="{ row }">
          <div class="flex flex-col items-start">
            <span>{{ row.name }}</span>
            <span class="text-[13px] text-gray-400">{{ row.id }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="Role" width="100">
        <template #default="{ row }">{{ row.managed ? 'manager' : 'member' }}</template>
      </el-table-column>
      <el-table-column label="Sources" min-width="240">
        <template #default="{ row }">
          <el-tag v-for="s in row.sources" :key="s" class="mr-1" size="small" type="info">
            {{ sourceLabel(s) }}
          </el-tag>
          <span v-if="!row.sources.length">-</span>
        </template>
      </el-table-column>
    </el-table>
    <template #footer>
      <el-button @click="emit('update:visible', false)">Close</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { ref } from 'vue'
import { getUserEffectiveWorkspaces, type UserSelfData, type Workspace } from '@/services'
import { useTeams } from '@/composables/useTeams'

const props = defineProps<{
  visible: boolean
  user: UserSelfData | null
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

interface Row {
  id: string
  name: string
  managed: boolean
  sources: string[]
}

const rows = ref<Row[]>([])
const loading = ref(false)
const { loadTeams, sourceLabel } = useTeams()

// Merges the accessible and managed lists into one row per workspace.
const toRows = (workspaces: Workspace[], managed: Workspace[]): Row[] => {
  const byId = new Map<string, Row>()
  for (const ws of workspaces) {
    byId.set(ws.id, { id: ws.id, name: ws.name, managed: false, sources: [...(ws.sources ?? [])] })
  }
  for (const ws of managed) {
    const row = byId.get(ws.id) ?? { id: ws.id, name: ws.name, managed: true, sources: [] }
    row.managed = true
    for (const s of ws.sources ?? []) if (!row.sources.includes(s)) row.sources.push(s)
    byId.set(ws.id, row)
  }
  return [...byId.values()]
}

// Loads the effective workspaces of the user and the team names used to label sources.
const onOpen = async () => {
  if (!props.user) return
  loading.value = true
  rows.value = []
  try {
    const [res] = await Promise.all([getUserEffectiveWorkspaces(props.user.id), loadTeams()])
    rows.value = toRows(res?.workspaces ?? [], res?.managedWorkspaces ?? [])
  } catch {
    emit('update:visible', false)
  } finally {
    loading.value = false
  }
}
</script>

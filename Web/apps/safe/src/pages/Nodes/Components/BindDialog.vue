<template>
  <el-dialog
    :model-value="visible"
    :title="dialogTitle"
    width="520px"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
  >
    <div class="space-y-3">
      <div class="textx-12" style="font-weight: 500">
        Current Node: <b class="opacity-70 ml-8">{{ nodeIds?.join(', ') || '-' }}</b>
      </div>
      <div
        v-if="action === 'migrate'"
        class="textx-12 mt-2 flex"
        style="font-weight: 500; align-items: center"
      >
        Current Workspace: <b class="opacity-70 ml-8">{{ props.wsId || '-' }}</b>
      </div>
      <div class="textx-12 mt-2 flex" style="font-weight: 500; align-items: center">
        Target Workspace:
        <el-select
          v-if="action === 'add' || action === 'migrate'"
          v-model="selectedId"
          size="default"
          class="mt-3 mb-3 ml-2"
          style="width: 300px"
        >
          <el-option
            v-for="ws in workspaceOptions"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
        <div v-else class="ml-3 textx-13">{{ selectedId }}</div>
      </div>
      <div v-if="action === 'migrate'" class="textx-12 opacity-70">
        Only workspaces in the same cluster running the same node flavor, or none yet, can take this
        node.
      </div>
      <div
        v-if="action === 'remove' || action === 'migrate'"
        class="textx-12 mt-2 flex"
        style="font-weight: 500; align-items: center"
      >
        {{ action === 'migrate' ? 'Force Migrate:' : 'Force Unbind:' }}
        <el-switch v-model="forceUnbind" class="ml-3" />
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :disabled="confirmDisabled" @click="onBindConfirm">
          Confirm
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, computed, onBeforeUnmount, ref, watch } from 'vue'
import { relateNodeToWs } from '@/services'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  buildNodeRelateRequest,
  migrateTargetOptions,
  relateWorkspaceId,
  type NodeBindAction,
} from './nodeBinding'

const wsStore = useWorkspaceStore()

// How long to leave a migration before looking again. A migration is a handful of patches
// between two controllers and settles in well under a second; this is that with room to
// spare, and costs one extra list call on an action a user takes by hand.
const MIGRATION_SETTLE_MS = 2000

const props = defineProps<{
  visible: boolean
  action: string
  nodeIds: string[]
  wsId: string
}>()
const bindLoading = ref(false)
const emit = defineEmits(['update:visible', 'success'])

const selectedId = ref(props.wsId)
const forceUnbind = ref(false)

// The second look is scheduled after the dialog has already closed, so it can outlive the
// page it was going to refresh -- a navigation away leaves it to reload a table that is no
// longer mounted.
let settleTimer: number | undefined
const clearSettleTimer = () => {
  if (settleTimer !== undefined) {
    clearTimeout(settleTimer)
    settleTimer = undefined
  }
}
onBeforeUnmount(clearSettleTimer)

const bindAction = computed(() => props.action as NodeBindAction)
const dialogTitle = computed(() => {
  if (bindAction.value === 'add') return 'Bind'
  return bindAction.value === 'migrate' ? 'Migrate' : 'UnBind'
})
const workspaceOptions = computed(() =>
  bindAction.value === 'migrate' ? migrateTargetOptions(wsStore.items, props.wsId) : wsStore.items,
)
// A migration starts with the node's own workspace selected, which is the one target the API
// will not accept.
const confirmDisabled = computed(
  () => !selectedId.value || (bindAction.value === 'migrate' && selectedId.value === props.wsId),
)

const successMessage = () => {
  if (bindAction.value === 'add') return 'bind success'
  return bindAction.value === 'migrate' ? 'migrate started' : 'unbind success'
}

// Submit bind/unbind/migrate operation
const onBindConfirm = async () => {
  if (!selectedId.value) {
    ElMessage.warning('please choose one Workspace')
    return
  }

  try {
    bindLoading.value = true
    await relateNodeToWs(
      relateWorkspaceId(bindAction.value, props.wsId, selectedId.value),
      buildNodeRelateRequest({
        action: bindAction.value,
        nodeIds: props.nodeIds,
        targetWorkspaceId: selectedId.value,
        force: forceUnbind.value,
      }),
    )
    ElMessage.success(successMessage())

    emit('update:visible', false)
    emit('success')
    if (bindAction.value === 'migrate') {
      clearSettleTimer()
      // A migration is carried out after the request returns, and for the moment it takes the
      // node belongs to neither workspace. Refreshing only on the response catches the node
      // in that gap: the row reads as unassigned, and the actions for a bound node -- UnBind
      // and Migrate among them -- disappear from it until something else reloads the list.
      // A second look once the crossing has had time to finish shows where the node landed.
      settleTimer = window.setTimeout(() => {
        settleTimer = undefined
        emit('success')
      }, MIGRATION_SETTLE_MS)
    }
  } catch (err) {
    console.error(err)
  } finally {
    emit('update:visible', false)
  }
}

watch(
  () => props.visible,
  () => {
    // A migration has to be sent somewhere other than where the node already is, so it opens
    // with nothing chosen rather than with the source pre-selected.
    selectedId.value = props.action === 'migrate' ? '' : props.wsId
    forceUnbind.value = false
  },
)
</script>

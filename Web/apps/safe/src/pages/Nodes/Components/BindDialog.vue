<template>
  <el-dialog
    :model-value="visible"
    :title="props.action === 'add' ? 'Bind' : 'UnBind'"
    width="520px"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
  >
    <div class="space-y-3">
      <div class="textx-12" style="font-weight: 500">
        Current Node: <b class="opacity-70 ml-8">{{ nodeIds?.join(', ') || '-' }}</b>
      </div>
      <div class="textx-12 mt-2 flex" style="font-weight: 500; align-items: center">
        Target Workspace:
        <el-select
          v-if="action === 'add'"
          v-model="selectedId"
          size="default"
          class="mt-3 mb-3 ml-2"
          style="width: 300px"
        >
          <el-option
            v-for="ws in wsStore.items"
            :key="ws.workspaceId"
            :label="ws.workspaceName"
            :value="ws.workspaceId"
          />
        </el-select>
        <div v-else class="ml-3 textx-13">{{ selectedId }}</div>
      </div>
      <div
        v-if="action === 'remove'"
        class="textx-12 mt-2 flex"
        style="font-weight: 500; align-items: center"
      >
        Force Unbind:
        <el-switch v-model="forceUnbind" class="ml-3" />
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" :disabled="!selectedId" @click="onBindConfirm">
          Confirm
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, ref, watch } from 'vue'
import { relateNodeToWs } from '@/services'
import { ElMessage } from 'element-plus'
import { useWorkspaceStore } from '@/stores/workspace'

const wsStore = useWorkspaceStore()

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

// Submit bind/unbind operation
const onBindConfirm = async () => {
  if (!selectedId.value) {
    ElMessage.warning('please choose one Workspace')
    return
  }

  try {
    bindLoading.value = true
    await relateNodeToWs(selectedId.value, {
      action: props.action,
      nodeIds: props.nodeIds,
      ...(props.action === 'remove' ? { force: forceUnbind.value } : {}),
    })
    ElMessage.success(props.action === 'add' ? 'bind success' : 'unbind success')

    emit('update:visible', false)
    emit('success')
  } catch (err) {
    console.error(err)
  } finally {
    emit('update:visible', false)
  }
}

watch(
  () => props.visible,
  () => {
    selectedId.value = props.wsId
    forceUnbind.value = false
  },
)
</script>

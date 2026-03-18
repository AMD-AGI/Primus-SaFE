<template>
  <el-dialog
    :model-value="visible"
    :title="action"
    width="500"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
  >
    <el-table
      :data="rowdata"
      row-key="nodeId"
      @selection-change="handleSelectionChange"
      height="50vh"
    >
      <el-table-column type="selection" width="55" :selectable="selectable" />
      <el-table-column prop="nodeId" label="Node Id" />
    </el-table>
    <template #footer>
      <div class="dialog-footer">
        <el-button @click="emit('update:visible', false)">Cancel</el-button>
        <el-button type="primary" @click="handleConfirm"> Confirm </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { defineProps, defineEmits, ref } from 'vue'
import { manageNodes } from '@/services/nodes/index'
import { ElMessage } from 'element-plus'

interface NodeRow {
  nodeId: string
  phase: string
}

const props = defineProps<{
  visible: boolean
  rowdata: unknown
  action: string
  id: string
}>()
const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
}>()

const nodeList = ref([] as string[])

const selectable = (row: NodeRow) =>
  row.phase !== (props.action === 'Manage' ? 'Managing' : 'Unmanaging')

const handleSelectionChange = (val: NodeRow[]) => {
  nodeList.value = val?.map((item: NodeRow) => item.nodeId)
}

const handleConfirm = async () => {
  await manageNodes(
    {
      action: props.action === 'Manage' ? 'add' : 'remove',
      nodeIds: nodeList.value,
    },
    props.id,
  )
  ElMessage({
    type: 'success',
    message: `${props.action} complete`,
  })

  emit('update:visible', false)
}
</script>

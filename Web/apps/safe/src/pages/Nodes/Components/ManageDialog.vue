<template>
  <el-dialog
    :model-value="visible"
    :title="props.action"
    width="520px"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
  >
    <div class="space-y-3">
      <div class="textx-12" style="font-weight: 500">
        Current Node: <b class="opacity-70 ml-8">{{ nodeIds?.join(', ') || '-' }}</b>
      </div>
      <div class="textx-12 mt-2 flex" style="font-weight: 500; align-items: center">
        Target Cluster:
        <el-select
          v-if="props.action === 'Manage'"
          v-model="selectedId"
          size="default"
          class="mt-3 mb-3 ml-2"
          style="width: 300px"
        >
          <el-option
            v-for="ws in clusterStore.items"
            :key="ws.clusterId"
            :label="ws.clusterId"
            :value="ws.clusterId"
          />
        </el-select>
        <div v-else class="ml-3 textx-13">{{ selectedId }}</div>
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
import { manageNodes } from '@/services'
import { ElMessage } from 'element-plus'
import { useClusterStore } from '@/stores/cluster'

const clusterStore = useClusterStore()

const props = defineProps<{
  visible: boolean
  action: string
  nodeIds: string[]
  clusterId: string
}>()
const bindLoading = ref(false)
const emit = defineEmits(['update:visible', 'success'])

const selectedId = ref(props.clusterId)

// Submit manage/unmanage operation
const onBindConfirm = async () => {
  if (!selectedId.value) {
    ElMessage.warning('please choose one Cluster')
    return
  }

  try {
    bindLoading.value = true
    await manageNodes(
      {
        action: props.action === 'Manage' ? 'add' : 'remove',
        nodeIds: props.nodeIds,
      },
      selectedId.value,
    )
    ElMessage.success(`${props.action} complete`)

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
    selectedId.value = props.clusterId
  },
)
</script>

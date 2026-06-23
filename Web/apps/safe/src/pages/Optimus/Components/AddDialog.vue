<template>
  <DynamoAddDialog
    :visible="visible"
    :wlid="wlid"
    :action="action"
    workload-type="optimus"
    @update:visible="(value) => emit('update:visible', value)"
    @success="emit('success')"
  />
</template>

<script lang="ts" setup>
import DynamoAddDialog from '@/pages/Dynamo/Components/AddDialog.vue'

defineProps<{
  visible: boolean
  wlid?: string
  action: string
}>()

const emit = defineEmits(['update:visible', 'success'])
</script>

<template>
  <div>
    <el-input :model-value="link" readonly>
      <template #append>
        <el-button :icon="CopyDocument" @click="copyText(link)">Copy</el-button>
      </template>
    </el-input>
    <el-text type="info" size="small" class="block mt-2">
      Send this link to a manager of the workspace.
    </el-text>
  </div>
</template>

<script lang="ts" setup>
import { CopyDocument } from '@element-plus/icons-vue'
import { copyText } from '@/utils'

defineProps<{
  // The approval link of a request
  link: string
}>()
</script>

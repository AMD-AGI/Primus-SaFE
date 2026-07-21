<template>
  <el-dialog
    v-model="visible"
    :title="`SSH to login node${clusterName ? ` — ${clusterName}` : ''}`"
    width="640px"
    :close-on-click-modal="false"
    data-testid="slurm-ssh-dialog"
  >
    <div v-loading="loading">
      <template v-if="info?.ready && info?.sshCommand">
        <el-text class="block m-b-2" size="small">
          Run this from a terminal where your SSH key is loaded. It connects you to the Slurm
          login node, where you can run <code>sinfo</code>, <code>squeue</code>, and submit jobs.
        </el-text>
        <div class="ssh-cmd-box">
          <el-input
            :model-value="info.sshCommand"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 4 }"
            readonly
            data-testid="slurm-ssh-command"
          />
          <el-button
            type="primary"
            :icon="CopyDocument"
            class="m-t-2"
            data-testid="slurm-ssh-copy"
            @click="onCopy"
          >
            Copy SSH command
          </el-button>
        </div>
        <el-alert
          type="info"
          :closable="false"
          show-icon
          class="m-t-4"
          title="Before you connect"
        >
          <template #default>
            <ul class="ssh-tips">
              <li>Register your SSH public key first under <b>Settings</b> (key-based auth only).</li>
              <li>
                The first partition is the default, so you can submit without
                <code>-p</code>, e.g. <code>srun -N1 hostname</code>.
              </li>
            </ul>
          </template>
        </el-alert>
      </template>
      <el-empty
        v-else-if="!loading"
        :description="info?.message || 'The login node is not reachable yet.'"
        data-testid="slurm-ssh-unavailable"
      />
    </div>
    <template #footer>
      <el-button @click="visible = false">Close</el-button>
    </template>
  </el-dialog>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { CopyDocument } from '@element-plus/icons-vue'
import { copyText } from '@/utils/index'
import type { SlurmLoginInfo } from '@/services/slurm/type'

const props = defineProps<{
  visible: boolean
  info: SlurmLoginInfo | null
  clusterName?: string
  loading?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
}>()

const visible = computed({
  get: () => props.visible,
  set: (v) => emit('update:visible', v),
})

const onCopy = () => {
  if (props.info?.sshCommand) {
    copyText(props.info.sshCommand)
  }
}

defineOptions({ name: 'SlurmSshDialog' })
</script>

<style scoped>
.ssh-cmd-box code,
.ssh-tips code {
  background: var(--el-fill-color-light);
  padding: 0 4px;
  border-radius: 4px;
}
.ssh-tips {
  margin: 0;
  padding-left: 18px;
}
</style>

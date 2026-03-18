<template>
  <el-dialog
    :model-value="visible"
    title="SSH Configuration"
    width="700"
    @close="emit('update:visible', false)"
    :close-on-click-modal="false"
    destroy-on-close
    @open="onOpen"
  >
    <el-form label-width="160px">
      <!-- Shell select -->
      <el-form-item label="Terminal Shell">
        <el-select v-model="selectedShell" placeholder="Select shell">
          <el-option v-for="s in shells" :key="s" :label="s" :value="s" />
        </el-select>
      </el-form-item>

      <!-- Generated SSH -->
      <el-tooltip :content="generatedSSH" placement="top">
        <el-form-item label="SSH Command" class="ssh-item">
          <div class="ssh-text">{{ generatedSSH }}</div>
        </el-form-item>
      </el-tooltip>
    </el-form>

    <template #footer>
      <el-button
        type="primary"
        :loading="initLoading || imageSavingLoading"
        @click="handleCopySSH"
        >Copy SSH</el-button
      >
      <el-button
        type="primary"
        :loading="initLoading || imageSavingLoading"
        :disabled="disWebshell"
        @click="openWebShellInNewTab"
        >Open Webshell</el-button
      >
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { getPodContainers, getImageCustom } from '@/services'
import { copyText } from '@/utils'
import { useWorkspaceStore } from '@/stores/workspace'
import { ElMessage } from 'element-plus'

const props = defineProps<{
  visible: boolean
  wlid?: string
  podid?: string
  sshCommand?: string
  enableImageSavingCheck?: boolean // Whether to enable image save state check (for authoring only)
}>()
const emit = defineEmits(['update:visible', 'success'])

const shells = ref<string[]>([])
const selectedShell = ref('')
const baseSshCommand = ref('')

const initLoading = ref(false)
const imageSavingLoading = ref(false)

const wsStore = useWorkspaceStore()

const disWebshell = computed(() => !props.podid)

// Parse user part from user@host in SSH command (split by .)
// Format: ssh ... hash.pod-name.container.shell.workspace@host ...
// Index:        [0]   [1]        [2]       [3]   [4]
// Example: b2796cafcf3aee529d7974ea0d94954d.cursor-rca-hsz4m-master-0.pytorch.zsh.tw-project2-control-plane@tw325...
const parsedUserDotParts = computed(() => {
  if (!baseSshCommand.value) return []
  const parts = baseSshCommand.value.split(/\s+/)
  const userHostPart = parts.find((p) => p.includes('@'))
  if (!userHostPart) return []
  const userPart = userHostPart.split('@')[0]
  return userPart.split('.')
})

// Extract container name from SSH command (index 2)
const extractedContainer = computed(() => parsedUserDotParts.value[2] || '')

// Extract current shell from SSH command (index 3)
const extractedShell = computed(() => parsedUserDotParts.value[3] || '')

const generatedSSH = computed(() => {
  if (!baseSshCommand.value) return ''
  if (!selectedShell.value || !extractedShell.value) return baseSshCommand.value
  if (selectedShell.value === extractedShell.value) return baseSshCommand.value
  // Replace .{currentShell}. with .{selectedShell}.
  return baseSshCommand.value.replace(
    `.${extractedShell.value}.`,
    `.${selectedShell.value}.`,
  )
})

// Check if there is an ongoing image save task
async function checkImageSaving(): Promise<boolean> {
  if (!props.enableImageSavingCheck || !props.wlid) return false

  try {
    imageSavingLoading.value = true
    const res = await getImageCustom({ workload: props.wlid })
    const firstItem = res?.items?.[0]
    // if (firstItem?.status === 'Succeeded') {
    if (firstItem?.status === 'Running') {
      ElMessage.warning('Image saving is in progress, please try again later')
      return true
    }
  } catch (error) {
    console.error('Failed to check image custom status:', error)
  } finally {
    imageSavingLoading.value = false
  }

  return false
}

async function handleCopySSH() {
  const isSaving = await checkImageSaving()
  if (isSaving) return
  copyText(generatedSSH.value)
}

async function openWebShellInNewTab() {
  const isSaving = await checkImageSaving()
  if (isSaving) return

  const q = new URLSearchParams({
    workloadId: props.wlid ?? '',
    pod: props.podid ?? '',
    container: extractedContainer.value,
    cmd: selectedShell.value ?? 'sh',
    namespace: wsStore.currentWorkspaceId ?? '',
  })
  window.open(`/webshell?${q.toString()}`, '_blank', 'noopener,noreferrer')
}

const onOpen = async () => {
  if (!props.wlid || !props.podid) return
  initLoading.value = true
  try {
    const res = await getPodContainers(props.wlid, props.podid)
    shells.value = res.shells ?? []
    baseSshCommand.value = props.sshCommand ?? ''

    if (shells.value.length > 0 && !selectedShell.value) {
      selectedShell.value = shells.value[0]
    }
  } finally {
    initLoading.value = false
  }
}
</script>
<style scoped>
.ssh-item .ssh-text {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  line-height: 1.4;
}
</style>

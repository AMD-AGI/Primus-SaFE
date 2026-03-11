import { ref } from 'vue'

/**
 * Composable for pod actions (SSH and Logs)
 */
export function usePodActions() {
  const curPodId = ref<string>()
  const curSshCommand = ref<string>()
  const logVisible = ref(false)
  const sshVisible = ref(false)

  const openLog = (podId: string) => {
    curPodId.value = podId
    logVisible.value = true
  }

  const openSsh = (podId: string, sshCommand?: string) => {
    curPodId.value = podId
    curSshCommand.value = sshCommand
    sshVisible.value = true
  }

  return {
    curPodId,
    curSshCommand,
    logVisible,
    sshVisible,
    openLog,
    openSsh,
  }
}

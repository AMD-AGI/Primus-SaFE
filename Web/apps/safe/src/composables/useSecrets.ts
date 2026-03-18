import { ref } from 'vue'
import { getSecrets } from '@/services'

export interface SecretOption {
  label: string
  value: string
}

/**
 * Composable for fetching and managing secrets
 * @param type - Secret type, default 'image'
 */
export function useSecrets(type: string = 'image') {
  const secretOptions = ref<SecretOption[]>([])
  const loading = ref(false)
  const error = ref<Error | null>(null)

  /**
   * Fetch secrets list
   */
  const fetchSecrets = async () => {
    loading.value = true
    error.value = null

    try {
      const res = await getSecrets({ type })
      secretOptions.value = (res?.items || []).map((item: any) => ({
        label: item.secretName,
        value: item.secretId,
      }))
    } catch (err) {
      error.value = err as Error
      console.error('Failed to fetch secrets:', err)
    } finally {
      loading.value = false
    }
  }

  /**
   * Reset secrets data
   */
  const resetSecrets = () => {
    secretOptions.value = []
    error.value = null
  }

  return {
    secretOptions,
    loading,
    error,
    fetchSecrets,
    resetSecrets,
  }
}

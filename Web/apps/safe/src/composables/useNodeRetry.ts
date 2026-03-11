import { h } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { retryNodes } from '@/services'

/**
 * Node Retry Composable
 * Handles node retry operations
 */
export function useNodeRetry(options?: {
  /** Callback function to refresh the list */
  onRefresh?: () => void
}) {
  const { onRefresh } = options || {}

  // Error message mapping
  const mapRetryError = (errMsg: string): string => {
    const lowerMsg = errMsg.toLowerCase()
    if (lowerMsg.includes('machine is not ready')) {
      return 'Node is not ready yet. Please wait for the node to be ready before retrying.'
    }
    if (lowerMsg.includes('already')) {
      return 'Node is already being processed. Please wait for the current operation to complete.'
    }
    return errMsg
  }

  // Handle retry operation (single or batch)
  const handleRetry = async (isBatch: boolean, nodeId?: string, nodeIds?: string[]) => {
    const ids = isBatch ? nodeIds || [] : nodeId ? [nodeId] : []

    if (!ids.length) {
      ElMessage.warning('Please select at least one node.')
      return
    }

    // Show confirmation dialog
    const msg = h('span', null, [
      'Are you sure you want to retry node(s): ',
      h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, ids.join(', ')),
      ' ?',
    ])

    try {
      await ElMessageBox.confirm(msg, 'Retry node', {
        confirmButtonText: 'Retry',
        cancelButtonText: 'Cancel',
        type: 'warning',
      })

      // Call batch retry API
      try {
        await retryNodes({ nodeIds: ids })
        ElMessage({ message: 'Retry started successfully', type: 'success' })

        // Refresh list
        onRefresh?.()
      } catch (err) {
        const errorMsg = (err as Error).message || 'Failed to retry node'
        ElMessage.error(mapRetryError(errorMsg))
      }
    } catch (err) {
      if (err === 'cancel' || err === 'close') {
        ElMessage.info('Retry canceled')
      }
    }
  }

  return {
    handleRetry,
  }
}

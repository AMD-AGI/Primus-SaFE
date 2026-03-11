import { ref, computed, watch, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getWorkloadDetail, deleteWorkload, stopWorkload, addWorkload } from '@/services'
import { useWorkspaceStore } from '@/stores/workspace'

export interface UseWorkloadDetailOptions {
  /** Redirect path after deletion */
  redirectPath: string
  /** Whether to extract failed node info */
  extractFailedNodes?: boolean
  /** Custom data processing function */
  processData?: (data: any) => void
  /** Override workspaceId (for scenarios like Bench that use a different API's workspaceId) */
  overrideWorkspaceId?: () => string | undefined
}

export function useWorkloadDetail(options: UseWorkloadDetailOptions) {
  const route = useRoute()
  const router = useRouter()
  const wsStore = useWorkspaceStore()

  const workloadId = computed(() => route.query.id as string)
  const detailData = ref<any>(null)
  const detailLoading = ref(false)

  // Get details
  const getDetail = async () => {
    if (!workloadId.value) return

    detailLoading.value = true
    try {
      const res = await getWorkloadDetail(workloadId.value)
      detailData.value = res

      // Extract failed nodes (if needed)
      if (options.extractFailedNodes && Array.isArray(res.conditions) && res.conditions.length) {
        const lastCond = res.conditions[res.conditions.length - 1]
        if (lastCond?.message?.includes('details')) {
          try {
            const match = lastCond.message.match(/details:\s*(\[.*\])/)
            if (match && match[1]) {
              const details = JSON.parse(match[1])
              const failedNodes = details.map((d: any) => d.node).filter((n: any) => !!n)
              detailData.value.failedNodes = failedNodes
            }
          } catch (err) {
            console.warn('Failed to parse details from message:', err)
          }
        }
      }

      // Custom data processing
      if (options.processData) {
        options.processData(res)
      }
    } finally {
      detailLoading.value = false
    }
  }

  // Delete workload
  const onDelete = () => {
    const msg = h('span', null, [
      'Are you sure you want to delete workload: ',
      h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, workloadId.value),
      ' ?',
    ])

    ElMessageBox.confirm(msg, 'Delete workload', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
      .then(async () => {
        await deleteWorkload(workloadId.value)
        ElMessage({
          type: 'success',
          message: 'Delete completed',
        })
        router.push(options.redirectPath)
      })
      .catch((err) => {
        if (err === 'cancel' || err === 'close') {
          ElMessage.info('Delete canceled')
        }
      })
  }

  // Stop workload
  const onStop = () => {
    const msg = h('span', null, [
      'Are you sure you want to stop workload: ',
      h('span', { style: 'color: var(--el-color-primary); font-weight: 600' }, workloadId.value),
      ' ?',
    ])

    ElMessageBox.confirm(msg, 'Stop workload', {
      confirmButtonText: 'Stop',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
      .then(async () => {
        await stopWorkload(workloadId.value)
        ElMessage({
          type: 'success',
          message: 'Stop completed',
        })
        getDetail()
      })
      .catch((err) => {
        if (err === 'cancel' || err === 'close') {
          ElMessage.info('Stop canceled')
        }
      })
  }

  // Resume workload
  const onResume = async () => {
    if (!workloadId.value) return

    try {
      const detail = await getWorkloadDetail(workloadId.value)

      const msg = h('div', null, [
        h('p', null, [
          'Are you sure you want to resume workload: ',
          h(
            'span',
            { style: 'color: var(--el-color-primary); font-weight: 600' },
            detail.displayName || workloadId.value,
          ),
          ' ?',
        ]),
        h('p', { class: 'mt-2 text-sm text-gray-500' }, [
          'This will create a new workload with the same configuration.',
        ]),
      ])

      await ElMessageBox.confirm(msg, 'Resume workload', {
        confirmButtonText: 'Resume',
        cancelButtonText: 'Cancel',
        type: 'info',
      })

      const kind = detail.groupVersionKind?.kind
      const secrets = detail.secrets?.map((s: { id: string }) => ({ id: s.id })) || []

      // Build parameters based on different kinds
      let payload: Record<string, unknown> = {}

      if (kind === 'AutoscalingRunnerSet') {
        // CICD - AutoscalingRunnerSet
        const envCopy = { ...(detail.env || {}) }
        const unifiedJobEnable = envCopy.UNIFIED_JOB_ENABLE === 'true'

        // Fixed resource values (same as CICD AddDialog)
        const fixedResource = {
          replica: 1,
          cpu: unifiedJobEnable ? '2' : '1',
          gpu: '0',
          memory: unifiedJobEnable ? '8Gi' : '4Gi',
          ephemeralStorage: '10Gi',
        }

        // Remove sensitive PAT from env for resume
        delete envCopy.GITHUB_PAT

        const excludedNodes = (detail.excludedNodes ?? []).filter(Boolean)

        payload = {
          workspace: wsStore.currentWorkspaceId!,
          displayName: detail.displayName,
          groupVersionKind: detail.groupVersionKind,
          description: detail.description,
          priority: detail.priority,
          maxRetry: detail.maxRetry ?? 50,
          isTolerateAll: detail.isTolerateAll ?? true,
          resources: [fixedResource],
          env: envCopy,
          ...(excludedNodes.length ? { excludedNodes } : {}),
          ...(secrets.length > 0 ? { secrets } : {}),
          // resume the same workload
          workloadId: detail.workloadId,
          ...(detail.useWorkspaceStorage !== undefined ? { useWorkspaceStorage: detail.useWorkspaceStorage } : {}),
        }
      } else if (kind === 'VLLMServer') {
        // Authoring - VLLMServer
        const resources = detail.resources || [detail.resource]
        const firstResource = resources[0] || {}
        const baseResource = {
          cpu: firstResource.cpu,
          gpu: firstResource.gpu,
          memory: firstResource.memory,
          ephemeralStorage: firstResource.ephemeralStorage,
          replica: 1,
        }

        payload = {
          workspace: wsStore.currentWorkspaceId!,
          displayName: detail.displayName,
          groupVersionKind: detail.groupVersionKind,
          description: detail.description,
          images: Array.isArray(detail.images) ? detail.images : detail.image ? [detail.image] : [],
          priority: detail.priority,
          resources: [baseResource],
          ...(detail.env ? { env: detail.env } : {}),
          ...(detail.isTolerateAll !== undefined ? { isTolerateAll: detail.isTolerateAll } : {}),
          ...(secrets.length > 0 ? { secrets } : {}),
          ...(detail.excludedNodes?.length ? { excludedNodes: detail.excludedNodes } : {}),
          // resume the same workload
          workloadId: detail.workloadId,
          ...(detail.useWorkspaceStorage !== undefined ? { useWorkspaceStorage: detail.useWorkspaceStorage } : {}),
        }
      } else if (kind === 'Deployment' || kind === 'StatefulSet') {
        // Infer - Deployment/StatefulSet
        const resources = detail.resources || [detail.resource]
        const firstResource = resources[0] || {}
        const baseResource = {
          cpu: firstResource.cpu,
          gpu: firstResource.gpu,
          memory: firstResource.memory,
          ephemeralStorage: firstResource.ephemeralStorage,
          replica: firstResource.replica || 1,
        }

        // Service payload
        const servicePayload = detail.service
          ? {
              service: {
                protocol: detail.service.protocol,
                port: detail.service.port,
                targetPort: detail.service.targetPort,
                serviceType: detail.service.serviceType,
                ...(detail.service.nodePort ? { nodePort: detail.service.nodePort } : {}),
              },
            }
          : {}

        // Health check payload (liveness + readiness)
        const healthCheckPayload = detail.liveness
          ? {
              liveness: {
                path: detail.liveness.path,
                port: detail.liveness.port,
              },
              readiness: {
                path: detail.liveness.path,
                port: detail.liveness.port,
              },
            }
          : {}

        payload = {
          workspace: wsStore.currentWorkspaceId!,
          displayName: detail.displayName,
          groupVersionKind: detail.groupVersionKind,
          description: detail.description,
          isSupervised: detail.isSupervised,
          entryPoints: Array.isArray(detail.entryPoints)
            ? detail.entryPoints
            : detail.entryPoint
              ? [detail.entryPoint]
              : [],
          images: Array.isArray(detail.images)
            ? detail.images
            : detail.image
              ? [detail.image]
              : [],
          priority: detail.priority,
          resources: [baseResource],
          ...(detail.env ? { env: detail.env } : {}),
          ...(detail.customerLabels ? { customerLabels: detail.customerLabels } : {}),
          ...servicePayload,
          ...healthCheckPayload,
          ...(detail.timeout ? { timeout: detail.timeout } : {}),
          ...(secrets.length > 0 ? { secrets } : {}),
          ...(detail.excludedNodes?.length ? { excludedNodes: detail.excludedNodes } : {}),
          // resume the same workload
          workloadId: detail.workloadId,
          ...(detail.useWorkspaceStorage !== undefined ? { useWorkspaceStorage: detail.useWorkspaceStorage } : {}),
        }
      } else {
        // PyTorchJob, RayJob, TorchFTJob and other types
        const resources = detail.resources || [detail.resource]
        const firstResource = resources[0] || {}
        const secondResource = resources[1] || {}
        const totalReplica =
          Number(firstResource.replica || 0) + Number(secondResource.replica || 0)

        const buildResources = () => {
          if (totalReplica > 1) {
            return [
              { ...firstResource, replica: 1 },
              { ...firstResource, replica: totalReplica - 1 },
            ]
          } else {
            return [{ ...firstResource, replica: totalReplica || 1 }]
          }
        }

        const resArr = buildResources()
        const images = Array.isArray(detail.images)
          ? detail.images
          : detail.image
            ? [detail.image]
            : []
        const entryPoints = Array.isArray(detail.entryPoints)
          ? detail.entryPoints
          : detail.entryPoint
            ? [detail.entryPoint]
            : []
        const normalizeToLen = <T,>(arr: T[], len: number) => {
          if (!len) return []
          if (arr.length === len) return arr
          if (arr.length === 1) return Array.from({ length: len }, () => arr[0])
          const first = arr[0] as T
          return Array.from({ length: len }, (_, i) => arr[i] ?? first)
        }

        payload = {
          workspace: wsStore.currentWorkspaceId!,
          displayName: detail.displayName,
          groupVersionKind: detail.groupVersionKind,
          description: detail.description,
          isSupervised: detail.isSupervised,
          entryPoints: normalizeToLen(entryPoints, resArr.length),
          images: normalizeToLen(images, resArr.length),
          maxRetry: detail.maxRetry || 0,
          priority: detail.priority,
          resources: resArr,
          ...(detail.env ? { env: detail.env } : {}),
          ...(detail.customerLabels ? { customerLabels: detail.customerLabels } : {}),
          dependencies: detail.dependencies,
          ...(detail.timeout ? { timeout: detail.timeout } : {}),
          ...(secrets.length > 0 ? { secrets } : {}),
          ...(detail.excludedNodes?.length ? { excludedNodes: detail.excludedNodes } : {}),
          ...(detail.stickyNodes !== undefined ? { stickyNodes: detail.stickyNodes } : {}),
          ...(detail.cronJobs?.length ? { cronJobs: detail.cronJobs } : {}),
          // resume the same workload
          workloadId: detail.workloadId,
          ...(detail.useWorkspaceStorage !== undefined ? { useWorkspaceStorage: detail.useWorkspaceStorage } : {}),
        }
      }

      // TypeScript workaround: payload already contains all required fields, but type inference has issues
      await addWorkload(payload as unknown as Parameters<typeof addWorkload>[0])

      ElMessage.success('Resume successful')
      getDetail()
    } catch (err: unknown) {
      if (err !== 'cancel' && err !== 'close') {
        const error = err as { message?: string }
        ElMessage.error(error?.message || 'Resume failed')
      }
    }
  }

  // Watch workspace sync
  const stopSync = watch(
    () => {
      // Prefer overridden workspaceId, otherwise use detailData workspaceId
      const overrideId = options.overrideWorkspaceId?.()
      return overrideId || (detailData.value?.workspaceId as string | undefined)
    },
    async (targetWs) => {
      if (!targetWs) return
      if (wsStore.currentWorkspaceId === targetWs) {
        stopSync()
        return
      }
      wsStore.setCurrentWorkspace(targetWs)
      await wsStore.fetchWorkspace(true)
      stopSync()
    },
    { immediate: true },
  )

  // Watch route changes to refetch data
  watch(
    () => route.query.id,
    () => getDetail(),
    { immediate: true },
  )

  return {
    workloadId,
    detailData,
    detailLoading,
    getDetail,
    onDelete,
    onStop,
    onResume,
  }
}

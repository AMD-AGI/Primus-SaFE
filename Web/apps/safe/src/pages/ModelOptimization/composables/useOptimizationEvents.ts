import { ref, onBeforeUnmount } from 'vue'
import { subscribeTaskEvents } from '@/services/model-optimization'
import type {
  OptimizationEvent,
  PhasePayload,
  BenchmarkPayload,
  KernelPayload,
  LogPayload,
  StatusPayload,
  DonePayload,
} from '@/services/model-optimization/type'

export function useOptimizationEvents(taskId: string) {
  const phases = ref<PhasePayload[]>([])
  const benchmarks = ref<BenchmarkPayload[]>([])
  const kernels = ref<KernelPayload[]>([])
  const logs = ref<LogPayload[]>([])
  const taskStatus = ref<string>('')
  const taskMessage = ref<string>('')
  const isDone = ref(false)
  const sseError = ref(false)

  let lastEventId = ''
  let es: EventSource | null = null

  const EVENT_TYPES = ['phase', 'benchmark', 'kernel', 'log', 'status', 'done'] as const

  function handleEvent(raw: MessageEvent) {
    try {
      const evt: OptimizationEvent = JSON.parse(raw.data)
      lastEventId = evt.id

      switch (evt.type) {
        case 'phase':
          phases.value.push(evt.payload as PhasePayload)
          break
        case 'benchmark':
          benchmarks.value.push(evt.payload as BenchmarkPayload)
          break
        case 'kernel': {
          const kp = evt.payload as KernelPayload
          const idx = kernels.value.findIndex((k) => k.name === kp.name)
          if (idx >= 0) kernels.value.splice(idx, 1, kp)
          else kernels.value.push(kp)
          break
        }
        case 'log':
          logs.value.push(evt.payload as LogPayload)
          break
        case 'status': {
          const sp = evt.payload as StatusPayload
          taskStatus.value = sp.status
          taskMessage.value = sp.message
          break
        }
        case 'done': {
          const dp = evt.payload as DonePayload
          taskStatus.value = dp.status
          taskMessage.value = dp.message
          isDone.value = true
          close()
          break
        }
      }
    } catch {
      // ignore malformed events
    }
  }

  function connect() {
    close()
    sseError.value = false
    es = subscribeTaskEvents(taskId, lastEventId || undefined)

    for (const t of EVENT_TYPES) {
      es.addEventListener(t, handleEvent)
    }
    es.addEventListener('message', handleEvent)

    es.onerror = () => {
      sseError.value = true
      close()
    }
  }

  function close() {
    if (es) {
      es.close()
      es = null
    }
  }

  function reset() {
    phases.value = []
    benchmarks.value = []
    kernels.value = []
    logs.value = []
    taskStatus.value = ''
    taskMessage.value = ''
    isDone.value = false
    lastEventId = ''
  }

  onBeforeUnmount(close)

  return {
    phases,
    benchmarks,
    kernels,
    logs,
    taskStatus,
    taskMessage,
    isDone,
    sseError,
    connect,
    close,
    reset,
  }
}

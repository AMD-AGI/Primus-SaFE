import { ref, reactive, watch, nextTick, type Ref } from 'vue'
import dayjs from 'dayjs'
import { ElMessage } from 'element-plus'
import {
  getWorkloadLogs,
  getLogContext,
  downloadWlLogs,
  getDownloadWlLogsUrl,
} from '@/services/workload/index'
import type { GetLogParams, LogTableRow } from '@/services'
import { fmtTs } from '@/utils'

export type RowData =
  | { _kind: 'header'; id: string; pod_name: string; host: string; count: number }
  | {
      _kind: 'row'
      id: string
      log: string
      pod_name: string
      host: string
      ts: string
      timeMessage: string
    }

export interface LogContextRow {
  id: string
  timeMessage: string
  log: string
  line: number
}

export interface NodeRow {
  node: string
  rank?: string | number
}

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms))

// Handle download link protocol issues
const normalizeUrlProtocol = (raw: string) => {
  const u = new URL(raw, window.location.origin)
  if (window.location.protocol === 'https:' && u.protocol === 'http:') {
    u.protocol = 'https:'
  }
  return u.toString()
}

const downloadFile = async (url: string) => {
  const finalUrl = normalizeUrlProtocol(url)
  const controller = new AbortController()
  const r = await fetch(finalUrl, { signal: controller.signal, credentials: 'include' })
  if (!r.ok) throw new Error(`HTTP ${r.status}`)
  const blob = await r.blob()
  const blobUrl = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = blobUrl
  a.download = ''
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(blobUrl)
}

// Poll until download link is available (every 5s, up to 30 minutes)
async function waitForDownloadUrl(jobId: string, interval = 5000, timeout = 30 * 60 * 1000) {
  const end = Date.now() + timeout
  while (Date.now() < end) {
    const res = await getDownloadWlLogsUrl(jobId)
    const phase = res?.phase || res?.data?.phase
    const url = res?.outputs?.find(
      (item: { name: string; value: string }) => item.name === 'endpoint',
    )?.value

    if (phase === 'Succeeded' && url) return url
    if (phase === 'Failed') throw new Error(res?.message || 'Download failed')

    await sleep(interval)
  }
  throw new Error('Download timeout')
}

export function useLogTable(
  wlid: string,
  selectedGroup: Ref<number>,
  dispatchCount: Ref<number | undefined>,
  tableRows: Ref<NodeRow[]>,
  failedNodes?: Ref<string[] | undefined>,
  /** Default check first N nodes (falls back to first+last strategy if not set) */
  selectFirstN?: number,
) {
  // Log table data
  const loading = ref(false)
  const rowData = ref<RowData[]>([])
  const searchParams = reactive({
    dateRange: '' as string | any[],
    order: 'desc' as 'asc' | 'desc',
    keywords: [] as string[],
    nodeNames: '',
  })
  const pagination = reactive({
    page: 1,
    pageSize: 200,
    total: 0,
  })

  // Node selection
  const nodeTableRef = ref()
  const selectedNodes = ref<string[]>([])
  const didInitSelect = ref(false)
  const selectingProg = ref(false)

  // Log download
  const downloadLoading = ref(false)

  // Context dialog
  const ctxVisible = ref(false)
  const ctxLoading = ref(false)
  const ctxRows = ref<LogContextRow[]>([])

  // Process logTable data and group
  const buildGroupedRows = (rows: LogTableRow[]): RowData[] => {
    const groups = new Map<string, { pod_name: string; host: string; items: LogTableRow[] }>()
    for (const r of rows) {
      const key = `${r.pod_name}@@${r.host}`
      if (!groups.has(key)) groups.set(key, { pod_name: r.pod_name, host: r.host, items: [] })
      if (r.message) {
        groups.get(key)!.items.push(r)
      }
    }

    const out: RowData[] = []
    for (const [key, g] of groups) {
      out.push({
        _kind: 'header',
        id: `header-${key}`,
        pod_name: g.pod_name,
        host: g.host,
        count: selectedGroup.value,
      })
      for (const r of g.items) {
        out.push({
          _kind: 'row',
          id: r.id,
          log: r.message,
          pod_name: r.pod_name,
          host: r.host,
          ts: r.timestamp,
          timeMessage: fmtTs(r.timestamp),
        })
      }
    }
    return out
  }

  // Fetch logTable main data
  const getLogRowData = async (extraParams: Partial<GetLogParams> = {}) => {
    try {
      loading.value = true
      const offset = (pagination.page - 1) * pagination.pageSize
      const limit = pagination.pageSize
      const [start, end] = searchParams.dateRange
      const baseParams: GetLogParams = {
        order: searchParams.order,
        keywords: searchParams.keywords,
        since: start ? dayjs(start).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : '',
        until: end ? dayjs(end).utc().format('YYYY-MM-DDTHH:mm:ss.SSS[Z]') : '',
        nodeNames: searchParams.nodeNames,
        dispatchCount: selectedGroup.value,
        offset,
        limit,
      }

      const res = await getWorkloadLogs(wlid, {
        ...baseParams,
        ...extraParams,
      })

      rowData.value = buildGroupedRows(res?.rows)
      pagination.total = res?.total || 0
    } catch (err) {
      if (err instanceof Error) throw err
    } finally {
      loading.value = false
    }
  }

  const handlePageChange = (newPage: number) => {
    pagination.page = newPage
    getLogRowData()
  }

  const handlePageSizeChange = (newSize: number) => {
    pagination.pageSize = newSize
    pagination.page = 1
    getLogRowData()
  }

  const onSearch = () => {
    pagination.page = 1
    getLogRowData()
  }

  const dedupeKeywords = (val: string[]) => {
    searchParams.keywords = Array.from(new Set(val.map((s) => s.trim()).filter(Boolean)))
  }

  // Remove nodeNames query param when all selected
  const applyNodeNamesParam = (rows: NodeRow[], selected: string[]) => {
    const total = rows.length
    const isAll = total > 0 && selected.length === total
    if (isAll) {
      if ('nodeNames' in searchParams) delete (searchParams as any).nodeNames
    } else {
      searchParams.nodeNames = selected.join(',')
    }
  }

  const onNodeSelectionChange = (rows: NodeRow[]) => {
    if (selectingProg.value) return
    selectedNodes.value = rows.map((r) => r.node)
    applyNodeNamesParam(tableRows.value, selectedNodes.value)
    onSearch()
  }

  // InitializeNode selection
  watch(
    tableRows,
    async (rows) => {
      if (didInitSelect.value || !rows?.length) return
      await nextTick()

      const table = nodeTableRef.value
      if (!table) return

      selectingProg.value = true
      table.clearSelection?.()

      const failedSet = new Set(failedNodes?.value ?? [])
      if (failedSet.size) {
        rows.forEach((r: NodeRow) => {
          if (failedSet.has(r.node)) table.toggleRowSelection?.(r, true)
        })
        selectedNodes.value = rows.filter((r) => failedSet.has(r.node)).map((r) => r.node)
      } else if (selectFirstN) {
        // Check first N nodes
        const targets = rows.slice(0, selectFirstN)
        targets.forEach((r) => table.toggleRowSelection?.(r, true))
        selectedNodes.value = targets.map((r) => r.node)
      } else {
        // Default: check first and last nodes
        const head = rows[0]
        const tail = rows.length > 1 ? rows[rows.length - 1] : undefined
        if (head) table.toggleRowSelection?.(head, true)
        if (tail && tail !== head) table.toggleRowSelection?.(tail, true)
        selectedNodes.value = tail && tail !== head ? [head.node, tail.node] : [head.node]
      }

      selectingProg.value = false
      applyNodeNamesParam(rows, selectedNodes.value)
      onSearch?.()
      didInitSelect.value = true
    },
    { immediate: true },
  )

  // Log download
  const downloadLogs = async () => {
    downloadLoading.value = true
    let msgInst = null
    try {
      const res = await downloadWlLogs({
        name: wlid,
        inputs: [{ name: 'workload', value: wlid }],
        type: 'dumplog',
        timeoutSecond: 1800,
      })

      await sleep(1000)

      if (!res.jobId) {
        ElMessage.error('Download failed, no jobId returned')
        return
      }

      msgInst = ElMessage({
        type: 'info',
        message: 'Preparing download…',
        duration: 0,
        showClose: true,
      })

      const url = await waitForDownloadUrl(res.jobId)
      msgInst?.close?.()
      msgInst = ElMessage({
        type: 'info',
        message: 'Starting download…',
        duration: 0,
        showClose: true,
      })

      await downloadFile(url)
      msgInst?.close?.()

      ElMessage.success('Download completed.')
    } catch (e) {
      ElMessage.error((e as Error).message || 'Download failed, please retry')
    } finally {
      downloadLoading.value = false
    }
  }

  // Context dialog
  const onFetchContext = async (id: string, ts: string, log: string) => {
    const selection = window.getSelection()?.toString()
    if (selection) return

    ctxVisible.value = true
    ctxLoading.value = true
    try {
      const res = await getLogContext(wlid, id, {
        limit: 100,
        since: ts,
        dispatchCount: selectedGroup.value,
        nodeNames: searchParams.nodeNames,
      })

      const rows = (res || []).map((r: any, i: number) => ({
        id: `${id}-${i}`,
        timeMessage: fmtTs(r.timestamp),
        log: r.message,
        line: r.line,
      }))

      const negArr = rows.filter((item: any) => item.line < 0).reverse()
      const posArr = rows.filter((item: any) => item.line > 0)
      ctxRows.value = [...negArr, { id: `${id}-0`, timeMessage: '', log, line: 0 }, ...posArr]

      nextTick(async () => {
        await new Promise(requestAnimationFrame)
        const el = document.getElementById('ctx-target') as HTMLElement | null
        if (!el) {
          console.warn('ctx-target not found yet')
          return
        }

        const container = el.closest('.el-scrollbar__wrap') as HTMLElement | null
        if (container) {
          const crect = container.getBoundingClientRect()
          const rect = el.getBoundingClientRect()
          const delta = rect.top - crect.top - (crect.height / 2 - rect.height / 2)
          const targetTop = container.scrollTop + delta
          container.scrollTo({
            top: targetTop,
            behavior: 'smooth',
          })
        } else {
          el.scrollIntoView({ block: 'center', inline: 'nearest', behavior: 'smooth' })
        }
      })
    } finally {
      ctxLoading.value = false
    }
  }

  const onCtxClosed = () => {
    ctxRows.value = []
  }

  return {
    // Log table
    loading,
    rowData,
    searchParams,
    pagination,
    getLogRowData,
    handlePageChange,
    handlePageSizeChange,
    onSearch,
    dedupeKeywords,
    // Node selection
    nodeTableRef,
    selectedNodes,
    onNodeSelectionChange,
    // Log download
    downloadLoading,
    downloadLogs,
    // Context dialog
    ctxVisible,
    ctxLoading,
    ctxRows,
    onFetchContext,
    onCtxClosed,
  }
}

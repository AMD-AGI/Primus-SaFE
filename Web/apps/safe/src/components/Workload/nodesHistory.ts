import type { WorkloadNodesHistoryItem } from '@/services/workload/type'

export interface WorkloadNodesHistoryRow {
  index: number
  phase: string
  dispatchCount: number
  startTime: string
  endTime: string
  nodes: string
}

// formatDispatchNodes joins each dispatch's node list, oldest dispatch first.
export function formatDispatchNodes(nodes?: string[][]): string {
  if (!nodes?.length) {
    return '-'
  }
  const groups = nodes.map((group) => {
    const names = (group ?? []).map((n) => n.trim()).filter(Boolean)
    return names.length ? names.join(', ') : '-'
  })
  return groups.join(' | ')
}

// toNodesHistoryRows renders archived runs newest first.
export function toNodesHistoryRows(history?: WorkloadNodesHistoryItem[]): WorkloadNodesHistoryRow[] {
  if (!history?.length) {
    return []
  }
  return history
    .map((entry, index) => ({
      index: index + 1,
      phase: entry.phase || '-',
      dispatchCount: entry.dispatchCount ?? 0,
      startTime: entry.startTime || '',
      endTime: entry.endTime || '',
      nodes: formatDispatchNodes(entry.nodes),
    }))
    .reverse()
}

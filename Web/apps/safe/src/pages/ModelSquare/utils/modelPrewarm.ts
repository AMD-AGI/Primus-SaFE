export type ModelPrewarmScopeType = 'node' | 'workspace' | 'cluster'

export interface ModelPrewarmNodeDetail {
  node?: string
  adminNodeId?: string
  phase?: string
  message?: string
  durationSeconds?: number
  bytesRead?: number
}

export interface ModelPrewarmOutputs {
  status?: string
  message?: string
  nodesTotal?: string
  nodesSucceeded?: string
  nodesFailed?: string
  nodesPending?: string
  prewarmProgress?: string
  nodesDetail: ModelPrewarmNodeDetail[]
}

/** Build ops job name from the last segment of a model NFS path. */
export function generateModelPrewarmName(modelPath: string): string {
  const trimmed = modelPath.trim().replace(/\/+$/, '')
  const lastSegment = trimmed.split('/').filter(Boolean).pop() || 'model'
  let slug = lastSegment.toLowerCase().replace(/[^a-z0-9.-]/g, '-')
  slug = slug.replace(/-+/g, '-').replace(/^-+|-+$/g, '')
  if (!slug) slug = 'model'
  slug = slug.slice(0, 30)
  return `prewarm-${slug}`
}

export function parseModelPrewarmOutputs(outputs: Array<{ name?: string; value?: string }> = []): ModelPrewarmOutputs {
  const map = new Map(outputs.map((o) => [o.name, o.value]))
  let nodesDetail: ModelPrewarmNodeDetail[] = []
  const raw = map.get('nodes_detail')
  if (raw) {
    try {
      nodesDetail = JSON.parse(raw)
    } catch {
      nodesDetail = []
    }
  }
  return {
    status: map.get('status'),
    message: map.get('message'),
    nodesTotal: map.get('nodes_total'),
    nodesSucceeded: map.get('nodes_succeeded'),
    nodesFailed: map.get('nodes_failed'),
    nodesPending: map.get('nodes_pending'),
    prewarmProgress: map.get('prewarm_progress'),
    nodesDetail,
  }
}

export function parseProgressPercent(progress?: string, phase?: string): number {
  if (progress) {
    const n = parseFloat(progress.replace('%', ''))
    if (!Number.isNaN(n)) return Math.min(100, Math.max(0, n))
  }
  if (phase === 'Succeeded') return 100
  if (phase === 'Failed') return 0
  return 0
}

export function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return '-'
  const gb = bytes / 1024 ** 3
  return `${gb.toFixed(1)} GB`
}

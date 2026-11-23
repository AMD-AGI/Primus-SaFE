import dayjs from 'dayjs'
import type { UsageQuery, UsageSortKey } from '@/services/usage/type'

/** 'name' leaves sortBy unset, so the server orders the workspaces by name. */
export type WorkspaceSort = UsageSortKey | 'name'

export type UsageRangeKey = '1d' | '7d' | '30d' | 'custom'

export interface UsageRangeValue {
  key: UsageRangeKey
  /** Only used when key is "custom" */
  custom: [Date, Date] | null
}

const HOUR_MS = 3600e3

const PRESET_HOURS: Record<Exclude<UsageRangeKey, 'custom'>, number> = {
  '1d': 24,
  '7d': 168,
  '30d': 720,
}

/** Bucket width for a range: finer for short ranges, coarser for long ones. */
export const stepForHours = (hours: number): string => {
  if (hours <= 24) return '15m'
  if (hours <= 168) return '1h'
  return '4h'
}

/** Maps the range selector to the start / end / step query of the usage APIs. */
export const buildUsageQuery = (range: UsageRangeValue, now = new Date()): UsageQuery => {
  if (range.key === 'custom') {
    if (!range.custom) return {}
    const [start, end] = range.custom
    const hours = (end.getTime() - start.getTime()) / HOUR_MS
    return { start: start.toISOString(), end: end.toISOString(), step: stepForHours(hours) }
  }
  const hours = PRESET_HOURS[range.key]
  return {
    start: new Date(now.getTime() - hours * HOUR_MS).toISOString(),
    end: now.toISOString(),
    step: stepForHours(hours),
  }
}

/** A ratio (0..1) as a percentage; null means "no data". */
export const fmtRatio = (v: number | null | undefined): string =>
  v == null || !Number.isFinite(v) ? '-' : `${(v * 100).toFixed(1)}%`

/** A GPU utilization that is already a percentage (0..100). */
export const fmtPercent = (v: number | null | undefined): string =>
  v == null || !Number.isFinite(v) ? '-' : `${v.toFixed(0)}%`

export const fmtNum = (v: number | null | undefined): string =>
  v == null || !Number.isFinite(v) ? '-' : Math.round(v).toLocaleString('en-US')

export type UsageTone = 'danger' | 'warning' | 'success' | 'info'

/** Color tone of a utilization ratio; unknown values stay neutral. */
export const utilizationTone = (ratio: number | null | undefined): UsageTone => {
  if (ratio == null) return 'info'
  if (ratio < 0.2) return 'danger'
  if (ratio < 0.35) return 'warning'
  return 'success'
}

/** Axis label of a series time: the clock for a short range, the date otherwise. */
export const fmtAxisTime = (time: string, spanHours: number): string =>
  dayjs(time).format(spanHours <= 24 ? 'HH:mm' : 'MM-DD HH:mm')

/** Utilization coverage below this share of the allocated time triggers a warning. */
export const COVERAGE_WARN_THRESHOLD = 0.9

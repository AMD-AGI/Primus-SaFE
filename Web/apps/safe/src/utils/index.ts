/* eslint-disable @typescript-eslint/no-explicit-any */
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import { ElMessage } from 'element-plus'

dayjs.extend(utc)

/**
 * Given an object and a field template,
 * if the object is missing a field, automatically fill in the default value from the template.
 *
 * Typical scenarios:
 * - Backend response data has missing fields (key not returned at all)
 * - Frontend needs to normalize structure to avoid component errors or undefined fields
 *
 * @param obj      The object whose fields need to be filled (actual data)
 * @param template Field template specifying which fields must exist and their default values
 * @returns        The object with filled fields (modified in place)
 *
 * @example
 * const template = { name: '', age: 0 }
 * const raw = { name: 'xxx' }
 * fillMissingKeys(raw, template)
 * // => { name: 'xxx', age: 0 }
 */
export function fillMissingKeys<T extends object>(obj: T, template: Partial<T>): T {
  for (const key in template) {
    if (!(key in obj)) {
      ;(obj as any)[key] = template[key]
    }
  }
  return obj
}

/**
 * Fill missing fields for API response data
 * @param fetchFn Original request function (must return Promise)
 * @param template Default field template
 * @returns Wrapped function that auto-fills fields on call
 */
export function withFieldDefaults<T extends object>(
  fetchFn: () => Promise<any>,
  template: Partial<T>,
): () => Promise<T[] | T> {
  return async () => {
    const result = await fetchFn()

    if (Array.isArray(result)) {
      return result.map((item) => fillMissingKeys(item, template))
    } else if (typeof result === 'object' && result !== null) {
      return fillMissingKeys(result, template)
    } else {
      return result // Return as-is
    }
  }
}

/**
 *  CamelCase to SnakeCaseN
 */
export function toSnakeCase(obj: any): any {
  if (Array.isArray(obj)) {
    return obj.map(toSnakeCase)
  } else if (obj !== null && typeof obj === 'object') {
    return Object.fromEntries(
      Object.entries(obj).map(([key, value]) => [
        key.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`),
        toSnakeCase(value),
      ]),
    )
  }
  return obj
}

/**
 * SnakeCase to CamelCase
 */
export function toCamelCase(obj: any): any {
  if (Array.isArray(obj)) {
    return obj.map(toCamelCase)
  } else if (obj !== null && typeof obj === 'object') {
    return Object.fromEntries(
      Object.entries(obj).map(([key, value]) => [
        key.replace(/_([a-z])/g, (_, letter) => letter.toUpperCase()),
        toCamelCase(value),
      ]),
    )
  }
  return obj
}

/**
 * Convert '…h', '…d' into milliseconds
 */
export function parseRangeToMs(range: string): number {
  const unit = range.slice(-1)
  const num = parseInt(range.slice(0, -1))

  switch (unit) {
    case 'h':
      return num * 60 * 60 * 1000
    case 'd':
      return num * 24 * 60 * 60 * 1000
    default:
      return 1 * 60 * 60 * 1000 // default 1 hour
  }
}

/**
 * Convert a snake_case key to "Capitalized Words" for display.
 * Example: static_gpu_details -> Static Gpu Details
 */
const ACRONYM_MAP = new Set(['OS', 'CPU', 'GPU', 'RAM', 'GB', 'TB', 'KB'])
function formatLabel(key: string): string {
  return key
    .split('_')
    .map((word) => {
      const upper = word.toUpperCase()
      if (ACRONYM_MAP.has(upper)) return upper
      return word.charAt(0).toUpperCase() + word.slice(1)
    })
    .join(' ')
}
/**
 * Clean value for display, e.g. trim extra spaces
 */
function cleanValue(value: any): string {
  if (typeof value === 'string') {
    return value.trim()
  }
  return String(value)
}

/**
 * Convert raw JSON object into display-friendly label-value pairs
 */
export function formatNodeInfo(raw: Record<string, any>) {
  return Object.entries(raw).map(([key, value]) => ({
    label: formatLabel(key),
    value: cleanValue(value),
  }))
}

/**
 * Get chart sampling step (in seconds) based on fixed time range key like '1h', '3h', etc.
 * Used for predefined time range dropdowns.
 *
 * @param range - Time range key (e.g., '1h', '1d')
 * @returns Step interval in seconds
 */
export const getStepByRangeKey = (range: string): number => {
  const stepMap: Record<string, number> = {
    '1h': 60,
    '3h': 180,
    '6h': 300,
    '12h': 600,
    '1d': 1800,
    '2d': 3600,
    '7d': 7200,
    '30d': 21600,
  }
  return stepMap[range] || 3600 // default 1point / 1h
}

/**
 * Dynamically calculate step interval (in seconds) based on actual timestamp range.
 * Used for custom time range pickers (e.g. <el-date-picker type="datetimerange" />).
 *
 * @param start - Start timestamp in seconds
 * @param end - End timestamp in seconds
 * @returns Step interval in seconds
 */
export const getStepByTimestampDiff = (start: number, end: number): number => {
  const duration = end - start

  if (duration <= 3600) return 30 // ≤1h → 30s
  if (duration <= 3 * 3600) return 60 // ≤3h → 1m
  if (duration <= 6 * 3600) return 180 // ≤6h → 3m
  if (duration <= 24 * 3600) return 300 // ≤24h → 5m
  return 600 // >24h → 10m default
}

export async function copyText(text: string): Promise<void> {
  // Try using modern Clipboard API
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      ElMessage.success('Copied')
      return
    } catch {
      // Safari may silently fail, fallback to execCommand
    }
  }

  // Fallback: use execCommand
  const ta = document.createElement('textarea')
  ta.value = text
  ta.setAttribute('readonly', '')
  ta.style.cssText = 'position:fixed;opacity:0;pointer-events:none;left:-9999px;top:-9999px;'
  document.body.appendChild(ta)

  // Safari requires focus before select
  ta.focus()
  ta.select()

  // Safari iOS requires setSelectionRange
  if (ta.setSelectionRange) {
    ta.setSelectionRange(0, text.length)
  }

  let ok = false
  try {
    ok = document.execCommand('copy')
  } catch {
    ok = false
  }

  document.body.removeChild(ta)

  if (ok) {
    ElMessage.success('Copied')
  } else {
    ElMessage.error('Failed to copy. Please copy manually.')
    throw new Error('Copy failed')
  }
}

export * from './share'
export function byte2Gi(byte: number, fractionDigits = 0, withUnit = true): string {
  if (isNaN(byte) || byte < 0) return withUnit ? '0 Gi' : '0'
  const gib = byte / 1024 / 1024 / 1024
  return withUnit ? `${gib.toFixed(fractionDigits)} Gi` : gib.toFixed(fractionDigits)
}
export function formatBytes(byte: number, fractionDigits = 2): string {
  if (isNaN(byte) || byte < 0) return '0 B'

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let size = byte
  let idx = 0

  while (size >= 1000 && idx < units.length - 1) {
    size = size / 1000
    idx++
  }

  // e.g. 71924621 -> 71.92 MB
  return `${size.toFixed(fractionDigits)} ${units[idx]}`
}
export function fmtVal(val: unknown, key: string) {
  if (val === null || val === undefined || val === '-') return '-'
  if (['memory', 'ephemeral-storage', 'dataDisk', 'rootDisk'].includes(key)) {
    return byte2Gi(val as number)
  }
  return String(val)
}

/**
 * Converts a plain key-value object (Record<string, any>) to a list of key-value pairs,
 * where each item is in the format { key: string, value: string }.
 *
 * If the input is nullish or not an object, returns a default list with one empty pair.
 * If the object is empty, also returns a list with one empty pair.
 *
 * @param record - The input key-value object (e.g., environment variables or labels)
 * @returns A normalized list of key-value pairs
 */
export function convertKeyValueMapToList(
  record?: Record<string, any>,
): { key: string; value: string }[] {
  if (!record || typeof record !== 'object') {
    return [{ key: '', value: '' }]
  }

  const list = Object.entries(record).map(([key, value]) => ({
    key,
    value: String(value),
  }))

  return list.length > 0 ? list : [{ key: '', value: '' }]
}

/**
 * Converts a list of key-value objects (e.g., from a form input) to a plain key-value map.
 *
 * Filters out items with empty keys. Each value is returned as-is (assumed to be string).
 *
 * @param list - The input list of key-value pairs (e.g., from envList or labelList)
 * @returns A plain object map with key-value pairs
 */
export function convertListToKeyValueMap(
  list: { key: string; value: string }[],
): Record<string, string> {
  return Object.fromEntries(
    list.filter((item) => item.key && item.value).map(({ key, value }) => [key, value]),
  )
}

export const fmtTs = (s: string) => {
  if (!s) return ''
  const noFrac = s.replace(/\.\d+/, '')
  const iso = noFrac.replace(' ', 'T')
  const d = new Date(iso)
  if (isNaN(d.getTime())) {
    return noFrac.replace('T', ' ').replace(/Z$/, '')
  }
  const pad = (n: number) => String(n).padStart(2, '0')
  const y = d.getFullYear()
  const M = pad(d.getMonth() + 1)
  const D = pad(d.getDate())
  const h = pad(d.getHours())
  const m = pad(d.getMinutes())
  const sec = pad(d.getSeconds())
  return `${y}-${M}-${D} ${h}:${m}:${sec}`
}

// Convert k,v to key-value object array in multi-line edit mapping scenario
type KV = { key: string; value: string }

export const toKVList = (src: unknown): KV[] => {
  if (Array.isArray(src)) {
    const list = src.map((x: any) => ({
      key: String(x?.key ?? ''),
      value: String(x?.value ?? ''),
    }))
    return list.length ? list : [{ key: '', value: '' }]
  }
  if (src && typeof src === 'object') {
    const entries = Object.entries(src as Record<string, unknown>)
    return entries.length
      ? entries.map(([k, v]) => ({ key: k, value: v == null ? '' : String(v) }))
      : [{ key: '', value: '' }]
  }
  return [{ key: '', value: '' }]
}

// Split string, match value + dropdown suffix unit separately
export const CAP_UNITS = ['Pi', 'Ti', 'Gi', 'Mi', 'Ki'] as const
type CapUnit = (typeof CAP_UNITS)[number]
export const parseQuantityWithUnit = (raw?: string): { val: string; unit: CapUnit } => {
  if (!raw) return { val: '', unit: 'Gi' }
  const s = String(raw).trim()

  // Number (decimal allowed) + optional space + unit (optional), case-insensitive for unit
  const m = s.match(/^(\d+(?:\.\d+)?)(?:\s*)(Ki|Mi|Gi|Ti|Pi)?$/i)
  if (!m) {
    // Invalid format: extract numeric part, default unit Gi
    return { val: s.replace(/[^\d.]/g, ''), unit: 'Gi' }
  }

  const val = m[1]
  // Normalize unit to Ki/Mi/Gi/Ti/Pi
  const unitRaw = m[2] ?? 'Gi'
  const unitNorm = (unitRaw[0].toUpperCase() + unitRaw.slice(1).toLowerCase()) as CapUnit

  return { val, unit: CAP_UNITS.includes(unitNorm) ? unitNorm : 'Gi' }
}
export function applyQuantityWithUnit(
  target: { quantity: any },
  source: string | undefined,
  setUnit: (unit: string) => void,
  defaultUnit = 'Gi',
) {
  if (source) {
    const { val, unit } = parseQuantityWithUnit(source)
    target.quantity = val
    setUnit(unit)
  } else {
    target.quantity = ''
    setUnit(defaultUnit)
  }
}

export function encodeToBase64String(str: string): string {
  if (!str) return ''
  return btoa(unescape(encodeURIComponent(str)))
}

export function decodeFromBase64String(b64: string): string {
  if (!b64) return ''

  // 1) Normalize: remove whitespace, support URL-safe Base64 (-_/)
  const norm = b64.trim().replace(/\s+/g, '').replace(/-/g, '+').replace(/_/g, '/')
  // Length 1 mod 4 is always invalid, return original value
  if (norm.length % 4 === 1) return b64
  // Pad with '='
  const padded = norm.padEnd(norm.length + ((4 - (norm.length % 4)) % 4), '=')

  try {
    const binary = atob(padded)

    // 2) Re-encode validation, prevent misinterpreting normal strings as Base64
    const reencoded = btoa(binary).replace(/=+$/, '')
    const inputNoPad = norm.replace(/=+$/, '')
    if (reencoded !== inputNoPad) return b64

    // 3) Decode symmetrically with existing encode
    return decodeURIComponent(escape(binary))
  } catch {
    // atob/decode failed: not Base64, return original value
    return b64
  }
}

export const formatTimeStr = (raw?: string) => {
  if (!raw) return '-'

  // Check if timezone info is included (Z or +HH:mm / -HH:mm)
  const hasTZ = /([zZ]|[+-]\d{2}:\d{2})$/.test(raw)

  // If backend didn't include timezone, append Z to treat as UTC
  const normalized = hasTZ ? raw : raw + 'Z'

  // 1. Parse as UTC
  // 2. Convert to current browser local timezone
  // 3. Format output
  return dayjs.utc(normalized).local().format('YYYY-MM-DD HH:mm:ss')
}

// YYYY-MM-DD HH:mm convert to/from standard time
export function toUTCISOString(minuteStr: string): string {
  // minuteStr: "YYYY-MM-DD HH:mm"
  const d = dayjs(minuteStr, 'YYYY-MM-DD HH:mm').second(0).millisecond(0)
  return d.toISOString() // Automatically converts to UTC, ends with Z
}
export function decodeScheduleFromApi(apiSchedule?: string): string | undefined {
  if (!apiSchedule) return undefined
  return dayjs(apiSchedule).format('YYYY-MM-DD HH:mm')
}

// Random state - for SSO
export function randomState(len = 16): string {
  const b = new Uint8Array(len)
  crypto.getRandomValues(b)
  return btoa(String.fromCharCode(...b))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '')
}

type Range = { start_time: string; end_time: string }
export function last24hUtcExact(): Range {
  const end = dayjs.utc() // Current UTC
  const start = end.subtract(24, 'hour')
  const fmt = (d: dayjs.Dayjs) => d.format('YYYY-MM-DDTHH:mm:ss[Z]')
  return { start_time: fmt(start), end_time: fmt(end) }
}

/**
 * Get API hostname
 * Extracts from VITE_API_BASE_URL in dev, uses window.location.hostname in production
 */
export function getApiHost(): string {
  const base = (import.meta.env.VITE_API_BASE_URL || '/api').trim()

  const toHostname = (raw: string): string => {
    try {
      const hasScheme = /^[a-zA-Z][a-zA-Z\d+\-.]*:\/\//.test(raw)
      const url = new URL(hasScheme ? raw : `http://${raw}`)
      return url.hostname
    } catch {
      // Fallback
      const s = raw.replace(/^https?:\/\//, '').replace(/\/.*$/, '')
      const noBrackets = s.replace(/^\[|]$/g, '') // IPv6 [::1]
      return noBrackets.replace(/:\d+$/, '')
    }
  }

  if (import.meta.env.DEV) {
    return toHostname(base)
  } else {
    return window.location.hostname
  }
}

/**
 * Calculate default time range for workloads (for Grafana and other monitoring components)
 *
 * @param startTime - Workload start time string (format: YYYY-MM-DD HH:mm:ss)
 * @param endTime - Workload end time string (format: YYYY-MM-DD HH:mm:ss), optional
 * @param creationTime - Workload creation time string (format: YYYY-MM-DD HH:mm:ss), fallback for startTime
 * @returns Time range array [start, end], or null if no valid start time
 *
 * @example
 * const timeRange = calculateDefaultTime(
 *   workloadDetail.value?.startTime,
 *   workloadDetail.value?.endTime,
 *   workloadDetail.value?.creationTime
 * )
 */
export function calculateDefaultTime(
  startTime?: string,
  endTime?: string,
  creationTime?: string,
): [Date, Date | 'now'] | null {
  const s = startTime || creationTime
  const e = endTime
  if (!s) return null

  // Backend returns UTC time, append 'Z' for browser UTC parsing, auto-converts to user's local timezone
  // Consistent processing logic with formatTimeStr
  const startRaw = new Date(s.replace(' ', 'T') + 'Z')
  const start = startRaw

  if (e) {
    const endRaw = new Date(e.replace(' ', 'T') + 'Z')
    const end = endRaw
    return end >= start ? [start, end] : [start, start]
  } else {
    return [start, 'now']
  }
}

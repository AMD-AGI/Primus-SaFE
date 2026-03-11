import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import timezone from 'dayjs/plugin/timezone'

dayjs.extend(utc)
dayjs.extend(timezone)

/**
 *  CamelCase to SnakeCase
 *  @param obj - The object to convert
 *  @param skipKeys - Keys whose values should not be recursively converted (for business data like dimensions)
 */
const DEFAULT_SKIP_KEYS = new Set(['dimensions', 'metrics', 'metricFilters', 'rawData', 'values'])

export function toSnakeCase(obj: any, skipKeys: Set<string> = DEFAULT_SKIP_KEYS): any {
  if (Array.isArray(obj)) {
    return obj.map(item => toSnakeCase(item, skipKeys))
  } else if (obj !== null && typeof obj === 'object') {
    return Object.fromEntries(
      Object.entries(obj).map(([key, value]) => {
        const snakeKey = key.replace(/[A-Z]/g, letter => `_${letter.toLowerCase()}`)
        // Skip recursive conversion for specified keys (business data)
        const newValue = skipKeys.has(key) ? value : toSnakeCase(value, skipKeys)
        return [snakeKey, newValue]
      })
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
        toCamelCase(value)
      ])
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
      case 'h': return num * 60 * 60 * 1000
      case 'd': return num * 24 * 60 * 60 * 1000
      default: return 1 * 60 * 60 * 1000 // default 1 hour
    }
  }

  /**
 * Convert a snake_case key to "Capitalized Words" for display.
 * Example: static_gpu_details -> Static Gpu Details
 */
  const ACRONYM_MAP = new Set([
    'OS', 'CPU', 'GPU', 'RAM', 'GB', 'TB', 'KB',
  ])
  function formatLabel(key: string): string {
    return key
      .split('_')
      .map(word => {
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

  if (duration <= 3600) return 30        // ≤1h → 30s
  if (duration <= 3 * 3600) return 60    // ≤3h → 1m
  if (duration <= 6 * 3600) return 180   // ≤6h → 3m
  if (duration <= 24 * 3600) return 300  // ≤24h → 5m
  return 600                             // >24h → 10m default
}

/**
 * Format a Unix timestamp (in seconds) into a readable label string based on the selected time range.
 *
 * - For short ranges like '1h', '3h', or '6h', returns only hour and minute (e.g. '14:30')
 * - For longer ranges, returns full date and time (e.g. '07-27 14:30')
 *
 * @param times - Unix timestamp in seconds
 * @param range - Time range string ('1h', '3h', '6h', '1d', etc.)
 * @returns Formatted time string for chart axis labels or display
 */
export const formatMetricTimeLabel = (times: number, range: string): string => {
  const d = dayjs(times * 1000)
  if(['1h', '3h', '6h'].includes(range)){
    return d.tz(dayjs.tz.guess()).format('HH:mm')
  }else{
    return d.tz(dayjs.tz.guess()).format('MM-DD HH:mm')
  }
}

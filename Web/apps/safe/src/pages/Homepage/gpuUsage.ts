export interface GPUAggregationItem {
  id?: number
  cluster_name?: string
  namespace?: string
  stat_hour: string
  avg_utilization?: number | null
  allocation_rate?: number | null
  allocated_gpu_count?: number | null
  active_workload_count?: number | null
  total_gpu_capacity?: number | null
  created_at?: string
  updated_at?: string
}

export interface GpuStats {
  avgUtilization: string
  avgAllocation: string
  totalWorkloads: number
  lowUtilization: number
}

export interface GpuUsageSeries {
  times: string[]
  utilization: number[]
  allocation: number[]
}

const normalizePercent = (value: number | null | undefined) => {
  if (value == null || !Number.isFinite(value)) return 0
  return value > 1 ? value : value * 100
}

const normalizePercentSamples = (values: Array<number | null | undefined>) =>
  values
    .filter((value): value is number => value != null && Number.isFinite(value))
    .map((value) => normalizePercent(value))

const average = (values: number[]) =>
  values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : 0

export const unwrapGpuAggregationRows = (response: unknown): GPUAggregationItem[] => {
  const payload = response as any
  const rows = payload?.data?.data ?? payload?.data ?? payload
  return Array.isArray(rows) ? rows : []
}

export const buildGpuStats = (rows: GPUAggregationItem[]): GpuStats => {
  if (!rows.length) {
    return {
      avgUtilization: '0.0',
      avgAllocation: '0.0',
      totalWorkloads: 0,
      lowUtilization: 0,
    }
  }

  const utilizationSamples = normalizePercentSamples(rows.map((item) => item.avg_utilization))
  const allocationSamples = normalizePercentSamples(rows.map((item) => item.allocation_rate))
  const avgUtilization = average(utilizationSamples)
  const avgAllocation = average(allocationSamples)

  return {
    avgUtilization: avgUtilization.toFixed(1),
    avgAllocation: avgAllocation.toFixed(1),
    totalWorkloads: rows[0]?.active_workload_count ?? 0,
    lowUtilization: utilizationSamples.filter((value) => value < 30).length,
  }
}

export const buildGpuUsageSeries = (rows: GPUAggregationItem[]): GpuUsageSeries => {
  const sortedRows = [...rows].reverse()

  return {
    times: sortedRows.map((item) =>
      new Date(item.stat_hour).toLocaleString('en-US', {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
    ),
    utilization: sortedRows.map((item) =>
      Number(normalizePercent(item.avg_utilization).toFixed(2)),
    ),
    allocation: sortedRows.map((item) => Number(normalizePercent(item.allocation_rate).toFixed(2))),
  }
}

export const getDateRange = (days: number): [Date, Date] => {
  const end = new Date()
  const start = new Date()
  start.setDate(start.getDate() - days)
  return [start, end]
}

export const formatDateWithTimezone = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  const seconds = String(date.getSeconds()).padStart(2, '0')
  const timezoneOffset = -date.getTimezoneOffset()
  const offsetHours = String(Math.floor(Math.abs(timezoneOffset) / 60)).padStart(2, '0')
  const offsetMinutes = String(Math.abs(timezoneOffset) % 60).padStart(2, '0')
  const offsetSign = timezoneOffset >= 0 ? '+' : '-'

  return `${year}-${month}-${day}T${hours}:${minutes}:${seconds}${offsetSign}${offsetHours}:${offsetMinutes}`
}

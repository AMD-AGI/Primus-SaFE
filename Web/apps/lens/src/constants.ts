export const TIME_RANGE_OPTIONS = [
    { label: 'Last 1 hour', value: '1h' },
    { label: 'Last 3 hours', value: '3h' },
    { label: 'Last 6 hours', value: '6h' },
    { label: 'Last 12 hours', value: '12h' },
    { label: 'Last 1 day', value: '1d' },
    { label: 'Last 2 days', value: '2d' },
    { label: 'Last 7 days', value: '7d' },
    { label: 'Last 30 days', value: '30d' },
  ]

export const NODE_STATUS_TAG = {
  'Ready': 'success',
  'NotReady': 'danger',
  'Unknown': 'info',
}

export const WORKLOAD_STATUS_TAG = {
  'Running': 'primary',
  'Done': 'success',
}

export type NodeStatus = keyof typeof NODE_STATUS_TAG
export type WorkloadStatus = keyof typeof WORKLOAD_STATUS_TAG
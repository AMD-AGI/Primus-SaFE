export enum ImagePhase {
  Succeeded = 'Succeeded',
  Failed = 'Failed',
  Pending = 'Pending',
  Running = 'Running',
  Stopped = 'Stopped',
}

type ImageTagType = 'success' | 'danger' | 'primary'

export const ImagePhaseButtonType: Record<string, { type: ImageTagType }> = {
  Ready: {
    type: 'success',
  },
  Failed: {
    type: 'danger',
  },
  Importing: {
    type: 'primary',
  },
}

export interface SubmitImageRequest {
  tag?: string
  image?: string
  page_num?: number
  page_size?: number
  orderBy?: string
  order?: 'asc' | 'desc'
  flat?: boolean
}

export interface SubmitImageRegRequest {
  name: string
  url: string
  username: string
  password?: string
}

export interface ImportImageRequest {
  source: string
  sourceRegistry?: string
}

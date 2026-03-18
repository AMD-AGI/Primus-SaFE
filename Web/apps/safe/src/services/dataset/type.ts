export interface DatasetItem {
  datasetId: string
  displayName: string
  datasetType?: string
  description?: string
  fileCount?: number
  s3Path?: string
  status?: string
  totalSize?: number
  totalSizeStr?: string
  creationTime?: string
  updateTime?: string
  userId?: string
  userName?: string
  workspace?: string
  workspaceName?: string
  source?: 'upload' | 'huggingface'
  sourceUrl?: string
  message?: string
}

export interface GetDatasetsParams {
  datasetType?: string
  workspace?: string
  search?: string
  source?: string
  pageNum?: number
  pageSize?: number
  orderBy?: string
  order?: 'asc' | 'desc'
}

export interface GetDatasetsResponse {
  items: DatasetItem[]
  total: number
  pageNum: number
  pageSize: number
}

export interface CreateDatasetParams {
  displayName: string
  description?: string
  datasetType: string
  workspace?: string
  files?: File[]
}

export interface DatasetTypeSchema {
  [key: string]: string
}

export interface DatasetType {
  name: string
  description: string
  schema: DatasetTypeSchema
}

export interface GetDatasetTypesResponse {
  types: DatasetType[]
}

export interface DatasetFile {
  fileName: string
  filePath: string
  fileSize: number
  sizeStr: string
}

export interface DatasetDetail extends DatasetItem {
  files: DatasetFile[]
}

export interface ImportHFDatasetParams {
  url: string
  datasetType: string
  workspace?: string
  token?: string
}

export interface ImportHFDatasetResponse {
  datasetId: string
  displayName: string
  description?: string
  status: string
  source: string
  sourceUrl: string
}

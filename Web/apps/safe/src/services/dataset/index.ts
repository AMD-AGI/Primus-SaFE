import request from '@/services/request'
import type {
  GetDatasetsParams,
  GetDatasetsResponse,
  CreateDatasetParams,
  GetDatasetTypesResponse,
  DatasetDetail,
  ImportHFDatasetParams,
  ImportHFDatasetResponse,
} from './type'

export const getDatasets = (params?: GetDatasetsParams): Promise<GetDatasetsResponse> =>
  request.get('/datasets', { params })

export const getDatasetTypes = (): Promise<GetDatasetTypesResponse> =>
  request.get('/datasets/types')

export const createDataset = (data: CreateDatasetParams): Promise<{ datasetId: string }> => {
  const formData = new FormData()
  formData.append('displayName', data.displayName)
  formData.append('datasetType', data.datasetType)
  if (data.workspace) {
    formData.append('workspace', data.workspace)
  }
  if (data.description) {
    formData.append('description', data.description)
  }
  if (data.files && data.files.length > 0) {
    data.files.forEach((file) => {
      formData.append('files', file)
    })
  }
  return request.post('/datasets', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    timeout: 30 * 60 * 1000, // 30 minutes for large file uploads
  })
}

export const deleteDataset = (id: string): Promise<{ message: string }> =>
  request.delete(`/datasets/${id}`)

export const getDatasetDetail = (id: string): Promise<DatasetDetail> =>
  request.get(`/datasets/${id}`)

export const previewDatasetFile = (
  id: string,
  path: string,
): Promise<{ fileName: string; content: string }> =>
  request.get(`/datasets/${id}/files/${encodeURIComponent(path)}`, { params: { preview: true } })

export const importHFDataset = (data: ImportHFDatasetParams): Promise<ImportHFDatasetResponse> =>
  request.post('/datasets/import-hf', data)

import request from '@/services/request'
import type { SubmitImageRegRequest, ImportImageRequest, SubmitImageRequest } from './type'

export const getImagesList = (params: SubmitImageRequest): Promise<any> =>
  request.get('/images', { params })

export const deleteImage = (id: number): Promise<any> => request.delete(`/images/${id}`)

export const importImage = (data: ImportImageRequest): Promise<any> =>
  request.post('/images:import', data)

export const getImportDetail = (id: number): Promise<any> =>
  request.get(`/images/${id}/importing-details`)

export const getImportLogs = (
  id: number,
  params?: { offset?: number; limit?: number; order?: string },
): Promise<any> =>
  request.get(`/images/${id}/importing-logs`, { params }).then((res: any) => {
    const hits = res?.hits?.hits ?? []
    const rows = hits.map((h: any) => ({
      id: h._id,
      timestamp: h._source?.['@timestamp'] ?? '',
      message: h._source?.message ?? '',
      pod_name: h._source?.kubernetes?.pod_name ?? '',
      host: h._source?.kubernetes?.host ?? '',
    }))
    return {
      rows,
      total: res?.hits?.total?.value ?? 0,
      took: res?.took ?? 0,
    }
  })

export const retryImage = (id: number) => request.put(`/images/${id}/importing:retry`)

export const getImageRegList = (params: any): Promise<any> =>
  request.get('/image-registries', { params })

export const addImageReg = (data: SubmitImageRegRequest) => request.post('/image-registries', data)

export const editImageReg = (id: number, data: SubmitImageRegRequest) =>
  request.put(`/image-registries/${id}`, data)

export const deleteImageReg = (id: string): Promise<any> =>
  request.delete(`/image-registries/${id}`)

// Authoring - get image custom details
export const getImageCustom = (params: any): Promise<any> =>
  request.get('/images/custom', { params })

export const deleteImageCustom = (jobId: string): Promise<any> =>
  request.delete(`/images/custom/${jobId}`)

export const getImagePrewarmList = (params: any): Promise<any> =>
  request.get('/images/prewarm', { params })

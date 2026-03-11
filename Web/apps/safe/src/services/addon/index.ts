import request from '@/services/request'
import type {
  SubmitAddonData,
  BaseAddonData,
  AddonTemplateDetail,
  AddonsItemResp,
  AddonDetailData,
  AddonsTempItemResp,
} from './type'

// addon temp

export const getAddontemps = (params: any): Promise<AddonsTempItemResp> =>
  request.get('/addontemplates', { params })

export const getAddontempDetail = (id: string): Promise<AddonTemplateDetail> =>
  request.get(`/addontemplates/${id}`)

// addon

export function createAddon(clusterId: string, data: SubmitAddonData) {
  return request.post(`/clusters/${clusterId}/addons`, data)
}

export const getAddonsList = (clusterId: string): Promise<AddonsItemResp> =>
  request.get(`/clusters/${clusterId}/addons`)

export const getAddonDetail = (clusterId: string, name: string): Promise<AddonDetailData> =>
  request.get(`/clusters/${clusterId}/addons/${name}`)

export function editAddon(clusterId: string, name: string, data: BaseAddonData) {
  return request.patch(`/clusters/${clusterId}/addons/${name}`, data)
}

export const deleteAddon = (clusterId: string, name: string) =>
  request.delete(`/clusters/${clusterId}/addons/${name}`)

export interface BaseAddonData {
  template: string
  values?: string
  description?: string
}
export interface SubmitAddonData extends BaseAddonData {
  releaseName: string
  namespace?: string
}

export interface AddonTemplateHelmStatus {
  values?: unknown
  valuesYaml?: string
}

export interface AddonTemplateDetail {
  addonTemplateId: string
  type: string
  category?: string
  version?: string
  description?: string
  gpuChip?: string
  required?: boolean
  creationTime?: string
  url?: string
  action?: string
  icon?: string
  helmDefaultValues?: string
  helmDefaultNamespace?: string
  helmStatus?: AddonTemplateHelmStatus
}

export interface AddonStatus {
  status?: string
  version?: number
}

export interface AddonsData {
  name: string
  releaseName: string
  template: string
  namespace: string
  cluster: string
  status: AddonStatus[]
}

export interface AddonTemp {
  addonTemplateId: string
  type: string
  category: string
  version: string
  description?: string
  gpuChip?: string
  required?: boolean
  creationTime?: string
}

export interface AddonsItemResp {
  totalCount: number
  items: AddonsData[]
}

export interface AddonsTempItemResp {
  totalCount: number
  items: AddonTemp[]
}

export interface AddonDetailData extends AddonsData {
  description?: string
  values?: string
}

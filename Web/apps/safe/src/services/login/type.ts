export interface RegisterReq {
  name: string
  type: string
  workspaces?: string[]
  password: string
}

export interface LoginReq {
  type: string
  name?: string
  password?: string
  code?: string | number
  state?: string | number
}

export interface BaseUserResp {
  id: string
  name: string
  email: string
  type: string
  roles: string[]
}

export interface EditUserResp {
  password?: string
  email?: string
  roles?: string[]
  workspaces?: string[]
  managedWorkspaces?: string[]
  restrictedType?: number
}

export interface LoginResp extends BaseUserResp {
  expire?: number
  token?: string
}

export interface Workspace {
  id: string
  name: string
}
export interface UserSelfData extends BaseUserResp {
  workspaces: Workspace[]
  managedWorkspaces?: Workspace[]
  creationTime: string
  restrictedType: number
}

export interface UsersItemResp {
  totalCount: number
  items: UserSelfData[]
}

export interface EnvsResp {
  enableLogDownload: boolean
  enableLog: boolean
  enableSsh: boolean
  sshPort?: number
  sshIP?: string
  authoringImage?: string
  // sso
  ssoEnable?: boolean
  ssoAuthUrl?: string
  // cd
  cdRequireApproval?: boolean
}

export interface UserSettings {
  enableNotification: boolean
}

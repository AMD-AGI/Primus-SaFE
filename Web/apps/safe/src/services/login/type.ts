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

// GET /users/self/workspaces: the workspaces the user can access and the ones the user manages
export interface SelfWorkspacesResp {
  workspaces: Workspace[]
  managedWorkspaces: Workspace[]
}

export interface UsersItemResp {
  totalCount: number
  items: UserSelfData[]
}

export interface EnvsResp {
  // Robust GPU aggregation. Absent or false means the homepage must not call it.
  enableRobust?: boolean
  enableLogDownload: boolean
  enableLog: boolean
  enableSsh: boolean
  sshPort?: number
  sshIP?: string
  authoringImage?: string
  // How requests are authenticated. With 'external' an identity provider owns sign-in,
  // accounts, API keys and platform roles; the console hides what the server refuses.
  authMode?: 'builtin' | 'external'
  // With 'external': the path on this console's own origin where the deployment routes API key
  // creation, listing and revocation to the identity provider. Absent when nothing is routed there.
  apiKeysUrl?: string
  // sso
  ssoEnable?: boolean
  ssoAuthUrl?: string
  // The console's sign-in when the server has one with PKCE (sso.login): used instead of ssoAuthUrl,
  // with an S256 code challenge, and the verifier is sent with the code
  ssoLoginUrl?: string
  // The console signs in only with PKCE (sso.login is on), even while ssoLoginUrl is empty because
  // the provider cannot be reached: it then does not fall back to ssoAuthUrl
  ssoPkce?: boolean
  // origins (optionally with a path prefix) other than this console allowed to receive the SSO code
  ssoBridgeAllowlist?: string[]
  // cd
  cdRequireApproval?: boolean
  // Whom a signed-in user without access should ask for it (plain text, may be empty or absent
  // on older servers). Never render it as HTML.
  accessContact?: string
  // KubeSpray image tags mapped to the Kubernetes version each one installs. The pair is
  // not free-form: picking an image determines the version, so the form derives it.
  kubeSprayK8sVersions?: Record<string, string>
}

export interface UserSettings {
  enableNotification: boolean
}

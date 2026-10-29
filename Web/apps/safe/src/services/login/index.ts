import axios from 'axios'
import request, { postForm } from '@/services/request'
import type {
  RegisterReq,
  LoginReq,
  LoginResp,
  UserSelfData,
  SelfWorkspacesResp,
  UsersItemResp,
  EnvsResp,
  EditUserResp,
  UserSettings,
} from './type'

export const login = (data: LoginReq): Promise<LoginResp> => postForm<LoginResp>(`/login`, data)

/**
 * SSO Login (bypass interceptor, get full response)
 * Even on failure, user info (e.g. email/name) can be obtained from response.data
 */
export async function ssoLoginRaw(code: string, state?: string) {
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
  const params = new URLSearchParams()
  params.append('type', 'sso')
  params.append('code', code)
  if (state) params.append('state', String(state))

  const resp = await axios.post(`${baseURL}/login`, params, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    withCredentials: true,
    timeout: 10000,
    validateStatus: () => true, // No exceptions, return for any status code
  })

  return resp
}

export const logout = (): Promise<void> => request.post(`/logout`)

export const register = (data: RegisterReq): Promise<void> => request.post(`/users`, data)

export const editUser = (id: string, data: EditUserResp) => request.patch(`/users/${id}`, data)

export const deleteUser = (id: string) => request.delete(`/users/${id}`)

// The caller's own record. The error handler is skipped: this is also the probe that asks
// whether the browser already holds a session (see the user store), and "no" is an answer, not a
// failure to toast, and must not end a session or redirect to the sign-in page.
export const getSelfData = (): Promise<UserSelfData> =>
  request.get(`/users/self`, { skipErrorHandler: true })

// The caller's own workspace lists. The error handler is skipped: the console falls back to the
// lists embedded in the user record when this route is missing or refuses, so no toast and no
// session teardown for it.
export const getSelfWorkspaces = (): Promise<SelfWorkspacesResp> =>
  request.get(`/users/self/workspaces`, { skipErrorHandler: true })

export const getUserData = (id: string): Promise<UserSelfData> => request.get(`/users/${id}`)

export const getUserDataList = (): Promise<UsersItemResp> => request.get(`/users`)

// Get log-related user permissions. With `quiet` a failure is left to the caller: no toast.
export const getEnvs = (quiet = false): Promise<EnvsResp> =>
  request.get(`/envs`, quiet ? { skipErrorHandler: true } : undefined)

export const getUserSettings = (name = 'self'): Promise<UserSettings> =>
  request.get(`/users/${name}/settings`)

export const updateUserSettings = (data: Partial<UserSettings>, name = 'self'): Promise<void> =>
  request.put(`/users/${name}/settings`, data)

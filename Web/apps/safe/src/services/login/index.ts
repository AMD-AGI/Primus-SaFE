import axios from 'axios'
import request, { postForm } from '@/services/request'
import type {
  RegisterReq,
  LoginReq,
  LoginResp,
  UserSelfData,
  UsersItemResp,
  EnvsResp,
  EditUserResp,
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

export const getSelfData = (): Promise<UserSelfData> => request.get(`/users/self`)

export const getUserData = (id: string): Promise<UserSelfData> => request.get(`/users/${id}`)

export const getUserDataList = (): Promise<UsersItemResp> => request.get(`/users`)

// Get log-related user permissions
export const getEnvs = (): Promise<EnvsResp> => request.get(`/envs`)

/**
 * HyperLoom Auth Service
 *
 * Reuses SaFE backend SSO endpoints:
 *   GET  /api/envs   → { ssoEnable, ssoAuthUrl, ... }
 *   POST /api/login   → Set-Cookie (session token)
 *   POST /api/logout
 *   GET  /api/users/self → current user profile
 */

import axios from 'axios';

const BASE = '/api';

const request = axios.create({
  baseURL: BASE,
  timeout: 15000,
  withCredentials: true,
});

request.interceptors.response.use(
  (resp) => resp.data,
  (error) => {
    const status = error?.response?.status;
    if (status === 401 && !location.pathname.includes('/login')) {
      sessionStorage.removeItem('hl-user');
      location.href = '/hyperloom/login?redirect=' + encodeURIComponent(location.pathname + location.search);
    }
    return Promise.reject(error);
  },
);

export async function getEnvs() {
  return request.get('/envs');
}

export async function ssoLoginRaw(code, state) {
  const params = new URLSearchParams();
  params.append('type', 'sso');
  params.append('code', code);
  if (state) params.append('state', String(state));

  return axios.post(`${BASE}/login`, params, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    withCredentials: true,
    timeout: 15000,
    validateStatus: () => true,
  });
}

export async function getUserSelf() {
  return request.get('/users/self');
}

export async function logoutApi() {
  return request.post('/logout');
}

export { request as authRequest };

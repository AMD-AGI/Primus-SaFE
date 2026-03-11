import axios from 'axios'

// Create a dedicated auth API instance
const authRequest = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
  withCredentials: true, // Important for session cookies - automatically sends and receives cookies
  headers: {
    'X-Requested-With': 'XMLHttpRequest', // Identify as AJAX request
  }
})

// Add request interceptor to attach auth info from localStorage
authRequest.interceptors.request.use(config => {
  // Try to get auth info from localStorage
  const authInfo = localStorage.getItem('lens_auth')
  if (authInfo) {
    try {
      const { token, userId } = JSON.parse(authInfo)
      // Add custom headers (if backend supports them)
      if (token) {
        config.headers['X-Auth-Token'] = token
      }
      if (userId) {
        config.headers['X-User-Id'] = userId
        // Temp backdoor: directly add userId header
        config.headers['userId'] = userId
      }
    } catch (e) {
      console.error('Failed to parse auth info:', e)
    }
  }

  return config
})

// Add response interceptor to handle special cases
authRequest.interceptors.response.use(
  response => {
    // If login response, save auth info
    if (response.config.url?.includes('/login') && response.data) {
      // Try to extract auth info from response
      const authInfo = {
        token: response.data.token || response.data.id,
        userId: response.data.id || response.data.userId,
        userType: response.data.userType || 'sso'
      }

      // Save to localStorage
      localStorage.setItem('lens_auth', JSON.stringify(authInfo))
    }

    return response
  },
  error => {
    // Print detailed error info
    if (error.response) {
      console.error('Auth API Error:', {
        status: error.response.status,
        data: error.response.data,
        headers: error.response.headers
      })

      // If 400 error, try to parse error info
      if (error.response.status === 400 && error.response.data) {
        const errorData = error.response.data
        console.error('400 Error Details:', errorData)

        // If error contains specific error code
        if (errorData.errorCode === 'Primus.00002') {
          // Possible state format issue
          console.error('State format error:', errorData.errorMessage)
        }
      }
    }
    return Promise.reject(error)
  }
)

export interface LoginReq {
  type: string
  name?: string
  password?: string
  code?: string
  state?: string
}

export interface LoginResp {
  id: string
  name?: string
  token?: string
}

export interface UserData {
  id: string
  name: string
  email?: string
  roles?: string[]
}


// Unified login endpoint (supports both SSO and traditional login)
export const login = (data: LoginReq): Promise<LoginResp> => {
  const params = new URLSearchParams()
  Object.entries(data).forEach(([key, value]) => {
    if (value !== undefined && value !== null) {
      params.append(key, String(value))
    }
  })

  // Only use URL encoded form submission, no multipart retry
  // Previous multipart retry caused the same SSO code to be submitted twice (500 + 400), worsening loop issues
  return authRequest.post('/login', params, {
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
    }
  }).then(res => {
    return res.data
  })
}

// Get current user data
export const getUserData = (userId: string): Promise<UserData> => {
  return authRequest.get(`/users/${userId}`).then(res => res.data)
}

// Logout
export const logout = (): Promise<void> => {
  return authRequest.post('/logout').then(res => res.data)
}

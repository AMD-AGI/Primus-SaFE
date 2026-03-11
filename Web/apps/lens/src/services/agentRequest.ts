import axios from 'axios'

/**
 * Agent API Dedicated request instance
 * Unlike the main API, Agent API does not need snake_case conversion or special response format handling
 */
const agentRequest = axios.create({
  baseURL: `${import.meta.env.BASE_URL}v1`,  // Uses Vite base config to auto-adapt paths
  timeout: 30000,  // Agent responses may take longer
  headers: {
    'Content-Type': 'application/json',
  }
})

// Request interceptor
agentRequest.interceptors.request.use(
  config => {
    // Agent API No snake_case conversion needed
    return config
  },
  error => {
    return Promise.reject(error)
  }
)

// Response interceptor
agentRequest.interceptors.response.use(
  response => {
    const data = response.data
    // Handle unified API response format: { meta: {...}, data: {...}, tracing: null }
    if (data && typeof data === 'object' && 'meta' in data && 'data' in data) {
      // Check if request was successful (meta.code === 2000 means OK)
      if (data.meta?.code !== 2000) {
        return Promise.reject(new Error(data.meta?.message || 'Request failed'))
      }
      return data.data
    }
    // Direct response format
    return data
  },
  error => {
    const message = error.response?.data?.meta?.message || 
                    error.response?.data?.detail || 
                    error.message || 
                    'Network Error'
    return Promise.reject(new Error(message))
  }
)

export default agentRequest


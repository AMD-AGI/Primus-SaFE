<template>
  <div class="sso-bridge-page">
    <div class="bridge-card">
      <el-icon class="loading-icon" :size="48">
        <Loading />
      </el-icon>
      <h2>Processing Authentication...</h2>
      <p>Redirecting from SaFE system to Lens...</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Loading } from '@element-plus/icons-vue'

const route = useRoute()
const router = useRouter()

onMounted(async () => {
  // This page receives redirects from another system (SaFE)
  // SaFE system should pass code and state as query parameters
  const { code, state } = route.query
  
  if (code && state) {
    // Check if this code has already been processed (avoid duplicate processing)
    const processedKey = `sso_processed_${code}`
    if (sessionStorage.getItem(processedKey)) {
      console.warn('[SSOBridge] This SSO code has already been processed:', code)
      router.replace('/')  // Redirect to homepage, do not add /lens/
      return
    }
    
    // Important: redirect to root path /, let route guard handle it
    router.replace({
      path: '/',  // Use root path, do not add /lens/
      query: { 
        code: code as string, 
        state: state as string 
      }
    })
  } else {
    console.error('Missing code or state in SSOBridge')
    // If no code or state, redirect to login page
    router.replace('/login')
  }
})
</script>

<style scoped lang="scss">
.sso-bridge-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--el-bg-color-page);
}

.bridge-card {
  text-align: center;
  padding: 40px;
  background: var(--el-bg-color);
  border-radius: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  
  .loading-icon {
    color: var(--el-color-primary);
    animation: rotate 2s linear infinite;
    margin-bottom: 20px;
  }
  
  h2 {
    font-size: 20px;
    margin-bottom: 12px;
    color: var(--el-text-color-primary);
  }
  
  p {
    font-size: 14px;
    color: var(--el-text-color-secondary);
  }
}

@keyframes rotate {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>

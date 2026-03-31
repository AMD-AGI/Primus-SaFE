<template>
  <div class="hl-app" :data-theme="theme">
    <template v-if="isLoginPage">
      <router-view />
    </template>

    <template v-else>
      <nav class="hl-topnav">
        <div class="hl-topnav-accent"></div>
        <div class="hl-topnav-brand">
          <span class="brand-amd">AMD</span>
          <span class="brand-name">HyperLoom</span>
        </div>
        <div class="hl-topnav-actions">
          <span v-if="userName" class="hl-user-label">{{ userName }}</span>
          <button
            v-if="userName"
            class="hl-logout-btn"
            title="Sign out"
            @click="handleLogout"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
          </button>
          <button class="hl-theme-toggle" @click="toggleTheme" :title="theme === 'dark' ? 'Light mode' : 'Dark mode'">
            <span v-if="theme === 'dark'" class="theme-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
            </span>
            <span v-else class="theme-icon">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </span>
          </button>
        </div>
      </nav>

      <div class="hl-layout">
        <aside class="hl-sidebar">
          <router-link to="/overview" class="hl-sidebar-item" active-class="active">
            <el-icon><DataAnalysis /></el-icon>
            <span>Overview</span>
          </router-link>
          <router-link to="/analysis" class="hl-sidebar-item" active-class="active">
            <el-icon><Search /></el-icon>
            <span>Analysis</span>
          </router-link>
          <router-link to="/optimization" class="hl-sidebar-item" active-class="active">
            <el-icon><Setting /></el-icon>
            <span>Optimization</span>
          </router-link>
          <router-link to="/report" class="hl-sidebar-item" active-class="active">
            <el-icon><Document /></el-icon>
            <span>Report</span>
          </router-link>
          <div class="hl-sidebar-divider"></div>
          <router-link to="/claw" class="hl-sidebar-item" active-class="active">
            <el-icon><ChatDotSquare /></el-icon>
            <span>Claw Agent</span>
          </router-link>
        </aside>

        <main class="hl-content">
          <router-view />
        </main>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, provide } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { DataAnalysis, Search, Setting, Document, ChatDotSquare } from '@element-plus/icons-vue'

const route = useRoute()
const router = useRouter()

const theme = ref<'light' | 'dark'>(
  (localStorage.getItem('hl-theme') as 'light' | 'dark') || 'light'
)

const isLoginPage = computed(() => route.path === '/login')

const userName = computed(() => {
  try {
    const u = JSON.parse(localStorage.getItem('hl-user') || '{}')
    return u?.name || u?.email || ''
  } catch {
    return ''
  }
})

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  localStorage.setItem('hl-theme', theme.value)
  document.documentElement.setAttribute('data-theme', theme.value)
}

function handleLogout() {
  localStorage.removeItem('hl-user')
  sessionStorage.removeItem('hl-sso.redirect')
  sessionStorage.removeItem('hl-oauth_state')
  router.push('/login')
}

onMounted(() => {
  document.documentElement.setAttribute('data-theme', theme.value)
})

provide('theme', theme)
</script>

import { createRouter, createWebHistory } from 'vue-router'
import setupAuthGuard from './authGuard.js'

const router = createRouter({
  history: createWebHistory('/hyperloom/'),
  routes: [
    { path: '/', redirect: '/overview' },
    {
      path: '/login',
      name: 'login',
      component: () => import('@/pages/LoginPage.vue'),
      meta: { public: true },
    },
    {
      path: '/overview',
      name: 'overview',
      component: () => import('@/pages/Dashboard.vue'),
    },
    {
      path: '/analysis',
      name: 'analysis',
      component: () => import('@/pages/Dashboard.vue'),
    },
    {
      path: '/optimization',
      name: 'optimization',
      component: () => import('@/pages/Dashboard.vue'),
    },
    {
      path: '/report',
      name: 'report',
      component: () => import('@/pages/Dashboard.vue'),
    },
    {
      path: '/claw',
      name: 'claw',
      component: () => import('@/pages/ClawPage.vue'),
    },
  ],
})

setupAuthGuard(router)

export default router

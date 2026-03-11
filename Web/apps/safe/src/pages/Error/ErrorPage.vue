<template>
  <div class="flex justify-center min-h-[100svh]">
    <el-result icon="error" title="Error" sub-title="cluster is not ready">
      <template #extra>
        <el-button type="primary" @click="refreshPage">Refresh</el-button>
        <el-button type="danger" @click="switchAccount">Switch Account</el-button>
      </template>
    </el-result>
  </div>
</template>

<script lang="ts" setup>
import { useClusterStore } from '@/stores/cluster'
import { useUserStore } from '@/stores/user'
import { useRouter } from 'vue-router'

const store = useClusterStore()
const userStore = useUserStore()
const router = useRouter()
const refreshPage = async () => {
  try {
    await store.fetchClusters()
    // On successful refresh, navigate to homepage
    await router.replace('/')
  } catch (_error) {
    // If refresh fails, reload the page
    window.location.reload()
  }
}
const switchAccount = async () => {
  await userStore.logout()
  const clusterStore = useClusterStore()
  clusterStore.$reset?.()
  await router.replace('/login')
}
</script>

<style></style>

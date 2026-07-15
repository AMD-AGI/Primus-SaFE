<template>
  <div class="flex justify-center min-h-[100svh]">
    <el-result
      icon="warning"
      title="403"
      sub-title="You don't have permission to access this page"
    >
      <template #extra>
        <el-button type="primary" @click="goHome">Back to Home</el-button>
      </template>
    </el-result>
  </div>
</template>

<script lang="ts" setup>
import { useRouter } from 'vue-router'

const router = useRouter()

const goHome = () => {
  router.replace('/')
}
</script>

<style></style>

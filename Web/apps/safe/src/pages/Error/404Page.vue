<template>
  <div class="flex justify-center h-[100vh]">
    <el-result icon="error" title="404 Error" sub-title="the page is gone...">
      <template #extra>
        <el-button type="primary" @click="refreshPage">Refresh</el-button>
      </template>
    </el-result>
  </div>
</template>

<script lang="ts" setup>
const refreshPage = () => {
  window.location.reload()
}
</script>

<style></style>

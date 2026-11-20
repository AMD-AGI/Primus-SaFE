<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">QoS</el-text>
    <el-text class="block mt-2" type="info">Workspace ceiling. An empty field is not limited.</el-text>
  </div>
  <el-card class="mt-6 safe-card" shadow="never" v-loading="loading">
    <el-descriptions v-if="item" :column="1" border>
      <el-descriptions-item label="Workspace">{{ item.name }}</el-descriptions-item>
      <el-descriptions-item label="CPU">{{ item.maxResources?.cpu || '-' }}</el-descriptions-item>
      <el-descriptions-item label="Memory">{{ item.maxResources?.memory || '-' }}</el-descriptions-item>
      <el-descriptions-item label="GPU">{{ item.maxResources?.gpu || '-' }}</el-descriptions-item>
      <el-descriptions-item label="Timeout">{{ formatRuntime(item.maxRuntime) || '-' }}</el-descriptions-item>
    </el-descriptions>
    <div class="mt-4">
      <el-button type="primary" @click="router.push({ path: '/qos/edit', query: { name: item?.name } })">
        Edit
      </el-button>
      <el-button type="danger" @click="onDelete">Delete</el-button>
      <el-button @click="router.push('/qos')">Back</el-button>
    </div>
  </el-card>
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteQos, getQos } from '@/services/qos'
import { formatRuntime, type QosItem } from '@/services/qos/type'

const route = useRoute()
const router = useRouter()
const loading = ref(false)
const item = ref<QosItem>()

/** load reads one workspace ceiling. */
const load = async () => {
  const name = String(route.query.name ?? '')
  if (!name) {
    router.replace('/qos')
    return
  }
  loading.value = true
  try {
    item.value = await getQos(name)
  } finally {
    loading.value = false
  }
}

/** onDelete removes this ceiling and returns to the list. */
const onDelete = () => {
  if (!item.value?.name) return
  const name = item.value.name
  ElMessageBox.confirm(`Delete QoS for ${name}? This workspace will have no limit.`, 'Delete QoS', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await deleteQos(name)
    ElMessage.success('Deleted')
    router.push('/qos')
  })
}

onMounted(load)
</script>

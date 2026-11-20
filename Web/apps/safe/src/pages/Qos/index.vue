<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">QoS</el-text>
    <el-button type="primary" round :icon="Plus" class="m-t-4 text-black" @click="router.push('/qos/create')">
      Create QoS
    </el-button>
    <div class="m-t-3 qos-hint">
      Each row sets the CPU, memory, GPU and timeout ceilings for one workspace. An empty value means that field is
      not limited. The example is not applied. A user's own ceiling in a workspace takes precedence for each field it
      sets; set it in
      <el-link type="primary" class="qos-hint-link" v-route="{ path: '/usermanage' }">User Management</el-link>.
    </div>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <div class="qos-example">
      <span class="qos-example-tag">Example</span>
      <span>workspace-a</span>
      <span>CPU 256</span>
      <span>Memory 2048Gi</span>
      <span>GPU 64</span>
      <span>{{ formatRuntime(exampleRuntime) }}</span>
    </div>
    <el-table
      :data="items"
      size="large"
      class="m-t-2"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column prop="name" label="Workspace" min-width="160">
        <template #default="{ row }">
          <el-link type="primary" v-route="{ path: '/qos/detail', query: { name: row.name } }">
            {{ row.name }}
          </el-link>
        </template>
      </el-table-column>
      <el-table-column label="CPU" min-width="100">
        <template #default="{ row }">{{ row.maxResources?.cpu || '-' }}</template>
      </el-table-column>
      <el-table-column label="Memory" min-width="120">
        <template #default="{ row }">{{ row.maxResources?.memory || '-' }}</template>
      </el-table-column>
      <el-table-column label="GPU" min-width="100">
        <template #default="{ row }">{{ row.maxResources?.gpu || '-' }}</template>
      </el-table-column>
      <el-table-column label="Timeout" min-width="240">
        <template #default="{ row }">{{ formatRuntime(row.maxRuntime) || '-' }}</template>
      </el-table-column>
      <el-table-column label="Actions" width="220" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="router.push({ path: '/qos/edit', query: { name: row.name } })">
            Edit
          </el-button>
          <el-button link type="primary" @click="router.push({ path: '/qos/create', query: { from: row.name } })">
            Clone
          </el-button>
          <el-button link type="danger" @click="onDelete(row.name)">Delete</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { deleteQos, listQos } from '@/services/qos'
import { formatRuntime, type QosItem } from '@/services/qos/type'

/** exampleRuntime illustrates timeouts. It is display-only and never sent to the server. */
const exampleRuntime = { Train: 168 * 3600, Infer: 720 * 3600 }

const router = useRouter()
const loading = ref(false)
const items = ref<QosItem[]>([])

/** load reads every workspace ceiling. */
const load = async () => {
  loading.value = true
  try {
    const res = await listQos()
    items.value = res.items ?? []
  } finally {
    loading.value = false
  }
}

/** onDelete removes one workspace ceiling after confirmation. */
const onDelete = (name: string) => {
  ElMessageBox.confirm(`Delete QoS for ${name}? This workspace will have no limit.`, 'Delete QoS', {
    confirmButtonText: 'Delete',
    cancelButtonText: 'Cancel',
    type: 'warning',
  }).then(async () => {
    await deleteQos(name)
    ElMessage.success('Deleted')
    await load()
  })
}

onMounted(load)
</script>

<style scoped>
.qos-hint {
  color: var(--safe-muted);
  font-size: 13px;
  line-height: 1.6;
}
.qos-hint-link {
  font-size: 13px;
  vertical-align: baseline;
}
.qos-example {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  align-items: center;
  margin-top: 8px;
  color: var(--safe-muted);
  font-size: 13px;
}
.qos-example-tag {
  font-weight: 500;
}
</style>

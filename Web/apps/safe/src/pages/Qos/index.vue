<template>
  <div>
    <el-text class="block textx-18 font-500" tag="b">QoS</el-text>
    <el-button type="primary" round :icon="Plus" class="m-t-4 text-black" @click="router.push('/qos/create')">
      Create QoS
    </el-button>
    <div class="m-t-3 qos-hint">
      Each row sets the CPU, memory, GPU and timeout ceilings for one cluster. An empty value means no limit.
      The gray row is an example only and is not applied. A user's own ceiling on a cluster takes precedence;
      set it in
      <el-link type="primary" class="qos-hint-link" v-route="{ path: '/usermanage' }">User Management</el-link>.
    </div>
  </div>
  <el-card class="mt-6 safe-card" shadow="never">
    <el-table
      :data="rows"
      size="large"
      class="m-t-2"
      :row-class-name="rowClass"
      v-loading="loading"
      :element-loading-text="$loadingText"
    >
      <el-table-column prop="name" label="Cluster" min-width="160">
        <template #default="{ row }">
          <span v-if="row.example">{{ row.name }} (example)</span>
          <el-link v-else type="primary" v-route="{ path: '/qos/detail', query: { name: row.name } }">
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
      <el-table-column label="Actions" width="160" fixed="right">
        <template #default="{ row }">
          <span v-if="row.example">-</span>
          <template v-else>
            <el-button link type="primary" @click="router.push({ path: '/qos/edit', query: { name: row.name } })">
              Edit
            </el-button>
            <el-button link type="danger" @click="onDelete(row.name)">Delete</el-button>
          </template>
        </template>
      </el-table-column>
    </el-table>
  </el-card>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { deleteQos, listQos } from '@/services/qos'
import { formatRuntime, type QosItem } from '@/services/qos/type'

type QosRow = QosItem & { example?: boolean }

/** exampleRow illustrates the fields. It is display-only and never sent to the server. */
const exampleRow: QosRow = {
  name: 'cluster-a',
  maxResources: { cpu: '256', memory: '2048Gi', gpu: '64' },
  maxRuntime: { Train: 168 * 3600, Infer: 720 * 3600 },
  example: true,
}

const router = useRouter()
const loading = ref(false)
const items = ref<QosItem[]>([])
const rows = computed<QosRow[]>(() => [exampleRow, ...items.value])

/** rowClass grays out the example row. */
const rowClass = ({ row }: { row: QosRow }) => (row.example ? 'qos-example-row' : '')

/** load reads every cluster ceiling. */
const load = async () => {
  loading.value = true
  try {
    const res = await listQos()
    items.value = (res.items ?? []).filter((item) => item.name !== 'default')
  } finally {
    loading.value = false
  }
}

/** onDelete removes one cluster ceiling after confirmation. */
const onDelete = (name: string) => {
  ElMessageBox.confirm(`Delete QoS for ${name}? This cluster will have no limit.`, 'Delete QoS', {
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
  color: var(--el-text-color-placeholder);
  font-size: 13px;
  line-height: 1.6;
}
.qos-hint-link {
  font-size: 13px;
  vertical-align: baseline;
}
:deep(.qos-example-row) {
  color: var(--el-text-color-placeholder);
}
</style>

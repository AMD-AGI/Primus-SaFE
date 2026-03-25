<template>
  <el-card class="mt-4 safe-card" shadow="never">
    <el-table
      :height="'calc(100vh - 235px)'"
      :data="data"
      size="large"
      v-loading="loading"
    >
      <el-table-column prop="id" label="ID" width="80" fixed="left" />
      <el-table-column prop="traceId" label="Trace ID" min-width="200" show-overflow-tooltip fixed="left" />
      <el-table-column prop="callerServiceName" label="Caller" min-width="120" show-overflow-tooltip />
      <el-table-column prop="callerUserName" label="Caller User" min-width="160" show-overflow-tooltip />
      <el-table-column prop="targetServiceName" label="Target" min-width="120" show-overflow-tooltip />
      <el-table-column label="Status" width="110">
        <template #default="{ row }">
          <el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">
            {{ row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Latency" width="110" align="right">
        <template #default="{ row }">{{ row.latencyMs }}ms</template>
      </el-table-column>
      <el-table-column label="Req Size" width="110" align="right">
        <template #default="{ row }">{{ formatBytes(row.requestSizeBytes) }}</template>
      </el-table-column>
      <el-table-column label="Resp Size" width="110" align="right">
        <template #default="{ row }">{{ formatBytes(row.responseSizeBytes) }}</template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="mt-4"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="$emit('pageChange', $event)"
      @size-change="$emit('sizeChange', $event)"
      layout="total, sizes, prev, pager, next"
      :page-sizes="[10, 20, 50, 100]"
    />
  </el-card>
</template>

<script lang="ts" setup>
import { formatBytes } from '@/utils'
import type { A2ACallLog } from '@/services'

defineProps<{
  data: A2ACallLog[]
  loading: boolean
  total: number
  page: number
  pageSize: number
}>()

defineEmits<{
  pageChange: [page: number]
  sizeChange: [size: number]
}>()
</script>

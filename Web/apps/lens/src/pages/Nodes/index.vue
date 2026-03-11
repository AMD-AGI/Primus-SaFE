<template>
  <el-row :gutter="20" class="py-2" style="width: 100%;">
    <el-col :span="6">
      <el-input v-model="filters.name" placeholder="name" size="default" clearable />
    </el-col>
    <el-col :span="6">
      <el-input v-model="filters.gpuName" placeholder="gpuName" size="default" clearable />
    </el-col>
    <el-col :span="6">
      <el-select
        v-model="filters.status"
        multiple
        collapse-tags
        collapse-tags-tooltip
        placeholder="status"
        size="default"
        clearable
      >
        <el-option
          v-for="item in statusOptions"
          :key="item.value"
          :label="item.label"
          :value="item.value"
        />
      </el-select>
    </el-col>
    <el-col :span="6">
      <el-button @click="onSearch" :icon="Search" size="default">Search</el-button>
      <el-button @click="resetFilters" :icon="Refresh" size="default">Reset</el-button>
    </el-col>
  </el-row>

  <el-table
    :height="'calc(100vh - 195px)'"
    :data="tableData"
    size="large"
    style="width: 100%;"
    v-loading="loading"
    @sort-change="handleSortChange"
  >
      <el-table-column prop="name" label="Name" minWidth="200" :fixed="true" sortable="custom" >
        <template #default="{ row }">
          <el-button
            link
            type="primary"
            size="default"
            @click="jumpToDetail(row.name)"
          >
            {{ row.name }}
          </el-button>
        </template>
      </el-table-column>
      <el-table-column prop="ip" label="Ip" width="180" />
      <el-table-column label="Utilization" minWidth="220" prop="gpuUtilization" sortable="custom">
        <template #default="{ row }">
          <el-progress :percentage="Number(row.gpuUtilization?.toFixed(2))" />
        </template>
      </el-table-column>
      <el-table-column prop="gpuAllocation" label="GPU Allocation" width="160" sortable="custom" />
      <el-table-column prop="gpuCount" label="GPU Count" width="140" sortable="custom" />
      <el-table-column prop="gpuName" label="GPU Name" minWidth="260" />
      <el-table-column label="Status" fixed="right" width="100">
          <template #default="{ row }">
              <el-tag :type="NODE_STATUS_TAG[row.status || 'Unknown']">{{ row.status }}</el-tag>
          </template>
      </el-table-column>
  </el-table>
  <el-pagination
      v-model:current-page="pagination.pageNum"
      v-model:page-size="pagination.pageSize"
      :total="pagination.total"
      @current-change="fetchData"
      @size-change="fetchData"
      class="p2.5"
  />
</template>

<script setup lang="ts">
import {ref} from 'vue'
import {usePaginatedTable} from '@/pages/useTable'
import {
  getNodesList,
} from '@/services/dashboard/index'
import {NODE_STATUS_TAG} from '@/constants'
import { Search, Refresh } from '@element-plus/icons-vue'
import { useRouter } from 'vue-router'
import { useClusterSync } from '@/composables/useClusterSync'

const router = useRouter()
const { selectedCluster } = useClusterSync()

const orderBy = ref('')
const desc = ref(false)

const jumpToDetail = (name: string) => {
  const query = selectedCluster.value ? `?cluster=${encodeURIComponent(selectedCluster.value)}` : ''
  router.push(`/nodedetail/${encodeURIComponent(name)}${query}`)
}

const statusOptions = Object.keys(NODE_STATUS_TAG).map(key => ({
  label: key,
  value: key
}))

const {
  tableData,
  loading,
  pagination,
  filters,
  fetchData,
  resetFilters
} = usePaginatedTable(
  getNodesList,
  undefined,
  {
    status: [],
    name: '',
    gpuName: ''
  },
  filters => ({
    ...filters,
    status: filters.status?.join(',') || '',
    order_by: orderBy.value,
    desc: desc.value
  })
)

const handleSortChange = ({ prop, order }: { prop: string; order: string }) => {
  orderBy.value = prop?.replace(/[A-Z]/g, letter => `_${letter.toLowerCase()}`)
  desc.value = order === 'descending'
  fetchData()
}

const onSearch = () => {
  pagination.pageNum = 1
  fetchData()
}

</script>

<style>

</style>
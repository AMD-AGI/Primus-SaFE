<template>
  <el-row :gutter="20" class="py-2" style="width: 100%;">
    <el-col :span="6">
      <el-input v-model="filters.name" placeholder="name" size="default" clearable />
    </el-col>
    <el-col :span="6">
      <el-select
        v-model="filters.namespace"
        placeholder="namespace"
        size="default"
        clearable
      >
        <el-option
          v-for="item in namespaceOptions"
          :key="item"
          :label="item"
          :value="item"
        />
      </el-select>
    </el-col>
    <el-col :span="6">
      <el-select
        v-model="filters.kind"
        placeholder="kind"
        size="default"
        clearable
      >
        <el-option
          v-for="item in kindsOptions"
          :key="item"
          :label="item"
          :value="item"
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
      <el-table-column prop="name" label="Name" width="200" :fixed="true">
        <template #default="{ row }">
          <el-tooltip :content="row.name" placement="top">
            <el-button
              link
              type="primary"
              size="default"
              @click="jumpToDetail(row.uid)"
              class="max-w-180px"
            >
              {{ row.name }}
            </el-button>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column prop="kind" label="Kind" width="110" :fixed="true" />
      <el-table-column prop="namespace" label="Namespace" width="180" />
      <el-table-column prop="gpuAllocated" label="GPU Allocated" width="160" />
      <el-table-column prop="gpuAllocation" label="GPU Allocation" minWidth="240">
        <template #default="{ row }">
          <div v-if="Object.keys(row.gpuAllocation).length">
            <div v-for="(val, key) in row.gpuAllocation" :key="key">
              Model: {{ key }} Card Hours: {{ val }}
            </div>
          </div>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="startAt" label="Start At" width="160" sortable="custom">
          <template #default="{ row }">
              {{ dayjs(row.startAt * 1000).format('YYYY-MM-DD HH:mm:ss') }}
          </template>
      </el-table-column>
      <el-table-column prop="endAt" label="End At" width="160" sortable="custom">
          <template #default="{ row }">
              {{ dayjs(row.endAt * 1000).format('YYYY-MM-DD HH:mm:ss') }}
          </template>
      </el-table-column>
      <el-table-column label="status" fixed="right" width="100" prop="status" sortable="custom">
          <template #default="{ row }">
              <el-tag :type="WORKLOAD_STATUS_TAG[row.status]">{{ row.status }}</el-tag>
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
  import dayjs from 'dayjs'
  import {usePaginatedTable} from '@/pages/useTable'
  import {
    getWorkloadsList,
    getWorkloadMeta,
  } from '@/services/dashboard/index'
  import {WORKLOAD_STATUS_TAG} from '@/constants'
  import { Search, Refresh } from '@element-plus/icons-vue'
  import { useRouter } from 'vue-router'
  import { onMounted, ref } from 'vue'
  import { useClusterSync } from '@/composables/useClusterSync'
  
  const router = useRouter()
  const { selectedCluster } = useClusterSync()

  const orderBy = ref('')
  const desc = ref(false)

  const namespaceOptions = ref([] as string[])
  const kindsOptions = ref([] as string[])
  
  const jumpToDetail = (uid: string) => {
    const query = selectedCluster.value ? `?cluster=${encodeURIComponent(selectedCluster.value)}` : ''
    router.push(`/workload/${encodeURIComponent(uid)}/detail${query}`)
  }
  
  const {
    tableData,
    loading,
    pagination,
    filters,
    fetchData,
    resetFilters
  } = usePaginatedTable(
    getWorkloadsList,
    undefined,
    {
      name: '',
      namespace: '',
      kind: '',
    },
    filters => ({
    ...filters,
    order_by: orderBy.value,
    desc: desc.value
  })
  )
  const onSearch = () => {
    pagination.pageNum = 1
    fetchData()
  }

  const getOptions = async () => {
    const res = await getWorkloadMeta()
    namespaceOptions.value = res?.namespaces || []
    kindsOptions.value = res?.kinds || []
  }

  const handleSortChange = ({ prop, order }: { prop: string; order: string }) => {
    orderBy.value = prop?.replace(/[A-Z]/g, letter => `_${letter.toLowerCase()}`)
    desc.value = order === 'descending'
    fetchData()
  }

  onMounted(() => {
    getOptions()
  })
  
</script>
  
<style scoped>
  ::v-deep .el-button span {
    display: inline-block;
    max-width: 150px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    height: 15px;
  }
</style>
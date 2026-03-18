<template>
  <div class="workload-stats">
    <!-- Header Section -->
    <div class="filter-section">
      <div class="filter-header">
        <h2 class="page-title">Github Workflows</h2>
        <div class="filters">
          <el-form :inline="true">
            <el-form-item>
              <el-input
                v-model="searchText"
                placeholder="Search by repository..."
                clearable
                size="default"
                style="width: 300px"
                :prefix-icon="Search"
              />
            </el-form-item>
          </el-form>
        </div>
      </div>
    </div>

    <!-- Stats Cards -->
    <div class="stats-cards">
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--primary">
            <i><el-icon><Link /></el-icon></i>
          </div>
          <div class="stat-info">
            <div class="stat-label">Repositories</div>
            <div class="stat-value stat-value--primary">{{ stats.totalRepositories }}</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--success">
            <i><el-icon><Check /></el-icon></i>
          </div>
          <div class="stat-info">
            <div class="stat-label">Active Runner Sets</div>
            <div class="stat-value stat-value--success">{{ stats.totalRunnerSets }}</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--warning">
            <i><el-icon><DataLine /></el-icon></i>
          </div>
          <div class="stat-info">
            <div class="stat-label">Total Runs</div>
            <div class="stat-value stat-value--warning">{{ formatNumber(stats.totalRuns) }}</div>
          </div>
        </div>
      </el-card>
      
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--info">
            <i><el-icon><VideoPlay /></el-icon></i>
          </div>
          <div class="stat-info">
            <div class="stat-label">Running Workflows</div>
            <div class="stat-value stat-value--info">{{ stats.runningWorkflows }}</div>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Main Table -->
    <el-card class="table-card">
      <div class="table-wrapper">
        <el-table
          v-loading="loading"
          :data="filteredData"
          style="width: 100%"
        >
        <el-table-column label="Repository" min-width="280">
          <template #default="{ row }">
            <div class="repo-cell">
              <el-link 
                type="primary" 
                :underline="false" 
                @click="goToRepoDetail(row)"
                class="workload-link"
              >
                <el-icon class="repo-icon"><Link /></el-icon>
                {{ row.owner }}/{{ row.repo }}
              </el-link>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="Runner Sets" width="140" align="center">
          <template #default="{ row }">
            <div class="runner-sets-cell">
              <span class="count">{{ row.runnerSetCount }}</span>
              <span v-if="row.configuredSets > 0" class="configured">
                ({{ row.configuredSets }} configured)
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="Runners" width="140" align="center">
          <template #default="{ row }">
            <div class="runners-cell">
              <span class="current">{{ row.totalRunners }}</span>
              <span class="separator">/</span>
              <span class="max">{{ row.maxRunners }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="Workflow Runs" width="200" align="center">
          <template #default="{ row }">
            <div class="runs-stats-cell">
              <el-tooltip content="Total Runs" placement="top">
                <span class="run-stat total">{{ row.totalRuns || 0 }}</span>
              </el-tooltip>
              <span class="separator">/</span>
              <el-tooltip content="Completed" placement="top">
                <span class="run-stat completed">{{ row.completedRuns || 0 }}</span>
              </el-tooltip>
              <span class="separator">/</span>
              <el-tooltip content="Failed" placement="top">
                <span class="run-stat failed">{{ row.failedRuns || 0 }}</span>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="Status" width="160" align="center">
          <template #default="{ row }">
            <el-tag
              v-if="row.runningWorkflows > 0"
              type="warning"
              effect="dark"
              class="running-tag"
            >
              <span class="running-dot"></span>
              {{ row.runningWorkflows }} Running
            </el-tag>
            <el-tag v-else type="success" effect="light">
              Idle
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="Last Activity" width="160">
          <template #default="{ row }">
            <span v-if="row.lastRunAt" class="time-text">
              {{ formatTime(row.lastRunAt) }}
            </span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>

      </el-table>
      
      <el-pagination
        v-if="pagination.total > 0"
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :total="pagination.total"
        :page-sizes="[10, 20, 50, 100]"
        layout="total, sizes, prev, pager, next, jumper"
        @current-change="handlePageChange"
        @size-change="handlePageChange"
        class="mt-4"
      />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  Check, DataLine, Search, Link, VideoPlay
} from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import {
  getRepositories,
  type RepositorySummary
} from '@/services/workflow-metrics'
import { useClusterSync } from '@/composables/useClusterSync'

dayjs.extend(relativeTime)

const router = useRouter()
const { selectedCluster } = useClusterSync()

// State
const loading = ref(false)
const repositories = ref<RepositorySummary[]>([])
const searchText = ref('')

const stats = reactive({
  totalRepositories: 0,
  totalRunnerSets: 0,
  totalRuns: 0,
  runningWorkflows: 0
})

// Pagination
const pagination = ref({
  page: 1,
  pageSize: 20,
  total: 0
})

// Computed
const filteredData = computed(() => {
  let data = repositories.value
  
  // Apply search filter
  if (searchText.value) {
    const search = searchText.value.toLowerCase()
    data = data.filter(repo =>
      repo.owner.toLowerCase().includes(search) ||
      repo.repo.toLowerCase().includes(search)
    )
  }
  
  // Update pagination total
  pagination.value.total = data.length
  
  // Apply pagination
  const start = (pagination.value.page - 1) * pagination.value.pageSize
  const end = start + pagination.value.pageSize
  return data.slice(start, end)
})

// Methods
const fetchData = async () => {
  loading.value = true
  try {
    const res = await getRepositories()
    const repoList = res.repositories || []
    repositories.value = repoList

    // Update stats from repositories data
    stats.totalRepositories = repoList.length
    stats.totalRunnerSets = repoList.reduce((sum, r) => sum + r.runnerSetCount, 0)
    stats.totalRuns = repoList.reduce((sum, r) => sum + (r.totalRuns || 0), 0)
    stats.runningWorkflows = repoList.reduce((sum, r) => sum + (r.runningWorkflows || 0), 0)
  } catch (error) {
    console.error('Failed to fetch data:', error)
    ElMessage.error('Failed to load repositories')
  } finally {
    loading.value = false
  }
}

const handlePageChange = () => {
  // Page change will be handled automatically by computed property
}

const goToRepoDetail = (row: RepositorySummary) => {
  const cluster = selectedCluster.value
  router.push({
    path: `/github-workflow/repos/${row.owner}/${row.repo}`,
    query: cluster ? { cluster } : undefined
  })
}

const formatTime = (time: string) => {
  return dayjs(time).fromNow()
}

const formatNumber = (num: number) => {
  if (num >= 1000000) return (num / 1000000).toFixed(1) + 'M'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return num.toString()
}

onMounted(() => {
  fetchData()
})

// Watch for cluster changes and reload data
watch(selectedCluster, (newCluster, oldCluster) => {
  if (newCluster && newCluster !== oldCluster) {
    fetchData()
  }
})
</script>

<style scoped lang="scss">
@import '@/styles/stats-layout.scss';

.workload-stats {
  .table-card {
    .repo-cell {
      display: flex;
      align-items: center;
      gap: 8px;

      .repo-icon {
        color: var(--el-text-color-secondary);
      }
      
      .workload-link {
        font-weight: 500;
        font-size: 14px;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
    }

    .runner-sets-cell {
      .count {
        font-weight: 600;
        color: var(--el-text-color-primary);
      }
      .configured {
        font-size: 12px;
        color: var(--el-text-color-secondary);
        margin-left: 4px;
      }
    }

    .runners-cell {
      font-family: monospace;
      .current {
        color: var(--el-color-success);
        font-weight: 600;
      }
      .separator {
        color: var(--el-text-color-secondary);
        margin: 0 2px;
      }
      .max {
        color: var(--el-text-color-secondary);
      }
    }

    .runs-stats-cell {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 2px;
      font-family: monospace;
      font-size: 13px;

      @media (min-width: 1920px) {
        font-size: 14px;
      }

      .run-stat {
        font-weight: 500;
        min-width: 24px;
        text-align: center;

        &.total {
          color: var(--el-text-color-primary);
        }
        &.completed {
          color: var(--el-color-success);
        }
        &.failed {
          color: var(--el-color-danger);
        }
      }

      .separator {
        color: var(--el-text-color-placeholder);
      }
    }

    .running-tag {
      .running-dot {
        display: inline-block;
        width: 6px;
        height: 6px;
        background: currentColor;
        border-radius: 50%;
        margin-right: 4px;
        animation: pulse 1.5s ease-in-out infinite;
      }
    }
  }
}

@keyframes pulse {
  0%, 100% {
    opacity: 1;
  }
  50% {
    opacity: 0.4;
  }
}
</style>

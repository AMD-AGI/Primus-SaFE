<template>
  <div class="cluster-stats">
    <div class="filter-section">
      <div class="filter-header">
        <h2 class="page-title">Cluster GPU Statistics</h2>
        <div class="filters">
          <el-form :inline="true" :model="filters">
          <el-form-item>
            <el-date-picker
              v-model="timeRange"
              type="datetimerange"
              range-separator="to"
              start-placeholder="Start Time"
              end-placeholder="End Time"
              format="YYYY-MM-DD HH:mm:ss"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              size="default"
              class="time-picker"
              popper-class="custom-date-picker"
            />
          </el-form-item>
          </el-form>
        </div>
      </div>
    </div>

    <!-- Statistics Cards -->
    <div class="stats-cards" v-if="statsData.length > 0">
      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--primary">
            <i i="ep-cpu" />
          </div>
          <div class="stat-info">
            <div class="stat-label">
              Avg Allocation Rate
              <el-tooltip
                content="Average percentage of cluster GPU resources allocated to users"
                placement="top"
              >
                <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="stat-value stat-value--primary">{{ avgAllocationRate.toFixed(2) }}%</div>
          </div>
        </div>
      </el-card>

      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--info">
            <i i="ep-data-line" />
          </div>
          <div class="stat-info">
            <div class="stat-label">
              Peak Allocation Rate
              <el-tooltip
                content="Maximum GPU allocation rate reached during the selected period"
                placement="top"
              >
                <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="stat-value stat-value--info">{{ peakAllocationRate.toFixed(2) }}%</div>
          </div>
        </div>
      </el-card>

      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--success">
            <i i="ep-odometer" />
          </div>
          <div class="stat-info">
            <div class="stat-label">
              Avg Utilization
              <el-tooltip
                content="Average GPU utilization rate of allocated resources"
                placement="top"
              >
                <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="stat-value stat-value--success">{{ avgUtilization.toFixed(2) }}%</div>
          </div>
        </div>
      </el-card>

      <el-card class="stat-card">
        <div class="stat-content">
          <div class="stat-icon stat-icon--warning">
            <i i="ep-aim" />
          </div>
          <div class="stat-info">
            <div class="stat-label">
              Peak Utilization
              <el-tooltip
                content="Maximum GPU utilization rate reached during the selected period"
                placement="top"
              >
                <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </div>
            <div class="stat-value stat-value--warning">{{ maxUtilization.toFixed(2) }}%</div>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Nodes Distribution Dashboard -->
    <div class="nodes-dashboard" v-if="clusterOverviewData || workspaceNodesData.length > 0">
      <h2 class="page-title">Cluster & Workspace Nodes Distribution</h2>

      <div class="unified-grid">
        <!-- Cluster Overview - Takes left 2/5 -->
        <el-card class="cluster-section" v-if="clusterOverviewData" v-loading="overviewLoading" shadow="hover">
          <template #header>
            <div class="section-header">
              <span class="section-title">Cluster Overview</span>
              <span class="node-count">{{ clusterOverviewData.totalNodes }} Total Nodes</span>
            </div>
          </template>
          <div class="overview-chart" ref="overviewChartRef"></div>
        </el-card>

        <!-- Workspaces Container - Takes right 3/5 -->
        <div class="workspaces-container">
          <el-card
            v-for="(workspace, index) in workspaceNodesData"
            :key="workspace.workspaceId"
            class="workspace-card"
            shadow="hover"
          >
            <div class="workspace-content">
              <div class="workspace-name">{{ workspace.workspaceName }}</div>
              <div class="workspace-chart" :ref="el => setWorkspaceChartRef(el, index)"></div>
              <div class="node-info">
                {{ workspace.currentNodeCount - (workspace.abnormalNodeCount || 0) }} /
                {{ workspace.targetNodeCount || 0 }}
                <span class="node-label">(Ready / Target)</span>
              </div>
            </div>
          </el-card>
        </div>
      </div>
    </div>

    <!-- Trend Chart -->
    <el-card class="chart-card mt-6" v-if="statsData.length > 0">
      <template #header>
        <div class="card-header">
          <span>Utilization & Allocation Trends</span>
        </div>
      </template>

      <LineChart
        :labels="chartLabels"
        :series="chartSeries"
        unit="%"
        height="400px"
        :colors="['#409eff', '#67c23a']"
      />
    </el-card>

    <!-- Data Table -->
    <el-card class="table-card">
      <div class="table-wrapper">
      <el-table
        v-loading="loading"
        :data="statsData"
        stripe
        style="width: 100%"
        @sort-change="handleTableSortChange"
      >
        <el-table-column prop="statHour" label="Stat Time" width="200" sortable="custom">
          <template #default="{ row }">
            {{ formatTime(row.statHour) }}
          </template>
        </el-table-column>

        <el-table-column prop="totalGpuCapacity" label="Total Capacity" width="150" sortable="custom" />

        <el-table-column prop="allocatedGpuCount" label="Allocated" width="140" sortable="custom">
          <template #default="{ row }">
            {{ row.allocatedGpuCount?.toFixed(2) ?? '0.00' }}
          </template>
        </el-table-column>

        <el-table-column prop="allocationRate" label="Allocation Rate" width="200" sortable="custom">
          <template #default="{ row }">
            <el-progress
              :percentage="row.allocationRate ?? 0"
              :color="getProgressColor(row.allocationRate ?? 0)"
              :stroke-width="10"
              :format="(percentage: number) => percentage.toFixed(2) + '%'"
            />
          </template>
        </el-table-column>

        <el-table-column prop="avgUtilization" label="Avg Utilization" width="160" sortable="custom">
          <template #default="{ row }">
            <el-tag :type="getUtilizationType(row.avgUtilization ?? 0)">
              {{ row.avgUtilization?.toFixed(2) ?? '0.00' }}%
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="maxUtilization" label="Max Utilization" min-width="160">
          <template #default="{ row }">
            {{ row.maxUtilization?.toFixed(2) ?? '0.00' }}%
          </template>
        </el-table-column>

        <el-table-column prop="minUtilization" label="Min Utilization" min-width="160">
          <template #default="{ row }">
            {{ row.minUtilization?.toFixed(2) ?? '0.00' }}%
          </template>
        </el-table-column>

        <el-table-column prop="p50Utilization" label="P50 Utilization" min-width="160">
          <template #default="{ row }">
            {{ row.p50Utilization?.toFixed(2) ?? '0.00' }}%
          </template>
        </el-table-column>

        <el-table-column prop="p95Utilization" label="P95 Utilization" min-width="160">
          <template #default="{ row }">
            {{ row.p95Utilization?.toFixed(2) ?? '0.00' }}%
          </template>
        </el-table-column>

        <el-table-column prop="sampleCount" label="Samples" min-width="120" />

        <template #empty>
          <el-empty description="No Data" />
        </template>
      </el-table>
      </div>
    </el-card>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { QuestionFilled } from '@element-plus/icons-vue'
import { getClusterHourlyStats, ClusterGpuHourlyStats } from '@/services/gpu-aggregation'
import { getClusterOverview, ClusterOverviewRes } from '@/services/dashboard'
import { getWorkspaces, WorkspaceItem } from '@/services/safe-api'
import { useClusterSync } from '@/composables/useClusterSync'
import LineChart from '@/components/base/LineChart.vue'
import dayjs from 'dayjs'
import utc from 'dayjs/plugin/utc'
import * as echarts from 'echarts'

dayjs.extend(utc)

// Get global cluster with URL sync
const { selectedCluster } = useClusterSync()

const loading = ref(false)
const statsData = ref<ClusterGpuHourlyStats[]>([])
const timeRange = ref<[string, string]>()
const filters = ref({})

// Cluster overview data
const clusterOverviewData = ref<ClusterOverviewRes | null>(null)
const overviewChartRef = ref<HTMLElement>()
const overviewChartInstance = ref<echarts.ECharts | null>(null)
const overviewLoading = ref(false)

// Workspace node distribution data
const workspaceNodesData = ref<WorkspaceItem[]>([])
const workspaceChartRefs = ref<HTMLElement[]>([])
const workspaceChartInstances = ref<echarts.ECharts[]>([])
const workspaceLoading = ref(false)

// Pie chart color configuration
const PIE_COLORS = ['#67c23a', '#f56c6c', '#409eff', '#e6a23c', '#909399']
const WORKSPACE_COLORS = ['#409eff', '#67c23a', '#e6a23c', '#f56c6c', '#909399', '#00b1a6', '#9c27b0', '#ff9800', '#3f51b5', '#009688']

// Sorting
const currentSortProp = ref<string>('statHour')
const currentSortOrder = ref<'ascending' | 'descending'>('descending')

// Calculate statistics
const avgAllocationRate = computed(() => {
  if (statsData.value.length === 0) return 0
  const sum = statsData.value.reduce((acc, item) => acc + item.allocationRate, 0)
  return sum / statsData.value.length
})

const peakAllocationRate = computed(() => {
  if (statsData.value.length === 0) return 0
  return Math.max(...statsData.value.map(item => item.allocationRate))
})

const avgUtilization = computed(() => {
  if (statsData.value.length === 0) return 0
  const sum = statsData.value.reduce((acc, item) => acc + item.avgUtilization, 0)
  return sum / statsData.value.length
})

const maxUtilization = computed(() => {
  if (statsData.value.length === 0) return 0
  return Math.max(...statsData.value.map(item => item.maxUtilization))
})

// Chart data
const chartLabels = computed(() => {
  return statsData.value
    .map((item: ClusterGpuHourlyStats) => formatTime(item.statHour))
    .reverse() // Sort in chronological order
})

const chartSeries = computed(() => {
  const sortedData = [...statsData.value].reverse() // Sort in chronological order

  return [
    {
      name: 'Allocation Rate',
      data: sortedData.map((item: ClusterGpuHourlyStats) => Number(item.allocationRate.toFixed(2)))
    },
    {
      name: 'Avg Utilization',
      data: sortedData.map((item: ClusterGpuHourlyStats) => Number(item.avgUtilization.toFixed(2)))
    }
  ]
})

// Fetch data
// Handle table sort change
const handleTableSortChange = ({ prop, order }: { prop: string; order: string | null }) => {
  if (order) {
    currentSortProp.value = prop
    currentSortOrder.value = order as 'ascending' | 'descending'
  } else {
    currentSortProp.value = 'statHour'
    currentSortOrder.value = 'descending'
  }
  fetchData()
}

// Fetch cluster overview data
const fetchClusterOverview = async () => {
  if (!selectedCluster.value) return

  overviewLoading.value = true
  try {
    // Dispose old chart instance first
    if (overviewChartInstance.value) {
      overviewChartInstance.value.dispose()
      overviewChartInstance.value = null
    }

    const data = await getClusterOverview()
    clusterOverviewData.value = data

    // Render overview pie chart
    await nextTick()
    renderOverviewChart()
  } catch (error) {
    console.error('Failed to fetch cluster overview:', error)
  } finally {
    overviewLoading.value = false
  }
}

// Render cluster overview pie chart
const renderOverviewChart = () => {
  if (!overviewChartRef.value || !clusterOverviewData.value) return

  if (!overviewChartInstance.value) {
    overviewChartInstance.value = echarts.init(overviewChartRef.value)
  }

  const chart = overviewChartInstance.value
  const data = clusterOverviewData.value

  // Get actual values of CSS variables
  const computedStyle = getComputedStyle(document.documentElement)
  const primaryTextColor = computedStyle.getPropertyValue('--el-text-color-primary').trim() || '#303133'
  const emptySliceColor =
    computedStyle.getPropertyValue('--el-fill-color-light').trim() ||
    computedStyle.getPropertyValue('--el-fill-color').trim() ||
    '#e5e7eb'

  if (data.totalNodes === 0) {
    const emptyOption = {
      backgroundColor: 'transparent',
      series: [
        {
          type: 'pie',
          radius: ['30%', '65%'],
          center: ['50%', '50%'],
          data: [{ value: 1, name: 'Empty', itemStyle: { color: emptySliceColor } }],
          label: {
            show: true,
            position: 'center',
            formatter: () => '{value|0}\n{label|No Nodes}',
            rich: {
              value: {
                fontSize: 24,
                fontWeight: 'bold',
                color: primaryTextColor,
                lineHeight: 32
              },
              label: {
                fontSize: 12,
                color: primaryTextColor,
                lineHeight: 20
              }
            }
          },
          emphasis: { scale: false },
          silent: true
        }
      ]
    }
    chart.setOption(emptyOption)
    setTimeout(() => {
      chart.resize()
    }, 100)
    return
  }

  const pieData = [
    {
      name: 'Healthy',
      value: data.healthyNodes,
      itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 1, y2: 1,
          colorStops: [
            { offset: 0, color: '#67c23a' },
            { offset: 1, color: '#85ce61' }
          ]
        },
        shadowBlur: 12,
        shadowColor: 'rgba(103, 194, 58, 0.25)',
        shadowOffsetX: 0,
        shadowOffsetY: 4
      }
    },
    {
      name: 'Faulty',
      value: data.faultyNodes,
      itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 1, y2: 1,
          colorStops: [
            { offset: 0, color: '#f56c6c' },
            { offset: 1, color: '#f78989' }
          ]
        },
        shadowBlur: 12,
        shadowColor: 'rgba(245, 108, 108, 0.25)',
        shadowOffsetX: 0,
        shadowOffsetY: 4
      }
    },
    {
      name: 'Busy',
      value: data.busyNodes,
      itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 1, y2: 1,
          colorStops: [
            { offset: 0, color: '#409eff' },
            { offset: 1, color: '#66b1ff' }
          ]
        },
        shadowBlur: 12,
        shadowColor: 'rgba(64, 158, 255, 0.25)',
        shadowOffsetX: 0,
        shadowOffsetY: 4
      }
    },
    {
      name: 'Fully Idle',
      value: data.fullyIdleNodes,
      itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 1, y2: 1,
          colorStops: [
            { offset: 0, color: '#e6a23c' },
            { offset: 1, color: '#ebb563' }
          ]
        },
        shadowBlur: 12,
        shadowColor: 'rgba(230, 162, 60, 0.25)',
        shadowOffsetX: 0,
        shadowOffsetY: 4
      }
    },
    {
      name: 'Partially Idle',
      value: data.partiallyIdleNodes,
      itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 1, y2: 1,
          colorStops: [
            { offset: 0, color: '#909399' },
            { offset: 1, color: '#a6a9ad' }
          ]
        },
        shadowBlur: 12,
        shadowColor: 'rgba(144, 147, 153, 0.25)',
        shadowOffsetX: 0,
        shadowOffsetY: 4
      }
    }
  ]

  const option = {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'item',
      formatter: (params: any) => {
        const percent = ((params.value / data.totalNodes) * 100).toFixed(1)
        return `${params.name}: ${params.value} nodes (${percent}%)`
      },
      backgroundColor: 'rgba(0, 0, 0, 0.7)',
      borderColor: 'transparent',
      textStyle: {
        color: '#fff'
      },
      extraCssText: 'box-shadow: 0 0 20px rgba(0, 0, 0, 0.2); z-index: 9999;',
      appendToBody: true
    },
    series: [
      {
        type: 'pie',
        radius: ['30%', '65%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        animationType: 'scale',
        animationEasing: 'elasticOut',
        animationDelay: function (idx: number) {
          return Math.random() * 200;
        },
        itemStyle: {
          borderRadius: 10,
          borderColor: 'transparent',
          borderWidth: 0
        },
        label: {
          show: true,
          position: 'center',
          formatter: () => {
            return `{value|${data.totalNodes}}\n{label|Total Nodes}`
          },
          rich: {
            value: {
              fontSize: 24,
              fontWeight: 'bold',
              color: primaryTextColor,
              lineHeight: 32
            },
            label: {
              fontSize: 12,
              color: primaryTextColor,
              lineHeight: 20
            }
          }
        },
        emphasis: {
          itemStyle: {
            shadowBlur: 20,
            shadowOffsetX: 0,
            shadowColor: 'rgba(0, 0, 0, 0.4)',
            borderColor: 'rgba(255, 255, 255, 0.2)',
            borderWidth: 1
          },
          scaleSize: 5
        },
        data: pieData
      }
    ]
  }

  chart.setOption(option)

  // Ensure chart size fits container
  setTimeout(() => {
    chart.resize()
  }, 100)

  // Remove previous event listeners
  chart.off('mouseover')
  chart.off('mouseout')

  // Listen for mouse hover events to dynamically change center text color
  chart.on('mouseover', (params: any) => {
    if (params.componentType === 'series' && params.seriesType === 'pie') {
      const hoverColor = params.name === 'Healthy' ? '#67c23a' :
                        params.name === 'Faulty' ? '#f56c6c' :
                        params.name === 'Busy' ? '#409eff' :
                        params.name === 'Fully Idle' ? '#e6a23c' :
                        params.name === 'Partially Idle' ? '#909399' : primaryTextColor

      const shadowColor = params.name === 'Healthy' ? 'rgba(103, 194, 58, 0.3)' :
                         params.name === 'Faulty' ? 'rgba(245, 108, 108, 0.3)' :
                         params.name === 'Busy' ? 'rgba(64, 158, 255, 0.3)' :
                         params.name === 'Fully Idle' ? 'rgba(230, 162, 60, 0.3)' :
                         params.name === 'Partially Idle' ? 'rgba(144, 147, 153, 0.3)' : 'rgba(0, 0, 0, 0.2)'

      // Update center text color
      chart.setOption({
        series: [{
          label: {
            rich: {
              value: {
                fontSize: 26,
                fontWeight: 'bold',
                color: hoverColor,
                lineHeight: 34,
                textShadowColor: shadowColor,
                textShadowBlur: 8
              },
              label: {
                fontSize: 13,
                color: hoverColor,
                fontWeight: 500
              }
            }
          }
        }]
      })
    }
  })

  // Restore default color on mouse out
  chart.on('mouseout', () => {
    chart.setOption({
      series: [{
        label: {
          rich: {
            value: {
              fontSize: 24,
              fontWeight: 'bold',
              color: primaryTextColor,
              lineHeight: 32
            },
            label: {
              fontSize: 12,
              color: primaryTextColor,
              fontWeight: 400
            }
          }
        }
      }]
    })
  })

  // Listen for window resize
  window.addEventListener('resize', () => {
    chart?.resize()
  })
}

// Fetch workspace node distribution data
const fetchWorkspaceNodes = async () => {
  if (!selectedCluster.value) return

  workspaceLoading.value = true
  try {
    // Dispose all old chart instances first
    workspaceChartInstances.value.forEach(chart => {
      if (chart) {
        chart.dispose()
      }
    })
    workspaceChartInstances.value = []
    workspaceChartRefs.value = []

    const response = await getWorkspaces(selectedCluster.value)
    // Check if response and items exist
    if (!response || !response.items) {
      console.warn('No workspace data received')
      workspaceNodesData.value = []
      return
    }

    // Filter current cluster's workspaces and sort by node count
    workspaceNodesData.value = response.items
      .filter(item => !selectedCluster.value || !item.clusterId || item.clusterId === selectedCluster.value)
      .sort((a, b) => b.currentNodeCount - a.currentNodeCount)

    // Render all workspace pie charts
    await nextTick()
    renderWorkspaceCharts()
  } catch (error) {
    console.error('Failed to fetch workspace nodes:', error)
    workspaceNodesData.value = []
  } finally {
    workspaceLoading.value = false
  }
}

// Set workspace chart ref
const setWorkspaceChartRef = (el: any, index: number) => {
  if (el && el instanceof HTMLElement) {
    workspaceChartRefs.value[index] = el
  }
}

// Render single workspace node pie chart
const renderSingleWorkspaceChart = (workspace: WorkspaceItem, index: number) => {
  const chartEl = workspaceChartRefs.value[index]
  if (!chartEl) return

  // Initialize or get existing instance
  if (!workspaceChartInstances.value[index]) {
    workspaceChartInstances.value[index] = echarts.init(chartEl)
  }

  const chart = workspaceChartInstances.value[index]

  // Get actual values of CSS variables
  const computedStyle = getComputedStyle(document.documentElement)
  const primaryTextColor = computedStyle.getPropertyValue('--el-text-color-primary').trim() || '#303133'
  const emptySliceColor =
    computedStyle.getPropertyValue('--el-fill-color-light').trim() ||
    computedStyle.getPropertyValue('--el-fill-color').trim() ||
    '#e5e7eb'

  // Prepare data
  const readyNodes = workspace.currentNodeCount - (workspace.abnormalNodeCount || 0)
  const abnormalNodes = workspace.abnormalNodeCount || 0
  const targetNodes = workspace.targetNodeCount || 0

  // If no nodes, show empty state
  if (workspace.currentNodeCount === 0 && targetNodes === 0) {
    const emptyOption = {
      backgroundColor: 'transparent',
      grid: {
        top: 0,
        right: 0,
        bottom: 0,
        left: 0,
        containLabel: false
      },
      series: [
        {
          type: 'pie',
          radius: ['35%', '80%'],
          center: ['50%', '50%'],
          data: [{ value: 1, name: 'Empty', itemStyle: { color: emptySliceColor } }],
          label: {
            show: true,
            position: 'center',
            formatter: () => '{value|0}\n{label|No Nodes}',
            rich: {
              value: {
                fontSize: 24,
            fontWeight: 'bold',
                color: primaryTextColor,
                lineHeight: 32
              },
              label: {
                fontSize: 12,
                color: primaryTextColor,
                lineHeight: 20
              }
            }
          },
          emphasis: { scale: false },
          silent: true
        }
      ]
    }
    chart.setOption(emptyOption)
    setTimeout(() => {
      chart.resize()
    }, 100)
    return
  }

  // Pie chart data: show ready and abnormal nodes
  const pieData = [
    {
      name: 'Ready',
      value: readyNodes,
          itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 1, y2: 1,
          colorStops: [
            { offset: 0, color: '#67c23a' },
            { offset: 1, color: '#85ce61' }
          ]
        },
        shadowBlur: 12,
        shadowColor: 'rgba(103, 194, 58, 0.25)',
            shadowOffsetX: 0,
        shadowOffsetY: 4
      }
    },
    {
      name: 'Abnormal',
      value: abnormalNodes,
      itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 1, y2: 1,
          colorStops: [
            { offset: 0, color: '#f56c6c' },
            { offset: 1, color: '#f78989' }
          ]
        },
        shadowBlur: 12,
        shadowColor: 'rgba(245, 108, 108, 0.25)',
        shadowOffsetX: 0,
        shadowOffsetY: 4
      }
    }
  ]

  // If target > current, show pending allocation
  if (targetNodes > workspace.currentNodeCount) {
    pieData.push({
      name: 'Pending',
      value: targetNodes - workspace.currentNodeCount,
      itemStyle: {
        color: {
          type: 'linear',
          x: 0, y: 0, x2: 1, y2: 1,
          colorStops: [
            { offset: 0, color: '#e6a23c' },
            { offset: 1, color: '#ebb563' }
          ]
        },
        shadowBlur: 12,
        shadowColor: 'rgba(230, 162, 60, 0.25)',
        shadowOffsetX: 0,
        shadowOffsetY: 4
      }
    })
  }

  const option = {
    backgroundColor: 'transparent',
    grid: {
      top: 0,
      right: 0,
      bottom: 0,
      left: 0,
      containLabel: false
    },
    tooltip: {
      trigger: 'item',
      formatter: (params: any) => {
        const total = workspace.targetNodeCount || workspace.currentNodeCount
        const percent = ((params.value / total) * 100).toFixed(1)
        return `${params.name}: ${params.value} nodes (${percent}%)`
      },
      backgroundColor: 'rgba(0, 0, 0, 0.7)',
      borderColor: 'transparent',
      textStyle: {
        color: '#fff'
      },
      extraCssText: 'box-shadow: 0 0 20px rgba(0, 0, 0, 0.2); z-index: 9999;',
      appendToBody: true // Append tooltip to body to avoid clipping
    },
    series: [
      {
        type: 'pie',
        radius: ['35%', '80%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: false,
        animationType: 'scale',
        animationEasing: 'elasticOut',
        animationDelay: function (idx: number) {
          return Math.random() * 200;
        },
        itemStyle: {
          borderRadius: 10,
          borderColor: 'transparent',
          borderWidth: 0
        },
        label: {
          show: true,
          position: 'center',
          formatter: () => {
            return `{value|${workspace.currentNodeCount}}\n{label|Nodes}`
          },
          rich: {
            value: {
              fontSize: 18,
              fontWeight: 'bold',
              color: primaryTextColor,
              lineHeight: 24,
              textBorderColor: 'rgba(0, 0, 0, 0.15)',
              textBorderWidth: 1,
              textShadowColor: 'rgba(0, 0, 0, 0.1)',
              textShadowBlur: 4,
              textShadowOffsetX: 0,
              textShadowOffsetY: 2
            },
            label: {
              fontSize: 10,
              color: primaryTextColor,
              lineHeight: 16
            }
          }
        },
        emphasis: {
          itemStyle: {
            shadowBlur: 20,
            shadowOffsetX: 0,
            shadowColor: 'rgba(0, 0, 0, 0.4)',
            borderColor: 'rgba(255, 255, 255, 0.2)',
            borderWidth: 1
          },
          scaleSize: 5
        },
        data: pieData
      }
    ]
  }

  chart.setOption(option)

  // Ensure chart size fits container
  setTimeout(() => {
    chart.resize()
  }, 100)

  // Remove previous event listeners
  chart.off('mouseover')
  chart.off('mouseout')

  // Listen for mouse hover events to dynamically change center text color
  chart.on('mouseover', (params: any) => {
    if (params.componentType === 'series' && params.seriesType === 'pie') {
      const hoverColor = params.name === 'Ready' ? '#67c23a' :
                        params.name === 'Abnormal' ? '#f56c6c' :
                        params.name === 'Pending' ? '#e6a23c' : primaryTextColor

      const shadowColor = params.name === 'Ready' ? 'rgba(103, 194, 58, 0.3)' :
                         params.name === 'Abnormal' ? 'rgba(245, 108, 108, 0.3)' :
                         params.name === 'Pending' ? 'rgba(230, 162, 60, 0.3)' : 'rgba(0, 0, 0, 0.2)'

      // Update center text color
      chart.setOption({
        series: [{
          label: {
            rich: {
              value: {
                fontSize: 20,
                fontWeight: 'bold',
                color: hoverColor,
                lineHeight: 26,
                textShadowColor: shadowColor,
                textShadowBlur: 8
              },
              label: {
                fontSize: 11,
                color: hoverColor,
                fontWeight: 500
              }
            }
          }
        }]
      })
    }
  })

  // Restore default color on mouse out
  chart.on('mouseout', () => {
    chart.setOption({
      series: [{
        label: {
          rich: {
            value: {
              fontSize: 18,
              fontWeight: 'bold',
              color: primaryTextColor,
              lineHeight: 24,
              textShadowColor: 'rgba(0, 0, 0, 0.1)',
              textShadowBlur: 4
            },
            label: {
              fontSize: 10,
              color: primaryTextColor,
              fontWeight: 400
            }
          }
        }
      }]
    })
  })
}

// Render all workspace node distribution pie charts
const renderWorkspaceCharts = () => {
  if (!workspaceNodesData.value || workspaceNodesData.value.length === 0) return

  // Wait for DOM update
  nextTick(() => {
    // Dynamically adjust container grid layout based on workspace count
    const container = document.querySelector('.workspaces-container') as HTMLElement
    if (container) {
      const count = workspaceNodesData.value.length
      if (count <= 6) {
        container.style.gridTemplateColumns = 'repeat(3, 1fr)'
        container.style.gridTemplateRows = 'repeat(2, 1fr)'
      } else if (count <= 8) {
        container.style.gridTemplateColumns = 'repeat(4, 1fr)'
        container.style.gridTemplateRows = 'repeat(2, 1fr)'
      } else if (count <= 9) {
        container.style.gridTemplateColumns = 'repeat(3, 1fr)'
        container.style.gridTemplateRows = 'repeat(3, 1fr)'
      } else if (count <= 12) {
        container.style.gridTemplateColumns = 'repeat(4, 1fr)'
        container.style.gridTemplateRows = 'repeat(3, 1fr)'
      } else {
        container.style.gridTemplateColumns = 'repeat(5, 1fr)'
        container.style.gridTemplateRows = 'repeat(3, 1fr)'
      }
    }

    workspaceNodesData.value.forEach((workspace, index) => {
      renderSingleWorkspaceChart(workspace, index)
    })

    // Listen for window resize
    const resizeHandler = () => {
      workspaceChartInstances.value.forEach(chart => chart?.resize())
    }
    window.removeEventListener('resize', resizeHandler)
    window.addEventListener('resize', resizeHandler)
  })
}

const fetchData = async () => {
  if (!selectedCluster.value) {
    ElMessage.warning('Please select a cluster from the header')
    return
  }

  if (!timeRange.value || timeRange.value.length !== 2) {
    ElMessage.warning('Please select time range')
    return
  }

  loading.value = true
  try {
    // Convert sort prop to API format
    let orderBy: 'time' | 'utilization' = 'time'
    if (currentSortProp.value === 'avgUtilization') {
      orderBy = 'utilization'
    }

    const params = {
      cluster: selectedCluster.value,
      startTime: timeRange.value[0],
      endTime: timeRange.value[1],
      order_by: orderBy,
      order_direction: (currentSortOrder.value === 'ascending' ? 'asc' : 'desc') as 'asc' | 'desc'
    }

    const response = await getClusterHourlyStats(params)
    statsData.value = response.data
    // ElMessage.success(`Loaded ${response.data.length} records successfully`) // Removed success message to avoid blocking the UI
  } catch (error: any) {
    console.error('Failed to fetch cluster stats:', error)
    ElMessage.error(error || 'Failed to load data')
  } finally {
    loading.value = false
  }
}

// Initialize with default time range
const initializeTimeRange = () => {
  currentSortProp.value = 'statHour'
  currentSortOrder.value = 'descending'
  // Default to last 24 hours
  const endTime = dayjs().utc()
  const startTime = endTime.subtract(24, 'hour')
  timeRange.value = [
    startTime.format('YYYY-MM-DDTHH:mm:ss') + 'Z',
    endTime.format('YYYY-MM-DDTHH:mm:ss') + 'Z'
  ]
  // No need to call fetchData here as watch will handle it
}

// Format time
const formatTime = (time: string) => {
  return dayjs(time).format('YYYY-MM-DD HH:mm:ss')
}

// Get progress bar color
const getProgressColor = (percentage: number) => {
  if (percentage < 50) return '#67c23a'
  if (percentage < 80) return '#e6a23c'
  return '#f56c6c'
}

// Get utilization type
const getUtilizationType = (utilization: number) => {
  if (utilization < 50) return 'success'
  if (utilization < 80) return 'warning'
  return 'danger'
}

// Watch for global cluster changes and auto-refresh
watch(selectedCluster, (newCluster) => {
  if (newCluster && timeRange.value) {
    fetchData()
    fetchClusterOverview() // Refresh cluster overview
    fetchWorkspaceNodes() // Refresh workspace node distribution
  }
})

// Watch for time range changes and auto-refresh
watch(timeRange, (newTimeRange) => {
  if (newTimeRange && newTimeRange.length === 2 && selectedCluster.value) {
    fetchData()
  }
})

onMounted(() => {
  // Delay initial load to prevent blocking page transition
  nextTick(() => {
    initializeTimeRange()
    // Fetch cluster data
    if (selectedCluster.value) {
      fetchClusterOverview() // Fetch cluster overview data
      fetchWorkspaceNodes() // Fetch workspace node distribution data
    }
  })
})
</script>

<style scoped lang="scss">
.cluster-stats {
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  box-sizing: border-box;
  padding: 0 20px;

  @media (max-width: 768px) {
    padding: 0 12px;
  }


  // Ensure all child elements don't exceed container
  .filter-card, .stats-cards, .chart-card, .table-card {
    width: 100%;
    max-width: 100%;
    box-sizing: border-box;
  }

  // Common title style - moved to outer level for all sections to use
  .page-title {
    font-size: 20px;
    font-weight: 600;
    color: var(--el-text-color-primary);
    margin: 0;
    flex-shrink: 0;

    @media (min-width: 1920px) {
      font-size: 22px;
    }

    @media (max-width: 768px) {
      font-size: 18px;
    }
  }

  .filter-section {
    margin-bottom: 20px;

    .filter-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      padding: 12px 0;
      gap: 20px;
      flex-wrap: wrap;

      @media (max-width: 768px) {
        flex-direction: column;
        align-items: stretch;
        gap: 12px;
      }
    }

    .card-header {
      font-weight: 600;
      font-size: 18px;
    }

    .filters {
      display: flex;
      align-items: center;
      gap: 12px;
      flex-wrap: wrap;

      @media (max-width: 768px) {
        width: 100%;
      }

      :deep(.el-form) {
        display: flex;
        flex-wrap: wrap;
        gap: 12px;
        align-items: center;

        @media (max-width: 768px) {
          flex-direction: column;
          align-items: stretch;
          width: 100%;
        }
      }

      :deep(.el-form-item) {
        align-items: center;
        margin-bottom: 0;

        @media (max-width: 768px) {
          display: flex;
          flex-direction: column;
          align-items: stretch;
          width: 100%;
        }
      }

      .time-picker {
        width: 400px;

        @media (max-width: 768px) {
          width: 100% !important;
        }
      }

      :deep(.el-form-item__label) {
        font-size: 14px;
        line-height: 32px;
        display: flex;
        align-items: center;
        height: 32px;

        @media (min-width: 1920px) {
          font-size: 16px;
        }

        @media (max-width: 768px) {
          line-height: 1.5;
          height: auto;
          margin-bottom: 4px;
        }
      }

      :deep(.el-form-item__content) {
        line-height: 32px;
        display: flex;
        align-items: center;

        @media (max-width: 768px) {
          width: 100%;
        }
      }

      // Button group responsive
      .button-group {
        @media (max-width: 768px) {
          width: 100%;

          :deep(.el-form-item__content) {
            display: flex;
            gap: 8px;
            width: 100%;

            .el-button {
              flex: 1;
            }
          }
        }
      }
    }
  }

  .stats-cards {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 16px;
    margin-bottom: 20px;

    @media (max-width: 1024px) {
      grid-template-columns: repeat(2, 1fr);
      gap: 12px;
    }

    @media (max-width: 600px) {
      grid-template-columns: 1fr;
      gap: 10px;
    }

    .stat-card {
      border-radius: 15px;
      transition: all 0.3s ease;
      position: relative;
      overflow: hidden;

      &::before {
        content: '';
        position: absolute;
        top: 0;
        left: 0;
        right: 0;
        height: 2px;
        background: linear-gradient(
          90deg,
          transparent 0%,
          rgba(64, 158, 255, 0.5) 20%,
          rgba(103, 194, 58, 0.5) 50%,
          rgba(245, 108, 108, 0.5) 80%,
          transparent 100%
        );
        opacity: 0.6;
      }

      &:hover {
        transform: translateY(-2px);
        box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
        border-color: var(--el-border-color);
      }

      :deep(.el-card__body) {
        padding: 20px;
      }

      .stat-content {
        display: flex;
        align-items: center;
        gap: 16px;

        .stat-icon {
          width: 48px;
          height: 48px;
          border-radius: 12px;
          display: flex;
          align-items: center;
          justify-content: center;
          font-size: 24px;
          flex-shrink: 0;
          transition: all 0.3s ease;

          i {
            font-size: inherit;
          }

          &--primary {
            background: linear-gradient(135deg, rgba(64, 158, 255, 0.15), rgba(64, 158, 255, 0.05));
            color: #409eff;
          }

          &--success {
            background: linear-gradient(135deg, rgba(103, 194, 58, 0.15), rgba(103, 194, 58, 0.05));
            color: #67c23a;
          }

          &--info {
            background: linear-gradient(135deg, rgba(0, 177, 166, 0.15), rgba(0, 177, 166, 0.05));
            color: #00b1a6;
          }

          &--warning {
            background: linear-gradient(135deg, rgba(230, 162, 60, 0.15), rgba(230, 162, 60, 0.05));
            color: #e6a23c;
          }
        }

        .stat-info {
          flex: 1;

          .stat-label {
            font-size: 14px;
            color: var(--el-text-color-secondary);
            margin-bottom: 4px;
            display: flex;
            align-items: center;
            gap: 4px;

            .stat-help-icon {
              font-size: 14px;
              color: var(--el-text-color-secondary);
              cursor: help;
              transition: all 0.3s ease;
              flex-shrink: 0;

              &:hover {
                color: var(--el-color-primary);
                transform: scale(1.1);
              }
            }
          }

          .stat-value {
            font-size: 24px;
            font-weight: 600;
            color: var(--el-text-color-primary);
            transition: color 0.3s ease;

            &--primary {
              color: #409eff;
            }

            &--success {
              color: #67c23a;
            }

            &--info {
              color: #00b1a6;
            }

            &--warning {
              color: #e6a23c;
            }
          }
        }
      }

      &:hover {
        .stat-icon {
          transform: scale(1.1) rotate(5deg);
        }

        .stat-value {
          &--primary {
            color: #66b1ff;
          }

          &--success {
            color: #85ce61;
          }

          &--info {
            color: #00c9bf;
          }

          &--warning {
            color: #ebb563;
          }
        }
      }
    }
  }

  .chart-card {
    border-radius: 15px;
    margin-bottom: 20px;

    .card-header {
      font-weight: 600;
      font-size: 18px;
    }
  }

  .table-card {
    border-radius: 15px;
    overflow: hidden;

    :deep(.el-card__body) {
      padding: 0;
    }

    .table-wrapper {
      overflow-x: auto;
      padding: 20px;

      @media (max-width: 768px) {
        padding: 12px;
      }

      :deep(.el-table) {
        min-width: 800px;

        @media (max-width: 768px) {
          font-size: 12px;

          th, td {
            padding: 8px 0;
          }
        }
      }
    }
    margin-bottom: 20px;

    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      font-weight: 600;
      font-size: 18px;

      .header-actions {
        display: flex;
        align-items: center;
      }
    }

    // Table overall font size
    :deep(.el-table) {
      font-size: 14px;

      @media (min-width: 1920px) {
        font-size: 15px;
      }

      // Table row height
      td {
        padding: 14px 0;

        @media (min-width: 1920px) {
          padding: 16px 0;
        }
      }

      th {
        font-size: 14px;
        font-weight: 600;
        padding: 14px 0;

        @media (min-width: 1920px) {
          font-size: 15px;
          padding: 16px 0;
        }
      }

      // Table cell padding
      .cell {
        padding-left: 12px;
        padding-right: 12px;
      }
    }
  }

  // Combined node distribution dashboard
  .nodes-dashboard {
    margin-top: 30px;

    .page-title {
      margin-bottom: 20px;
    }

    .unified-grid {
      display: grid;
      grid-template-columns: 2fr 3fr;
      grid-template-rows: 1fr;
      gap: 20px;
      min-height: 450px;

      @media (max-width: 1400px) {
        min-height: 400px;
        gap: 15px;
      }

      @media (max-width: 768px) {
        min-height: 350px;
        gap: 12px;
      }
    }

    // Cluster section card - takes left 2/5
    .cluster-section {
      border-radius: 15px;
      background: var(--el-bg-color);
      border: 1px solid var(--el-border-color-lighter);
      box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.06);
      overflow: visible;
      transition: all 0.3s ease;

      &:hover {
        transform: translateY(-2px);
        box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08) !important;
      }

      ::v-deep(.el-card__header) {
        background: linear-gradient(135deg,
          rgba(64, 158, 255, 0.04) 0%,
          rgba(64, 158, 255, 0.01) 100%);
        border-bottom: 1px solid var(--el-border-color-lighter);
      }

      ::v-deep(.el-card__body) {
        padding: 30px;
        background: radial-gradient(ellipse at center,
          rgba(64, 158, 255, 0.03) 0%,
          transparent 70%);
        overflow: visible;
      }

      .section-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding: 0;

        .section-title {
          font-weight: 600;
          font-size: 18px;
          color: var(--el-text-color-primary);
          background: linear-gradient(135deg, #409eff, #67c23a);
          -webkit-background-clip: text;
          -webkit-text-fill-color: transparent;
          background-clip: text;
        }

        .node-count {
          font-size: 16px;
          font-weight: 600;
          color: #409eff;
        }
      }

      }

      .overview-chart {
      width: 100%;
      height: 100%;
      min-height: 380px;
      position: relative;

      &::before {
        content: '';
        position: absolute;
        top: 50%;
        left: 50%;
        transform: translate(-50%, -50%);
        width: 85%;
        height: 85%;
        background: radial-gradient(circle,
          rgba(64, 158, 255, 0.05) 0%,
          transparent 60%);
        border-radius: 50%;
        pointer-events: none;
      }

      @media (max-width: 1400px) {
        min-height: 340px;
      }

        @media (max-width: 768px) {
        min-height: 280px;
      }
    }

    // Workspaces container - takes right 3/5
    .workspaces-container {
      display: grid;
      grid-template-columns: repeat(3, 1fr);
      grid-template-rows: repeat(2, 1fr);
      gap: 12px;
      height: 100%;

      @media (max-width: 1200px) {
        gap: 10px;
      }

      @media (max-width: 768px) {
        gap: 8px;
      }
    }

    // Workspace card
    .workspace-card {
      border-radius: 12px;
      transition: all 0.3s ease;
      background: var(--el-bg-color);
      border: 1px solid var(--el-border-color-lighter);
      box-shadow: 0 2px 10px 0 rgba(0, 0, 0, 0.05);
      overflow: visible;
      position: relative;
      z-index: 1;
      height: 100%;
        display: flex;
        flex-direction: column;

      &:hover {
        transform: translateY(-3px);
        box-shadow: 0 8px 20px rgba(0, 0, 0, 0.08) !important;
        background: linear-gradient(135deg,
          var(--el-bg-color) 0%,
          rgba(64, 158, 255, 0.02) 50%,
          rgba(103, 194, 58, 0.02) 100%);
        z-index: 10;
      }

      ::v-deep(.el-card__body) {
        padding: 0;
        background: radial-gradient(ellipse at center,
          rgba(64, 158, 255, 0.02) 0%,
          transparent 70%);
        overflow: visible;
        flex: 1;
        display: flex;
      }

      .workspace-content {
        width: 100%;
        height: 100%;
          display: flex;
          flex-direction: column;
        padding: 0;

        .workspace-name {
          font-weight: 600;
          font-size: 12px;
          color: var(--el-text-color-primary);
          text-align: center;
          padding: 6px 4px;
          background: linear-gradient(135deg,
            rgba(64, 158, 255, 0.05) 0%,
            rgba(64, 158, 255, 0.02) 100%);
          border-bottom: 1px solid var(--el-border-color-lighter);
          overflow: hidden;
          text-overflow: ellipsis;
          white-space: nowrap;
          line-height: 1.2;
          flex-shrink: 0;
        }

        .node-info {
          font-size: 11px;
          font-weight: 600;
          color: #409eff;
          text-align: center;
          padding: 6px 0;
          display: flex;
          align-items: center;
          justify-content: center;
          gap: 2px;
          border-top: 1px solid var(--el-border-color-lighter);
          background: linear-gradient(135deg,
            rgba(64, 158, 255, 0.02) 0%,
            transparent 100%);
          flex-shrink: 0;

          .node-label {
            font-size: 9px;
            color: var(--el-text-color-secondary);
            font-weight: normal;
            margin-left: 2px;
          }
        }

        .workspace-chart {
          flex: 1;
          width: 100%;
          min-height: 150px;
          position: relative;
          display: flex;
          align-items: center;
          justify-content: center;

          &::before {
            content: '';
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            width: 90%;
            height: 90%;
            background: radial-gradient(circle,
              rgba(64, 158, 255, 0.03) 0%,
              transparent 60%);
            border-radius: 50%;
            pointer-events: none;
            z-index: 0;
          }
        }
      }
    }
  }
}
</style>


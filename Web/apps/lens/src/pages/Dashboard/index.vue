<template>
  <div class="dashboard-page">
    <!-- overview -->
    <p class="large-title">Cluster Overview</p>
    <el-row :gutter="20" v-loading="loadingMap.loadingCard">
      <el-col :span="6" class="mb5" v-for="(item, index) in overviewCards" :key="index">
        <el-card 
          @click="item.path && router.push(item.path)" 
          :class="['overview-card', item.path ? 'cursor-pointer' : '']"
        >
          <div class="card-content">
            <div class="card-bg-icon" :style="{ color: item.iconColor }">
              <el-icon>
                <component :is="item.icon" />
              </el-icon>
            </div>
            <div class="card-info">
              <div class="card-title">{{ item.title }}</div>
              <div class="card-value">{{ item.display ?? item.value }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

  <!-- trends -->
  <p class="default-title">GPU Allocation and Utilization Trends</p>
  <TimePickerSelect v-model="defaultTimeVal" />
  <div class="charts-box">
    <div class="rate-info">
      <div class="text-lg mb-2">GPU Trend</div>
      <div class="flex gap-x-6 text-sm text-gray-400 mb-2">
        <span>Alloc: <span class="dark:text-white fw500">{{ allocationRatePercent }}%</span></span>
        <span>Util: <span class="dark:text-white fw500">{{ utilizationPercent }}%</span></span>
      </div>
      <div class="text-sm">
        Avg over {{ TIME_RANGE_OPTIONS.find(i => i.value === defaultTimeVal)?.label }}
      </div>
    </div>
    <LineChart
      v-if="chartData.labels.length"
      :labels="chartData.labels"
      :series="chartData.series"
      :loading="loadingMap.loadingCard"
      :colors="['#4fc3f7', '#81c784']"
      style="margin-top: 20px;"
    />
  </div>

  <p class="default-title">GPU Spotlight</p>
  <el-card class="section-card">
    <el-tabs v-model="activeTab">
      <el-tab-pane
        v-for="(item, key) in sortedHeatState"
        :key="key"
        :label="key"
        :name="key"
        lazy
      >
        <HeatmapChart
          :rawData="item.Data"
          xKey="nodeName"
          yKey="gpuId"
          valueKey="value"
          :unit="item.unit"
          :min="item.yaxisMin"
          :max="item.yaxisMax"
        />
      </el-tab-pane>
    </el-tabs>
  </el-card>

  

  <!-- resource -->
  <p class="default-title">Top Resource Consumers</p>
  <el-card class="section-card">
    <div class="table-box">
      <el-table :data="tableData" size="large" style="width: 100%" cell-class-name="resource-table-header" v-loading="loading">
        <el-table-column prop="name" label="Resource Name" min-width="180">
        <template #default="{ row }">
            <el-tooltip :content="row.name" placement="top">
              <el-button
                link
                type="primary"
                size="default"
                @click="jumpToDetail(row.uid)"
              >
                {{ row.name }}
              </el-button>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column prop="kind" label="Kind" min-width="180" />
        <el-table-column prop="stat.gpuRequest" label="Request" width="180" />
        <el-table-column label="Utilization" min-width="240" >
          <template #default="{ row }">
            <el-progress :percentage="row.stat.gpuUtilization" />
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
    </div>
  </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, reactive, computed } from 'vue'
import { 
  Monitor,
  Select, 
  Warning, 
  CoffeeCup, 
  PartlyCloudy, 
  Cpu, 
  PieChart, 
  Odometer,
  Download,
  Upload,
  FolderOpened,
  Edit,
  Files,
  DataAnalysis,
  Document,
  DataLine
} from '@element-plus/icons-vue'
import TimePickerSelect from '@/components/base/TimePickerSelect.vue';
import LineChart from '@/components/base/LineChart.vue';
import HeatmapChart from '@/components/base/HeatmapChart.vue'
import {TIME_RANGE_OPTIONS} from '@/constants'
import {
  getClusterOverview,
  ClusterOverviewRes,
  getGpuUtilization,
  getGpuUtilHistory,
  getConsumers,
  getGpuHeatmap,
} from '@/services/dashboard/index'
import {
  parseRangeToMs,
  formatMetricTimeLabel,
  getStepByRangeKey,
} from '@/utils/index'
import {usePaginatedTable} from '@/pages/useTable'
import { useRouter } from 'vue-router'
import { useClusterSync } from '@/composables/useClusterSync'

const router = useRouter()
const { selectedCluster } = useClusterSync()

const activeTab = ref('')
const sortedHeatState = ref<Record<string, { Data: any[]; serial?: number; unit: string; yaxisMax?: number; yaxisMin?: number }>>({})

const loadingMap = reactive<Record<string, boolean>>({
  loadingCard: false,
})
const {
  tableData,
  loading,
  pagination,
  fetchData
} = usePaginatedTable(getConsumers)

// overview card initial val
type OverviewKey = keyof ClusterOverviewRes
interface OverviewCard {
  title: string
  key: OverviewKey
  value: number
  path?: string
  display?: string
  icon?: any
  iconColor?: string
}
const overviewCards = ref<OverviewCard[]>([
  { title: 'Total Nodes', key: 'totalNodes', value: 0, path: '/nodes', icon: Monitor, iconColor: '#409eff' },
  { title: 'Healthy Nodes', key: 'healthyNodes', value: 0, icon: Select, iconColor: '#67c23a' },
  { title: 'Faulty Nodes', key: 'faultyNodes', value: 0, icon: Warning, iconColor: '#f56c6c' },
  { title: 'Fully Idle Nodes', key: 'fullyIdleNodes', value: 0, icon: CoffeeCup, iconColor: '#909399' },
  { title: 'Partially Idle Nodes', key: 'partiallyIdleNodes', value: 0, icon: PartlyCloudy, iconColor: '#e6a23c' },
  { title: 'Busy Nodes', key: 'busyNodes', value: 0, icon: Cpu, iconColor: '#f56c6c' },
  { title: 'GPU Allocation Rate', key: 'allocationRate', value: 0, icon: PieChart, iconColor: '#409eff' },
  { title: 'GPU Utilization', key: 'utilization', value: 0, icon: Odometer, iconColor: '#67c23a' },
  { title: 'RDMA RX Total', key: 'totalRx', value: 0, icon: Download, iconColor: '#409eff' },
  { title: 'RDMA TX Total', key: 'totalTx', value: 0, icon: Upload, iconColor: '#e6a23c' },
  { title: 'Storage Read', key: 'readBandwidth', value: 0, icon: FolderOpened, iconColor: '#409eff' },
  { title: 'Storage Write', key: 'writeBandwidth', value: 0, icon: Edit, iconColor: '#e6a23c' },
  { title: 'Storage Space', key: 'usedSpace', value: 0, icon: Files, iconColor: '#909399' },
  { title: 'Storage Space Utilization', key: 'usagePercentage', value: 0, icon: DataAnalysis, iconColor: '#67c23a' },
  { title: 'Storage Inodes', key: 'usedInodes', value: 0, icon: Document, iconColor: '#909399' },
  { title: 'Storage Inodes Utilization', key: 'inodesUsagePercentage', value: 0, icon: DataLine, iconColor: '#67c23a' },
])
// progress bar initial val
const allocationRatePercent = ref(0)
const utilizationPercent = ref(0)
// line chart initial val
const defaultTimeVal = ref('1h')
const chartData = ref<{
  labels: string[]
  series: { name: string; data: number[] }[]
}>({ labels: [], series: [] })

// format line chart data&label
const formatGpuLineChartData = async (range: string) => {
  const alloPoints: number[] = [] // allocation-y
  const utilPoints: number[] = [] // utilization-y
  const labels: string[] = [] // x

  const end = Date.now()
  const start = end - parseRangeToMs(range)

  const res = await getGpuUtilHistory({
    start: Math.floor(start / 1000),
    end: Math.floor(end / 1000),
    step: getStepByRangeKey(range)
  })

  if(!res.allocationRate?.length || !res.utilization?.length) return { labels: [], series: [] }

  for(let i = 0; i < Math.min(res.allocationRate.length, res.utilization.length); i++){
    alloPoints.push(res.allocationRate[i].value)
    utilPoints.push(res.utilization[i].value)
    labels.push(formatMetricTimeLabel(res.utilization[i].timestamp, range)) 
  }

  return {
    labels,
    series: [
      { name: 'Allocation Rate', data: alloPoints },
      { name: 'Utilization', data: utilPoints },
    ]
  }
}

const updateChart = async () => {
  chartData.value = await formatGpuLineChartData(defaultTimeVal.value)
}

// Adaptive byte formatting
function formatBytes(
  num: number | null | undefined,
  opts: { perSecond?: boolean; decimals?: number } = {}
) {
  const { perSecond = false, decimals = 2 } = opts
  if (num == null || isNaN(Number(num))) return '-'

  let val = Math.abs(Number(num))
  const units = ['Byte', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024
    i++
  }
  // Preserve sign
  const signed = Number(num) < 0 ? -val : val
  const unit = units[i] + (perSecond ? '/s' : '')
  return `${signed.toFixed(decimals)} ${unit}`
}
// Format percentage data
const makePercent = (v: number) => {
  if (v == null) return '-'
  const pct = v <= 1 ? v * 100 : v
  return `${pct.toFixed(2)}%`
}
const flowKeys = new Set(['totalRx', 'totalTx', 'readBandwidth', 'writeBandwidth'])

const getData = async() => {
  loadingMap.loadingCard = true
  const overviewRes = await getClusterOverview()

  overviewCards.value.forEach((card) => {
    const key: OverviewKey = card.key
    // 1) Bytes/s adaptive formatting
    if (flowKeys.has(key)) {
      const raw = overviewRes[key] ?? 0
      card.value = raw
      ;(card as any).display = formatBytes(raw, { perSecond: true })
      return
    }

    // 2) Storage Space: usedSpace / totalSpace (adaptive byte formatting)
    if (key === 'usedSpace') {
      const used = overviewRes.usedSpace ?? 0
      const total = overviewRes.totalSpace ?? 0
      card.value = used
      ;(card as any).display = `${formatBytes(used)} / ${formatBytes(total)}`
      return
    }

    // 3) Storage Inodes: usedInodes / totalInodes (plain numbers)
    if (key === 'usedInodes') {
      const used = overviewRes.usedInodes ?? 0
      const total = overviewRes.totalInodes ?? 0
      card.value = used
      ;(card as any).display = `${used} / ${total}`
      return
    }

    // 4) Other percentage types
    if (['allocationRate', 'utilization', 'usagePercentage', 'inodesUsagePercentage'].includes(key)) {
      const raw = overviewRes[key] ?? 0
      card.value = raw
      ;(card as any).display = makePercent(raw)
      return
    }

    // 5) Direct assignment
    card.value = overviewRes[key] ?? 0
    ;(card as any).display = `${card.value}`
  })

  loadingMap.loadingCard = false

  const progressRes = await getGpuUtilization()
  allocationRatePercent.value = Number((progressRes?.allocationRate)?.toFixed(2)) 
  utilizationPercent.value = Number((progressRes?.utilization)?.toFixed(2))

  const heatRes = await getGpuHeatmap()
  sortedHeatState.value = Object.entries(heatRes)
  .sort(([, a], [, b]) => (a.serial ?? 0) - (b.serial ?? 0)) // Sort by serial
  .reduce((acc, [key, value]) => {
    acc[key] = value
    return acc
  }, {})
  activeTab.value = sortedHeatState.value ? Object.keys(sortedHeatState.value)[0] : ''
  await updateChart()
}

const jumpToDetail = (uid: string) => {
  const query = selectedCluster.value ? `?cluster=${encodeURIComponent(selectedCluster.value)}` : ''
  router.push(`/workload/${encodeURIComponent(uid)}/detail${query}`)
}

onMounted(() => {
  getData()
})

watch(defaultTimeVal, () => {
  updateChart()
})

</script>

<style scoped lang="scss">
  .dashboard-page {
    position: relative;
    
    // Decorative background elements for glassmorphism
    &::before {
      content: '';
      position: absolute;
      top: 50px;
      left: -100px;
      width: 500px;
      height: 500px;
      background: radial-gradient(circle, rgba(64, 158, 255, 0.1) 0%, transparent 70%);
      border-radius: 50%;
      pointer-events: none;
      z-index: 0;
      animation: float 20s ease-in-out infinite;
    }
    
    &::after {
      content: '';
      position: absolute;
      top: 300px;
      right: -150px;
      width: 600px;
      height: 600px;
      background: radial-gradient(circle, rgba(103, 194, 58, 0.08) 0%, transparent 70%);
      border-radius: 50%;
      pointer-events: none;
      z-index: 0;
      animation: float 25s ease-in-out infinite reverse;
    }
    
    > * {
      position: relative;
      z-index: 1;
    }
  }
  
  @keyframes float {
    0%, 100% {
      transform: translate(0, 0) scale(1);
    }
    33% {
      transform: translate(30px, -30px) scale(1.05);
    }
    66% {
      transform: translate(-20px, 20px) scale(0.95);
    }
  }
  
  .el-card {
    --el-card-border-color: none;
    border-radius: 16px;
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
    transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  }
  
  .overview-card {
    position: relative;
    overflow: hidden;
    border: 1px solid rgba(0, 0, 0, 0.06);
    background: rgba(255, 255, 255, 0.7);
    backdrop-filter: blur(10px);
    -webkit-backdrop-filter: blur(10px);
    
    // Shimmer/glass shine effect
    &::before {
      content: '';
      position: absolute;
      top: 0;
      left: -100%;
      width: 100%;
      height: 100%;
      background: linear-gradient(
        90deg,
        transparent 0%,
        rgba(255, 255, 255, 0.4) 50%,
        transparent 100%
      );
      transition: left 0.6s ease;
      z-index: 3;
    }
    
    // Glow border effect
    &::after {
      content: '';
      position: absolute;
      top: -2px;
      left: -2px;
      right: -2px;
      bottom: -2px;
      background: linear-gradient(
        45deg,
        rgba(64, 158, 255, 0.4),
        rgba(103, 194, 58, 0.4),
        rgba(230, 162, 60, 0.4),
        rgba(245, 108, 108, 0.4)
      );
      background-size: 300% 300%;
      border-radius: 17px;
      opacity: 0;
      z-index: -1;
      transition: opacity 0.3s ease;
      animation: gradient-shift 6s ease infinite;
      filter: blur(8px);
    }
    
    &:hover::before {
      left: 100%;
    }
    
    &:hover::after {
      opacity: 1;
    }
    
    &:hover {
      transform: translateY(-4px);
      box-shadow: 0 12px 28px rgba(0, 0, 0, 0.12);
      border-color: rgba(64, 158, 255, 0.3);
      background: rgba(255, 255, 255, 0.95);
      backdrop-filter: blur(20px);
      -webkit-backdrop-filter: blur(20px);
    }
    
    &.cursor-pointer {
      cursor: pointer;
    }
    
    .card-content {
      position: relative;
      z-index: 2;
      display: flex;
      align-items: center;
      gap: 20px;
      min-height: 100px;
      padding: 8px;
    }
  }
  
  // Gradient animation
  @keyframes gradient-shift {
    0%, 100% {
      background-position: 0% 50%;
    }
    50% {
      background-position: 100% 50%;
    }
  }
  
  // Dark mode glassmorphism
  .dark .overview-card {
    background: rgba(30, 30, 30, 0.7);
    border-color: rgba(255, 255, 255, 0.1);
    
    &::before {
      background: linear-gradient(
        90deg,
        transparent 0%,
        rgba(255, 255, 255, 0.15) 50%,
        transparent 100%
      );
    }
    
    &::after {
      background: linear-gradient(
        45deg,
        rgba(64, 158, 255, 0.6),
        rgba(103, 194, 58, 0.5),
        rgba(230, 162, 60, 0.5),
        rgba(245, 108, 108, 0.5)
      );
    }
    
    &:hover {
      background: rgba(30, 30, 30, 0.9);
      border-color: rgba(64, 158, 255, 0.4);
    }
    
    .card-bg-icon {
      position: absolute;
      right: -15px;
      top: 50%;
      transform: translateY(-50%);
      font-size: 140px;
      opacity: 0.06;
      z-index: 1;
      pointer-events: none;
    }
    
    .card-info {
      flex: 1;
      position: relative;
      z-index: 2;
      
      .card-title {
        font-size: 14px;
        color: var(--el-text-color-secondary);
        margin-bottom: 10px;
        font-weight: 500;
        letter-spacing: 0.3px;
        
        @media (min-width: 1920px) {
          font-size: 15px;
          margin-bottom: 12px;
        }
      }
      
      .card-value {
        font-size: 26px;
        font-weight: 700;
        color: var(--el-text-color-primary);
        line-height: 1.2;
        
        @media (min-width: 1920px) {
          font-size: 28px;
        }
      }
    }
  }
  
  .charts-box {
    border-radius: 16px;
    border: 1px solid rgba(0, 0, 0, 0.06);
    padding: 24px;
    margin: 24px 0;
    background: rgba(255, 255, 255, 0.75);
    backdrop-filter: blur(12px) saturate(180%);
    -webkit-backdrop-filter: blur(12px) saturate(180%);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
    transition: all 0.3s ease;
    
    &:hover {
      box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
    }
    
    @media (min-width: 1920px) {
      padding: 28px;
      margin: 28px 0;
    }
  }
  
  // Dark mode for charts box
  .dark .charts-box {
    background: rgba(30, 30, 30, 0.75);
    border-color: rgba(255, 255, 255, 0.1);
    
    &:hover {
      background: rgba(30, 30, 30, 0.85);
    }
  }
  
  .rate-info {
    display: flex;
    align-items: flex-start;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 16px;
    
    .text-lg {
      font-size: 18px;
      font-weight: 600;
      color: var(--el-text-color-primary);
      
      @media (min-width: 1920px) {
        font-size: 20px;
      }
    }
  }
  
  // Section cards with glassmorphism
  .section-card {
    background: rgba(255, 255, 255, 0.75);
    backdrop-filter: blur(12px) saturate(180%);
    -webkit-backdrop-filter: blur(12px) saturate(180%);
    border: 1px solid rgba(0, 0, 0, 0.06);
    border-radius: 16px;
    padding: 24px;
    margin-bottom: 24px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
    transition: all 0.3s ease;
    
    &:hover {
      box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
    }
    
    @media (min-width: 1920px) {
      padding: 28px;
      margin-bottom: 28px;
    }
    
    // Tabs styling inside card
    :deep(.el-tabs) {
      .el-tabs__header {
        margin-bottom: 20px;
      }
      
      .el-tabs__nav-wrap {
        &::after {
          background-color: var(--el-border-color-lighter);
        }
      }
      
      .el-tabs__item {
        font-size: 14px;
        font-weight: 500;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
        
        &.is-active {
          font-weight: 600;
        }
      }
    }
  }
  
  // Dark mode for section cards
  .dark .section-card {
    background: rgba(30, 30, 30, 0.75);
    border-color: rgba(255, 255, 255, 0.1);
    
    &:hover {
      background: rgba(30, 30, 30, 0.85);
    }
  }
  
  .table-box {
    // Remove top margin since it's now inside a card
    
    :deep(.el-table) {
      border-radius: 12px;
      overflow: hidden;
      
      th {
        background-color: var(--el-fill-color-light);
        font-weight: 600;
        font-size: 14px;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
      
      td {
        font-size: 14px;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
    }
    
    :deep(.el-pagination) {
      margin-top: 20px;
      justify-content: center;
      
      .el-pager li {
        font-size: 14px;
        
        @media (min-width: 1920px) {
          font-size: 15px;
        }
      }
    }
  }
  
  // Title styling
  .large-title {
    font-size: 28px;
    font-weight: 700;
    margin-bottom: 24px;
    color: var(--el-text-color-primary);
    letter-spacing: -0.5px;
    
    @media (min-width: 1920px) {
      font-size: 32px;
      margin-bottom: 28px;
    }
  }
  
  .default-title {
    font-size: 20px;
    font-weight: 600;
    margin: 32px 0 20px 0;
    color: var(--el-text-color-primary);
    
    @media (min-width: 1920px) {
      font-size: 22px;
      margin: 36px 0 24px 0;
    }
  }
  
  // Button ellipsis
  ::v-deep .el-button span {
    display: inline-block;
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    height: 15px;
  }
</style>
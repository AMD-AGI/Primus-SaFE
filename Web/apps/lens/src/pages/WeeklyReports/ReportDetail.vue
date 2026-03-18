<template>
  <div class="report-detail-page">
    <div class="page-header">
      <el-button @click="goBack" :icon="ArrowLeft">Back to Reports</el-button>
      <h2 class="page-title" v-if="reportDetail">Report: {{ reportDetail.id }}</h2>
    </div>

    <div v-loading="loading" class="content-wrapper">
      <el-tabs v-if="reportDetail" v-model="activeTab">
        <el-tab-pane label="Overview" name="overview">

          <el-card class="metrics-card glass-card">
            <template #header>
              <span>Cluster Metrics</span>
            </template>
            <el-row :gutter="20">
              <el-col :span="8">
                <el-statistic
                  :value="reportDetail.metadata?.avgUtilization || 0"
                  suffix="%"
                  :value-style="{ color: getUtilizationColor(reportDetail.metadata?.avgUtilization) }"
                >
                  <template #title>
                    <span class="stat-title-with-tip">
                      Average Utilization
                      <el-tooltip
                        content="The average GPU utilization rate across all allocated resources during the reporting period"
                        placement="top"
                      >
                        <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
                      </el-tooltip>
                    </span>
                  </template>
                </el-statistic>
              </el-col>
              <el-col :span="8">
                <el-statistic
                  :value="reportDetail.metadata?.avgAllocation || 0"
                  suffix="%"
                  :value-style="{ color: getUtilizationColor(reportDetail.metadata?.avgAllocation) }"
                >
                  <template #title>
                    <span class="stat-title-with-tip">
                      Average Allocation
                      <el-tooltip
                        content="The average percentage of GPU resources that were allocated to users during the reporting period"
                        placement="top"
                      >
                        <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
                      </el-tooltip>
                    </span>
                  </template>
                </el-statistic>
              </el-col>
              <el-col :span="8">
                <el-statistic :value="reportDetail.metadata?.totalGpus || 0">
                  <template #title>
                    <span class="stat-title-with-tip">
                      Total GPUs
                      <el-tooltip
                        content="The total number of GPU devices available in the cluster"
                        placement="top"
                      >
                        <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
                      </el-tooltip>
                    </span>
                  </template>
                </el-statistic>
              </el-col>
            </el-row>
            <el-row :gutter="20" class="mt-4">
              <el-col :span="8">
                <el-statistic
                  :value="reportDetail.metadata?.lowUtilCount || 0"
                  :value-style="{ color: '#f56c6c' }"
                >
                  <template #title>
                    <span class="stat-title-with-tip">
                      Low Utilization Users
                      <el-tooltip
                        content="Number of users with GPU utilization below 30% during the reporting period"
                        placement="top"
                      >
                        <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
                      </el-tooltip>
                    </span>
                  </template>
                </el-statistic>
              </el-col>
              <el-col :span="8">
                <el-statistic
                  :value="reportDetail.metadata?.wastedGpuDays || 0"
                  :value-style="{ color: '#e6a23c' }"
                >
                  <template #title>
                    <span class="stat-title-with-tip">
                      Wasted GPU Days
                      <el-tooltip
                        content="Total GPU days wasted due to low utilization (below 30%)"
                        placement="top"
                      >
                        <el-icon class="stat-help-icon"><QuestionFilled /></el-icon>
                      </el-tooltip>
                    </span>
                  </template>
                </el-statistic>
              </el-col>
            </el-row>
          </el-card>

          <!-- Merge Trend Chart into Overview -->
          <el-card v-if="reportDetail.jsonContent?.chartData?.clusterUsageTrend" class="chart-card glass-card">
            <template #header>
              <span>Usage Trend</span>
            </template>
            <div ref="trendChartRef" class="trend-chart"></div>
          </el-card>
        </el-tab-pane>

        <el-tab-pane label="Report Content" name="markdown" v-if="reportDetail.jsonContent?.markdownReport">
          <el-card class="glass-card">
            <div class="markdown-content" v-html="parsedMarkdown"></div>
          </el-card>
        </el-tab-pane>

      </el-tabs>

      <el-empty v-else-if="!loading" description="No report data available" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Document, DocumentCopy, DataAnalysis, QuestionFilled } from '@element-plus/icons-vue'
import { getWeeklyReportDetail } from '@/services/weekly-reports'
import dayjs from 'dayjs'
import * as echarts from 'echarts'
import { marked } from 'marked'
import { useClusterSync } from '@/composables/useClusterSync'

const route = useRoute()
const router = useRouter()
const { selectedCluster, syncFromUrl, updateUrlWithCluster } = useClusterSync()

// Data refs
const loading = ref(false)
const reportDetail = ref<any>(null)
const activeTab = ref('overview')
const trendChartRef = ref<HTMLElement>()
let chartInstance: echarts.ECharts | null = null
const htmlContent = ref<string>('')
const htmlLoading = ref(false)

// Configure marked options
marked.setOptions({
  breaks: true, // Support line breaks
  gfm: true, // Support GitHub Flavored Markdown
  pedantic: false,
  renderer: new marked.Renderer()
})

// Computed property to parse markdown to HTML
const parsedMarkdown = computed(() => {
  if (!reportDetail.value?.jsonContent?.markdownReport) {
    return '<p>No content available</p>'
  }

  try {
    let markdownText = reportDetail.value.jsonContent.markdownReport

    // If content is wrapped in ```markdown code block, extract it
    if (typeof markdownText === 'string' && markdownText.includes('```markdown')) {
      // Remove leading ```markdown
      markdownText = markdownText.replace(/^```markdown\n?/, '')
      // Remove trailing ```
      markdownText = markdownText.replace(/\n?```$/, '')
    }

    // Handle chart placeholders {{CHART:xxx}}
    markdownText = markdownText.replace(/\{\{CHART:([^}]+)\}\}/g, (match: string, chartName: string) => {
      return `<div class="chart-placeholder">📊 Chart: ${chartName}</div>`
    })

    // Use marked to parse Markdown
    const htmlContent = marked.parse ? marked.parse(markdownText) : marked(markdownText)

    return htmlContent
  } catch (error) {
    console.error('Failed to parse markdown:', error)
    // If parsing fails, fall back to formatted raw text
    const escapedText = reportDetail.value.jsonContent.markdownReport
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
    return `<pre class="markdown-fallback">${escapedText}</pre>`
  }
})

// Fetch report detail
const fetchReportDetail = async () => {
  const reportId = route.params.id as string
  if (!reportId) {
    ElMessage.error('Report ID is missing')
    return
  }

  loading.value = true
  try {
    const response: any = await getWeeklyReportDetail(reportId)

    if (response) {
      reportDetail.value = response

      // Render chart after data is loaded
      if (response.jsonContent?.chartData?.clusterUsageTrend) {
        nextTick(() => {
          if (activeTab.value === 'overview') {
            renderTrendChart()
          }
        })
      }
    }
  } catch (error) {
    ElMessage.error('Failed to load report details')
    console.error('Error loading report:', error)
  } finally {
    loading.value = false
  }
}

// Render trend chart
const renderTrendChart = () => {
  if (!trendChartRef.value || !reportDetail.value?.jsonContent?.chartData?.clusterUsageTrend) return

  const chartData = reportDetail.value.jsonContent.chartData.clusterUsageTrend

  if (chartInstance) {
    chartInstance.dispose()
  }

  chartInstance = echarts.init(trendChartRef.value)

  const isDark = document.documentElement.classList.contains('dark')
  const textColor = isDark ? '#E5EAF3' : '#303133'
  const borderColor = isDark ? '#FFFFFF1A' : '#00000012'

  const option = {
    title: {
      text: chartData.title,
      left: 'center',
      textStyle: {
        fontSize: 16,
        fontWeight: 600,
        color: textColor
      }
    },
    tooltip: {
      trigger: 'axis',
      axisPointer: {
        type: 'cross'
      }
    },
    legend: {
      data: chartData.series.map((s: any) => s.name),
      bottom: 10,
      textStyle: { color: textColor }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '15%',
      top: '10%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: chartData.xAxis,
      axisLabel: {
        rotate: 45,
        interval: Math.floor(chartData.xAxis.length / 12),
        fontSize: 11,
        color: textColor
      },
      axisLine: { lineStyle: { color: borderColor } }
    },
    yAxis: {
      type: 'value',
      name: 'Percentage (%)',
      axisLabel: {
        formatter: '{value}%',
        color: textColor
      },
      axisLine: { lineStyle: { color: borderColor } },
      splitLine: { lineStyle: { color: borderColor } }
    },
    series: chartData.series.map((s: any, index: number) => ({
      ...s,
      smooth: true,
      lineStyle: {
        width: 2
      },
      areaStyle: {
        opacity: 0.1
      },
      color: index === 0 ? '#409eff' : '#67c23a'
    }))
  }

  chartInstance.setOption(option)

  // Resize on window resize
  const handleResize = () => chartInstance?.resize()
  window.addEventListener('resize', handleResize)
}

// Watch tab change to render chart or load HTML
watch(activeTab, (newVal) => {
  if (newVal === 'overview') {
    nextTick(() => {
      renderTrendChart()
    })
  } else if (newVal === 'html' && !htmlContent.value && reportDetail.value?.hasHtml) {
    fetchHtmlContent()
  }
})

// Fetch HTML content
const fetchHtmlContent = async () => {
  if (!reportDetail.value?.id) return

  htmlLoading.value = true
  try {
    const base = import.meta.env.BASE_URL
    const url = `${base}v1/weekly-reports/gpu_utilization/${reportDetail.value.id}/html`
    const response = await fetch(url)
    if (response.ok) {
      htmlContent.value = await response.text()
    } else {
      ElMessage.error('Failed to load HTML content')
    }
  } catch (error) {
    console.error('Error fetching HTML:', error)
    ElMessage.error('Failed to load HTML content')
  } finally {
    htmlLoading.value = false
  }
}

// Handle iframe load event
const handleIframeLoad = () => {
}

// Download report
const downloadReport = (format: 'html' | 'json' | 'pdf') => {
  if (!reportDetail.value) return

  const base = import.meta.env.BASE_URL
  const url = `${base}v1/weekly-reports/gpu_utilization/${reportDetail.value.id}/${format}`

  const a = document.createElement('a')
  a.href = url
  a.style.display = 'none'
  a.setAttribute('download', '')
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

// Go back to list
const goBack = () => {
  router.push({
    path: '/weekly-reports',
    query: selectedCluster.value ? { cluster: selectedCluster.value } : {}
  })
}

// Format datetime
const formatDateTime = (date: string) => {
  return dayjs(date).format('YYYY-MM-DD HH:mm:ss')
}

// Get status type for tag
const getStatusType = (status: string) => {
  const statusMap: Record<string, 'success' | 'info' | 'warning' | 'danger'> = {
    'generated': 'success',
    'pending': 'info',
    'generating': 'warning',
    'failed': 'danger'
  }
  return statusMap[status] || 'info'
}

// Get utilization color
const getUtilizationColor = (value?: number) => {
  if (!value) return '#909399'
  if (value < 30) return '#f56c6c'
  if (value < 60) return '#e6a23c'
  if (value < 80) return '#67c23a'
  return '#409eff'
}

// Lifecycle
onMounted(() => {
  // Sync cluster from URL to global state
  syncFromUrl()
  // Ensure URL contains cluster parameter
  updateUrlWithCluster()

  fetchReportDetail()
})
</script>

<style scoped lang="scss">
.report-detail-page {
  padding: 20px;
  position: relative;

  // Decorative background glow effect
  &::before {
    content: '';
    position: absolute;
    top: -50px;
    right: 15%;
    width: 450px;
    height: 450px;
    background: radial-gradient(circle, rgba(64, 158, 255, 0.07) 0%, transparent 70%);
    border-radius: 50%;
    pointer-events: none;
    z-index: 0;
  }

  &::after {
    content: '';
    position: absolute;
    bottom: 50px;
    left: 10%;
    width: 400px;
    height: 400px;
    background: radial-gradient(circle, rgba(103, 194, 58, 0.06) 0%, transparent 70%);
    border-radius: 50%;
    pointer-events: none;
    z-index: 0;
  }

  .page-header {
    display: flex;
    align-items: center;
    gap: 20px;
    margin-bottom: 20px;
    position: relative;
    z-index: 1;

    .page-title {
      margin: 0;
      font-size: 20px;
      font-weight: 600;
      background: linear-gradient(135deg, var(--el-text-color-primary) 0%, var(--el-text-color-regular) 100%);
      -webkit-background-clip: text;
      background-clip: text;
    }

    // Back button enhancement
    :deep(.el-button) {
      transition: all 0.3s ease;

      &:hover {
        transform: translateY(-2px);
        box-shadow: 0 4px 12px rgba(64, 158, 255, 0.2);
      }
    }
  }

  .content-wrapper {
    // height: 100%; // Subtract header and padding
    height: calc(100vh - 200px); // Subtract header and padding
    display: flex;
    flex-direction: column;
    position: relative;
    z-index: 1;

    :deep(.el-tabs) {
      height: 100%;
      display: flex;
      flex-direction: column;

      .el-tabs__header {
        flex-shrink: 0;
        margin-bottom: 16px;

        .el-tabs__nav-wrap {
          &::after {
            background: linear-gradient(
              90deg,
              transparent 0%,
              rgba(64, 158, 255, 0.1) 50%,
              transparent 100%
            );
          }
        }

        .el-tabs__item {
          font-weight: 500;
          transition: all 0.3s ease;

          &:hover {
            color: var(--el-color-primary);
            transform: translateY(-1px);
          }

          &.is-active {
            color: var(--el-color-primary);
            font-weight: 600;
          }
        }

        .el-tabs__active-bar {
          background: linear-gradient(90deg, var(--el-color-primary), var(--el-color-primary-light-3));
          height: 3px;
        }
      }

      .el-tabs__content {
        flex: 1;
        overflow: hidden;

        .el-tab-pane {
          // height: 100%;

          .el-card {
            // height: 100%;
            display: flex;
            flex-direction: column;

            .el-card__header {
              flex-shrink: 0;
            }

            .el-card__body {
              flex: 1;
              overflow: auto;
              padding: 20px;
            }
          }
        }
      }
    }
  }

  .detail-card,
  .metrics-card {
    margin-bottom: 20px;

    :deep(.el-card__header) {
      font-weight: 600;
      font-size: 16px;
      padding: 16px 20px;
      border-bottom: 1px solid rgba(64, 158, 255, 0.1);
    }

    // Statistics number enhancement effect
    :deep(.el-statistic) {
      .el-statistic__head {
        color: var(--el-text-color-regular);
        font-size: 13px;
        margin-bottom: 8px;
      }

      .el-statistic__content {
        .el-statistic__number {
          font-weight: 600;
          transition: all 0.3s ease;
        }
      }

      &:hover {
        .el-statistic__number {
          transform: scale(1.05);
        }
      }
    }

    // Statistics title and help icon styles
    .stat-title-with-tip {
      display: inline-flex;
      align-items: center;
      gap: 4px;

      .stat-help-icon {
        font-size: 14px;
        color: var(--el-text-color-secondary);
        cursor: help;
        transition: all 0.3s ease;

        &:hover {
          color: var(--el-color-primary);
          transform: scale(1.1);
        }
      }
    }

    .card-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
    }
  }

  // Glassmorphism card effect
  .glass-card {
    background: linear-gradient(135deg, rgba(255, 255, 255, 0.1) 0%, rgba(255, 255, 255, 0.05) 100%);
    backdrop-filter: blur(16px) saturate(180%);
    -webkit-backdrop-filter: blur(16px) saturate(180%);
    border-radius: 15px !important;
    border: 1px solid rgba(255, 255, 255, 0.18);
    box-shadow: 0 8px 32px rgba(17, 24, 39, 0.06);
    transition: all 0.3s ease;
    position: relative;
    overflow: hidden;

    // Colorful gradient top border
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
      box-shadow: 0 12px 40px rgba(64, 158, 255, 0.18);
      border-color: rgba(64, 158, 255, 0.35);

      &::before {
        opacity: 1;
      }
    }
  }

  .chart-card {
    margin-bottom: 20px;

    :deep(.el-card__header) {
      font-weight: 600;
      font-size: 16px;
      padding: 16px 20px;
      border-bottom: 1px solid rgba(64, 158, 255, 0.1);
    }

    :deep(.el-card__body) {
      padding: 20px;
    }
  }

  .mt-4 {
    margin-top: 24px;
  }

  .trend-chart {
    width: 100%;
    height: 400px;
    min-height: 400px;
    animation: fadeIn 0.6s ease-in-out;
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
      transform: translateY(10px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .markdown-content {
    height: calc(100vh - 350px);
    overflow: auto;
    padding: 24px;
    font-size: 14px;
    line-height: 1.8;
    color: var(--el-text-color-primary);
    background: linear-gradient(135deg, rgba(255, 255, 255, 0.02) 0%, rgba(255, 255, 255, 0.01) 100%);
    border-radius: 8px;

    // Heading styles
    :deep(h1) {
      font-size: 24px;
      font-weight: 600;
      margin: 24px 0 16px;
      padding-bottom: 8px;
      border-bottom: 1px solid var(--el-border-color-lighter);
    }

    :deep(h2) {
      font-size: 20px;
      font-weight: 600;
      margin: 20px 0 12px;
      padding-bottom: 6px;
      border-bottom: 1px solid var(--el-border-color-lighter);
    }

    :deep(h3) {
      font-size: 18px;
      font-weight: 600;
      margin: 16px 0 8px;
    }

    :deep(h4) {
      font-size: 16px;
      font-weight: 600;
      margin: 12px 0 6px;
    }

    // Paragraphs
    :deep(p) {
      margin: 12px 0;
      line-height: 1.8;
    }

    // Lists
    :deep(ul), :deep(ol) {
      margin: 12px 0;
      padding-left: 24px;

      li {
        margin: 6px 0;
        line-height: 1.6;
      }
    }

    // Tables
    :deep(table) {
      width: 100%;
      border-collapse: collapse;
      margin: 16px 0;

      th, td {
        padding: 10px 12px;
        border: 1px solid var(--el-border-color);
        text-align: left;
      }

      th {
        background: var(--el-fill-color-light);
        font-weight: 600;
      }

      tr:hover {
        background: var(--el-fill-color-lighter);
      }
    }

    // Code block
    :deep(pre) {
      background: var(--el-fill-color-darker);
      border: 1px solid var(--el-border-color);
      border-radius: 6px;
      padding: 16px;
      margin: 16px 0;
      overflow-x: auto;

      code {
        font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
        font-size: 13px;
        line-height: 1.6;
        color: var(--el-text-color-primary);
      }
    }

    // Inline code
    :deep(code:not(pre code)) {
      background: var(--el-fill-color);
      padding: 2px 6px;
      border-radius: 3px;
      font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
      font-size: 13px;
      color: var(--el-color-danger);
    }

    // Blockquotes
    :deep(blockquote) {
      margin: 16px 0;
      padding: 12px 20px;
      border-left: 4px solid var(--el-color-primary);
      background: var(--el-fill-color-lighter);

      p {
        margin: 0;
      }
    }

    // Emphasis
    :deep(strong) {
      font-weight: 600;
      color: var(--el-text-color-primary);
    }

    :deep(em) {
      font-style: italic;
    }

    // Links
    :deep(a) {
      color: var(--el-color-primary);
      text-decoration: none;

      &:hover {
        text-decoration: underline;
      }
    }

    // Horizontal rule
    :deep(hr) {
      margin: 24px 0;
      border: none;
      border-top: 1px solid var(--el-border-color-lighter);
    }

    // Images
    :deep(img) {
      max-width: 100%;
      height: auto;
      margin: 16px 0;
      border-radius: 6px;
    }

    // Chart placeholder (if any)
    :deep(.chart-placeholder) {
      background: var(--el-fill-color-lighter);
      padding: 40px;
      text-align: center;
      border-radius: 6px;
      margin: 16px 0;
      color: var(--el-text-color-secondary);
    }
  }

  .html-preview-container {
    width: 100%;
    height: calc(100vh - 300px);
    position: relative;
    display: flex;
    flex-direction: column;

    .html-iframe {
      width: 100%;
      flex: 1;
      border: none;
      background: white;
      border-radius: 4px;
    }

    // Dark mode support for iframe
    @media (prefers-color-scheme: dark) {
      .html-iframe {
        filter: invert(0.95) hue-rotate(180deg);
      }
    }
  }
}
</style>

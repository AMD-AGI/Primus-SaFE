<template>
  <el-text class="block textx-18 font-500" tag="b">LLM Gateway</el-text>
  <p class="mt-2 text-gray-500 text-sm">
    Manage your Azure APIM Key binding to enable LLM services.
  </p>

  <!-- Bound state -->
  <template v-if="binding?.has_apim_key">
    <el-card class="mt-4 safe-card" shadow="never">
      <div class="status-banner status-bound">
        <el-icon :size="20"><CircleCheckFilled /></el-icon>
        <span>APIM Key is bound</span>
      </div>

      <el-descriptions :column="2" border class="mt-6">
        <el-descriptions-item label="Email">
          {{ binding.user_email }}
        </el-descriptions-item>
        <el-descriptions-item label="Key Alias">
          {{ binding.key_alias || '-' }}
        </el-descriptions-item>
        <el-descriptions-item label="Created At">
          {{ formatTimeStr(binding.created_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="Updated At">
          {{ formatTimeStr(binding.updated_at) }}
        </el-descriptions-item>
      </el-descriptions>

      <el-divider />

      <el-text class="block font-500 mb-4" tag="b">Update APIM Key</el-text>
      <div class="key-input-row">
        <el-input
          v-model="apimKeyInput"
          placeholder="Enter new APIM Key"
          show-password
          clearable
          class="key-input"
        />
        <el-button
          type="primary"
          :loading="submitLoading"
          :disabled="!apimKeyInput.trim()"
          @click="handleUpdate"
        >
          Update
        </el-button>
      </div>
    </el-card>

    <!-- Usage Statistics -->
    <el-card class="mt-4 safe-card" shadow="never">
      <div class="flex items-center justify-between flex-wrap gap-3 mb-6">
        <el-text class="block font-500" tag="b">Usage Statistics</el-text>
        <div class="flex items-center gap-3">
          <el-radio-group v-model="quickRange" size="small" @change="onQuickRangeChange">
            <el-radio-button :value="7">7 Days</el-radio-button>
            <el-radio-button :value="14">14 Days</el-radio-button>
            <el-radio-button :value="30">30 Days</el-radio-button>
          </el-radio-group>
          <el-date-picker
            v-model="dateRange"
            type="daterange"
            range-separator="–"
            start-placeholder="Start"
            end-placeholder="End"
            size="small"
            value-format="YYYY-MM-DD"
            :clearable="false"
            @change="onDateRangeChange"
          />
        </div>
      </div>

      <!-- Summary cards -->
      <div class="summary-grid" v-loading="usageLoading">
        <div v-for="s in summaryCards" :key="s.label" class="summary-item">
          <div class="summary-value">{{ s.value }}</div>
          <div class="summary-label">{{ s.label }}</div>
        </div>
      </div>

      <!-- Daily spend chart -->
      <div class="mt-6">
        <el-text class="block font-500 mb-4" tag="b">Daily Spend</el-text>
        <div ref="chartRef" class="spend-chart" v-loading="usageLoading" />
      </div>

      <!-- Model breakdown table -->
      <div class="mt-6" v-if="modelRows.length">
        <el-text class="block font-500 mb-4" tag="b">Model Breakdown</el-text>
        <el-table :data="modelRows" size="small" stripe>
          <el-table-column prop="model" label="Model" min-width="160" />
          <el-table-column prop="spend" label="Spend (USD)" width="140" align="right">
            <template #default="{ row }">
              ${{ row.spend.toFixed(2) }}
            </template>
          </el-table-column>
          <el-table-column prop="prompt_tokens" label="Prompt Tokens" width="150" align="right">
            <template #default="{ row }">
              {{ fmtNum(row.prompt_tokens) }}
            </template>
          </el-table-column>
          <el-table-column
            prop="completion_tokens"
            label="Completion Tokens"
            width="170"
            align="right"
          >
            <template #default="{ row }">
              {{ fmtNum(row.completion_tokens) }}
            </template>
          </el-table-column>
          <el-table-column prop="api_requests" label="Requests" width="120" align="right">
            <template #default="{ row }">
              {{ fmtNum(row.api_requests) }}
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>

    <!-- Code example -->
    <el-card class="mt-4 safe-card" shadow="never">
      <el-text class="block font-500 mb-2" tag="b">Quick Start</el-text>
      <el-text class="block mb-4 text-sm text-gray-500">
        Use any SaFE API Key to call the LLM:
      </el-text>
      <div class="code-block">
        <pre><code>from openai import OpenAI

client = OpenAI(
    api_key="ak-&lt;your-safe-key&gt;",
    base_url="{{ llmGatewayBaseUrl }}"
)

response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)</code></pre>
      </div>
    </el-card>
  </template>

  <!-- Unbound / error fallback state -->
  <el-card
    v-else
    class="mt-4 safe-card gateway-card"
    shadow="never"
    v-loading="pageLoading"
  >
    <div class="gateway-center">
      <div class="gateway-content">
        <template v-if="!pageLoading">
          <div v-if="binding" class="status-banner status-unbound">
            <el-icon :size="20"><WarningFilled /></el-icon>
            <span>APIM Key is not bound</span>
          </div>

          <el-descriptions v-if="binding" :column="1" border class="mt-6">
            <el-descriptions-item label="Email">
              {{ binding.user_email }}
            </el-descriptions-item>
          </el-descriptions>

          <el-empty v-else description="Unable to load binding status" :image-size="80" />

          <el-divider />

          <el-text class="block font-500 mb-4" tag="b">Bind APIM Key</el-text>
          <el-text class="block mb-4 text-sm text-gray-500">
            Please upload your Azure APIM Subscription Key to enable LLM services.
          </el-text>
          <div class="key-input-row">
            <el-input
              v-model="apimKeyInput"
              placeholder="Enter your APIM Key"
              show-password
              clearable
              class="key-input"
            />
            <el-button
              type="primary"
              :loading="submitLoading"
              :disabled="!apimKeyInput.trim()"
              @click="handleBind"
            >
              Bind
            </el-button>
          </div>
        </template>
      </div>
    </div>
  </el-card>
</template>

<script lang="ts" setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import {
  getLLMGatewayBinding,
  createLLMGatewayBinding,
  updateLLMGatewayBinding,
  getLLMGatewayUsage,
} from '@/services'
import type { LLMGatewayBinding, LLMGatewayUsage } from '@/services'
import { formatTimeStr } from '@/utils/index'
import { ElMessage } from 'element-plus'
import { CircleCheckFilled, WarningFilled } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import * as echarts from 'echarts/core'
import { BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
} from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([BarChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

defineOptions({ name: 'LLMGatewayPage' })

const pageLoading = ref(false)
const submitLoading = ref(false)
const binding = ref<LLMGatewayBinding | null>(null)
const apimKeyInput = ref('')

const llmGatewayBaseUrl = `${location.origin}/llm-gateway/v1`

// ── Usage ──
const usageLoading = ref(false)
const usage = ref<LLMGatewayUsage | null>(null)
const quickRange = ref(7)
const dateRange = ref<[string, string]>([
  dayjs().subtract(6, 'day').format('YYYY-MM-DD'),
  dayjs().format('YYYY-MM-DD'),
])

const chartRef = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

const fmtNum = (n: number) => n?.toLocaleString('en-US') ?? '0'
const fmtCompact = (n: number) => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

const summaryCards = computed(() => {
  const u = usage.value
  if (!u) {
    return [
      { label: 'Total Spend', value: '$0.00' },
      { label: 'Total Requests', value: '0' },
      { label: 'Total Tokens', value: '0' },
      { label: 'Models Used', value: '0' },
    ]
  }
  const modelSet = new Set<string>()
  u.daily?.forEach((d) => Object.keys(d.models ?? {}).forEach((m) => modelSet.add(m)))
  return [
    { label: 'Total Spend', value: `$${u.total_spend?.toFixed(2) ?? '0.00'}` },
    { label: 'Total Requests', value: fmtCompact(u.total_api_requests ?? 0) },
    { label: 'Total Tokens', value: fmtCompact(u.total_tokens ?? 0) },
    { label: 'Models Used', value: String(modelSet.size) },
  ]
})

const modelRows = computed(() => {
  if (!usage.value?.daily) return []
  const map: Record<string, { spend: number; prompt_tokens: number; completion_tokens: number; api_requests: number }> = {}
  usage.value.daily.forEach((d) => {
    for (const [name, m] of Object.entries(d.models ?? {})) {
      if (!map[name]) map[name] = { spend: 0, prompt_tokens: 0, completion_tokens: 0, api_requests: 0 }
      map[name].spend += m.spend
      map[name].prompt_tokens += m.prompt_tokens
      map[name].completion_tokens += m.completion_tokens
      map[name].api_requests += m.api_requests
    }
  })
  return Object.entries(map)
    .map(([model, v]) => ({ model, ...v }))
    .sort((a, b) => b.spend - a.spend)
})

const onQuickRangeChange = (days: number) => {
  dateRange.value = [
    dayjs().subtract(days - 1, 'day').format('YYYY-MM-DD'),
    dayjs().format('YYYY-MM-DD'),
  ]
  fetchUsage()
}

const onDateRangeChange = (val: [string, string] | null) => {
  if (!val) return
  quickRange.value = 0 as never
  fetchUsage()
}

const fetchBinding = async () => {
  try {
    pageLoading.value = true
    binding.value = await getLLMGatewayBinding()
  } catch {
    binding.value = null
  } finally {
    pageLoading.value = false
  }
}

const fetchUsage = async () => {
  if (!binding.value?.has_apim_key) return
  try {
    usageLoading.value = true
    usage.value = await getLLMGatewayUsage({
      start_date: dateRange.value[0],
      end_date: dateRange.value[1],
    })
    await nextTick()
    renderChart()
  } catch {
    usage.value = null
  } finally {
    usageLoading.value = false
  }
}

const renderChart = () => {
  if (!chartRef.value) return
  if (!chart) {
    chart = echarts.init(chartRef.value)
    window.addEventListener('resize', resizeChart)
  }
  const daily = usage.value?.daily ?? []
  const isDark = document.documentElement.classList.contains('dark')

  chart.setOption({
    tooltip: {
      trigger: 'axis',
      formatter: (params: { name: string; value: number }[]) => {
        const p = params[0]
        return `${p.name}<br/>Spend: <b>$${p.value.toFixed(2)}</b>`
      },
    },
    grid: { top: 16, right: 16, bottom: 32, left: 60, containLabel: false },
    xAxis: {
      type: 'category',
      data: daily.map((d) => d.date),
      axisLabel: { color: isDark ? '#aaa' : '#666', fontSize: 11 },
      axisLine: { lineStyle: { color: isDark ? '#444' : '#ddd' } },
    },
    yAxis: {
      type: 'value',
      axisLabel: {
        color: isDark ? '#aaa' : '#666',
        fontSize: 11,
        formatter: (v: number) => `$${v}`,
      },
      splitLine: { lineStyle: { color: isDark ? '#333' : '#eee' } },
    },
    series: [
      {
        type: 'bar',
        data: daily.map((d) => d.spend),
        itemStyle: { color: '#409eff', borderRadius: [4, 4, 0, 0] },
        barMaxWidth: 36,
      },
    ],
  })
  chart.resize()
}

const resizeChart = () => chart?.resize()

const handleBind = async () => {
  const key = apimKeyInput.value.trim()
  if (!key) return
  try {
    submitLoading.value = true
    await createLLMGatewayBinding({ apim_key: key })
    ElMessage.success('APIM Key bound successfully')
    apimKeyInput.value = ''
    await fetchBinding()
  } catch (err: unknown) {
    if (typeof err === 'string' && err.includes('already exists')) {
      ElMessage.warning('Already bound. Please use the Update function.')
    }
  } finally {
    submitLoading.value = false
  }
}

const handleUpdate = async () => {
  const key = apimKeyInput.value.trim()
  if (!key) return
  try {
    submitLoading.value = true
    await updateLLMGatewayBinding({ apim_key: key })
    ElMessage.success('APIM Key updated successfully')
    apimKeyInput.value = ''
    await fetchBinding()
  } catch (err: unknown) {
    if (typeof err === 'string' && err.includes('no binding found')) {
      ElMessage.warning('Not bound yet. Please bind first.')
    }
  } finally {
    submitLoading.value = false
  }
}

watch(
  () => binding.value?.has_apim_key,
  (bound) => {
    if (bound) fetchUsage()
  },
)

onMounted(() => {
  fetchBinding()
})

onBeforeUnmount(() => {
  chart?.dispose()
  window.removeEventListener('resize', resizeChart)
})
</script>

<style scoped>
/* Unbound state card */
.gateway-card {
  min-height: calc(100vh - 180px);
}
.gateway-card :deep(.el-card__body) {
  height: 100%;
  display: flex;
}
.gateway-center {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 0;
}
.gateway-content {
  width: 100%;
  max-width: 720px;
}

/* Status banners */
.status-banner {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 20px;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 500;
}
.status-bound {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success);
}
.status-unbound {
  background: var(--el-color-warning-light-9);
  color: var(--el-color-warning-dark-2);
}

/* Key input */
.key-input-row {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.key-input {
  flex: 1;
  max-width: 480px;
}

/* Summary cards */
.summary-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
}
.summary-item {
  padding: 20px;
  border-radius: 8px;
  background: var(--el-fill-color-light);
  text-align: center;
}
.summary-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  line-height: 1.2;
}
.summary-label {
  margin-top: 6px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

/* Chart */
.spend-chart {
  width: 100%;
  height: 280px;
}

/* Code block */
.code-block {
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 16px;
  overflow-x: auto;
}
.code-block pre {
  margin: 0;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
}
</style>

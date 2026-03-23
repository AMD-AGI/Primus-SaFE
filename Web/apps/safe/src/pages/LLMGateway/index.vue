<template>
  <el-text class="block textx-18 font-500" tag="b">LLM Gateway</el-text>
  <p class="mt-2 text-gray-500 text-sm">
    Manage your AMD LLM Subscription Key binding to enable LLM services.
  </p>

  <!-- Bound state -->
  <template v-if="binding?.has_apim_key">
    <el-card class="mt-4 safe-card" shadow="never">
      <div class="status-banner status-bound">
        <el-icon :size="20"><CircleCheckFilled /></el-icon>
        <span>AMD LLM Subscription Key is bound</span>
      </div>

      <el-descriptions :column="2" border class="mt-6">
        <el-descriptions-item label="Email">
          {{ binding.user_email }}
        </el-descriptions-item>
        <el-descriptions-item label="Summary Usage">
          <span v-if="summary">
            ${{ summary.total_spend?.toFixed(5) ?? '0.00000' }}
          </span>
          <el-text v-else type="info">-</el-text>
        </el-descriptions-item>
        <el-descriptions-item label="Created At">
          {{ formatTimeStr(binding.created_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="Updated At">
          {{ formatTimeStr(binding.updated_at) }}
        </el-descriptions-item>
      </el-descriptions>

      <el-divider />

      <el-text class="block font-500 mb-4" tag="b">Update AMD LLM Subscription Key</el-text>
      <div class="key-input-row">
        <el-input
          v-model="apimKeyInput"
          type="password"
          :placeholder="binding?.apim_key_hint ? `Current: ${binding.apim_key_hint}` : 'Enter new AMD LLM Subscription Key'"
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
        <div class="flex items-center gap-3">
          <h3 class="section-title">Usage View</h3>
          <el-dropdown trigger="click" @command="onUsageViewChange">
            <el-button size="small" round type="success">
              <el-icon class="mr-1"><PriceTag /></el-icon>
              {{ usageView === 'user' ? 'User Usage' : 'Tag Usage' }}
              <el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="user" :class="{ 'is-active-item': usageView === 'user' }">User Usage</el-dropdown-item>
                <el-dropdown-item command="tag" :class="{ 'is-active-item': usageView === 'tag' }">Tag Usage</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
        <div class="flex items-center gap-3">
          <el-select
            v-if="usageView === 'tag'"
            v-model="selectedTag"
            placeholder="Filter by tag..."
            clearable
            size="small"
            style="width: 200px"
            @change="onTagFilterChange"
          >
            <el-option label="All Tags" value="" />
            <el-option label="(Untagged)" value="__untagged__" />
            <el-option
              v-for="t in availableTagNames"
              :key="t"
              :label="t"
              :value="t"
            />
          </el-select>
          <span class="text-sm text-gray-400">Select Time Range</span>
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
      <h4 class="sub-title mb-4">Usage Statistics</h4>
      <div class="summary-grid" v-loading="usageLoading">
        <div v-for="s in activeSummaryCards" :key="s.label" class="summary-card">
          <div class="summary-icon" :style="{ color: s.color }">
            <el-icon :size="18"><component :is="s.icon" /></el-icon>
          </div>
          <div class="summary-body">
            <div class="summary-value">{{ s.value }}</div>
            <div class="summary-label">{{ s.label }}</div>
          </div>
        </div>
      </div>

      <!-- Daily spend chart -->
      <div class="mt-6">
        <h4 class="sub-title mb-4">Daily Spend</h4>
        <div ref="chartRef" class="spend-chart" v-loading="usageLoading" />
      </div>

      <!-- Model breakdown table (User Usage) -->
      <div class="mt-6" v-if="usageView === 'user' && modelRows.length">
        <div class="flex items-center gap-3 mb-4">
          <h4 class="sub-title">Model Breakdown</h4>
          <el-select v-model="selectedDate" size="small" style="width: 160px">
            <el-option
              v-for="d in dateOptions"
              :key="d"
              :label="d"
              :value="d"
            />
          </el-select>
        </div>
        <el-table :data="modelRows" size="default" stripe class="model-table">
          <el-table-column prop="model" label="Model" min-width="180" />
          <el-table-column prop="spend" label="Spend (USD)" width="130" align="right">
            <template #default="{ row }">
              ${{ row.spend.toFixed(5) }}
            </template>
          </el-table-column>
          <el-table-column prop="total_tokens" label="Tokens" width="120" align="right">
            <template #default="{ row }">
              {{ fmtCompact(row.total_tokens) }}
            </template>
          </el-table-column>
          <el-table-column
            prop="successful_requests"
            label="Successful"
            width="120"
            align="right"
          >
            <template #default="{ row }">
              {{ fmtNum(row.successful_requests) }}
            </template>
          </el-table-column>
          <el-table-column prop="failed_requests" label="Failed" width="100" align="right">
            <template #default="{ row }">
              <el-text :type="row.failed_requests > 0 ? 'danger' : ''">
                {{ fmtNum(row.failed_requests) }}
              </el-text>
            </template>
          </el-table-column>
          <el-table-column prop="api_requests" label="Total" width="100" align="right">
            <template #default="{ row }">
              {{ fmtNum(row.api_requests) }}
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- Tag breakdown table (Tag Usage) -->
      <div class="mt-6" v-if="usageView === 'tag' && tagRows.length">
        <h4 class="sub-title mb-4">Tag Breakdown</h4>
        <el-table :data="tagRows" size="default" stripe class="model-table">
          <el-table-column prop="tag_name" label="TAG" min-width="180">
            <template #default="{ row }">
              <el-link v-if="row.tag_name" type="primary" :underline="false" @click="onTagFilterChange(row.tag_name)">
                {{ row.tag_name }}
              </el-link>
              <el-text v-else type="info">(Untagged)</el-text>
            </template>
          </el-table-column>
          <el-table-column prop="spend" label="SPEND" width="140" align="right">
            <template #default="{ row }">
              <el-text type="success">${{ row.spend.toFixed(5) }}</el-text>
            </template>
          </el-table-column>
          <el-table-column label="TOKENS" width="140" align="right">
            <template #default="{ row }">
              {{ fmtCompact(row.prompt_tokens + row.completion_tokens) }}
            </template>
          </el-table-column>
          <el-table-column prop="api_requests" label="REQUESTS" width="120" align="right">
            <template #default="{ row }">
              {{ fmtNum(row.api_requests) }}
            </template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>

    <!-- Code examples -->
    <el-card class="mt-4 safe-card" shadow="never">
      <h3 class="section-title" style="margin-bottom: 14px">Quick Start</h3>

      <el-tabs v-model="activeTab">
        <el-tab-pane label="SaFE API Key" name="safeKey">
          <div class="code-block">
            <el-icon class="copy-btn" @click="copyText(codeSnippets.safeKey)"><CopyDocument /></el-icon>
            <pre><code>{{ codeSnippets.safeKey }}</code></pre>
          </div>
        </el-tab-pane>

        <el-tab-pane label="LLM Virtual Key" name="virtualKey">
          <div class="code-block">
            <el-icon class="copy-btn" @click="copyText(codeSnippets.virtualKey)"><CopyDocument /></el-icon>
            <pre><code>{{ codeSnippets.virtualKey }}</code></pre>
          </div>
        </el-tab-pane>

        <el-tab-pane label="Certificates Setup" name="certs">
          <el-text class="block font-500 mb-2 text-xs" type="info">
            Linux (SaFE Authoring / Remote SSH) — requires root user
          </el-text>
          <div class="code-block mb-4">
            <el-icon class="copy-btn" @click="copyText(codeSnippets.linux)"><CopyDocument /></el-icon>
            <pre><code>{{ codeSnippets.linux }}</code></pre>
          </div>

          <el-text class="block font-500 mb-2 text-xs" type="info">
            Windows (PowerShell as Administrator)
          </el-text>
          <div class="code-block">
            <el-icon class="copy-btn" @click="copyText(codeSnippets.windows)"><CopyDocument /></el-icon>
            <pre><code>{{ codeSnippets.windows }}</code></pre>
          </div>
        </el-tab-pane>
      </el-tabs>
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
            <span>AMD LLM Subscription Key is not bound</span>
          </div>

          <el-descriptions v-if="binding" :column="1" border class="mt-6">
            <el-descriptions-item label="Email">
              {{ binding.user_email }}
            </el-descriptions-item>
          </el-descriptions>

          <el-empty v-else description="Unable to load binding status" :image-size="80" />

          <el-divider />

          <el-text class="block font-500 mb-4" tag="b">Bind AMD LLM Subscription Key</el-text>
          <el-text class="block mb-4 text-sm text-gray-500">
            Please upload your AMD LLM Subscription Key to enable LLM services.
          </el-text>
          <div class="key-input-row">
            <el-input
              v-model="apimKeyInput"
              type="password"
              placeholder="Enter your AMD LLM Subscription Key"
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

  <!-- Virtual Key Success Dialog -->
  <el-dialog
    v-model="virtualKeyVisible"
    title="LLM Virtual Key Created Successfully"
    width="600"
    :close-on-click-modal="false"
  >
    <el-alert
      title="Important: Save your LLM Virtual Key"
      type="warning"
      :closable="false"
      show-icon
      class="mb-4"
    >
      <template #default>
        <div class="text-sm">
          This is the only time you will see this Key. Please save it securely.
        </div>
      </template>
    </el-alert>

    <el-form label-width="auto" class="p-3">
      <el-form-item label="LLM Virtual Key">
        <div class="flex items-center gap-2 w-full">
          <el-input :model-value="createdVirtualKey" readonly class="font-mono">
            <template #append>
              <el-button :icon="CopyDocument" @click="copyVirtualKey" />
            </template>
          </el-input>
        </div>
      </el-form-item>
    </el-form>
  </el-dialog>
</template>

<script lang="ts" setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import {
  getLLMGatewayBinding,
  createLLMGatewayBinding,
  updateLLMGatewayBinding,
  getLLMGatewayUsage,
  getLLMGatewaySummary,
  getLLMGatewayTagUsage,
} from '@/services'
import type { LLMGatewayBinding, LLMGatewayUsage, LLMGatewaySummary, LLMGatewayTagUsage, LLMGatewayTagItem, LLMGatewayTagUsageParams } from '@/services'
import { formatTimeStr, copyText } from '@/utils/index'
import { ElMessage } from 'element-plus'
import {
  CircleCheckFilled,
  WarningFilled,
  CopyDocument,
  Money,
  Connection,
  Coin,
  Grid,
  PriceTag,
  ArrowDown,
  SuccessFilled,
  CircleCloseFilled,
} from '@element-plus/icons-vue'
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

const activeTab = ref('safeKey')

const codeSnippets = {
  safeKey: `from openai import OpenAI
import httpx

http_client = httpx.Client(verify=False)

client = OpenAI(
    api_key="ak-<your-safe-apikey>",
    base_url="https://oci-slc.primus-safe.amd.com/api/v1/llm-proxy/v1",
    http_client=http_client,
)

models = client.models.list()
for model in models.data:
    print(model.id)

response = client.chat.completions.create(
    model="openai/gpt-5.2",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)`,
  virtualKey: `from openai import OpenAI
import httpx

http_client = httpx.Client(verify=False)

client = OpenAI(
    api_key="sk-<your-llm-virtual-key>",
    base_url="https://project1.tw325.primus-safe.amd.com/llm-gateway/v1",
    http_client=http_client,
)

models = client.models.list()
for model in models.data:
    print(model.id)

response = client.chat.completions.create(
    model="openai/gpt-5.2",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)`,
  linux: `curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.sh | bash`,
  windows: `irm https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.bat -OutFile $env:TEMP\\setup.bat; cmd /c $env:TEMP\\setup.bat`,
}

// ── Virtual Key Dialog ──
const virtualKeyVisible = ref(false)
const createdVirtualKey = ref('')

const copyVirtualKey = () => {
  copyText(createdVirtualKey.value)
}

// ── Summary ──
const summary = ref<LLMGatewaySummary | null>(null)

const fetchSummary = async () => {
  try {
    summary.value = await getLLMGatewaySummary()
  } catch {
    summary.value = null
  }
}

// ── Usage View Toggle ──
type UsageViewType = 'user' | 'tag'
const usageView = ref<UsageViewType>('tag')

const onUsageViewChange = (view: UsageViewType) => {
  usageView.value = view
  fetchCurrentUsage()
}

// ── Usage ──
const usageLoading = ref(false)
const usage = ref<LLMGatewayUsage | null>(null)
const dateRange = ref<[string, string]>([
  dayjs().subtract(6, 'day').format('YYYY-MM-DD'),
  dayjs().format('YYYY-MM-DD'),
])

const chartRef = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

const generateDateRange = (start: string, end: string): string[] => {
  const dates: string[] = []
  let cur = dayjs(start)
  const endDay = dayjs(end)
  while (cur.isBefore(endDay) || cur.isSame(endDay, 'day')) {
    dates.push(cur.format('YYYY-MM-DD'))
    cur = cur.add(1, 'day')
  }
  return dates
}

const fmtNum = (n: number) => n?.toLocaleString('en-US') ?? '0'
const fmtCompact = (n: number) => {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

const userSummaryCards = computed(() => {
  const u = usage.value
  const modelCount = u?.daily
    ? new Set(u.daily.flatMap((d) => Object.keys(d.models ?? {}))).size
    : 0
  return [
    { label: 'Total Spend', value: `$${u?.total_spend?.toFixed(5) ?? '0.00000'}`, icon: Money, color: '#10b981' },
    { label: 'Total Requests', value: fmtCompact(u?.total_api_requests ?? 0), icon: Connection, color: '#3b82f6' },
    { label: 'Total Tokens', value: fmtCompact(u?.total_tokens ?? 0), icon: Coin, color: '#f59e0b' },
    { label: 'Models Used', value: String(modelCount), icon: Grid, color: '#8b5cf6' },
  ]
})

// ── Tag Usage ──
const tagUsage = ref<LLMGatewayTagUsage | null>(null)
const selectedTag = ref('')
const availableTagNames = ref<string[]>([])

const tagSummaryCards = computed(() => {
  const t = tagUsage.value
  return [
    { label: 'Total Spend', value: `$${t?.total_spend?.toFixed(5) ?? '0.00000'}`, icon: Money, color: '#10b981' },
    { label: 'Total Requests', value: fmtCompact(t?.total_requests ?? 0), icon: Connection, color: '#3b82f6' },
    { label: 'Successful Requests', value: fmtCompact(t?.total_successful_requests ?? 0), icon: SuccessFilled, color: '#10b981' },
    { label: 'Failed Requests', value: fmtCompact(t?.total_failed_requests ?? 0), icon: CircleCloseFilled, color: '#ef4444' },
    { label: 'Total Tokens', value: fmtCompact(t?.total_tokens ?? 0), icon: Coin, color: '#f59e0b' },
  ]
})

const tagRows = computed<LLMGatewayTagItem[]>(() => {
  return (tagUsage.value?.tags ?? []).map((t) => ({
    ...t,
    tag_name: t.tag_name,
  })).sort((a, b) => b.spend - a.spend)
})

const activeSummaryCards = computed(() =>
  usageView.value === 'tag' ? tagSummaryCards.value : userSummaryCards.value,
)

interface ModelRow {
  model: string
  spend: number
  total_tokens: number
  api_requests: number
  successful_requests: number
  failed_requests: number
}

const selectedDate = ref('')

const dateOptions = computed(() => {
  if (!usage.value?.daily) return []
  return usage.value.daily.map((d) => d.date).sort((a, b) => b.localeCompare(a))
})

watch(dateOptions, (opts) => {
  if (opts.length && !opts.includes(selectedDate.value)) {
    selectedDate.value = opts[0]
  }
})

const modelRows = computed<ModelRow[]>(() => {
  if (!usage.value?.daily || !selectedDate.value) return []
  const day = usage.value.daily.find((d) => d.date === selectedDate.value)
  if (!day?.models) return []
  return Object.entries(day.models)
    .map(([model, m]) => ({
      model,
      spend: m.spend,
      total_tokens: (m.prompt_tokens ?? 0) + (m.completion_tokens ?? 0),
      api_requests: m.api_requests,
      successful_requests: m.successful_requests ?? 0,
      failed_requests: m.failed_requests ?? 0,
    }))
    .sort((a, b) => b.spend - a.spend)
})

const onDateRangeChange = (val: [string, string] | null) => {
  if (!val) return
  fetchCurrentUsage()
}

const onTagFilterChange = (tag: string) => {
  selectedTag.value = tag
  fetchTagUsage()
}

const fetchTagUsage = async () => {
  if (!binding.value?.has_apim_key) return
  try {
    usageLoading.value = true
    const params: LLMGatewayTagUsageParams = {
      start_date: dateRange.value[0],
      end_date: dateRange.value[1],
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      ...(selectedTag.value ? { tag: selectedTag.value } : {}),
    }
    tagUsage.value = await getLLMGatewayTagUsage(params)

    if (!selectedTag.value && tagUsage.value?.tags) {
      availableTagNames.value = tagUsage.value.tags
        .map((t) => t.tag_name)
        .filter((n): n is string => n !== null)
    }

    await nextTick()
    renderChart()
  } catch {
    tagUsage.value = null
  } finally {
    usageLoading.value = false
  }
}

const fetchCurrentUsage = () => {
  if (usageView.value === 'tag') {
    fetchTagUsage()
  } else {
    fetchUsage()
  }
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
  const rawDaily = usageView.value === 'tag'
    ? (tagUsage.value?.daily ?? [])
    : [...(usage.value?.daily ?? [])].reverse()

  const allDates = generateDateRange(dateRange.value[0], dateRange.value[1])
  const spendMap = new Map(rawDaily.map((d) => [d.date, d.spend]))
  const daily = allDates.map((date) => ({ date, spend: spendMap.get(date) ?? 0 }))

  const isDark = document.documentElement.classList.contains('dark')

  chart.setOption({
    tooltip: {
      trigger: 'axis',
      formatter: (params: { name: string; value: number }[]) => {
        const p = params[0]
        return `${p.name}<br/>Spend: <b>$${p.value.toFixed(5)}</b>`
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
    const res = await createLLMGatewayBinding({ apim_key: key })
    ElMessage.success('AMD LLM Subscription Key bound successfully')
    apimKeyInput.value = ''

    if (res?.virtual_key) {
      createdVirtualKey.value = res.virtual_key
      virtualKeyVisible.value = true
    }

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
    const res = await updateLLMGatewayBinding({ apim_key: key })
    ElMessage.success('AMD LLM Subscription Key updated successfully')
    apimKeyInput.value = ''

    if (res?.virtual_key) {
      createdVirtualKey.value = res.virtual_key
      virtualKeyVisible.value = true
    }

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
    if (bound) {
      fetchCurrentUsage()
      fetchSummary()
    }
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
/* Section titles */
.section-title {
  font-size: 17px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin: 0;
}
.sub-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
  margin: 0;
}

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

/* Summary cards – glass + tilt */
.summary-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
}
.summary-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 20px;
  border-radius: 12px;
  border: 1px solid var(--el-border-color-lighter);
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.6),
    rgba(255, 255, 255, 0.15)
  );
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.04);
  transition:
    transform 0.3s ease,
    box-shadow 0.3s ease;
}
.dark .summary-card {
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.06),
    rgba(255, 255, 255, 0.02)
  );
}
.summary-card:hover {
  transform: perspective(600px) rotateX(-2deg) rotateY(3deg) translateY(-4px);
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.10);
}
.summary-icon {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, currentColor 12%, transparent);
}
.summary-body {
  min-width: 0;
}
.summary-value {
  font-size: 22px;
  font-weight: 700;
  color: var(--el-text-color-primary);
  line-height: 1.2;
}
.summary-label {
  margin-top: 2px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* Chart */
.spend-chart {
  width: 100%;
  height: 280px;
}

/* Model table – scale up for large screens */
.model-table {
  font-size: 14px;
}
@media (min-width: 1600px) {
  .model-table {
    font-size: 15px;
  }
  .model-table :deep(.el-table__header th) {
    font-size: 15px;
    padding: 14px 0;
  }
  .model-table :deep(.el-table__body td) {
    padding: 14px 0;
  }
}
@media (min-width: 1920px) {
  .model-table {
    font-size: 16px;
  }
  .model-table :deep(.el-table__header th) {
    font-size: 16px;
    padding: 16px 0;
  }
  .model-table :deep(.el-table__body td) {
    padding: 16px 0;
  }
}

/* Code block */
.code-block {
  position: relative;
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 16px;
  overflow-x: auto;
}
.copy-btn {
  position: absolute;
  top: 10px;
  right: 10px;
  font-size: 15px;
  cursor: pointer;
  color: var(--el-text-color-placeholder);
  transition: color 0.2s;
}
.copy-btn:hover {
  color: var(--el-color-primary);
}
.code-block pre {
  margin: 0;
  font-family: 'Cascadia Code', 'Fira Code', Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
}
</style>
<style>
.is-active-item {
  color: var(--el-color-success) !important;
  font-weight: 600;
}
</style>

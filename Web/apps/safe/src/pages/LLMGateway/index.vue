<template>
  <el-text class="block textx-18 font-500" tag="b">LLM Gateway</el-text>
  <p class="mt-2 text-gray-500 text-sm">
    Manage your AMD LLM API Key binding to enable LLM services.
  </p>

  <!-- Bound state -->
  <template v-if="binding?.has_apim_key">
    <el-card class="mt-4 safe-card" shadow="never">
      <div class="status-banner status-bound">
        <el-icon :size="20"><CircleCheckFilled /></el-icon>
        <span>AMD LLM API Key is bound</span>
      </div>

      <div class="bound-columns mt-6">
        <!-- Left: Binding info + update key -->
        <div class="bound-col-left">
          <el-descriptions :column="1" border class="mb-6">
            <el-descriptions-item label="Email">
              {{ binding.user_email }}
            </el-descriptions-item>
            <el-descriptions-item label="Created At">
              {{ formatTimeStr(binding.created_at) }}
            </el-descriptions-item>
            <el-descriptions-item label="Updated At">
              {{ formatTimeStr(binding.updated_at) }}
            </el-descriptions-item>
          </el-descriptions>

          <el-text class="block font-500 mb-4" tag="b">Update AMD LLM API Key</el-text>
          <div class="key-input-row">
            <el-input
              v-model="apimKeyInput"
              type="password"
              :placeholder="binding?.apim_key_hint ? `Current: ${binding.apim_key_hint}` : 'Enter new AMD LLM API Key'"
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
        </div>

        <!-- Right: Summary Usage + Budget -->
        <div class="bound-col-right">
          <div class="budget-header">
            <span class="budget-title">Summary Usage</span>
            <span class="budget-amount">${{ summary?.total_spend?.toFixed(5) ?? '0.00000' }}</span>
          </div>

          <fieldset class="budget-fieldset" v-loading="budgetLoading">
            <legend>Budget</legend>
            <template v-if="budget">
              <div class="budget-stats">
                <div class="budget-stat">
                  <span class="budget-stat-label">Spend</span>
                  <span class="budget-stat-value">${{ budget.spend?.toFixed(2) ?? '0.00' }}</span>
                </div>
                <div class="budget-stat">
                  <span class="budget-stat-label">Budget</span>
                  <span class="budget-stat-value">${{ budget.max_budget?.toFixed(2) ?? '∞' }}</span>
                </div>
                <div class="budget-stat">
                  <span class="budget-stat-label">Remaining</span>
                  <span class="budget-stat-value">${{ budget.remaining?.toFixed(2) ?? '∞' }}</span>
                </div>
              </div>
              <el-progress
                v-if="budget.usage_percent != null"
                :percentage="Math.min(budget.usage_percent, 100)"
                :color="budget.budget_exceeded ? 'var(--el-color-danger)' : undefined"
                :stroke-width="10"
                class="mt-3"
              >
                <span class="text-xs">{{ budget.usage_percent.toFixed(1) }}%</span>
              </el-progress>
              <el-alert
                v-if="budget.budget_exceeded"
                title="Budget exceeded"
                type="error"
                :closable="false"
                show-icon
                class="mt-3"
              />
            </template>
            <el-empty v-else description="No budget data" :image-size="40" />
            <div class="budget-adjust mt-3">
              <span class="text-sm text-gray-400">Adjust Budget:</span>
              <el-input-number
                v-model="budgetInput"
                :min="1"
                :precision="2"
                :controls="false"
                size="small"
                style="width: 120px"
                placeholder="$"
              />
              <el-button
                size="small"
                type="primary"
                :loading="budgetSaving"
                @click="handleSaveBudget"
              >
                Save
              </el-button>
            </div>
          </fieldset>
        </div>
      </div>
    </el-card>

    <!-- Usage Statistics -->
    <el-card class="mt-4 safe-card" shadow="never">
      <div class="flex items-center justify-between flex-wrap gap-3 mb-4">
        <div class="flex items-center gap-3">
          <h3 class="section-title">Usage View</h3>
          <el-dropdown trigger="click" @command="onUsageViewChange">
            <el-button size="small" round class="usage-view-toggle">
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

      <!-- Tag filter (shown when Tag Usage is selected) -->
      <div v-if="usageView === 'tag'" class="flex items-center gap-3 mb-6">
        <span class="text-sm text-gray-400">Filter by Tags:</span>
        <el-select
          v-model="selectedTags"
          placeholder="All Tags"
          multiple
          collapse-tags
          collapse-tags-tooltip
          clearable
          size="small"
          style="width: 320px"
          @change="onTagFilterChange"
        >
          <el-option label="(Untagged)" value="__untagged__" />
          <el-option
            v-for="t in availableTagNames"
            :key="t"
            :label="t"
            :value="t"
          />
        </el-select>
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
              <el-link v-if="row.tag_name" type="primary" :underline="false" @click="onTagRowClick(row.tag_name)">
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

        <!-- LLM Virtual Key tab hidden: no longer exposing virtual key to users
        <el-tab-pane label="LLM Virtual Key" name="virtualKey">
          <div class="code-block">
            <el-icon class="copy-btn" @click="copyText(codeSnippets.virtualKey)"><CopyDocument /></el-icon>
            <pre><code>{{ codeSnippets.virtualKey }}</code></pre>
          </div>
        </el-tab-pane>
        -->

        <el-tab-pane label="Tag Usage" name="tagUsage">
          <div class="code-block">
            <el-icon class="copy-btn" @click="copyText(codeSnippets.tagUsage)"><CopyDocument /></el-icon>
            <pre><code>{{ codeSnippets.tagUsage }}</code></pre>
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
            <span>AMD LLM API Key is not bound</span>
          </div>

          <el-descriptions v-if="binding" :column="1" border class="mt-6">
            <el-descriptions-item label="Email">
              {{ binding.user_email }}
            </el-descriptions-item>
          </el-descriptions>

          <el-empty v-else description="Unable to load binding status" :image-size="80" />

          <el-divider />

          <el-text class="block font-500 mb-4" tag="b">Bind AMD LLM API Key</el-text>

          <div class="bind-steps mb-4">
            <ol class="bind-steps-list">
              <li>Search for <b>Engineering-AI-Suite</b> in <b>Software Center</b> on your computer and install it.</li>
              <li>In the left navigation bar of <b>Engineering-AI-Suite</b>, click <b>Credentials</b>, find the <b>LLM API Key</b> item, and click <b>Generate API Key</b>.</li>
              <li>Paste the generated <b>LLM API Key</b> into the binding field below to complete the binding.</li>
              <li>After binding, you can use any SaFE API Key to access our LiteLLM service, and view your usage, set budgets, and more on the SaFE platform.</li>
            </ol>
          </div>
          <div class="key-input-row">
            <el-input
              v-model="apimKeyInput"
              type="password"
              placeholder="Enter your AMD LLM API Key"
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

  <!-- SaFE API Key Success Dialog -->
  <el-dialog
    v-model="apiKeyVisible"
    title="SaFE API Key Created Successfully"
    width="600"
    :close-on-click-modal="false"
  >
    <el-alert
      title="Important: Save your SaFE API Key"
      type="warning"
      :closable="false"
      show-icon
      class="mb-4"
    >
      <template #default>
        <div class="text-sm">
          A SaFE API Key has been automatically created for you. This is the only time you will see the full key. Please save it securely.
        </div>
      </template>
    </el-alert>

    <el-form label-width="auto" class="p-3">
      <el-form-item label="Name">
        <el-text>{{ createdApiKeyData?.name }}</el-text>
      </el-form-item>
      <el-form-item label="API Key">
        <div class="flex items-center gap-2 w-full">
          <el-input :model-value="createdApiKeyData?.apiKey" readonly class="font-mono">
            <template #append>
              <el-button :icon="CopyDocument" @click="copyApiKey" />
            </template>
          </el-input>
        </div>
      </el-form-item>
      <el-form-item label="Expiration">
        <el-text>{{ formatTimeStr(createdApiKeyData?.expirationTime) }}</el-text>
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
  getLLMGatewayBudget,
  updateLLMGatewayBudget,
  getLLMGatewayTagUsage,
  createAPIKey,
} from '@/services'
import type {
  LLMGatewayBinding,
  LLMGatewayUsage,
  LLMGatewaySummary,
  LLMGatewayBudget,
  LLMGatewayTagUsage,
  LLMGatewayTagItem,
  LLMGatewayTagUsageParams,
  CreateAPIKeyResponse,
} from '@/services'
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

const codeSnippets = computed(() => {
  const origin = window.location.origin
  return {
    safeKey: `from openai import OpenAI
import httpx

http_client = httpx.Client(verify=False)

client = OpenAI(
    api_key="ak-<your-safe-apikey>",
    base_url="${origin}/api/v1/llm-proxy/v1",
    http_client=http_client,
)

models = client.models.list()
for model in models.data:
    print(model.id)

response = client.chat.completions.create(
    model="claude-opus-4-7",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)`,
    virtualKey: `from openai import OpenAI
import httpx

http_client = httpx.Client(verify=False)

client = OpenAI(
    api_key="sk-<your-llm-virtual-key>",
    base_url="${origin}/llm-gateway/v1",
    http_client=http_client,
)

models = client.models.list()
for model in models.data:
    print(model.id)

response = client.chat.completions.create(
    model="claude-opus-4-7",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)`,
    tagUsage: `from openai import OpenAI

client = OpenAI(
    api_key="ak-<your-safe-apikey>",
    base_url="${origin}/api/v1/llm-proxy/v1",
)

response = client.chat.completions.create(
    model="claude-opus-4-7",
    messages=[{"role": "user", "content": "Hello!"}],
    extra_body={"tags": ["project-A"]}  # Tag is optional; omit to leave untagged
)
print(response.choices[0].message.content)`,
    linux: `curl -fsSL https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.sh | bash`,
    windows: `irm https://raw.githubusercontent.com/AMD-AGI/Primus-SaFE/main/Scripts/setup-certs/setup.bat -OutFile $env:TEMP\\setup.bat; cmd /c $env:TEMP\\setup.bat`,
  }
})

// ── SaFE API Key Dialog ──
const apiKeyVisible = ref(false)
const createdApiKeyData = ref<CreateAPIKeyResponse | null>(null)

const copyApiKey = () => {
  if (createdApiKeyData.value?.apiKey) {
    copyText(createdApiKeyData.value.apiKey)
  }
}

const autoCreateApiKey = async () => {
  try {
    const response = await createAPIKey({ name: 'llm', ttlDays: 365 })
    createdApiKeyData.value = response
    apiKeyVisible.value = true
  } catch {
    // Non-critical: don't block the user if API key creation fails
  }
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

// ── Budget ──
const budget = ref<LLMGatewayBudget | null>(null)
const budgetLoading = ref(false)
const budgetSaving = ref(false)
const budgetInput = ref<number | undefined>(undefined)

const fetchBudget = async () => {
  try {
    budgetLoading.value = true
    budget.value = await getLLMGatewayBudget()
    if (budget.value?.max_budget != null) {
      budgetInput.value = budget.value.max_budget
    }
  } catch {
    budget.value = null
  } finally {
    budgetLoading.value = false
  }
}

const handleSaveBudget = async () => {
  if (!budgetInput.value || budgetInput.value <= 0) return
  try {
    budgetSaving.value = true
    budget.value = await updateLLMGatewayBudget({ max_budget: budgetInput.value })
    ElMessage.success('Budget updated successfully')
  } catch {
    ElMessage.error('Failed to update budget')
  } finally {
    budgetSaving.value = false
  }
}

// ── Usage View Toggle ──
type UsageViewType = 'user' | 'tag'
const usageView = ref<UsageViewType>('user')

const onUsageViewChange = (view: UsageViewType) => {
  usageView.value = view
  fetchCurrentUsage()
}

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
const selectedTags = ref<string[]>([])
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

const onQuickRangeChange = (val: string | number | boolean | undefined) => {
  const days = Number(val) || 7
  dateRange.value = [
    dayjs().subtract(days - 1, 'day').format('YYYY-MM-DD'),
    dayjs().format('YYYY-MM-DD'),
  ]
  fetchCurrentUsage()
}

const onDateRangeChange = (val: [string, string] | null) => {
  if (!val) return
  quickRange.value = 0 as never
  fetchCurrentUsage()
}

const onTagFilterChange = () => {
  fetchTagUsage()
}

const onTagRowClick = (tag: string) => {
  if (!selectedTags.value.includes(tag)) {
    selectedTags.value = [...selectedTags.value, tag]
  }
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
      ...(selectedTags.value.length ? { tag: selectedTags.value.join(',') } : {}),
    }
    tagUsage.value = await getLLMGatewayTagUsage(params)

    if (!selectedTags.value.length && tagUsage.value?.tags) {
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
    await createLLMGatewayBinding({ apim_key: key })
    ElMessage.success('AMD LLM API Key bound successfully')
    apimKeyInput.value = ''
    await fetchBinding()
    await autoCreateApiKey()
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
    ElMessage.success('AMD LLM API Key updated successfully')
    apimKeyInput.value = ''
    await fetchBinding()
    await autoCreateApiKey()
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
      fetchBudget()
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

/* Bound state two-column layout */
.bound-columns {
  display: flex;
  gap: 24px;
}
.bound-col-left {
  flex: 1;
  min-width: 0;
}
.bound-col-right {
  flex: 1;
  min-width: 0;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 12px;
  padding: 20px;
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.6),
    rgba(255, 255, 255, 0.15)
  );
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}
.dark .bound-col-right {
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.06),
    rgba(255, 255, 255, 0.02)
  );
}

/* Budget header */
.budget-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin-bottom: 16px;
}
.budget-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.budget-amount {
  font-size: 20px;
  font-weight: 700;
  color: var(--el-color-success);
}

/* Budget fieldset */
.budget-fieldset {
  border: 1px solid var(--el-border-color);
  border-radius: 8px;
  padding: 16px;
  margin: 0;
}
.budget-fieldset legend {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-secondary);
  padding: 0 6px;
}
.budget-stats {
  display: flex;
  gap: 24px;
}
.budget-stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.budget-stat-label {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}
.budget-stat-value {
  font-size: 16px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}
.budget-adjust {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* Usage view toggle button — teal style matching Tools SKILL tag */
.usage-view-toggle {
  --el-button-bg-color: rgba(0, 229, 229, 0.12);
  --el-button-text-color: #00a3a3;
  --el-button-border-color: rgba(0, 229, 229, 0.3);
  --el-button-hover-bg-color: rgba(0, 229, 229, 0.2);
  --el-button-hover-text-color: #008a8a;
  --el-button-hover-border-color: rgba(0, 229, 229, 0.45);
  font-weight: 500;
}

@media (max-width: 900px) {
  .bound-columns {
    flex-direction: column;
  }
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

/* Binding steps */
.bind-steps {
  background: var(--el-fill-color-light);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 16px 16px 16px 20px;
}
.bind-steps-list {
  margin: 0;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  font-size: 14px;
  line-height: 1.6;
  color: var(--el-text-color-regular);
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
  color: #00a3a3 !important;
  font-weight: 600;
}
</style>

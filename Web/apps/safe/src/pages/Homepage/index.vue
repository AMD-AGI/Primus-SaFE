<template>
  <div class="header-row">
    <h3 class="chart-title">Usage breakdown</h3>
    <div class="header-links">
      <!-- Hyperloom entry - shown only in production (same gating as Lens) -->
      <el-button v-if="isProd" link @click="goToHyperloom" class="lens-link">
        Go to Hyperloom
        <el-icon class="ml-1"><Right /></el-icon>
      </el-button>
      <!-- Lens entry - shown only in production -->
      <el-button v-if="isProd" link @click="goToLens" class="lens-link">
        Go to Lens
        <el-icon class="ml-1"><Right /></el-icon>
      </el-button>
    </div>
  </div>

  <div class="usage-dashboard">
    <div class="stat-grid">
      <el-card
        v-for="(s, i) in statCards"
        :key="s.key"
        shadow="never"
        class="stat-card safe-card"
        :class="[{ 'stat-card--tall': i === 0 }, `stat-card--tone-${s.tone}`]"
        :style="{
          '--accent': PIE_COLORS[0] || '#47b881',
          '--accent-bad': PIE_COLORS[1] || '#d65f68',
          '--accent-used': PIE_COLORS[2] || '#0d9488',
        }"
      >
        <div class="stat-header">
          <div>
            <div class="stat-kicker">{{ s.kicker }}</div>
            <div class="stat-title">{{ s.title }}</div>
          </div>
          <div class="stat-total">
            <span class="stat-total__num">{{ s.total }}</span>
          </div>
        </div>

        <div class="stat-bottom">
          <div class="stat-details">
            <div class="stat-badges">
              <span class="badge badge--state badge--ok" v-if="s.avail !== undefined">
                <i class="dot" :style="{ background: PIE_COLORS[0] || '' }"></i>
                <span class="badge-text">Available {{ s.avail }}</span>
              </span>
              <span class="badge badge--state badge--bad">
                <i class="dot" :style="{ background: PIE_COLORS[1] || '' }"></i>
                <span class="badge-text">Abnormal {{ s.abnormal }}</span>
              </span>
              <span class="badge badge--state badge--used" v-if="s.used !== undefined">
                <i class="dot" :style="{ background: PIE_COLORS[2] || '' }"></i>
                <span class="badge-text">Used {{ s.used }}</span>
              </span>
            </div>

            <div class="stat-meter" aria-hidden="true">
              <span class="stat-meter__ok" :style="{ width: s.availPercent }"></span>
              <span class="stat-meter__bad" :style="{ width: s.abnormalPercent }"></span>
              <span class="stat-meter__used" :style="{ width: s.usedPercent }"></span>
            </div>
            <div class="stat-caption" v-if="i === 0">{{ s.caption }}</div>
          </div>

          <div v-if="i === 0" class="small-pie-box" :ref="(el) => (nodePieRef = el as any)" />
        </div>
      </el-card>
    </div>

    <el-card shadow="never" class="capacity-map-card safe-card">
      <div class="capacity-map-copy">
        <span class="capacity-eyebrow">Workspace Capacity</span>
        <h3>Capacity overview</h3>
        <p>
          Current quota, availability, and usage by resource pool. This section stays intentionally
          quiet so the status cards remain the focus.
        </p>
        <div class="capacity-tags">
          <span>Total nodes {{ nodeNumbers.total }}</span>
          <span>{{ RES_KEYS.length }} resource pools</span>
          <span>{{ nodeNumbers.avail }} available nodes</span>
        </div>
      </div>

      <div class="capacity-illustration" aria-hidden="true">
        <div class="orbit orbit--outer"></div>
        <div class="orbit orbit--middle"></div>
        <div class="orbit orbit--inner"></div>
        <div class="core-node"></div>
        <span class="satellite satellite--gpu">GPU</span>
        <span class="satellite satellite--cpu">CPU</span>
        <span class="satellite satellite--mem">MEM</span>
        <span class="satellite satellite--net">RDMA</span>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onBeforeUnmount } from 'vue'
import * as echarts from 'echarts'
import { Right } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/stores/workspace'
import { getWorkspaceDetail } from '@/services/workspace/index'
import { byte2Gi } from '@/utils/index'

// Check if in production environment
const isProd = import.meta.env.PROD

// Navigate to Lens system
const goToLens = () => {
  window.open(`${location.origin}/lens`, '_blank')
}

const goToHyperloom = () => {
  window.open(`${location.origin}/hyperloom/`, '_blank')
}

const store = useWorkspaceStore()
const PIE_COLORS = ['#47b881', '#d65f68', '#0d9488'] as const

const detailData = ref<any>(null)
const RES_KEYS = ['amd.com/gpu', 'rdma/hca', 'cpu', 'memory'] as const
const unitOf = (k: string) => (k === 'ephemeral-storage' || k === 'memory' ? 'Gi' : '')
const normalized = computed(() => {
  const d = detailData.value || {}
  const pick = (group: any) =>
    RES_KEYS.map((k) => {
      const raw = group?.[k] ?? 0
      return k === 'memory' ? byte2Gi(raw, 0, false) : raw
    })
  return { total: pick(d.totalQuota), avail: pick(d.availQuota), abnormal: pick(d.abnormalQuota) }
})
const usedNormal = computed(() =>
  normalized.value.total.map((t, i) =>
    Math.max(t - (normalized.value.avail[i] ?? 0) - (normalized.value.abnormal[i] ?? 0), 0),
  ),
)

const legends = ['Available', 'Abnormal', 'Used']

// Nodes pie chart option
const nodeNumbers = computed(() => {
  const total = Number(detailData.value?.currentNodeCount ?? 0)
  const used = Number(detailData.value?.usedNodeCount ?? 0)
  const abnormal = Number(detailData.value?.abnormalNodeCount ?? 0)
  const avail = Math.max(total - used - abnormal, 0)
  return { total, avail, abnormal, used }
})

const fmtPct = (x: number | undefined) =>
  `${Number.isFinite(x as number) ? (x as number).toFixed(1) : '0.0'}%`

const buildNodePieOption = (): echarts.EChartsOption => {
  const n = nodeNumbers.value
  return {
    color: [...PIE_COLORS],
    tooltip: {
      trigger: 'item',
      appendToBody: true,
      formatter: (p: any) => `Nodes<br/>${p.name}: ${p.value} (${p.percent}%)`,
    },
    series: [
      {
        type: 'pie',
        radius: ['45%', '90%'],
        center: ['50%', '50%'],
        avoidLabelOverlap: true,
        minShowLabelAngle: 5,
        stillShowZeroSum: true,
        label: {
          position: 'inside',
          formatter: (p: any) => (p.value > 0 ? fmtPct(p.percent) : ''),
          fontSize: 16,
          fontWeight: 700,
          color: '#fff',
          textBorderColor: 'rgba(0,0,0,.35)',
          textBorderWidth: 2,
          textShadowColor: 'rgba(0,0,0,.25)',
          textShadowBlur: 2,
        },
        labelLine: { show: false },
        data: [
          { name: legends[0], value: n.avail },
          { name: legends[1], value: n.abnormal },
          { name: legends[2], value: n.used },
        ],
      },
    ],
  }
}

const nodePieRef = ref<HTMLElement | null>(null)
let nodePieChart: echarts.ECharts | null = null

function renderAllCharts() {
  // Nodes pie chart
  if (!nodePieChart && nodePieRef.value) {
    nodePieChart = echarts.init(nodePieRef.value)
  }
  nodePieChart?.setOption(buildNodePieOption(), true)
}

type StatCard = {
  key: string
  tone: string
  kicker: string
  title: string
  total: string
  avail?: string
  abnormal?: string
  used?: string
  availPercent: string
  abnormalPercent: string
  usedPercent: string
  caption: string
}
const fmt = (v: number, u: string) => (u ? `${v} ${u}` : String(v))
const toPercent = (value: number, total: number) =>
  total > 0 ? `${Math.min(Math.max((value / total) * 100, 0), 100).toFixed(2)}%` : '0%'
const toneOf = (key: string) => {
  if (key === 'amd.com/gpu') return 'compute'
  if (key === 'rdma/hca') return 'network'
  if (key === 'memory') return 'memory'
  return 'compute'
}
const statCards = computed<StatCard[]>(() => {
  const res = RES_KEYS.map<StatCard>((k, i) => {
    const unit = unitOf(k)
    const total = normalized.value.total[i] ?? 0
    const avail = normalized.value.avail[i] ?? 0
    const abnormal = normalized.value.abnormal[i] ?? 0
    const used = usedNormal.value[i] ?? 0
    return {
      key: k as string,
      tone: toneOf(k),
      kicker: 'Resource Pool',
      title: `${k}${unit ? ` (${unit})` : ''}`,
      total: fmt(total, unit),
      avail: fmt(avail, unit),
      abnormal: fmt(abnormal, unit),
      used: fmt(used, unit),
      availPercent: toPercent(avail, total),
      abnormalPercent: toPercent(abnormal, total),
      usedPercent: toPercent(used, total),
      caption: total > 0 ? 'Capacity split' : 'No capacity yet',
    }
  })

  const nodes = nodeNumbers.value
  res.unshift({
    key: 'nodes',
    tone: 'nodes',
    kicker: 'Cluster Health',
    title: 'Nodes',
    avail: String(nodes.avail),
    total: String(nodes.total),
    abnormal: String(nodes.abnormal),
    used: String(nodes.used),
    availPercent: toPercent(nodes.avail, nodes.total),
    abnormalPercent: toPercent(nodes.abnormal, nodes.total),
    usedPercent: toPercent(nodes.used, nodes.total),
    caption: nodes.total > 0 ? `${nodes.avail} nodes ready` : 'No node capacity reported',
  })

  return res
})

async function getDetail() {
  if (!store.currentWorkspaceId) return
  detailData.value = await getWorkspaceDetail(store.currentWorkspaceId)
}

watch(
  () => store.currentWorkspaceId,
  (id) => {
    if (id) getDetail()
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  if (nodePieChart) {
    nodePieChart.dispose()
    nodePieChart = null
  }
})

watch(
  () => detailData.value,
  async (v) => {
    if (!v) return
    await nextTick()
    renderAllCharts()
  },
)

</script>

<style scoped>
/* Title row: title and link side by side */
.header-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: clamp(10px, 1vw, 16px) 0;
  flex-wrap: wrap;
  gap: 12px;
}

.header-links {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* Lens link style */
.lens-link {
  font-size: 14px;
  color: var(--el-color-primary);
  padding: 4px 8px;
  transition: all 0.2s ease;
}

.lens-link:hover {
  color: var(--el-color-primary-light-3);
}

.usage-dashboard {
  display: grid;
  gap: 16px;
}

/* Small pie chart (inside Nodes card) */
.small-pie-box {
  flex: 0 0 min(38%, 220px);
  max-width: 260px;
  margin-left: auto;
  aspect-ratio: 1 / 1;
  min-height: 160px;
  max-height: 220px;
}

.chart-title {
  font-weight: 600;
  color: var(--el-text-color-primary);
  font-size: calc(clamp(16px, 0.9vw + 12px, 24px) * var(--scale));
  margin: 0;
}

/* Card group styles */
/* ===== Grid container: three columns ===== */
.stat-grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  grid-auto-rows: 232px;
}

/* Small screen responsive: two columns / one column */
@media (max-width: 1024px) {
  .stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    grid-auto-rows: 180px;
  }

  /* First card does not span rows on small screens */
  .stat-card--tall {
    grid-row: span 1;
  }

  /* Adjust first card number size */
  .stat-card--tall .stat-total__num {
    font-size: 2rem;
  }

  /* Adjust small pie chart size */
  .small-pie-box {
    flex: 0 0 140px;
    max-width: 160px;
    min-height: 120px;
    max-height: 140px;
  }
}
@media (max-width: 640px) {
  .stat-grid {
    grid-template-columns: 1fr;
    grid-auto-rows: auto;
  }

  .small-pie-box {
    display: none;
  }
}

/* ===== Card body ===== */
.stat-card {
  --tone: var(--safe-primary);
  position: relative;
  min-width: 0;
  border-radius: var(--safe-radius-xl);
  background: var(--safe-card);
  border: 1px solid var(--safe-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
  overflow: hidden;
  transition:
    transform 0.25s ease,
    box-shadow 0.25s ease,
    border-color 0.25s ease;

  display: flex;
  flex-direction: column;
}

.stat-card::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: linear-gradient(180deg, color-mix(in oklab, var(--safe-primary) 7%, transparent), transparent 54%);
  pointer-events: none;
}

.stat-card::after {
  display: none;
}

.stat-card--tone-nodes {
  --tone: var(--safe-primary);
}

.stat-card--tone-compute,
.stat-card--tone-network,
.stat-card--tone-memory {
  --tone: var(--safe-primary);
}

/* First card spans two rows vertically */
.stat-card--tall {
  grid-row: span 2;
  justify-content: space-between;
}
.stat-card--tall .stat-total__num {
  font-size: 2.8rem;
  line-height: 1;
}

.stat-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in oklab, var(--safe-border) 55%, var(--safe-primary) 45%);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
}

/* el-card inner layer */
.stat-card :deep(.el-card__body) {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  overflow: hidden !important;
  padding: 18px;
}

/* ===== Header: title + large number in top right ===== */
.stat-header {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: start;
  gap: 14px;
  padding-bottom: 10px;
}

.stat-kicker {
  margin-bottom: 6px;
  color: var(--safe-muted);
  font-size: calc(10px * var(--scale));
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.stat-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  font-size: calc(clamp(14px, 0.55vw + 11px, 17px) * var(--scale));
  font-weight: 700;
  line-height: 1.4;
  color: var(--el-text-color-primary);
  letter-spacing: 0.2px;
}
.stat-total {
  white-space: nowrap;
  line-height: 1;
}
/* Prominent total in top right (gradient text) */
.stat-total__num {
  color: var(--safe-primary);
  font-weight: 800;
  font-size: calc(clamp(22px, 0.9vw + 14px, 30px) * var(--scale));
  line-height: 1;

  background-image: none;
  -webkit-background-clip: text;
  background-clip: text;

  -webkit-text-fill-color: currentColor;
  text-shadow: none;
}
@container (max-width: 420px) {
  .stat-total__num {
    font-size: calc(16px * var(--scale));
  }
  .stat-title {
    font-size: calc(14px * var(--scale));
  }
}

.stat-bottom {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-top: auto;
}

.stat-details {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  flex-direction: column;
  gap: 12px;
}

.stat-badges {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  flex: 1 1 auto;
}

.badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: calc(11px * var(--scale));
  color: var(--el-text-color-secondary);
  padding: 4px 8px;
  border-radius: 999px;
  background: var(--safe-card-2);
  box-shadow: inset 0 0 0 1px var(--safe-border);
  white-space: nowrap;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dot {
  width: 9px;
  height: 9px;
  border-radius: 999px;
  display: inline-block;
}

.stat-meter {
  display: flex;
  width: 100%;
  height: 8px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--safe-card-2);
  box-shadow: inset 0 0 0 1px var(--safe-border);
}

.stat-meter span {
  transition: width 0.25s ease;
}

.stat-meter__ok {
  background: #47b881;
}

.stat-meter__bad {
  background: #d65f68;
}

.stat-meter__used {
  background: var(--safe-primary);
}

.stat-caption {
  color: var(--el-text-color-secondary);
  font-size: calc(12px * var(--scale));
  line-height: 1.45;
}

.capacity-map-card {
  position: relative;
  min-height: 220px;
  overflow: hidden;
  border-radius: var(--safe-radius-xl);
  background: var(--safe-card);
  border: 1px solid var(--safe-border);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08);
}

.capacity-map-card :deep(.el-card__body) {
  display: grid;
  min-height: 220px;
  box-sizing: border-box;
  overflow: hidden;
  grid-template-columns: minmax(0, 0.9fr) minmax(360px, 1.1fr);
  align-items: center;
  gap: 24px;
  padding: 28px;
}

.capacity-map-copy {
  position: relative;
  z-index: 1;
  max-width: 560px;
}

.capacity-eyebrow {
  display: inline-flex;
  margin-bottom: 10px;
  border-radius: 999px;
  padding: 6px 12px;
  color: var(--safe-primary);
  background: var(--safe-primary-plain-bg);
  box-shadow: inset 0 0 0 1px var(--safe-primary-plain-border);
  font-size: calc(12px * var(--scale));
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.capacity-map-copy h3 {
  margin: 0 0 10px;
  color: var(--el-text-color-primary);
  font-size: calc(clamp(22px, 1.1vw + 18px, 34px) * var(--scale));
  line-height: 1.15;
  font-weight: 700;
}

.capacity-map-copy p {
  margin: 0;
  max-width: 520px;
  color: var(--safe-muted);
  font-size: calc(14px * var(--scale));
  line-height: 1.7;
}

.capacity-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 20px;
}

.capacity-tags span {
  border-radius: 999px;
  padding: 7px 12px;
  color: var(--safe-muted);
  background: var(--safe-card-2);
  box-shadow: inset 0 0 0 1px var(--safe-border);
  font-size: calc(12px * var(--scale));
  font-weight: 700;
}

.capacity-illustration {
  position: relative;
  height: 190px;
  min-width: 320px;
}

.orbit {
  position: absolute;
  inset: 50% auto auto 50%;
  border: 1px solid color-mix(in oklab, var(--safe-border) 70%, var(--safe-primary) 30%);
  border-radius: 999px;
  transform: translate(-50%, -50%);
}

.orbit--outer {
  width: 430px;
  height: 146px;
}

.orbit--middle {
  width: 320px;
  height: 104px;
  border-color: var(--safe-border);
}

.orbit--inner {
  width: 210px;
  height: 70px;
  border-color: var(--safe-border);
}

.core-node {
  position: absolute;
  inset: 50% auto auto 50%;
  width: 76px;
  height: 76px;
  border-radius: 22px;
  background: var(--safe-primary);
  box-shadow:
    0 16px 36px color-mix(in oklab, var(--safe-primary) 18%, transparent 82%),
    inset 0 1px 0 rgba(255, 255, 255, 0.28);
  transform: translate(-50%, -50%) rotate(45deg);
}

.satellite {
  position: absolute;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 64px;
  height: 34px;
  border-radius: 999px;
  color: var(--safe-muted);
  background: var(--safe-card-2);
  box-shadow:
    inset 0 0 0 1px var(--safe-border),
    0 8px 18px rgba(0, 0, 0, 0.08);
  font-size: 12px;
  font-weight: 800;
}

.satellite--gpu {
  top: 14px;
  left: 54%;
}

.satellite--cpu {
  top: 86px;
  right: 7%;
}

.satellite--mem {
  bottom: 18px;
  left: 42%;
}

.satellite--net {
  top: 72px;
  left: 10%;
}

@media (max-width: 1024px) {
  .capacity-map-card :deep(.el-card__body) {
    grid-template-columns: 1fr;
  }

  .capacity-illustration {
    min-width: 0;
  }
}

@media (max-width: 640px) {
  .capacity-map-card :deep(.el-card__body) {
    padding: 20px;
  }

  .capacity-illustration {
    height: 150px;
  }

  .orbit--outer {
    width: 300px;
    height: 112px;
  }

  .orbit--middle {
    width: 228px;
    height: 82px;
  }

  .orbit--inner {
    width: 150px;
    height: 54px;
  }

  .satellite {
    min-width: 52px;
    height: 30px;
    font-size: 11px;
  }
}

.badge--ok {
  --c: #47b881;
}
.badge--bad {
  --c: #d65f68;
}
.badge--used {
  --c: var(--safe-primary);
}

.badge--state .badge-text {
  display: inline-block;
  font-weight: 700;
  color: var(--c);
  -webkit-text-fill-color: currentColor;
  text-shadow: none;

  transition:
    color 0.2s ease,
    opacity 0.2s ease;
}

.badge--state:hover .badge-text {
  opacity: 0.86;
}

.badge--state {
  position: relative;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
  overflow: hidden;
}
.badge--state:hover {
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.12);
}

.badge--state::after {
  display: none;
}

/* With reduced motion preference, keep only color transitions */
@media (prefers-reduced-motion: reduce) {
  .badge--state::after {
    transition: none;
    display: none;
  }
  .badge--state .badge-text {
    transition: none;
  }
}
</style>

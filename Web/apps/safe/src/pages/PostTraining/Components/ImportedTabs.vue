<template>
  <template v-if="tab === 'overview'">
    <div class="pt-cards">
      <div v-for="c in kpis" :key="c.label" class="pt-stat">
        <div class="text-[12px] text-[var(--safe-muted)]">{{ c.label }}</div>
        <div class="text-[20px] font-600">{{ c.value }}</div>
        <div class="text-[12px] text-[var(--safe-muted)]">{{ c.sub }}</div>
      </div>
    </div>
    <div v-if="overviewPanels.length" class="pt-grid mt-3">
      <MetricsPanel
        v-for="p in overviewPanels"
        :key="p.title"
        :title="p.title"
        :series="p.series"
        :smoothing="0.6"
      />
    </div>
    <el-empty
      v-else
      description="No step, rollout or eval lines, so there are no curves"
      :image-size="60"
    />

    <el-card v-if="finalEval" class="safe-card mt-3" shadow="never">
      <div class="mb-2 flex items-center gap-2">
        <span class="font-500">Final evaluation</span>
        <el-tag size="small" :type="finalEval.result === 'pass' ? 'success' : 'danger'">
          {{ finalEval.result === 'pass' ? 'PASS' : 'FAIL' }}
        </el-tag>
        <span class="text-[12px] text-[var(--safe-muted)]">
          {{ finalEval.gate }}{{ finalEvalEnd ? ` · ${finalEvalEnd}` : '' }}
        </span>
      </div>
      <el-descriptions :column="2" border size="small">
        <el-descriptions-item v-for="[k, v] in finalEvalRows" :key="k" :label="k">
          <span class="break-all">{{ v }}</span>
          <el-icon
            v-if="v.startsWith('/')"
            class="ml-1 cursor-pointer align-middle"
            @click="copyText(v)"
          >
            <CopyDocument />
          </el-icon>
        </el-descriptions-item>
      </el-descriptions>
      <div class="mt-2 text-[12px] text-[var(--safe-muted)]">
        From the line the {{ finalEval.gate }} check printed at the end of {{ mainLogName }}; the
        verdict is its result in results.tsv.
      </div>
    </el-card>

    <el-card class="safe-card mt-3" shadow="never">
      <div class="mb-2 font-500">Source</div>
      <el-descriptions :column="1" border size="small" label-width="170px">
        <el-descriptions-item v-for="r in sourceRows" :key="r.label" :label="r.label">
          <span class="break-all">{{ r.value }}</span>
          <el-icon v-if="r.copy" class="ml-1 cursor-pointer align-middle" @click="copyText(r.copy)">
            <CopyDocument />
          </el-icon>
        </el-descriptions-item>
      </el-descriptions>
    </el-card>
  </template>

  <template v-else-if="tab === 'gates'">
    <div class="mb-2 flex items-baseline gap-2">
      <span class="font-500">Gate results</span>
      <span class="text-[12px] text-[var(--safe-muted)]"
        >From node-*.log and results/results.tsv</span
      >
    </div>
    <el-table :data="s.gates" size="small" :row-class-name="gateRowClass">
      <el-table-column prop="gate" label="Gate" min-width="140" />
      <el-table-column label="Node" min-width="200">
        <template #default="{ row }">
          {{ row.node }}
          <el-tag v-if="row.node === s.head" size="small" type="info" class="ml-1">head</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Result" width="120">
        <template #default="{ row }">
          <span :class="isFailed(row) ? 'text-[var(--el-color-danger)]' : ''">{{
            row.result
          }}</span>
        </template>
      </el-table-column>
      <el-table-column label="Began" width="110">
        <template #default="{ row }">{{ clock(row.start) }}</template>
      </el-table-column>
      <el-table-column label="Duration" width="110">
        <template #default="{ row }">{{ gateDuration(row.seconds) }}</template>
      </el-table-column>
    </el-table>

    <div class="mb-2 mt-4 flex items-baseline gap-2">
      <span class="font-500">Check output</span>
      <span class="text-[12px] text-[var(--safe-muted)]">End of {{ mainLogName }}</span>
    </div>
    <pre
      v-if="s.check.length"
      class="pt-import-check"
    ><span v-for="(line, i) in s.check" :key="i" :class="{ 'is-fail': line.trim().startsWith('FAIL') }">{{ line }}
</span></pre>
    <div v-else class="text-[12px] text-[var(--safe-muted)]">
      The main log ends without check output.
    </div>

    <div class="mb-2 mt-4 flex items-baseline gap-2">
      <span class="font-500">Markers</span>
      <span class="text-[12px] text-[var(--safe-muted)]">markers/ · time from file mtime</span>
    </div>
    <el-table :data="markerRows" size="small">
      <el-table-column prop="name" label="Marker" min-width="160" />
      <el-table-column prop="written" label="Written" width="120" />
      <el-table-column prop="after" label="T+" width="120" />
    </el-table>
  </template>

  <template v-else-if="tab === 'resources'">
    <div class="mb-2 flex items-baseline gap-2">
      <span class="font-500">Peaks per node</span>
      <span class="text-[12px] text-[var(--safe-muted)]"
        >Every 30 s sample in mon/&lt;node&gt;.tsv</span
      >
    </div>
    <el-table :data="peaks" size="small">
      <el-table-column label="Node" min-width="200">
        <template #default="{ row }">
          {{ row.host }}
          <el-tag v-if="row.head" size="small" type="info" class="ml-1">head</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="Host memory peak" width="160">
        <template #default="{ row }">{{ (row.hostMemBytes / 1e12).toFixed(2) }} TB</template>
      </el-table-column>
      <el-table-column label="VRAM peak per GPU" width="160">
        <template #default="{ row }">{{ (row.vramBytes / 1e9).toFixed(1) }} GB</template>
      </el-table-column>
      <el-table-column prop="samples" label="Samples" width="100" />
    </el-table>
    <template v-if="peaks.length">
      <div class="pt-grid mt-3">
        <MetricsPanel title="Host memory used (TB)" :series="curves.memory" :smoothing="0" />
        <MetricsPanel
          title="VRAM per GPU, highest of 8 (GB)"
          :series="curves.vram"
          :smoothing="0"
        />
      </div>
      <div class="mt-1 text-[12px] text-[var(--safe-muted)]">Minutes from the first gate.</div>
    </template>
  </template>

  <template v-else-if="tab === 'checkpoints'">
    <div class="mb-2 text-[13px] text-[var(--safe-muted)]">
      Source {{ s.saveSet ? `…/${s.name}/ckpt` : '--save not set' }} · Checked at import{{
        importedAt ? ` · ${importedAt}` : ''
      }}
    </div>
    <el-table v-if="s.checkpoints.iterations.length" :data="s.checkpoints.iterations" size="small">
      <el-table-column prop="step" label="Step" width="90" />
      <el-table-column label="Directory" min-width="150">
        <template #default="{ row }">iter_{{ String(row.iteration).padStart(7, '0') }}</template>
      </el-table-column>
      <el-table-column label="Saved" width="130">
        <template #default="{ row }">{{
          row.time ? dayjs(row.time).format('MM-DD HH:mm') : '—'
        }}</template>
      </el-table-column>
      <el-table-column label="Size" width="110">
        <template #default="{ row }">{{ formatBytes(row.bytes, 1) }}</template>
      </el-table-column>
      <el-table-column prop="files" label="Files" width="90" />
      <el-table-column label="Actions" width="150">
        <template #default>
          <el-tooltip :content="NO_EXPORT" placement="top">
            <span><el-button link type="primary" disabled>Export as Model</el-button></span>
          </el-tooltip>
        </template>
      </el-table-column>
    </el-table>
    <el-empty
      v-else
      :image-size="60"
      :description="
        s.saveSet
          ? 'No checkpoints: ckpt/ had no iter_* directory at import.'
          : 'No checkpoints: --save was not set for this run.'
      "
    />
    <el-alert
      v-if="s.checkpoints.iterations.length && trainable"
      class="mt-2"
      type="info"
      :closable="false"
      :title="`Trainable parameters only (${trainable}). It can't be resumed or registered as a model.`"
    />
    <template v-if="s.checkpoints.rollout">
      <div class="mb-2 mt-4 font-500">Rollout dumps</div>
      <el-descriptions :column="3" border size="small">
        <el-descriptions-item label="Directory">rollout/</el-descriptions-item>
        <el-descriptions-item label="Files">{{ s.checkpoints.rollout.files }}</el-descriptions-item>
        <el-descriptions-item label="Size">{{
          formatBytes(s.checkpoints.rollout.bytes, 1)
        }}</el-descriptions-item>
      </el-descriptions>
      <div class="mt-1 text-[12px] text-[var(--safe-muted)]">
        One file per rollout with the generated samples and their rewards. They stay on Spur and are
        only listed here.
      </div>
    </template>
    <div class="mt-3 text-[12px] text-[var(--safe-muted)]">
      SaFE keeps a reference to this directory, not a copy. If it changes on Spur, this list stays
      as it was until the run is imported again.
    </div>
  </template>

  <template v-else-if="tab === 'config'">
    <div class="mb-2 font-500">Key settings</div>
    <el-descriptions :column="2" border size="small" label-width="150px">
      <el-descriptions-item v-for="r in configRows" :key="r.label" :label="r.label">
        <span class="break-all">{{ r.value }}</span>
      </el-descriptions-item>
    </el-descriptions>
    <div class="mb-2 mt-4 flex items-baseline gap-2">
      <span class="font-500">Arguments</span>
      <span class="text-[12px] text-[var(--safe-muted)]">
        {{ keptRows.length }} of {{ s.arguments.total.toLocaleString('en-US') }}
      </span>
    </div>
    <el-table :data="keptRows" size="small" max-height="480">
      <el-table-column prop="name" label="Name" min-width="220" />
      <el-table-column label="Value" min-width="320">
        <template #default="{ row }"
          ><span class="break-all">{{ row.value }}</span></template
        >
      </el-table-column>
    </el-table>
  </template>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { CopyDocument } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { copyText, formatBytes } from '@/utils'
import {
  failedGateOf,
  gateDuration,
  parallelismText,
  resourcePeaks,
  resourceSeries,
  trainableTotal,
  type ImportGateRow,
  type MetricSeries,
  type MetricsResp,
  type RunDetail,
  type RunImport,
} from '@/services/postTraining'
import MetricsPanel from './MetricsPanel.vue'

const props = defineProps<{
  tab: 'overview' | 'gates' | 'resources' | 'checkpoints' | 'config'
  run: RunDetail
  info: RunImport
  metrics?: MetricsResp
}>()

const NO_EXPORT =
  "Can't export: it holds trainable parameters only, so exporting needs a merge onto the base weights, and an " +
  'imported run has no registered base model.'

const s = computed(() => props.info.summary)
const kept = computed(() => s.value.arguments.kept ?? {})
const mainLogName = computed(() => s.value.mainLog?.name ?? 'the main log')
const trainable = computed(() => trainableTotal(s.value.trainable))
const importedAt = computed(() =>
  props.info.importedAt ? dayjs(props.info.importedAt).format('MM-DD HH:mm') : '',
)

/** A value as the summary keeps it, numbers to four significant digits. */
const show = (v: unknown): string => {
  if (v == null || v === '') return '—'
  if (typeof v === 'number')
    return Number.isInteger(v) ? v.toLocaleString('en-US') : String(+v.toPrecision(4))
  return typeof v === 'object' ? JSON.stringify(v) : String(v)
}
const clock = (t?: string | null) => (t ? dayjs(t).format('HH:mm:ss') : '—')

const seriesByName = computed(() => new Map((props.metrics?.series ?? []).map((x) => [x.name, x])))
const pick = (name: string) => seriesByName.value.get(name)
const evalLoss = computed(() =>
  (props.metrics?.series ?? []).find((x) => /^eval\/[^/]+\/loss$/.test(x.name)),
)
const lastOf = (x?: MetricSeries) => x?.last ?? x?.points[x.points.length - 1]

const peaks = computed(() => resourcePeaks(props.info.resources, s.value.head))
const curves = computed(() => resourceSeries(props.info.resources, s.value.startTime))

/** The Train stage's outcome and duration, for the Logged card. */
const trainNote = computed(() => {
  const st = s.value.stages.find((x) => x.stage === 'train')
  if (!st) return 'No Train gate'
  if (!st.end) return 'Train gate has no END line'
  const took = gateDuration((dayjs(st.end).valueOf() - dayjs(st.start ?? st.end).valueOf()) / 1000)
  return st.result === 'fail' ? `Train gate failed after ${took}` : `Train gate passed in ${took}`
})

/** The overview's cards: steps or rollouts logged, the last loss or reward, the last eval loss or response length,
 * and the peak per-GPU VRAM. */
const kpis = computed(() => {
  const rl = props.run.trainType === 'rl'
  const step = props.run.metricSummary?.main?.step
  const n = step != null ? step + 1 : 0
  const out = [
    { label: 'Logged', value: `${n} ${rl ? 'rollouts' : 'steps'}`, sub: trainNote.value },
  ]
  const card = (label: string, x: MetricSeries | undefined, sub: (x: MetricSeries) => string) => {
    const last = lastOf(x)
    out.push({ label, value: last ? show(last[1]) : '—', sub: x && last ? sub(x) : 'Not logged' })
  }
  const from = (x: MetricSeries) =>
    x.points.length
      ? `From ${show(x.points[0][1])} at ${rl ? 'rollout' : 'step'} ${x.points[0][0]}`
      : ''
  if (rl) {
    card('Reward · last', pick('rollout/raw_reward'), from)
    const len = pick('rollout/response_len/mean') ?? pick('rollout/response_len/median')
    card('Response length · last', len, (x) => x.name)
  } else {
    card('Train loss · last', pick('train/loss'), from)
    card(
      'Eval loss · last',
      evalLoss.value,
      (x) => `${x.name} · step ${lastOf(x)?.[0]} · ${x.count} evals`,
    )
  }
  const top = [...peaks.value].sort((a, b) => b.vramBytes - a.vramBytes)[0]
  const hostPeak = Math.max(0, ...peaks.value.map((p) => p.hostMemBytes))
  out.push({
    label: 'Peak VRAM per GPU',
    value: top ? `${(top.vramBytes / 1e9).toFixed(1)} GB` : '—',
    sub: top
      ? `On ${top.host} · host memory peak ${(hostPeak / 1e12).toFixed(2)} TB`
      : 'No mon/ samples',
  })
  return out
})

/** The overview's two curves, as the run's panels draw them: loss and learning rate, or reward and length. */
const overviewPanels = computed(() => {
  const panels: { title: string; series: MetricSeries[] }[] = []
  const add = (title: string, list: (MetricSeries | undefined)[]) => {
    const series = list.filter((x): x is MetricSeries => !!x)
    if (series.length) panels.push({ title, series })
  }
  if (props.run.trainType === 'rl') {
    add('Reward', [pick('rollout/raw_reward')])
    add('Response length', [pick('rollout/response_len/mean')])
  } else {
    add('Loss', [pick('train/loss'), evalLoss.value])
    add('Learning rate', [(props.metrics?.series ?? []).find((x) => x.name.startsWith('train/lr'))])
  }
  return panels
})

const finalEval = computed(() => s.value.finalEval ?? undefined)
const finalEvalRows = computed(() =>
  Object.entries(finalEval.value?.data ?? {})
    .filter(([k]) => k !== 'gate')
    .map(([k, v]) => [k, show(v)] as [string, string]),
)
const finalEvalEnd = computed(() => {
  const g = s.value.gates.find((x) => x.gate === finalEval.value?.gate && x.end)
  return g?.end ? dayjs(g.end).format('MM-DD HH:mm') : ''
})

/** Where the run came from: its directory, nodes, weights, data, parallelism, trainable size, image and log. */
const sourceRows = computed(() => {
  const k = kept.value
  const log = s.value.mainLog
  const rows: { label: string; value: string; copy?: string }[] = [
    { label: 'Run directory', value: s.value.path, copy: s.value.path },
    {
      label: 'Nodes',
      value: s.value.nodes.map((n) => (n.head ? `${n.host} (head)` : n.host)).join(' · ') || '—',
    },
    {
      label: 'Base weights',
      value: show(k.load ?? k.hf_checkpoint),
      copy: String(k.load ?? '') || undefined,
    },
    { label: 'Data', value: show(k.prompt_data), copy: String(k.prompt_data ?? '') || undefined },
    { label: 'Parallelism', value: parallelismText(k) || '—' },
    {
      label: 'Trainable parameters',
      value: trainable.value ? `${trainable.value} (from the check output)` : '—',
    },
    { label: 'Image', value: s.value.image || '—', copy: s.value.image || undefined },
    {
      label: 'Main log',
      value: log ? `${log.name} · ${formatBytes(log.bytes, 1)}` : '—',
      copy: log ? `${s.value.path}/logs/${log.name}` : undefined,
    },
  ]
  return rows
})

const isFailed = (row: ImportGateRow) => row === failedGateOf(s.value)
const gateRowClass = ({ row }: { row: ImportGateRow }) =>
  isFailed(row) ? 'pt-import-failed-row' : ''

const markerRows = computed(() =>
  s.value.markers.map((m) => ({
    name: m.name,
    written: clock(m.time),
    after:
      m.time && s.value.startTime
        ? gateDuration((dayjs(m.time).valueOf() - dayjs(s.value.startTime).valueOf()) / 1000)
        : '—',
  })),
)

/** The key settings of the arguments section, as a run's recipe would show them. */
const configRows = computed(() => {
  const k = kept.value
  const rl = props.run.trainType === 'rl'
  const rows: [string, unknown][] = [
    ['Base weights', k.load],
    ['HF checkpoint', k.hf_checkpoint],
    ['Data', k.prompt_data],
    ['Eval data', k.eval_prompt_data],
    ['Resources', `${props.run.nodeCount} nodes × ${props.run.gpuPerNode} GPUs`],
    ['Parallelism', parallelismText(k)],
    ['Global batch', k.global_batch_size],
    [
      'Learning rate',
      k.lr != null ? `${show(k.lr)} → ${show(k.min_lr)} · ${show(k.lr_decay_style)}` : '',
    ],
    ['Loss', k.loss_type],
  ]
  if (rl)
    rows.push(
      [
        'Rollout batch',
        `${show(k.rollout_batch_size)} prompts × ${show(k.n_samples_per_prompt)} samples`,
      ],
      ['Rollouts', k.num_rollout],
      ['Advantage', k.advantage_estimator],
      ['KL loss', k.use_kl_loss ? `coef ${show(k.kl_loss_coef)}` : 'off'],
      ['Reward', k.custom_rm_path ?? k.rm_type],
      ['Rollout function', k.rollout_function_path],
    )
  rows.push(
    ['Saving', k.save ? `${show(k.save)} every ${show(k.save_interval)}` : '--save not set'],
    ['Image', s.value.image],
  )
  return rows
    .filter(([, v]) => v != null && v !== '')
    .map(([label, v]) => ({ label, value: show(v) }))
})

const keptRows = computed(() =>
  Object.entries(kept.value)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([name, v]) => ({ name, value: show(v) })),
)
</script>

<style scoped>
.pt-cards {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.pt-stat {
  padding: 12px;
  border: 1px solid var(--safe-border);
  border-radius: 8px;
}
.pt-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}
.pt-import-check {
  max-height: 360px;
  margin: 0;
  padding: 10px 12px;
  overflow: auto;
  border-radius: 6px;
  background: var(--el-fill-color-light);
  font-size: 12px;
  line-height: 18px;
  white-space: pre-wrap;
  word-break: break-all;
}
.pt-import-check .is-fail {
  color: var(--el-color-danger);
}
/* The theme paints table rows itself, so the failed gate's cells override it. */
:deep(.pt-import-failed-row > td.el-table__cell) {
  background-color: color-mix(in srgb, var(--el-color-danger) 14%, transparent) !important;
}
</style>

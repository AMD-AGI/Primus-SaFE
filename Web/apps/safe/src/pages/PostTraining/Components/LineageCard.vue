<template>
  <el-card class="mt-4 safe-card" shadow="never">
    <div class="flex items-center justify-between">
      <div class="flex items-center">
        <div class="w-1 hx-16 bg-[var(--safe-primary)] mr-2 rounded-sm"></div>
        <span class="fs-subtitle font-medium">Lineage</span>
      </div>
      <el-tooltip
        v-if="model.origin === 'fine_tuned'"
        :content="startRlReason"
        :disabled="!startRlReason"
        placement="top"
      >
        <span>
          <el-button round size="small" :disabled="!!startRlReason" @click="startRl">
            Start RL from This Model
          </el-button>
        </span>
      </el-tooltip>
    </div>
    <div v-loading="loading" class="pt-chain mt-3">
      <template v-for="(n, i) in nodes" :key="`${n.kind}-${n.id}`">
        <span v-if="i > 0" class="text-[var(--safe-muted)]">→</span>
        <el-tooltip :content="n.name" placement="top" :show-after="300">
          <div
            class="pt-node"
            :class="{ current: n.current, deleted: n.deleted, link: !n.deleted && !n.current }"
            @click="open(n)"
          >
            <div class="text-[12px] text-[var(--safe-muted)]">{{ n.label }}</div>
            <div class="truncate">{{ n.name }}</div>
            <el-button
              v-if="n.sftJobId"
              link
              type="primary"
              class="text-[12px]"
              @click.stop="router.push({ path: '/training/detail', query: { id: n.sftJobId } })"
            >
              Open Post Train job
            </el-button>
          </div>
        </el-tooltip>
      </template>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { getModelDetail, type PlaygroundModel } from '@/services/playground'
import { getPostTrainingRun, type RunDetail } from '@/services/postTraining'

const props = defineProps<{
  model: PlaygroundModel
  /** Why Start RL from This Model is disabled, or an empty string. */
  startRlReason: string
}>()

const router = useRouter()

/** How far up the chain is followed. */
const MAX_DEPTH = 8

type ChainNode = {
  kind: 'model' | 'run'
  id: string
  name: string
  label: string
  current?: boolean
  deleted?: boolean
  sftJobId?: string
}

const nodes = ref<ChainNode[]>([])
const loading = ref(false)

const originLabel = (origin?: string) =>
  origin === 'fine_tuned' ? 'SFT' : origin === 'rl_trained' ? 'RL' : 'Base'

const runLabel = (run: RunDetail) => {
  const how = run.algorithm ? run.algorithm.toUpperCase() : run.method === 'lora' ? 'LoRA' : 'Full'
  return `Post Training run · ${run.trainType.toUpperCase()} · ${how}`
}

/**
 * Follows the model's Post Training run to its base model, and on through the base's own run, until a model that no
 * Post Training run produced, a model from Post Train or a deleted base model.
 */
const build = async (model: PlaygroundModel) => {
  const chain: ChainNode[] = []
  let current = model
  for (let depth = 0; depth < MAX_DEPTH; depth++) {
    const isCurrent = depth === 0
    const name = current.displayName || current.id
    if (!current.postTrainingRunId) {
      chain.unshift(
        current.sftJobId
          ? {
              kind: 'model',
              id: current.id,
              name,
              label: 'Model · from Post Train',
              sftJobId: current.sftJobId,
            }
          : { kind: 'model', id: current.id, name, label: 'Base model', current: isCurrent },
      )
      break
    }
    const run = (await getPostTrainingRun(current.postTrainingRunId, true).catch(
      () => undefined,
    )) as unknown as RunDetail | undefined
    const step = run?.metricSummary?.main?.step
    chain.unshift({
      kind: 'model',
      id: current.id,
      name,
      label: isCurrent
        ? `This model${step !== undefined ? ` · step ${step.toLocaleString()}` : ''}`
        : `Model · ${originLabel(current.origin)}`,
      current: isCurrent,
    })
    if (!run) break
    chain.unshift({ kind: 'run', id: run.runId, name: run.displayName, label: runLabel(run) })
    const base = (await getModelDetail(run.baseModelId, true).catch(() => undefined)) as unknown as
      | PlaygroundModel
      | undefined
    if (!base || base.isDeleted) {
      chain.unshift({
        kind: 'model',
        id: run.baseModelId,
        name: run.baseModelName || run.baseModelId,
        label: 'Base model · deleted',
        deleted: true,
      })
      break
    }
    current = base
  }
  return chain
}

const open = (n: ChainNode) => {
  if (n.deleted || n.current) return
  if (n.kind === 'run') router.push({ path: '/post-training/detail', query: { runId: n.id } })
  else router.push(`/model-square/detail/${n.id}`)
}

const startRl = () => {
  router.push({
    path: '/post-training/create',
    query: { modelId: props.model.id, trainType: 'rl' },
  })
}

watch(
  () => props.model.id,
  async () => {
    loading.value = true
    try {
      nodes.value = await build(props.model)
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)
</script>

<style scoped>
.pt-chain {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-height: 48px;
}
.pt-node {
  max-width: 240px;
  padding: 6px 10px;
  border: 1px solid var(--safe-border);
  border-radius: 8px;
}
.pt-node.link {
  cursor: pointer;
}
.pt-node.current {
  border-color: var(--safe-primary);
}
.pt-node.deleted {
  border-style: dashed;
  color: var(--safe-muted);
}
</style>

<template>
  <el-card class="safe-card pt-preflight" shadow="never">
    <div class="flex items-center gap-2">
      <span class="font-500">Preflight</span>
      <el-tag v-if="w.errors.value.length" size="small" type="danger">{{
        count(w.errors.value.length, 'error')
      }}</el-tag>
      <el-tag v-if="w.warnings.value.length" size="small" type="warning">
        {{ count(w.warnings.value.length, 'warning') }}
      </el-tag>
      <span v-if="w.checking.value" class="text-[12px] text-[var(--safe-muted)]">Checking…</span>
    </div>

    <div v-if="w.previewError.value" class="mt-3">
      <div class="font-500">Preflight unavailable</div>
      <div class="text-[12px] text-[var(--safe-muted)]">{{ w.previewError.value }}</div>
      <el-button class="mt-2" size="small" round @click="w.runPreview()">Retry</el-button>
    </div>

    <template v-else>
      <div v-if="w.errors.value.length" class="mt-3">
        <div class="pt-preflight-title">Errors</div>
        <div v-for="(d, i) in w.errors.value" :key="`e${i}`" class="pt-preflight-item">
          <span class="text-[var(--el-color-danger)]">{{ d.message }}</span>
          <el-link
            v-if="fieldStep(d.field)"
            type="primary"
            class="ml-1"
            @click="emit('goto', fieldStep(d.field))"
          >
            Step {{ fieldStep(d.field) }}
          </el-link>
        </div>
      </div>
      <div v-if="w.warnings.value.length" class="mt-3">
        <div class="pt-preflight-title">Warnings</div>
        <div
          v-for="(d, i) in w.warnings.value"
          :key="`w${i}`"
          class="pt-preflight-item text-[var(--safe-warning)]"
        >
          {{ d.message }}
        </div>
      </div>

      <div class="mt-3">
        <div class="pt-preflight-title">Resources</div>
        <el-descriptions :column="1" size="small">
          <el-descriptions-item label="Framework">{{
            w.form.framework || '—'
          }}</el-descriptions-item>
          <el-descriptions-item label="Nodes">{{ nodesText }}</el-descriptions-item>
          <el-descriptions-item label="Layout">{{ layoutText }}</el-descriptions-item>
          <el-descriptions-item label="Convert">{{ convertText }}</el-descriptions-item>
          <el-descriptions-item label="Image">{{ imageText }}</el-descriptions-item>
        </el-descriptions>
      </div>

      <div v-if="w.preview.value" class="mt-3">
        <div class="pt-preflight-title">On this platform</div>
        <div v-if="w.preview.value.verifiedRuns" class="flex items-center gap-2 text-[13px]">
          <el-tag size="small" type="primary" effect="light" round
            >✓ Verified ×{{ w.preview.value.verifiedRuns }}</el-tag
          >
          <el-link
            v-if="w.preview.value.latestVerifiedRun && router.hasRoute('PostTrainingDetail')"
            type="primary"
            :underline="false"
            @click="openLatest"
          >
            Latest run
          </el-link>
        </div>
        <div v-else class="text-[13px] text-[var(--safe-muted)]">Not run on this platform yet</div>
      </div>

      <div v-if="w.checkedAt.value" class="mt-2 text-[12px] text-[var(--safe-muted)]">
        Checked {{ checkedAgo }}
      </div>
    </template>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { useNow } from '@vueuse/core'
import { fieldStep } from '@/services/postTraining'
import { useWizard } from './wizard'

const emit = defineEmits<{ goto: [step: number] }>()
const w = useWizard()
const router = useRouter()
const now = useNow({ interval: 5000 })

/** Opens the latest successful run of this recipe in a new tab, keeping the wizard as it is. */
const openLatest = () => {
  const runId = w.preview.value?.latestVerifiedRun
  if (runId)
    window.open(router.resolve({ path: '/post-training/detail', query: { runId } }).href, '_blank')
}

const count = (n: number, noun: string) => `${n} ${noun}${n === 1 ? '' : 's'}`

const layout = computed(() => w.preview.value?.layout)
const recipe = computed(() => w.preview.value?.recipe as Record<string, unknown> | undefined)

const nodesText = computed(() => {
  const l = layout.value
  return l
    ? `${l.nodes} × ${l.gpusPerNode} GPUs`
    : `${w.form.resources.nodes} × ${w.form.resources.gpusPerNode} GPUs`
})

const layoutText = computed(() => {
  const l = layout.value
  if (!l || w.form.trainType !== 'rl')
    return w.form.trainType === 'rl' ? '—' : 'Training on every GPU'
  return l.colocate
    ? 'Colocated'
    : `Disaggregated · ${l.actorGpus} train + ${l.rolloutGpus} rollout`
})

const convertText = computed(() => {
  const convert = recipe.value?.convert as { cacheKey?: string; cacheHit?: boolean } | undefined
  if (!convert) return '—'
  if (convert.cacheHit) return 'Cached · torch_dist'
  return convert.cacheKey ? 'Converts on the first run, then cached' : 'Converts in this run'
})

const imageText = computed(() => {
  const image = (recipe.value?.provenance as { image?: string } | undefined)?.image
  return image ? image.split('/').pop() : '—'
})

const checkedAgo = computed(() => {
  const seconds = Math.round((now.value.getTime() - w.checkedAt.value) / 1000)
  return seconds < 10 ? 'just now' : `${seconds}s ago`
})
</script>

<style scoped>
.pt-preflight-title {
  font-size: 12px;
  color: var(--safe-muted);
  margin-bottom: 4px;
}
.pt-preflight-item {
  font-size: 13px;
  line-height: 1.5;
  margin-bottom: 4px;
}
</style>

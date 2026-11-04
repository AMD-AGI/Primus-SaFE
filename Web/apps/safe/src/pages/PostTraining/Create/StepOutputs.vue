<template>
  <el-form label-position="top">
    <el-form-item label="Export Hugging Face model">
      <el-switch v-model="exportHf" />
      <div class="pt-help">
        The exported model is registered in Model Square and can be evaluated.
      </div>
    </el-form-item>
    <el-form-item v-if="w.form.method === 'lora'" label="Also export PEFT adapter">
      <el-switch v-model="peftAdapter" />
    </el-form-item>

    <el-form-item label="Automatic evaluation" :error="fieldError('evaluation.benchmarks')">
      <el-tooltip content="Needs an exported model." :disabled="exportHf" placement="top">
        <span><el-switch v-model="evalEnabled" :disabled="!exportHf" /></span>
      </el-tooltip>
    </el-form-item>
    <template v-if="evalEnabled">
      <el-form-item label="Benchmarks" required>
        <el-select
          v-model="benchmarks"
          multiple
          filterable
          placeholder="Select evaluation datasets"
          style="width: 420px"
        >
          <el-option
            v-for="d in w.evalDatasets.value"
            :key="d.datasetId"
            :label="d.displayName"
            :value="d.datasetId"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="Keep the inference service after evaluation">
        <el-switch v-model="keepService" />
      </el-form-item>
    </template>

    <el-form-item label="Priority">
      <el-select v-model="priority" style="width: 160px">
        <el-option label="Low" :value="0" />
        <el-option label="Medium" :value="1" />
        <el-option v-if="canHighPriority" label="High" :value="2" />
      </el-select>
    </el-form-item>
    <el-form-item label="Timeout (hours, 0 for none)">
      <el-input-number v-model="timeoutHours" :min="0" :max="720" />
    </el-form-item>
    <el-form-item label="Retries on failure">
      <el-input-number v-model="w.form.resources.maxRetry" :min="0" :max="3" />
      <div class="pt-help">
        A failed attempt resumes from the latest checkpoint. Preemptions do not count; a full disk,
        missing dataset fields or a rejected reward key are not retried.
      </div>
    </el-form-item>

    <el-collapse>
      <el-collapse-item title="Advanced" name="advanced">
        <el-form-item label="Image" :error="fieldError('resources.image')">
          <el-input
            v-model="image"
            placeholder="Default image of this GPU architecture"
            style="width: 420px"
          />
        </el-form-item>
        <el-form-item label="Weights & Biases">
          <el-switch v-model="wandb.enabled" />
        </el-form-item>
        <template v-if="wandb.enabled">
          <el-form-item label="W&B secret" required>
            <el-select
              v-model="wandb.secretId"
              filterable
              placeholder="General secret with WANDB_API_KEY"
              style="width: 300px"
            >
              <el-option
                v-for="s in secrets"
                :key="s.secretId"
                :label="s.secretName || s.secretId"
                :value="s.secretId"
              />
            </el-select>
          </el-form-item>
          <el-form-item label="Project" required>
            <el-input v-model="wandb.project" style="width: 300px" />
          </el-form-item>
          <el-form-item label="Team">
            <el-input v-model="wandb.team" style="width: 300px" />
          </el-form-item>
        </template>
      </el-collapse-item>
    </el-collapse>
  </el-form>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getSecrets } from '@/services/nodes'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import { useWizard } from './wizard'

const w = useWizard()
const userStore = useUserStore()
const wsStore = useWorkspaceStore()

const canHighPriority = computed(() => userStore.isManager || wsStore.isCurrentWorkspaceAdmin())

const exportHf = computed({
  get: () => w.form.export.hf !== false,
  set: (v: boolean) => {
    w.form.export.hf = v
    if (!v) w.form.evaluation.enabled = false
  },
})
const peftAdapter = computed({
  get: () => !!w.form.export.peftAdapter,
  set: (v: boolean) => (w.form.export.peftAdapter = v || undefined),
})
const evalEnabled = computed({
  get: () => !!w.form.evaluation.enabled,
  set: (v: boolean) => (w.form.evaluation.enabled = v || undefined),
})
const benchmarks = computed({
  get: () => w.form.evaluation.benchmarks ?? [],
  set: (v: string[]) => (w.form.evaluation.benchmarks = v),
})
const keepService = computed({
  get: () => !!w.form.evaluation.keepService,
  set: (v: boolean) => (w.form.evaluation.keepService = v || undefined),
})
const priority = computed({
  get: () => w.form.resources.priority ?? 1,
  set: (v: number) => (w.form.resources.priority = v),
})
const timeoutHours = computed({
  get: () => Math.round((w.form.resources.timeoutSeconds ?? 0) / 3600),
  set: (v?: number) => (w.form.resources.timeoutSeconds = v ? v * 3600 : undefined),
})
const image = computed({
  get: () => w.form.resources.image ?? '',
  set: (v: string) => (w.form.resources.image = v.trim() || undefined),
})
const wandb = computed(() => w.form.tracking.wandb)

const secrets = ref<{ secretId: string; secretName?: string }[]>([])
onMounted(async () => {
  const res = (await getSecrets({ type: 'general' }).catch(() => ({ items: [] }))) as {
    items?: { secretId: string; secretName?: string }[]
  }
  secrets.value = res.items ?? []
})

/** The first error of the preview on a field. */
const fieldError = (field: string) => w.errors.value.find((d) => d.field === field)?.message ?? ''
</script>

<style scoped>
.pt-help {
  width: 100%;
  margin-top: 4px;
  font-size: 12px;
  color: var(--safe-muted);
}
</style>

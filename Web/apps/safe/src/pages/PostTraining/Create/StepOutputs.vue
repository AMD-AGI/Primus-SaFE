<template>
  <el-form label-width="auto">
    <el-form-item label="Export Hugging Face model">
      <el-switch v-model="exportHf" />
      <div class="pt-help">The exported model is registered in Model Square.</div>
    </el-form-item>

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
          <div>
            <el-input
              v-model="image"
              placeholder="Default image of this GPU architecture"
              style="width: 420px"
            />
            <div
              v-for="d in imageWarnings"
              :key="d.message"
              class="mt-1 text-[12px] text-[var(--safe-warning)]"
            >
              {{ d.message }}
              <el-button link type="primary" @click="image = ''">Use the default image</el-button>
            </div>
          </div>
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
  set: (v: boolean) => (w.form.export.hf = v),
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
const imageWarnings = computed(() => w.warnings.value.filter((d) => d.field === 'resources.image'))
</script>

<style scoped>
.pt-help {
  width: 100%;
  margin-top: 4px;
  font-size: 12px;
  color: var(--safe-muted);
}
</style>

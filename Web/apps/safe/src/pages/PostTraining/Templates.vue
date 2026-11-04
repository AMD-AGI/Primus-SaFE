<template>
  <div class="pt-templates">
    <div class="flex items-center gap-2">
      <el-button link :icon="ArrowLeft" @click="router.push('/post-training')" />
      <span class="text-[20px] font-600">Templates</span>
      <span class="text-[13px] text-[var(--safe-muted)]">
        {{ workspaceName }} · {{ items.length }} {{ items.length === 1 ? 'template' : 'templates' }}
      </span>
    </div>

    <div class="flex items-center gap-3 mt-3">
      <el-radio-group v-model="trainType" @change="load">
        <el-radio-button value="">All</el-radio-button>
        <el-radio-button value="sft">SFT</el-radio-button>
        <el-radio-button value="rl">RL</el-radio-button>
      </el-radio-group>
      <el-input v-model="search" placeholder="Search by name" clearable style="width: 220px" />
    </div>

    <el-card class="safe-card mt-3" shadow="never">
      <el-table v-loading="loading" :data="filtered" size="large">
        <template #empty>
          <div class="py-6">
            <div class="text-[15px] font-500">No templates in this workspace yet.</div>
            <div class="text-[13px] text-[var(--safe-muted)] mt-1">
              Save one from the last step of the wizard or from a run's detail page.
            </div>
            <el-button
              class="mt-3"
              type="primary"
              round
              :disabled="isReadonly"
              @click="router.push('/post-training/create')"
            >
              New Run
            </el-button>
          </div>
        </template>
        <el-table-column label="Name" min-width="240">
          <template #default="{ row }">
            <div class="font-500">{{ row.name }}</div>
            <div v-if="row.description" class="text-[12px] text-[var(--safe-muted)]">
              {{ row.description }}
            </div>
          </template>
        </el-table-column>
        <el-table-column label="Type / Method" width="170">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ row.trainType.toUpperCase() }}</el-tag>
            <el-tag size="small" type="info" effect="plain" class="ml-1">
              {{ row.method === 'lora' ? 'LoRA' : 'Full' }}
            </el-tag>
            <span v-if="algorithmOf(row)" class="text-[12px] text-[var(--safe-muted)] ml-1">
              {{ algorithmOf(row) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="Applies To" width="170">
          <template #default="{ row }">{{ row.spec || 'Any supported model' }}</template>
        </el-table-column>
        <el-table-column label="Includes" min-width="200">
          <template #default="{ row }">{{ includesText(row) }}</template>
        </el-table-column>
        <el-table-column label="Version" width="90">
          <template #default="{ row }">v{{ row.version }}</template>
        </el-table-column>
        <el-table-column label="Succeeded Runs" width="130" prop="succeededRuns" />
        <el-table-column label="Updated" width="150">
          <template #default="{ row }">
            <div>{{ updatedAt(row) }}</div>
            <div class="text-[12px] text-[var(--safe-muted)]">{{ row.userName }}</div>
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="190" fixed="right">
          <template #default="{ row }">
            <el-tooltip :content="readOnlyTip" :disabled="!isReadonly" placement="top">
              <span>
                <el-button link type="primary" :disabled="isReadonly" @click="use(row)"
                  >Use</el-button
                >
                <el-button link type="primary" :disabled="isReadonly" @click="edit(row)"
                  >Edit</el-button
                >
                <el-button link type="danger" :disabled="isReadonly" @click="remove(row)"
                  >Delete</el-button
                >
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft } from '@element-plus/icons-vue'
import dayjs from 'dayjs'
import { useUserStore } from '@/stores/user'
import { useWorkspaceStore } from '@/stores/workspace'
import {
  deletePostTrainingTemplate,
  listPostTrainingTemplates,
  timeOf,
  type TemplateItem,
  type TemplateListResp,
} from '@/services/postTraining'

defineOptions({ name: 'PostTrainingTemplatesPage' })

const readOnlyTip = 'Read-only access'
const router = useRouter()
const userStore = useUserStore()
const wsStore = useWorkspaceStore()
const isReadonly = computed(() => userStore.isReadonlyManager)
const workspace = computed(() => wsStore.currentWorkspaceId ?? '')
const workspaceName = computed(
  () =>
    wsStore.items.find((i) => i.workspaceId === workspace.value)?.workspaceName || workspace.value,
)

const items = ref<TemplateItem[]>([])
const loading = ref(false)
const trainType = ref('')
const search = ref('')

/** Templates whose name matches the search, which is filtered here rather than by the API. */
const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return q ? items.value.filter((t) => t.name.toLowerCase().includes(q)) : items.value
})

/** Loads the workspace's templates of the selected train type. */
const load = async () => {
  if (!workspace.value) return
  loading.value = true
  try {
    const res = (await listPostTrainingTemplates({
      workspace: workspace.value,
      trainType: trainType.value || undefined,
    })) as unknown as TemplateListResp
    items.value = res.items ?? []
  } finally {
    loading.value = false
  }
}

const algorithmOf = (t: TemplateItem) =>
  t.trainType === 'rl' ? (t.content.choices?.advantageEstimator ?? '').toUpperCase() : ''

/** What the template stores besides the options: base model, datasets, benchmarks and W&B. */
const includesText = (t: TemplateItem) => {
  const c = t.content
  const parts = [c.modelId, [c.datasetId, c.evalDatasetId].filter(Boolean).join(', ')].filter(
    Boolean,
  )
  const benchmarks = c.evaluation?.benchmarks?.length ?? 0
  if (benchmarks) parts.push(`${benchmarks} benchmark${benchmarks === 1 ? '' : 's'}`)
  if (c.tracking?.wandb?.enabled) parts.push('W&B')
  return parts.length ? parts.join(' · ') : 'Options only'
}

const updatedAt = (t: TemplateItem) => {
  const time = timeOf(t.updatedAt) ?? timeOf(t.createdAt)
  return time ? dayjs(time).format('MM-DD HH:mm') : '—'
}

const use = (t: TemplateItem) => {
  router.push({ path: '/post-training/create', query: { templateId: t.templateId } })
}

const edit = (t: TemplateItem) => {
  router.push({
    path: '/post-training/create',
    query: { templateId: t.templateId, mode: 'template' },
  })
}

const remove = async (t: TemplateItem) => {
  try {
    await ElMessageBox.confirm(
      'Runs created from it are not affected.',
      `Delete template ${t.name}?`,
      { confirmButtonText: 'Delete', cancelButtonText: 'Cancel', type: 'warning' },
    )
  } catch {
    return
  }
  await deletePostTrainingTemplate(t.templateId)
  ElMessage.success('Template deleted')
  load()
}

watch(workspace, load)
onMounted(load)
</script>

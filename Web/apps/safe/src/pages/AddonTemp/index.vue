<template>
  <div class="addon-template-page page-decor">
    <!-- Page Title -->
    <el-text class="block textx-18 font-500 mb-4" tag="b"
      >Catalog ({{ addonType === 'helm' ? 'Helm' : 'Node' }})</el-text
    >

    <!-- Content -->
    <el-skeleton :loading="loading" animated>
      <template #template>
        <el-row :gutter="20" class="mb-2">
          <el-col v-for="i in 8" :key="i" :xs="24" :sm="12" :md="12" :lg="8" :xl="6" class="mb-4">
            <el-skeleton-item
              variant="image"
              style="width: 100%; height: 180px; border-radius: 16px"
            />
            <div class="mt-2">
              <el-skeleton-item variant="h3" style="width: 60%" />
              <el-skeleton-item variant="text" style="width: 95%" />
              <el-skeleton-item variant="text" style="width: 80%" />
            </div>
          </el-col>
        </el-row>
      </template>

      <template #default>
        <template v-if="filteredItems.length">
          <el-row :gutter="24">
            <el-col
              v-for="item in filteredItems"
              :key="item.addonTemplateId"
              :xs="24"
              :sm="24"
              :md="12"
              :lg="8"
              :xl="6"
              class="mb-6"
            >
              <el-card
                shadow="never"
                class="addon-card glass-card use-grid-head"
                :data-watermark="logoText(item)"
              >
                <!-- Header -->
                <div class="card-head grid-head">
                  <div class="logo-circle" :title="item.category">
                    <span class="logo-text">{{ logoText(item) }}</span>
                  </div>
                  <div class="title-wrap">
                    <el-text tag="b" class="title" :title="item.addonTemplateId">
                      {{ item.addonTemplateId }}
                    </el-text>
                    <div class="subline">
                      <span class="muted">Created {{ formatDate(item.creationTime) }}</span>
                    </div>
                  </div>
                  <el-tag v-if="item.required" type="danger" effect="dark" size="small"
                    >Required</el-tag
                  >
                </div>

                <!-- Description -->
                <el-text class="desc two-line" :title="item.description || '-'">
                  {{ item.description || '-' }}
                </el-text>

                <!-- Meta -->
                <div class="meta-peas">
                  <span class="pea"><i class="pea-dot"></i>Type: {{ item.type || '-' }}</span>
                  <span class="pea"><i class="pea-dot"></i>Version: {{ item.version || '-' }}</span>
                  <span class="pea hide-sm"
                    ><i class="pea-dot"></i>Category: {{ item.category || '-' }}</span
                  >
                  <span class="pea hide-sm"
                    ><i class="pea-dot"></i>GPU: {{ (item.gpuChip || '-').toUpperCase() }}</span
                  >
                </div>

                <!-- Bottom actions -->
                <div class="card-foot">
                  <div class="foot-spacer"></div>
                  <div class="actions">
                    <el-button
                      type="primary"
                      size="small"
                      @click="handleDeploy(item)"
                      >Deploy</el-button
                    >
                    <!-- <el-button size="small" @click="handleDetail(item)">Details</el-button> -->
                  </div>
                </div>

                <div class="corner-watermark" aria-hidden="true">{{ logoText(item) }}</div>
              </el-card>
            </el-col>
          </el-row>
        </template>

        <el-empty v-else description="No templates found" class="mt-10">
          <el-button @click="clearFilters">Clear Filters</el-button>
        </el-empty>
      </template>
    </el-skeleton>
  </div>

  <!-- Deploy dialog -->
  <DeployDialog
    v-model:visible="deployVisible"
    :action="'Create'"
    :id="curTempId"
    @success="
      () => {
        router.push('/addons')
      }
    "
  />
</template>

<script lang="ts" setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
// import { Search } from '@element-plus/icons-vue'
import { getAddontemps, type AddonTemp } from '@/services'
import DeployDialog from './Components/DeployDialog.vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const loading = ref(false)
const items = ref<AddonTemp[]>([])

const typeOptions = ref<string[]>([]) // derived from data
const categoryOptions = ref<string[]>([]) // derived from data

const deployVisible = ref(false)
const curTempId = ref('')

const query = reactive({
  keyword: '',
  type: '' as string | '',
  category: '' as string | '',
  gpuChip: '' as string | '',
  requiredOnly: false,
  sortBy: 'newest' as 'newest' | 'name_asc' | 'version_desc',
})

const addonType = computed(() => {
  const t = route.query.type
  const val = Array.isArray(t) ? t[0] : t
  return (val as string) || 'helm' // Default: helm
})
const fetchData = async () => {
  loading.value = true
  try {
    const res = await getAddontemps({ type: addonType.value })
    const list: AddonTemp[] = res?.items ?? []
    items.value = Array.isArray(list) ? list : []
    // build filter options
    typeOptions.value = uniq(list.map((i) => i.type).filter(Boolean))
    categoryOptions.value = uniq(list.map((i) => i.category).filter(Boolean))
  } catch (e) {
    ElMessage.error('Failed to load addon templates')
  } finally {
    loading.value = false
  }
}

onMounted(fetchData)

const filteredItems = computed<AddonTemp[]>(() => {
  let data = [...items.value]

  if (query.keyword.trim()) {
    const kw = query.keyword.trim().toLowerCase()
    data = data.filter(
      (i) =>
        (i.addonTemplateId || '').toLowerCase().includes(kw) ||
        (i.description || '').toLowerCase().includes(kw),
    )
  }
  if (query.type) data = data.filter((i) => i.type === query.type)
  if (query.category) data = data.filter((i) => i.category === query.category)
  if (query.gpuChip)
    data = data.filter((i) => (i.gpuChip || '').toLowerCase() === query.gpuChip.toLowerCase())
  if (query.requiredOnly) data = data.filter((i) => !!i.required)

  // sort
  switch (query.sortBy) {
    case 'name_asc':
      data.sort((a, b) => (a.addonTemplateId || '').localeCompare(b.addonTemplateId || ''))
      break
    case 'version_desc':
      data.sort((a, b) => (b.version || '').localeCompare(a.version || ''))
      break
    default: // newest
      data.sort((a, b) => {
        const ta = new Date(a.creationTime || 0).getTime()
        const tb = new Date(b.creationTime || 0).getTime()
        return tb - ta
      })
  }
  return data
})

function clearFilters() {
  query.keyword = ''
  query.type = ''
  query.category = ''
  query.gpuChip = ''
  query.requiredOnly = false
  query.sortBy = 'newest'
}

function formatDate(iso?: string) {
  if (!iso) return '-'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '-'
  // locale date (English UI)
  return d.toLocaleString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function logoText(i: AddonTemp) {
  // short hint in the circle: H for helm / C for chart / G for gpu category etc.
  if (i.type) return i.type.slice(0, 1).toUpperCase()
  return (i.category || 'A').slice(0, 1).toUpperCase()
}

function uniq(arr: (string | undefined)[]) {
  return Array.from(new Set(arr.filter(Boolean) as string[]))
}

function handleDeploy(item: AddonTemp) {
  curTempId.value = item?.addonTemplateId
  deployVisible.value = true
}

watch(
  () => addonType.value,
  () => {
    fetchData()
  },
  { immediate: true },
)
</script>

<style scoped>
.addon-template-page {
  padding: 16px 8px;
}
.page-decor {
  border-radius: 12px;
}

/* ----------- Card glass style ----------- */
.addon-card {
  position: relative;
  border-radius: 16px;
  overflow: clip;
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-height: 220px;
  padding: 14px; /* Sync with el-card__body */
  transition:
    transform 0.18s ease,
    box-shadow 0.18s ease,
    border-color 0.18s ease;
}
.glass-card {
  /* Semi-transparent + blur */
  background: color-mix(in oklab, var(--el-fill-color) 50%, transparent);
  backdrop-filter: blur(10px) saturate(140%);
  -webkit-backdrop-filter: blur(10px) saturate(140%);
  border: 1px solid color-mix(in oklab, var(--el-color-primary) 15%, transparent);
  box-shadow: 0 6px 22px rgba(0, 0, 0, 0.06);
}
.glass-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.1);
  border-color: color-mix(in oklab, var(--el-color-primary) 28%, transparent);
}

/* Corner oversized watermark */
.corner-watermark {
  position: absolute;
  inset: auto -6px -12px auto;
  font-weight: 800;
  font-size: 72px;
  line-height: 1;
  letter-spacing: 0.02em;
  color: var(--el-text-color-regular);
  opacity: 0.06;
  pointer-events: none;
  user-select: none;
  transform: rotate(-8deg);
}

/* Header */
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.head-left {
  display: flex;
  align-items: center;
  gap: 10px;
}
.logo-circle {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  background: color-mix(in oklab, var(--el-color-primary) 12%, transparent);
  border: 1px solid color-mix(in oklab, var(--el-color-primary) 25%, transparent);
}
.logo-text {
  font-weight: 800;
  font-size: 14px;
  letter-spacing: 0.5px;
  color: var(--el-color-primary);
}

.title-wrap {
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.title {
  max-width: 28ch;
  display: block;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.subline .muted {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* Description (two-line clamp) */
.desc.two-line {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  line-height: 1.5;
  color: var(--el-text-color-regular);
  margin-top: 2px;
}

.meta-peas {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 10px;
  margin-top: 2px;
}
.pea {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 999px;
  background: color-mix(in oklab, var(--el-color-primary) 8%, transparent);
  border: 1px solid color-mix(in oklab, var(--el-color-primary) 18%, transparent);
  color: var(--el-text-color-regular);
}
.pea-dot {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: var(--el-color-primary);
  display: inline-block;
}

/* Bottom action bar */
.card-foot {
  margin-top: auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.foot-spacer {
  height: 0;
}

/* Hide some meta on small screens */
@media (max-width: 768px) {
  .hide-sm {
    display: none !important;
  }
  .addon-template-page {
    padding: 12px 6px;
  }
  .title {
    max-width: 22ch;
  }
}

.title {
  font-weight: 700;
}
.toolbar :deep(.el-input__inner) {
  height: 36px;
}

.addon-card {
  padding: 18px;
  gap: 12px;
  min-height: 228px;
}
.desc.two-line {
  margin-top: 6px;
  margin-bottom: 6px;
}
.meta-peas {
  margin-top: 6px;
  margin-bottom: 12px;
}
.card-foot {
  margin-top: 14px; /* Separate body from button area */
  padding-top: 8px;
}

.use-grid-head .grid-head {
  display: grid;
  grid-template-columns: 44px 1fr auto; /* Fixed badge column + flexible title column + right tags */
  align-items: center;
  column-gap: 12px;
}
.grid-head .logo-circle {
  grid-column: 1;
}
.grid-head .title-wrap {
  grid-column: 2;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
}
.grid-head .title {
  width: 100%;
  max-width: none;
  text-align: left;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.grid-head .subline {
  width: 100%;
}
.grid-head .el-tag {
  grid-column: 3;
  justify-self: end;
}

/* Badge and title details */
.logo-circle {
  width: 40px;
  height: 40px;
}
.title {
  font-weight: 700;
  letter-spacing: 0.2px;
}

/* -- Small screen: single column + collapse extra meta + prevent button crowding -- */
@media (max-width: 768px) {
  .corner-watermark {
    font-size: 56px;
    opacity: 0.05;
  }
  .addon-card {
    padding: 16px;
  }
  .meta-peas .pea.hide-sm {
    display: none !important;
  }
  .card-foot {
    margin-top: 12px;
    padding-top: 6px;
  }
  .actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
}

.card-foot {
  border-top: 1px dashed color-mix(in oklab, var(--el-border-color) 70%, transparent);
}
.actions .el-button + .el-button {
  margin-left: 0;
} /* Use gap to control spacing */
</style>

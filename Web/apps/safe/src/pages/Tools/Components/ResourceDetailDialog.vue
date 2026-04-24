<template>
  <el-dialog
    :model-value="visible"
    :title="resource?.name || 'Resource Details'"
    width="560px"
    @close="handleClose"
    :close-on-click-modal="false"
  >
    <div v-if="resource" class="detail-view">
      <!-- Top row: type + version + timeout -->
      <div class="top-row">
        <el-tag :type="resource.type === 'gpu' ? 'success' : 'primary'" effect="light">
          {{ resource.type.toUpperCase() }}
        </el-tag>
        <span class="top-meta">v{{ resource.version || '–' }}</span>
        <span class="top-meta">Timeout: {{ resource.timeout }}s</span>
      </div>

      <!-- Image -->
      <div v-if="resource.image" class="image-row">
        <code>{{ resource.image }}</code>
      </div>

      <!-- Resource limits -->
      <div class="limits-row">
        <div v-if="resource.resources.gpu" class="limit-chip">
          <span class="chip-label">GPU</span>
          <span class="chip-value">{{ resource.resources.gpu }}</span>
        </div>
        <div v-if="resource.resources.cpu" class="limit-chip">
          <span class="chip-label">CPU</span>
          <span class="chip-value">{{ resource.resources.cpu }}</span>
        </div>
        <div v-if="resource.resources.memory" class="limit-chip">
          <span class="chip-label">Memory</span>
          <span class="chip-value">{{ resource.resources.memory }}</span>
        </div>
        <div v-if="resource.resources.ephemeralStorage" class="limit-chip">
          <span class="chip-label">Ephemeral</span>
          <span class="chip-value">{{ resource.resources.ephemeralStorage }}</span>
        </div>
      </div>

      <!-- Env + Labels + Annotations (collapsed into one section) -->
      <div
        v-if="hasExtras"
        class="extras-section"
      >
        <div
          class="extras-toggle"
          @click="extrasOpen = !extrasOpen"
        >
          <span>Environment & Metadata</span>
          <el-icon :class="['chevron', { open: extrasOpen }]"><ArrowDown /></el-icon>
        </div>
        <transition name="fade-slide">
          <div v-show="extrasOpen" class="extras-body">
            <template v-if="resource.env?.length">
              <div class="extras-subtitle">Environment Variables</div>
              <div class="kv-grid">
                <div v-for="e in resource.env" :key="e.key" class="kv-item">
                  <code class="kv-key">{{ e.key }}</code>
                  <span class="kv-val">{{ e.val }}</span>
                </div>
              </div>
            </template>
            <template v-if="resource.labels && Object.keys(resource.labels).length">
              <div class="extras-subtitle">Labels</div>
              <div class="kv-tags">
                <el-tag v-for="(v, k) in resource.labels" :key="k" size="small" effect="light" class="mr-1 mb-1">
                  {{ k }}={{ v }}
                </el-tag>
              </div>
            </template>
            <template v-if="resource.annotations && Object.keys(resource.annotations).length">
              <div class="extras-subtitle">Annotations</div>
              <div class="kv-tags">
                <el-tag v-for="(v, k) in resource.annotations" :key="k" size="small" type="primary" effect="light" class="mr-1 mb-1">
                  {{ k }}={{ v }}
                </el-tag>
              </div>
            </template>
          </div>
        </transition>
      </div>

      <!-- Timestamps -->
      <div class="time-row">
        <span>Created: {{ resource.created_at.split(' ')[0] }}</span>
        <span>Updated: {{ resource.updated_at.split(' ')[0] }}</span>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-between w-full">
        <el-button type="danger" plain @click="handleDelete" :loading="deleting">Delete</el-button>
        <div class="flex gap-2">
          <el-button @click="handleClose">Close</el-button>
          <el-button type="primary" @click="emit('edit', resource!.id); handleClose()">Edit</el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown } from '@element-plus/icons-vue'
import { deleteResource, type Resource } from '@/services/tools'

const props = defineProps<{
  visible: boolean
  resource: Resource | null
}>()

const emit = defineEmits<{
  'update:visible': [val: boolean]
  edit: [id: number]
  deleted: []
}>()

const deleting = ref(false)
const extrasOpen = ref(false)

const hasExtras = computed(() => {
  if (!props.resource) return false
  return (props.resource.env?.length) ||
    (props.resource.labels && Object.keys(props.resource.labels).length) ||
    (props.resource.annotations && Object.keys(props.resource.annotations).length)
})

const handleDelete = async () => {
  if (!props.resource) return
  try {
    await ElMessageBox.confirm(
      `Delete resource "${props.resource.name}"? This cannot be undone.`,
      'Delete Resource',
      { confirmButtonText: 'Delete', type: 'warning' },
    )
    deleting.value = true
    await deleteResource(props.resource.id)
    ElMessage.success('Resource deleted')
    emit('deleted')
    handleClose()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error((e as Error).message || 'Delete failed')
  } finally {
    deleting.value = false
  }
}

const handleClose = () => emit('update:visible', false)
</script>

<style scoped lang="scss">
.detail-view {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.top-row {
  display: flex;
  align-items: center;
  gap: 12px;

  .top-meta {
    font-size: 13px;
    color: var(--el-text-color-secondary);
  }
}

.image-row code {
  display: block;
  padding: 8px 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  font-size: 12px;
  word-break: break-all;
  color: var(--el-text-color-regular);
}

.limits-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;

  .limit-chip {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    background: var(--el-fill-color-lighter);
    border-radius: 6px;
    font-size: 13px;

    .chip-label { color: var(--el-text-color-secondary); }
    .chip-value { font-weight: 600; color: var(--el-text-color-primary); }
  }
}

.extras-section {
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  overflow: hidden;

  .extras-toggle {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 10px 14px;
    cursor: pointer;
    font-size: 13px;
    font-weight: 500;
    color: var(--el-text-color-primary);

    &:hover { background: var(--el-fill-color-lighter); }
  }

  .chevron {
    transition: transform 0.2s;
    &.open { transform: rotate(180deg); }
  }

  .extras-body {
    padding: 0 14px 14px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .extras-subtitle {
    font-size: 12px;
    font-weight: 600;
    color: var(--el-text-color-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
}

.kv-grid {
  display: flex;
  flex-direction: column;
  gap: 4px;

  .kv-item {
    display: flex;
    gap: 12px;
    padding: 2px 0;
    font-size: 13px;
    .kv-key { font-weight: 500; min-width: 100px; }
    .kv-val { color: var(--el-text-color-regular); }
  }
}

.time-row {
  display: flex;
  gap: 24px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.fade-slide-enter-active, .fade-slide-leave-active {
  transition: all 0.2s ease;
}
.fade-slide-enter-from, .fade-slide-leave-to {
  opacity: 0;
  max-height: 0;
}

</style>

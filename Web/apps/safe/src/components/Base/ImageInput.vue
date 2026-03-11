<template>
  <div class="image-input w-full">
    <el-button-group class="image-input__mode mb-2">
      <el-button
        size="small"
        :type="mode === 'select' ? 'primary' : 'default'"
        :plain="mode === 'select'"
        @click="mode = 'select'"
      >
        <el-icon class="mr-1"><Search /></el-icon>Select
      </el-button>
      <el-button
        size="small"
        :type="mode === 'input' ? 'primary' : 'default'"
        :plain="mode === 'input'"
        @click="mode = 'input'"
      >
        <el-icon class="mr-1"><EditPen /></el-icon>Custom
      </el-button>
    </el-button-group>

    <div class="flex items-center gap-2 w-full">
      <!-- Select mode -->
      <el-select
        v-if="mode === 'select'"
        v-model="model"
        placeholder="Search and select image"
        clearable
        filterable
        :filter-method="filterImageOptions"
        class="flex-1"
        @visible-change="onSelectVisible"
      >
        <el-option
          v-for="item in imageOptions"
          :key="item.id"
          :label="item.tag"
          :value="item.tag"
        />
      </el-select>

      <!-- Custom mode -->
      <el-input
        v-else
        v-model="model"
        placeholder="Paste or type image address"
        clearable
        class="flex-1"
      />

      <el-button
        v-if="showCopy && model"
        :icon="CopyDocument"
        @click="handleCopy"
        text
        title="Copy image version"
      />
    </div>
  </div>
</template>

<script lang="ts" setup>
import { ref, watch } from 'vue'
import { CopyDocument, EditPen, Search } from '@element-plus/icons-vue'
import { getImagesList } from '@/services'
import { copyText } from '@/utils/index'
import { debounce } from 'lodash'

defineOptions({ name: 'ImageInput' })

const model = defineModel<string>({ default: '' })

withDefaults(
  defineProps<{
    showCopy?: boolean
  }>(),
  {
    showCopy: true,
  },
)

const mode = ref<'select' | 'input'>('select')

const imageOptions = ref<Array<{ id: number; tag: string }>>([])

const fetchImage = async (tag?: string) => {
  try {
    const res = await getImagesList({ flat: true, tag })
    imageOptions.value = res ?? []
  } catch {
    imageOptions.value = []
  }
}

const filterImageOptions = debounce(async (query: string) => {
  await fetchImage(query || undefined)
}, 300)

const onSelectVisible = (visible: boolean) => {
  if (visible && imageOptions.value.length === 0) {
    fetchImage()
  }
}

const handleCopy = async () => {
  if (!model.value) return
  await copyText(model.value)
}

// If initial value exists and not in list, auto-switch to input mode
watch(
  model,
  (val) => {
    if (val && mode.value === 'select' && imageOptions.value.length > 0) {
      const found = imageOptions.value.some((o) => o.tag === val)
      if (!found) mode.value = 'input'
    }
  },
  { once: true },
)
</script>

<style scoped>
/* Soften unselected button hover, darken background */
.image-input__mode :deep(.el-button--default:not(.el-button--primary):hover) {
  color: var(--el-text-color-regular);
  border-color: var(--el-border-color-hover);
  background-color: var(--el-fill-color-darker);
}
</style>

<template>
  <div v-for="(item, index) in list" :key="item._uid ?? index" class="flex flex-col w-full mb-2">
    <div class="flex gap-2 w-full">
      <!-- Key area: show red border and hint only when validating key -->
      <div class="flex-1 flex flex-col">
        <el-input
          v-if="keyMode === 'input'"
          v-model="item.key"
          placeholder="Key"
          :class="{
            'is-error':
              props.validate && !props.valuePlaceholderFromKey && !!keyErrorsByUid[item._uid || ''],
          }"
        />
        <el-select
          v-else
          v-model="item.key"
          placeholder="Key"
          :class="{
            'is-error':
              props.validate && !props.valuePlaceholderFromKey && !!keyErrorsByUid[item._uid || ''],
          }"
        >
          <el-option
            v-for="opt in KeyOptions"
            :key="opt.value"
            :label="opt.label"
            :value="opt.value"
          />
        </el-select>
        <!-- Error message aligned to Key -->
        <el-text
          v-if="props.validate && !props.valuePlaceholderFromKey && keyErrorsByUid[item._uid || '']"
          type="danger"
          size="small"
          class="w-full !text-left"
        >
          {{ keyErrorsByUid[item._uid || ''] }}
        </el-text>
      </div>

      <!-- Value area: show red border and hint only when validating value (Taints) -->
      <div class="flex-1 flex flex-col">
        <el-input
          v-model="item.value"
          :placeholder="props.valuePlaceholderFromKey ? 'Key' : 'Value'"
          :class="{
            'is-error':
              props.validate && props.valuePlaceholderFromKey && !!keyErrorsByUid[item._uid || ''],
          }"
        />
        <!-- Error message aligned to Value -->
        <el-text
          v-if="props.validate && props.valuePlaceholderFromKey && keyErrorsByUid[item._uid || '']"
          type="danger"
          size="small"
          class="w-full !text-left"
        >
          {{ keyErrorsByUid[item._uid || ''] }}
        </el-text>
      </div>

      <el-button type="danger" @click="handleDelete(index)">
        {{ deleteButtonText || '-' }}
      </el-button>
    </div>

    <!-- Validation error message -->
    <!-- <el-text v-if="keyErrorsByUid[item._uid || '']" type="danger" size="small" class="mt-1">
      {{ keyErrorsByUid[item._uid || ''] }}
    </el-text> -->
  </div>

  <el-button type="primary" @click="handleAdd" :disabled="list.length >= (max ?? 50)">
    {{ addButtonText || '+' }}
  </el-button>

  <el-text size="small" type="info" class="w-full mt-1">
    {{ info || `Add up to ${max ?? 50} tags` }}
  </el-text>
</template>
<script setup lang="ts">
import { ref, onMounted, watch, nextTick, computed } from 'vue'

// const fields = ['key', 'value'] as const
interface Item {
  key: string
  value: string
  _uid?: string
}

const nameRegex = /^[a-z0-9][-a-z0-9.]{0,43}[a-z0-9]$/

const props = withDefaults(
  defineProps<{
    modelValue: Item[]
    max?: number
    addButtonText?: string
    deleteButtonText?: string
    info?: string
    keyMode?: 'input' | 'select'
    KeyOptions?: Array<{ label: string; value: string }>
    /** Whether to use key as value's placeholder (for Taints scenario) */
    valuePlaceholderFromKey?: boolean
    /** Switch: validation and error display only when explicitly enabled */
    validate?: boolean
  }>(),
  {
    validate: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', val: Item[]): void
}>()

const list = ref<Item[]>([])
const keyErrorsByUid = computed<Record<string, string>>(() => {
  if (!props.validate) return {}

  const out: Record<string, string> = {}
  for (const it of list.value) {
    const uid = it._uid ?? ''
    const k = props.valuePlaceholderFromKey ? it?.value : (it?.key ?? '')

    if (!k) {
      out[uid] = '' // No hint for empty key
    } else if (!nameRegex.test(k)) {
      out[uid] = 'Please enter a valid key.'
    } else {
      out[uid] = ''
    }
  }
  return out
})

const handleAdd = () => {
  if (list.value.length < (props.max ?? 50)) {
    const newItem: Item = {
      key: props.KeyOptions?.[0]?.value ?? '', // Default to first option's value
      value: '',
    }
    ensureUid(newItem)
    list.value = [...list.value, newItem]
  }
}

const handleDelete = (index: number) => {
  list.value = list.value.filter((_, i) => i !== index)
}

function ensureUid(it: Item) {
  if (!it._uid) {
    it._uid = `${Date.now()}-${Math.random().toString(36).slice(2)}`
  }
}

onMounted(() => {
  // Edit scenario: assign _uid to existing rows to prevent DOM reuse misalignment caused by index keys
  list.value?.forEach(ensureUid)
})

let syncing = false

watch(
  () => props.modelValue,
  (val) => {
    syncing = true
    const next = (val ?? []).map((v) => {
      const it: Item = { key: v.key ?? '', value: v.value ?? '', _uid: v._uid }
      ensureUid(it)
      return it
    })
    list.value = next
    nextTick(() => {
      syncing = false
    })
  },
  { immediate: true, deep: true },
)

watch(
  list,
  (val) => {
    if (syncing) return
    emit(
      'update:modelValue',
      val.map(({ key, value, _uid }) => ({ key, value, _uid })),
    )
  },
  { deep: true },
)
</script>
<style scoped>
.is-error :deep(.el-input__wrapper),
.is-error :deep(.el-select .el-input__wrapper) {
  box-shadow: 0 0 0 1px var(--el-color-danger) inset !important;
}
</style>

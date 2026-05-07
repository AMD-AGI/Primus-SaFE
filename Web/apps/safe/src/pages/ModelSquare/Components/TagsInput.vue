<template>
  <el-form-item label="Tags">
    <div class="flex gap-2 flex-wrap">
      <el-tag
        v-for="tag in modelValue"
        :key="tag"
        closable
        type="primary"
        effect="plain"
        :disable-transitions="false"
        @close="handleRemove(tag)"
      >
        {{ tag }}
      </el-tag>
      <el-input
        v-if="inputVisible"
        ref="inputRef"
        v-model="inputValue"
        class="tag-input"
        size="small"
        @keyup.enter="handleConfirm"
        @blur="handleConfirm"
      />
      <el-button v-else size="small" @click="showInput">+ New Tag</el-button>
    </div>
  </el-form-item>
</template>

<script setup lang="ts">
import { ref, nextTick } from 'vue'
import type { InputInstance } from 'element-plus'

const props = defineProps<{ modelValue: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [val: string[]] }>()

const inputVisible = ref(false)
const inputValue = ref('')
const inputRef = ref<InputInstance>()

const showInput = () => {
  inputVisible.value = true
  nextTick(() => inputRef.value?.input?.focus())
}

const handleConfirm = () => {
  const val = inputValue.value.trim()
  if (val && !props.modelValue.includes(val)) {
    emit('update:modelValue', [...props.modelValue, val])
  }
  inputVisible.value = false
  inputValue.value = ''
}

const handleRemove = (tag: string) => {
  emit('update:modelValue', props.modelValue.filter((t) => t !== tag))
}
</script>

<style scoped>
.flex { display: flex; }
.gap-2 { gap: 8px; }
.flex-wrap { flex-wrap: wrap; }
.tag-input { width: 80px; }
</style>

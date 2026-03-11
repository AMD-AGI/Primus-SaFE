<template>
  <div class="json-editor">
    <div class="editor-header">
      <span class="editor-label">{{ label }}</span>
      <el-tooltip content="Format JSON (Cmd/Ctrl + /)" placement="top">
        <el-button
          v-if="modelValue"
          size="small"
          text
          @click="formatJson"
          :icon="Refresh"
          class="format-btn"
        >
          Format
        </el-button>
      </el-tooltip>
    </div>
    <div class="editor-container" :class="{ 'has-error': !!error, 'has-success': showSuccess }">
      <div class="line-numbers" v-if="showLineNumbers">
        <div
          v-for="line in lineCount"
          :key="line"
          class="line-number"
        >
          {{ line }}
        </div>
      </div>
      <textarea
        ref="textareaRef"
        :value="modelValue"
        @input="handleInput"
        @blur="handleBlur"
        @keydown="handleKeyDown"
        :placeholder="placeholder"
        :rows="rows"
        class="json-textarea"
        spellcheck="false"
      />
    </div>
    <transition name="fade">
      <div v-if="error" class="message error-message">
        <el-icon><CircleClose /></el-icon>
        <span>{{ error }}</span>
      </div>
      <div v-else-if="showSuccess" class="message success-message">
        <el-icon><CircleCheck /></el-icon>
        <span>Valid JSON</span>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Refresh, CircleClose, CircleCheck } from '@element-plus/icons-vue'

const props = withDefaults(defineProps<{
  modelValue: string
  label?: string
  placeholder?: string
  rows?: number
  showLineNumbers?: boolean
}>(), {
  label: 'JSON',
  placeholder: '{\n  \n}',
  rows: 12,
  showLineNumbers: true,
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  'validate': [isValid: boolean, error?: string]
}>()

const textareaRef = ref<HTMLTextAreaElement>()
const error = ref<string>('')
const showSuccess = ref(false)
const isValid = ref(false)

const lineCount = computed(() => {
  return (props.modelValue || '').split('\n').length
})

const handleInput = (e: Event) => {
  const target = e.target as HTMLTextAreaElement
  emit('update:modelValue', target.value)
  // Clear error message (on input)
  if (error.value) {
    error.value = ''
  }
  showSuccess.value = false
  isValid.value = false
}

const validateJson = (value: string): { valid: boolean; error?: string } => {
  if (!value.trim()) {
    return { valid: true }
  }

  try {
    JSON.parse(value)
    return { valid: true }
  } catch (e) {
    const err = e as Error
    let errorMsg = err.message
    
    // Improve common error messages
    if (value.match(/,\s*[}\]]/)) {
      errorMsg = 'Trailing comma detected. Remove the comma before } or ]'
    } else if (errorMsg.includes('Unexpected token')) {
      errorMsg = 'Invalid JSON syntax. Please check your JSON format.'
    } else if (errorMsg.includes('property name')) {
      errorMsg = 'Invalid property name. Property names must be in double quotes.'
    }
    
    return { valid: false, error: errorMsg }
  }
}

const handleBlur = () => {
  const result = validateJson(props.modelValue)
  if (!result.valid) {
    error.value = result.error || 'Invalid JSON'
    isValid.value = false
    emit('validate', false, result.error)
  } else if (props.modelValue.trim()) {
    error.value = ''
    isValid.value = true
    showSuccess.value = true
    emit('validate', true)
    // Auto-hide success message
    setTimeout(() => {
      showSuccess.value = false
    }, 2000)
  }
}

// Handle keyboard events
const handleKeyDown = (e: KeyboardEvent) => {
  const textarea = e.target as HTMLTextAreaElement
  
  // Tab key indentation
  if (e.key === 'Tab') {
    e.preventDefault()
    
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    const value = textarea.value
    
    if (e.shiftKey) {
      // Shift+Tab: decrease indentation
      const lineStart = value.lastIndexOf('\n', start - 1) + 1
      const lineEnd = value.indexOf('\n', end)
      const actualEnd = lineEnd === -1 ? value.length : lineEnd
      
      const selectedLines = value.substring(lineStart, actualEnd)
      const lines = selectedLines.split('\n')
      
      let newText = lines.map(line => {
        if (line.startsWith('  ')) {
          return line.substring(2)
        } else if (line.startsWith('\t')) {
          return line.substring(1)
        }
        return line
      }).join('\n')
      
      const newValue = value.substring(0, lineStart) + newText + value.substring(actualEnd)
      emit('update:modelValue', newValue)
      
      // Restore cursor position
      setTimeout(() => {
        const removed = selectedLines.length - newText.length
        textarea.selectionStart = Math.max(lineStart, start - Math.min(removed, start - lineStart))
        textarea.selectionEnd = end - removed
      })
    } else {
      // Tab: increase indentation
      if (start === end) {
        // No selection, insert two spaces
        const newValue = value.substring(0, start) + '  ' + value.substring(end)
        emit('update:modelValue', newValue)
        
        setTimeout(() => {
          textarea.selectionStart = textarea.selectionEnd = start + 2
        })
      } else {
        // Text selected, add indentation to each line
        const lineStart = value.lastIndexOf('\n', start - 1) + 1
        const lineEnd = value.indexOf('\n', end)
        const actualEnd = lineEnd === -1 ? value.length : lineEnd
        
        const selectedLines = value.substring(lineStart, actualEnd)
        const lines = selectedLines.split('\n')
        const newText = lines.map(line => '  ' + line).join('\n')
        
        const newValue = value.substring(0, lineStart) + newText + value.substring(actualEnd)
        emit('update:modelValue', newValue)
        
        setTimeout(() => {
          textarea.selectionStart = start + 2
          textarea.selectionEnd = end + (lines.length * 2)
        })
      }
    }
  }
  
  // Cmd/Ctrl + / : Format
  if ((e.metaKey || e.ctrlKey) && e.key === '/') {
    e.preventDefault()
    formatJson()
  }
  
  // Auto-complete brackets
  if (e.key === '{' && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    
    if (start === end) {
      e.preventDefault()
      const value = textarea.value
      const newValue = value.substring(0, start) + '{\n  \n}' + value.substring(end)
      emit('update:modelValue', newValue)
      
      setTimeout(() => {
        textarea.selectionStart = textarea.selectionEnd = start + 4
      })
    }
  }
  
  // Enter key auto-indentation
  if (e.key === 'Enter' && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
    const start = textarea.selectionStart
    const value = textarea.value
    const lineStart = value.lastIndexOf('\n', start - 1) + 1
    const currentLine = value.substring(lineStart, start)
    const indent = currentLine.match(/^\s*/)?.[0] || ''
    
    // If current line ends with { or [, increase indentation
    const trimmed = currentLine.trim()
    if (trimmed.endsWith('{') || trimmed.endsWith('[')) {
      e.preventDefault()
      const newValue = value.substring(0, start) + '\n' + indent + '  ' + value.substring(start)
      emit('update:modelValue', newValue)
      
      setTimeout(() => {
        textarea.selectionStart = textarea.selectionEnd = start + indent.length + 3
      })
    } else if (indent) {
      e.preventDefault()
      const newValue = value.substring(0, start) + '\n' + indent + value.substring(start)
      emit('update:modelValue', newValue)
      
      setTimeout(() => {
        textarea.selectionStart = textarea.selectionEnd = start + indent.length + 1
      })
    }
  }
}

const formatJson = () => {
  try {
    const parsed = JSON.parse(props.modelValue)
    const formatted = JSON.stringify(parsed, null, 2)
    emit('update:modelValue', formatted)
    error.value = ''
    isValid.value = true
    showSuccess.value = true
    emit('validate', true)
    setTimeout(() => {
      showSuccess.value = false
    }, 2000)
  } catch (e) {
    const result = validateJson(props.modelValue)
    error.value = result.error || 'Invalid JSON'
    isValid.value = false
    emit('validate', false, result.error)
  }
}
</script>

<style scoped lang="scss">
.json-editor {
  width: 100%;

  .editor-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 10px;

    .editor-label {
      font-size: 14px;
      font-weight: 600;
      color: var(--el-text-color-primary);
    }

    .format-btn {
      font-size: 13px;
      padding: 4px 12px;
      height: 28px;
      
      &:hover {
        color: var(--el-color-primary);
        background: var(--el-color-primary-light-9);
      }
    }
  }

  .editor-container {
    position: relative;
    border-radius: 6px;
    background: var(--el-bg-color);
    overflow: hidden;
    border: 1px solid var(--el-border-color-light);
    transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
    box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.03);

    &:hover {
      border-color: var(--el-border-color);
      box-shadow: 0 2px 4px 0 rgba(0, 0, 0, 0.06);
    }

    &:focus-within {
      border-color: var(--el-color-primary);
      box-shadow: 
        0 0 0 3px rgba(0, 229, 229, 0.1),
        0 2px 4px 0 rgba(0, 0, 0, 0.06);
    }

    &.has-error {
      border-color: var(--el-color-danger);
      background: var(--el-color-danger-light-9);
      
      &:focus-within {
        box-shadow: 
          0 0 0 3px rgba(245, 108, 108, 0.1),
          0 2px 4px 0 rgba(0, 0, 0, 0.06);
      }
    }

    &.has-success {
      border-color: var(--el-color-success-light-3);
    }

    .line-numbers {
      position: absolute;
      top: 0;
      left: 0;
      width: 44px;
      padding: 14px 10px 14px 8px;
      background: var(--el-fill-color-lighter);
      border-right: 1px solid var(--el-border-color-lighter);
      text-align: right;
      user-select: none;
      pointer-events: none;
      z-index: 1;

      .line-number {
        font-family: 'SF Mono', 'Consolas', 'Monaco', 'Courier New', monospace;
        font-size: 12px;
        line-height: 1.65;
        color: var(--el-text-color-disabled);
        height: 19.8px; // 12px * 1.65
        font-weight: 400;
      }
    }

    .json-textarea {
      width: 100%;
      padding: 14px 16px 14px 60px;
      border: none;
      outline: none;
      resize: vertical;
      min-height: 200px;
      font-family: 'SF Mono', 'Consolas', 'Monaco', 'Courier New', monospace;
      font-size: 12px;
      line-height: 1.65;
      color: var(--el-text-color-primary);
      background: transparent;
      tab-size: 2;
      transition: background-color 0.2s;

      &::placeholder {
        color: var(--el-text-color-placeholder);
        opacity: 0.6;
      }

      &::-webkit-scrollbar {
        width: 8px;
        height: 8px;
      }

      &::-webkit-scrollbar-track {
        background: transparent;
      }

      &::-webkit-scrollbar-thumb {
        background: var(--el-border-color);
        border-radius: 4px;

        &:hover {
          background: var(--el-border-color-darker);
        }
      }
    }
  }

  .message {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    margin-top: 8px;
    padding: 10px 12px;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 500;
    line-height: 1.5;

    .el-icon {
      font-size: 14px;
      flex-shrink: 0;
      margin-top: 1px;
    }

    span {
      flex: 1;
      word-break: break-word;
    }

    &.error-message {
      background: var(--el-color-danger-light-9);
      color: var(--el-color-danger);
      border: 1px solid var(--el-color-danger-light-5);
    }

    &.success-message {
      background: var(--el-color-success-light-9);
      color: var(--el-color-success);
      border: 1px solid var(--el-color-success-light-5);
    }
  }

  // Fade in/out animation
  .fade-enter-active,
  .fade-leave-active {
    transition: all 0.2s ease;
  }

  .fade-enter-from,
  .fade-leave-to {
    opacity: 0;
    transform: translateY(-4px);
  }
}

// Dark mode adaptation
.dark {
  .json-editor {
    .editor-container {
      background: rgba(0, 0, 0, 0.2);
      box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.2);

      &:hover {
        box-shadow: 0 2px 4px 0 rgba(0, 0, 0, 0.3);
      }

      .line-numbers {
        background: rgba(0, 0, 0, 0.3);
        border-right-color: var(--el-border-color-dark);
      }

      .json-textarea {
        &::-webkit-scrollbar-thumb {
          background: var(--el-border-color-dark);
        }
      }
    }

    .message {
      &.error-message {
        background: rgba(245, 108, 108, 0.15);
        border-color: rgba(245, 108, 108, 0.3);
      }

      &.success-message {
        background: rgba(103, 194, 58, 0.15);
        border-color: rgba(103, 194, 58, 0.3);
      }
    }
  }
}
</style>

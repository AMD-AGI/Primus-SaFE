import { ref, nextTick, type Ref } from 'vue'
import { ElMessage } from 'element-plus'

export interface SelectOption {
  label: string
  value: string
  [key: string]: any
}

export interface UseSelectPasteOptions {
  // Options list
  options: Ref<SelectOption[]>
  // Form field (multi-select values to update)
  modelValue: Ref<string[]>
  // Success message prefix, default 'Matched and selected'
  successMessagePrefix?: string
  // Warning message prefix, default 'Could not find'
  warningMessagePrefix?: string
  // Whether to show messages, default true
  showMessage?: boolean
}

/**
 * Add paste support for comma/newline separated strings in el-select multi-select
 * 
 * Usage example: 
 * ```ts
 * const nodeSelectRef = ref()
 * const { handleSelectVisibleChange } = useSelectPaste({
 *   options: nodeOptions,
 *   modelValue: toRef(form, 'nodeList'),
 * })
 * ```
 * 
 * In template:
 * ```vue
 * <el-select
 *   v-model="form.nodeList"
 *   ref="nodeSelectRef"
 *   @visible-change="handleSelectVisibleChange(nodeSelectRef, $event)"
 * />
 * ```
 */
export function useSelectPaste(options: UseSelectPasteOptions) {
  const {
    options: selectOptions,
    modelValue,
    successMessagePrefix = 'Matched and selected',
    warningMessagePrefix = 'Could not find',
    showMessage = true,
  } = options

  // Handle paste event
  const handlePaste = (event: ClipboardEvent) => {
    event.preventDefault()
    const pastedText = event.clipboardData?.getData('text') || ''
    processPastedText(pastedText)
  }

  // Process pasted text
  const processPastedText = (text: string) => {
    if (!text.trim()) return

    // Split by comma or newline
    const inputValues = text
      .split(/[,\n]/)
      .map((val) => val.trim())
      .filter((val) => val)

    if (inputValues.length === 0) return

    // Find matching options
    const matchedValues: string[] = []
    const notFoundValues: string[] = []

    inputValues.forEach((inputVal) => {
      // Use exact matching to avoid tus1-p15-g3 matching tus1-p15-g30
      const option = selectOptions.value.find(
        (opt) =>
          opt.label === inputVal ||
          opt.value === inputVal,
      )

      if (option) {
        matchedValues.push(option.value)
      } else {
        notFoundValues.push(inputVal)
      }
    })

    // Merge existing selections with new matches (deduplicated)
    if (matchedValues.length > 0) {
      modelValue.value = [...new Set([...modelValue.value, ...matchedValues])]
      if (showMessage) {
        ElMessage.success(`${successMessagePrefix} ${matchedValues.length} item(s)`)
      }
    }

    if (notFoundValues.length > 0 && showMessage) {
      ElMessage.warning(
        `${warningMessagePrefix}: ${notFoundValues.slice(0, 5).join(', ')}${notFoundValues.length > 5 ? '...' : ''}`,
      )
    }
  }

  // Handle dropdown visibility change, add paste event listener to input
  const handleSelectVisibleChange = (selectRef: Ref<any> | any, visible: boolean) => {
    if (visible) {
      nextTick(() => {
        // Find input element inside the dropdown
        const refValue = selectRef?.value || selectRef
        const inputEl = refValue?.$el?.querySelector('.el-select__input')
        if (inputEl) {
          inputEl.addEventListener('paste', handlePaste)
        }
      })
    }
  }

  return {
    handleSelectVisibleChange,
    processPastedText,
  }
}


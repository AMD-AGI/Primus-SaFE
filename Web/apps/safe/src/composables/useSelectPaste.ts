import { nextTick, watch, type Ref } from 'vue'
import { ElMessage } from 'element-plus'

export interface SelectOption {
  label: string
  value: string
  [key: string]: any
}

export interface UseSelectPasteOptions {
  options: Ref<SelectOption[]>
  modelValue: Ref<string[]>
  successMessagePrefix?: string
  warningMessagePrefix?: string
  showMessage?: boolean
}

/**
 * Enhances el-select (multiple + filterable) with:
 * - Paste support for comma/newline separated strings
 * - Auto-clear filter text after each selection (so users can immediately type a new query)
 * - Prevent Enter from toggling already-selected items when filter is empty
 *
 * Usage:
 * ```ts
 * const { handleSelectVisibleChange } = useSelectPaste({
 *   options: nodeOptions,
 *   modelValue: toRef(form, 'nodeList'),
 * })
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

  let cachedInputEl: HTMLInputElement | null = null

  const handlePaste = (event: ClipboardEvent) => {
    event.preventDefault()
    const pastedText = event.clipboardData?.getData('text') || ''
    processPastedText(pastedText)
  }

  const handleKeydown = (event: KeyboardEvent) => {
    if (event.key !== 'Enter') return
    const input = event.target as HTMLInputElement
    if (!input.value.trim()) {
      event.stopPropagation()
      event.preventDefault()
    }
  }

  const processPastedText = (text: string) => {
    if (!text.trim()) return

    const inputValues = text
      .split(/[,\n]/)
      .map((val) => val.trim())
      .filter((val) => val)

    if (inputValues.length === 0) return

    const matchedValues: string[] = []
    const notFoundValues: string[] = []

    inputValues.forEach((inputVal) => {
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

  const clearSelectQuery = () => {
    nextTick(() => {
      if (!cachedInputEl || cachedInputEl.value === '') return
      cachedInputEl.value = ''
      cachedInputEl.dispatchEvent(new Event('input', { bubbles: true }))
    })
  }

  watch(modelValue, clearSelectQuery)

  const cleanup = () => {
    if (cachedInputEl) {
      cachedInputEl.removeEventListener('paste', handlePaste)
      cachedInputEl.removeEventListener('keydown', handleKeydown)
      cachedInputEl = null
    }
  }

  const handleSelectVisibleChange = (selectRef: Ref<any> | any, visible: boolean) => {
    if (visible) {
      nextTick(() => {
        const refValue = selectRef?.value || selectRef
        const inputEl = refValue?.$el?.querySelector('.el-select__input') as HTMLInputElement | null
        if (inputEl) {
          cleanup()
          cachedInputEl = inputEl
          inputEl.addEventListener('paste', handlePaste)
          inputEl.addEventListener('keydown', handleKeydown)
        }
      })
    } else {
      cleanup()
    }
  }

  return {
    handleSelectVisibleChange,
    processPastedText,
  }
}


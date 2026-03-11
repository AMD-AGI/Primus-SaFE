import { useDark, useToggle } from '@vueuse/core'

const isDark = useDark({
    initialValue: 'dark'
  })
export const toggleDark = useToggle(isDark)

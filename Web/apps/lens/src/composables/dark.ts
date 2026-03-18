import { useDark, useToggle } from '@vueuse/core'

export const isDark = useDark({
    initialValue: 'dark'
  })

// Optimized dark mode toggle with performance improvements
const originalToggle = useToggle(isDark)

export const toggleDark = () => {
  // Add transition class to prevent lag
  document.documentElement.classList.add('dark-switching')
  
  // Perform the toggle
  originalToggle()
  
  // Remove transition class after a short delay
  setTimeout(() => {
    document.documentElement.classList.remove('dark-switching')
  }, 100)
}

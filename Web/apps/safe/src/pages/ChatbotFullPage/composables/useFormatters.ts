import { marked } from 'marked'

// Format structured list items (with | separators)
const formatStructuredList = (content: string): string => {
  const normalized = content.replace(/<br\s*\/?>/gi, '\n')
  const lines = normalized.split('\n')

  const formattedLines = lines.map((line) => {
    const trimmedLine = line.trim()

    if (/^[•·]\s*.+\|.+/.test(trimmedLine)) {
      const parts = trimmedLine
        .substring(1)
        .trim()
        .split('|')
        .map((p) => p.trim())

      if (parts.length > 1) {
        return `<div class="workload-item">
          <div class="workload-main">${parts[0]}</div>
          <div class="workload-details">
            ${parts
              .slice(1)
              .map((part) => `<span class="workload-detail">${part}</span>`)
              .join('')}
          </div>
        </div>`
      }
    }

    return line
  })

  return formattedLines.join('\n')
}

// Format message with markdown
export const formatMessage = (content: string): string => {
  const formatted = formatStructuredList(content)
  return marked(formatted, { breaks: true }) as string
}

// Format time string
export const formatTimeStr = (timeStr: string): string => {
  const date = new Date(timeStr)
  const now = new Date()
  const diff = now.getTime() - date.getTime()
  const days = Math.floor(diff / (1000 * 60 * 60 * 24))

  if (days === 0) {
    return 'Today'
  } else if (days === 1) {
    return 'Yesterday'
  } else if (days < 7) {
    return `${days} days ago`
  } else {
    return date.toLocaleDateString()
  }
}

// Format thinking time
export const formatThinkingTime = (timeMs: number): string => {
  const seconds = Math.floor(timeMs / 1000)
  if (seconds < 1) {
    return '< 1s'
  } else if (seconds < 60) {
    return `${seconds}s`
  } else {
    const minutes = Math.floor(seconds / 60)
    const remainingSeconds = seconds % 60
    return `${minutes}m ${remainingSeconds}s`
  }
}

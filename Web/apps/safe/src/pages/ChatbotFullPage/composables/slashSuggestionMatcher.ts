import { RESOURCE_COMMANDS } from '../constants/slashCommands'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CommandSuggestion {
  id: string
  label: string
  command: string
  args: string[]
  description: string
}

// ---------------------------------------------------------------------------
// Intent rules — each rule matches natural language patterns to a command
// ---------------------------------------------------------------------------

interface IntentRule {
  patterns: RegExp[]
  /** Extract workload type from match groups. Return null if no match. */
  extract: (match: RegExpMatchArray) => { type?: string; action?: string } | null
}

const WL_TYPES = Object.keys(RESOURCE_COMMANDS.wl?.types ?? {})
const WL_TYPE_PATTERN = WL_TYPES.join('|')

const intentRules: IntentRule[] = [
  // "create training" / "创建 training workload" / "新建一个 infer"
  {
    patterns: [
      new RegExp(`(?:create|创建|新建|添加|新增)\\s+(?:a\\s+|an\\s+|一个\\s*)?(?:new\\s+)?(${WL_TYPE_PATTERN})`, 'i'),
      new RegExp(`(${WL_TYPE_PATTERN})\\s+(?:create|创建|新建)`, 'i'),
    ],
    extract: (m) => ({ type: m[1].toLowerCase(), action: 'create' }),
  },
  // "go to training" / "打开 training 页面" / "跳转到 rayjob"
  {
    patterns: [
      new RegExp(`(?:go\\s+to|open|navigate\\s+to|打开|跳转到?|进入|去)\\s+(?:the\\s+)?(${WL_TYPE_PATTERN})`, 'i'),
      new RegExp(`(${WL_TYPE_PATTERN})\\s+(?:page|页面|列表)`, 'i'),
    ],
    extract: (m) => ({ type: m[1].toLowerCase() }),
  },
]

// ---------------------------------------------------------------------------
// Matcher
// ---------------------------------------------------------------------------

/** Match natural language input against intent rules. Returns suggestions (0-2 max). */
export function matchSuggestions(input: string): CommandSuggestion[] {
  const trimmed = input.trim()
  if (!trimmed || trimmed.startsWith('/')) return []
  if (trimmed.length < 3) return []

  const results: CommandSuggestion[] = []

  for (const rule of intentRules) {
    for (const pattern of rule.patterns) {
      const match = trimmed.match(pattern)
      if (!match) continue

      const extracted = rule.extract(match)
      if (!extracted?.type) continue
      if (!WL_TYPES.includes(extracted.type)) continue

      if (extracted.action) {
        results.push({
          id: `wl:${extracted.type}:${extracted.action}`,
          label: `/wl ${extracted.type} ${extracted.action}`,
          command: 'wl',
          args: [extracted.type, extracted.action],
          description: `${capitalize(extracted.action)} ${extracted.type} workload`,
        })
      }

      // Always include the navigate-only suggestion as fallback
      const navId = `wl:${extracted.type}`
      if (!results.some((r) => r.id === navId)) {
        results.push({
          id: navId,
          label: `/wl ${extracted.type}`,
          command: 'wl',
          args: [extracted.type],
          description: `Go to ${extracted.type} page`,
        })
      }

      return results
    }
  }

  return results
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

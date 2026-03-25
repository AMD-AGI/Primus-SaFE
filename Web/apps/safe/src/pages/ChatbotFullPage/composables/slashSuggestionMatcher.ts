import { builtinSlashCommands, RESOURCE_COMMANDS, type SlashCommand } from '../constants/slashCommands'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CommandSuggestion {
  id: string
  label: string
  command: string
  args: string[]
  description: string
  priority: number
}

// ---------------------------------------------------------------------------
// Action classification
// ---------------------------------------------------------------------------

type ActionIntent = 'create' | 'navigate' | 'agent'

const ACTION_PATTERNS: { intent: ActionIntent; patterns: RegExp[] }[] = [
  {
    intent: 'create',
    patterns: [
      /(?:create|创建|新建|添加|新增|add)\b/i,
    ],
  },
  {
    intent: 'agent',
    patterns: [
      /(?:stop|停止|删除|delete|remove|kill|restart|重启|分析|analyze|diagnose|诊断|move|迁移|设置|set|taint|cordon|drain)\b/i,
    ],
  },
  {
    intent: 'navigate',
    patterns: [
      /(?:go\s+to|open|navigate|show|view|list|打开|跳转|进入|去|查看|列表|看看)\b/i,
    ],
  },
]

function detectAction(input: string): ActionIntent {
  for (const { intent, patterns } of ACTION_PATTERNS) {
    if (patterns.some((p) => p.test(input))) return intent
  }
  return 'navigate'
}

// ---------------------------------------------------------------------------
// Module keyword map — built from builtinSlashCommands at module load
// ---------------------------------------------------------------------------

interface ModuleEntry {
  command: SlashCommand
  subValue?: string
}

interface KeywordEntry {
  keyword: RegExp
  modules: ModuleEntry[]
}

const WL_TYPES = Object.keys(RESOURCE_COMMANDS.wl?.types ?? {})

function buildKeywordIndex(): KeywordEntry[] {
  const entries: KeywordEntry[] = []
  const seen = new Set<string>()

  for (const cmd of builtinSlashCommands) {
    if (cmd.action !== 'navigate') continue

    // Collect all searchable terms for this command
    const terms = [
      cmd.command,
      cmd.title.toLowerCase(),
      ...(cmd.aliases ?? []),
      ...(cmd.keywords ?? []),
    ]

    // Also index subcommands
    if (cmd.subcommands) {
      for (const sub of cmd.subcommands) {
        const subTerms = [
          sub.value,
          sub.title.toLowerCase(),
          ...(sub.keywords ?? []),
        ]
        for (const t of subTerms) {
          if (t.length < 2 || seen.has(`${cmd.command}:${sub.value}:${t}`)) continue
          seen.add(`${cmd.command}:${sub.value}:${t}`)
          const existing = entries.find((e) => e.keyword.source === escapeForRegex(t))
          const mod: ModuleEntry = { command: cmd, subValue: sub.value }
          if (existing) {
            existing.modules.push(mod)
          } else {
            entries.push({ keyword: new RegExp(escapeForRegex(t), 'i'), modules: [mod] })
          }
        }
      }
    }

    for (const t of terms) {
      if (t.length < 2 || seen.has(`${cmd.command}::${t}`)) continue
      seen.add(`${cmd.command}::${t}`)
      const existing = entries.find((e) => e.keyword.source === escapeForRegex(t))
      const mod: ModuleEntry = { command: cmd }
      if (existing) {
        existing.modules.push(mod)
      } else {
        entries.push({ keyword: new RegExp(escapeForRegex(t), 'i'), modules: [mod] })
      }
    }
  }

  // Sort longer keywords first to prefer specific matches
  entries.sort((a, b) => b.keyword.source.length - a.keyword.source.length)
  return entries
}

function escapeForRegex(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

const keywordIndex = buildKeywordIndex()

// ---------------------------------------------------------------------------
// Matcher
// ---------------------------------------------------------------------------

export function matchSuggestions(input: string): CommandSuggestion[] {
  const trimmed = input.trim()
  if (!trimmed || trimmed.startsWith('/')) return []
  if (trimmed.length < 2) return []

  const actionIntent = detectAction(trimmed)
  const matched = new Map<string, CommandSuggestion>()

  for (const entry of keywordIndex) {
    if (!entry.keyword.test(trimmed)) continue

    for (const mod of entry.modules) {
      const cmd = mod.command

      if (mod.subValue) {
        // Subcommand match (e.g. "training" → /wl training)
        const hasCreate = WL_TYPES.includes(mod.subValue)

        if (actionIntent === 'create' && hasCreate) {
          addSuggestion(matched, {
            id: `${cmd.command}:${mod.subValue}:create`,
            label: `/${cmd.command} ${mod.subValue} create`,
            command: cmd.command,
            args: [mod.subValue, 'create'],
            description: `Create ${mod.subValue} workload`,
            priority: 0,
          })
        }

        addSuggestion(matched, {
          id: `${cmd.command}:${mod.subValue}`,
          label: `/${cmd.command} ${mod.subValue}`,
          command: cmd.command,
          args: [mod.subValue],
          description: `Go to ${mod.subValue} page`,
          priority: actionIntent === 'navigate' ? 1 : 2,
        })
      } else {
        // Top-level command match (e.g. "node" → /node)
        addSuggestion(matched, {
          id: cmd.command,
          label: `/${cmd.command}`,
          command: cmd.command,
          args: [],
          description: cmd.description,
          priority: actionIntent === 'navigate' ? 0 : 1,
        })
      }

      if (matched.size >= 4) break
    }
    if (matched.size >= 4) break
  }

  // For agent-type actions with matches, prepend a hint to switch mode
  if (actionIntent === 'agent' && matched.size > 0) {
    addSuggestion(matched, {
      id: '__agent_hint',
      label: '/agent',
      command: 'agent',
      args: [],
      description: 'Switch to Agent mode for this operation',
      priority: -1,
    })
  }

  return Array.from(matched.values()).sort((a, b) => a.priority - b.priority).slice(0, 4)
}

function addSuggestion(map: Map<string, CommandSuggestion>, s: CommandSuggestion) {
  const existing = map.get(s.id)
  if (!existing || s.priority < existing.priority) {
    map.set(s.id, s)
  }
}

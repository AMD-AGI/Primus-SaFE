import {
  builtinSlashCommands,
  RESOURCE_COMMANDS,
  type SlashCommand,
} from '../constants/slashCommands'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ParsedSlashInput {
  raw: string
  commandName: string
  args: string[]
}

export interface ExecuteResult {
  success: boolean
  reason?: string
}

export interface SlashExecutionContext {
  mode: 'ask' | 'agent'
  handlers: SlashCommandHandlers
}

export interface SlashCommandHandlers {
  onClear: () => void
  onNewChat: () => void
  onSwitchMode: (mode: 'ask' | 'agent') => void
  onToggleThink: () => void
  onHelp: () => void
  onNavigate: (route: string) => void
  onFillInput: (text: string) => void
}

/** Unified display item for command / subcommand / action menu entries. */
export interface MenuDisplayItem {
  id: string
  displayCommand: string
  title: string
  description: string
  icon?: string
  parentCommand: SlashCommand
  subValue?: string
  actionValue?: string
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

/** Parse raw user input into command name + args. Returns null if not a slash input. */
export function parseSlashInput(input: string): ParsedSlashInput | null {
  const trimmed = input.trim()
  if (!trimmed.startsWith('/')) return null

  const parts = trimmed.slice(1).split(/\s+/)
  const commandName = parts[0]?.toLowerCase()
  if (!commandName) return null

  return {
    raw: trimmed,
    commandName,
    args: parts.slice(1),
  }
}

/** Construct a ParsedSlashInput directly (no string parsing). Used by menu clicks. */
export function makeParsedInput(
  commandName: string,
  args: string[] = [],
): ParsedSlashInput {
  const raw = args.length ? `/${commandName} ${args.join(' ')}` : `/${commandName}`
  return { raw, commandName, args }
}

// ---------------------------------------------------------------------------
// Resolver
// ---------------------------------------------------------------------------

/** Find the command definition matching a command name (checks command + aliases). */
export function resolveCommand(parsed: ParsedSlashInput): SlashCommand | undefined {
  return builtinSlashCommands.find(
    (cmd) =>
      cmd.command === parsed.commandName ||
      cmd.aliases?.includes(parsed.commandName),
  )
}

// ---------------------------------------------------------------------------
// Executor
// ---------------------------------------------------------------------------

/** Execute a slash command. Shared by menu-click and direct-input paths. */
export function executeSlashCommand(
  cmd: SlashCommand,
  parsed: ParsedSlashInput,
  ctx: SlashExecutionContext,
): ExecuteResult {
  const { handlers } = ctx

  switch (cmd.action) {
    case 'run_handler': {
      const handlerMap: Record<string, (() => void) | undefined> = {
        clear: handlers.onClear,
        new: handlers.onNewChat,
        help: handlers.onHelp,
      }
      const fn = cmd.handlerKey ? handlerMap[cmd.handlerKey] : undefined
      if (fn) {
        fn()
        return { success: true }
      }
      return { success: false, reason: `Unknown handler: ${cmd.handlerKey}` }
    }

    case 'switch_mode': {
      const target = cmd.command as 'ask' | 'agent'
      handlers.onSwitchMode(target)
      return { success: true }
    }

    case 'toggle_think': {
      handlers.onToggleThink()
      return { success: true }
    }

    case 'fill_input': {
      if (cmd.fillText) {
        handlers.onFillInput(cmd.fillText)
        return { success: true }
      }
      return { success: false, reason: 'No fill text configured' }
    }

    case 'navigate':
      return executeNavigate(cmd, parsed, handlers)

    default:
      return { success: false, reason: `Unknown action: ${cmd.action}` }
  }
}

// ---------------------------------------------------------------------------
// Navigate executor — data-driven via RESOURCE_COMMANDS config
// ---------------------------------------------------------------------------

function executeNavigate(
  cmd: SlashCommand,
  parsed: ParsedSlashInput,
  handlers: SlashCommandHandlers,
): ExecuteResult {
  // Simple navigate (no subcommands)
  if (!cmd.subcommands) {
    if (cmd.route) {
      handlers.onNavigate(cmd.route)
      return { success: true }
    }
    return { success: false, reason: 'No route configured' }
  }

  // Resource command: resolve via config
  const resourceConfig = RESOURCE_COMMANDS[cmd.command]
  if (!resourceConfig) {
    return { success: false, reason: `No resource config for: ${cmd.command}` }
  }

  if (!parsed.args.length) {
    return { success: false, reason: 'Resource type required' }
  }

  const typeName = parsed.args[0].toLowerCase()
  const typeConfig = resourceConfig.types[typeName]
  if (!typeConfig) {
    return { success: false, reason: `Unknown type: ${typeName}` }
  }

  const actionArg = parsed.args[1]?.toLowerCase()
  if (actionArg) {
    const actionConfig = typeConfig.actions?.[actionArg]
    if (!actionConfig) {
      return { success: false, reason: `Unknown action: ${actionArg}` }
    }
    const query = new URLSearchParams(actionConfig.query).toString()
    handlers.onNavigate(`${typeConfig.route}?${query}`)
    return { success: true }
  }

  handlers.onNavigate(typeConfig.route)
  return { success: true }
}

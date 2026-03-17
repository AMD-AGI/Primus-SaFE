import { ref, computed, watch } from 'vue'
import type { Ref } from 'vue'
import {
  builtinSlashCommands,
  type SlashCommand,
  type SubcommandOption,
  type SubcommandAction,
} from '../constants/slashCommands'
import {
  parseSlashInput,
  makeParsedInput,
  resolveCommand,
  executeSlashCommand,
  type SlashCommandHandlers,
  type MenuDisplayItem,
} from './slashCommandExecutor'
import { matchSuggestions, type CommandSuggestion } from './slashSuggestionMatcher'

export type { SlashCommandHandlers, MenuDisplayItem, CommandSuggestion }

export interface SlashCommandGroup {
  label: string
  commands: SlashCommand[]
}

// ---------------------------------------------------------------------------
// Input state: determines what the menu shows
// ---------------------------------------------------------------------------

type InputState =
  | { type: 'idle' }
  | { type: 'searching'; term: string }
  | { type: 'subcommand'; parentCmd: SlashCommand; argPrefix: string }
  | { type: 'action'; parentCmd: SlashCommand; subOption: SubcommandOption; actionPrefix: string }

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useSlashCommands(
  userInput: Ref<string>,
  mode: Ref<'ask' | 'agent'>,
  isAdmin: Ref<boolean> | boolean,
  handlers: SlashCommandHandlers,
) {
  const showMenu = ref(false)
  const activeIndex = ref(0)

  // ---- Derive three-level input state ----
  const inputState = computed<InputState>(() => {
    const input = userInput.value
    if (!input.startsWith('/')) return { type: 'idle' }

    const afterSlash = input.slice(1)
    if (!afterSlash) return { type: 'idle' }

    const firstSpace = afterSlash.indexOf(' ')

    // No space: one-level command search  (e.g. "/w", "/wl")
    if (firstSpace === -1) {
      return { type: 'searching', term: afterSlash.toLowerCase() }
    }

    // Has first space: resolve parent command
    const cmdName = afterSlash.slice(0, firstSpace).toLowerCase()
    const parentCmd = builtinSlashCommands.find(
      (c) => c.command === cmdName || c.aliases?.includes(cmdName),
    )

    if (!parentCmd?.subcommands?.length) {
      return { type: 'searching', term: afterSlash.toLowerCase() }
    }

    const restAfterCmd = afterSlash.slice(firstSpace + 1)
    const secondSpace = restAfterCmd.indexOf(' ')

    // No second space: subcommand completion  (e.g. "/wl ", "/wl t", "/wl training")
    if (secondSpace === -1) {
      return { type: 'subcommand', parentCmd, argPrefix: restAfterCmd.toLowerCase() }
    }

    // Has second space: check if subcommand has actions
    const subName = restAfterCmd.slice(0, secondSpace).toLowerCase()
    const subOption = parentCmd.subcommands.find((s) => s.value === subName)

    if (subOption?.actions?.length) {
      const actionPrefix = restAfterCmd.slice(secondSpace + 1).toLowerCase()
      return { type: 'action', parentCmd, subOption, actionPrefix }
    }

    // Sub exists but no actions → stay in subcommand mode
    return { type: 'subcommand', parentCmd, argPrefix: restAfterCmd.toLowerCase() }
  })

  const isSearching = computed(() => inputState.value.type !== 'idle')

  const adminFlag = computed(() => (typeof isAdmin === 'boolean' ? isAdmin : isAdmin.value))

  const isCommandVisible = (cmd: SlashCommand): boolean => {
    if (cmd.requiredRole === 'admin' && !adminFlag.value) return false
    return cmd.mode === 'all' || cmd.mode === mode.value
  }

  // ---- Multi-field matching helpers ----
  const matchesCommandSearch = (cmd: SlashCommand, term: string): boolean => {
    if (!term) return true
    const fields = [
      cmd.command, cmd.title, cmd.description,
      ...(cmd.aliases ?? []),
      ...(cmd.keywords ?? []),
    ]
    if (cmd.subcommands) {
      for (const sub of cmd.subcommands) {
        fields.push(sub.value, sub.title, sub.description, ...(sub.keywords ?? []))
      }
    }
    return fields.some((f) => f.toLowerCase().includes(term))
  }

  const matchesSubcommand = (sub: SubcommandOption, prefix: string): boolean => {
    if (!prefix) return true
    const fields = [sub.value, sub.title, sub.description, ...(sub.keywords ?? [])]
    return fields.some((f) => f.toLowerCase().includes(prefix))
  }

  const matchesAction = (act: SubcommandAction, prefix: string): boolean => {
    if (!prefix) return true
    const fields = [act.value, act.title, act.description, ...(act.keywords ?? [])]
    return fields.some((f) => f.toLowerCase().includes(prefix))
  }

  // ---- Build display items based on input state ----
  const displayItems = computed<MenuDisplayItem[]>(() => {
    const state = inputState.value

    switch (state.type) {
      case 'idle':
        return []

      case 'searching':
        return builtinSlashCommands
          .filter((cmd) => isCommandVisible(cmd) && matchesCommandSearch(cmd, state.term))
          .map(cmdToDisplayItem)

      case 'subcommand':
        return state.parentCmd.subcommands!
          .filter((sub) => matchesSubcommand(sub, state.argPrefix))
          .map((sub) => subToDisplayItem(state.parentCmd, sub))

      case 'action':
        return state.subOption.actions!
          .filter((act) => matchesAction(act, state.actionPrefix))
          .map((act) => actionToDisplayItem(state.parentCmd, state.subOption, act))

      default:
        return []
    }
  })

  // ---- Grouped commands for the idle grouped view ----
  const allVisibleCommands = computed(() =>
    builtinSlashCommands.filter(isCommandVisible),
  )

  const groupedCommands = computed<SlashCommandGroup[]>(() => {
    const map = new Map<string, SlashCommand[]>()
    for (const cmd of allVisibleCommands.value) {
      if (!map.has(cmd.category)) map.set(cmd.category, [])
      map.get(cmd.category)!.push(cmd)
    }
    return Array.from(map, ([label, cmds]) => ({ label, commands: cmds }))
  })

  // ---- Menu visibility ----
  watch(userInput, (val) => {
    if (val.startsWith('/')) {
      showMenu.value = true
      activeIndex.value = 0
    } else {
      showMenu.value = false
    }
  })

  // ---- Unified execution ----

  /** Execute a display item (from menu click or keyboard selection). */
  const selectDisplayItem = (item: MenuDisplayItem) => {
    showMenu.value = false
    const args = [item.subValue, item.actionValue].filter(Boolean) as string[]
    const parsed = makeParsedInput(item.parentCommand.command, args)
    const result = executeSlashCommand(item.parentCommand, parsed, {
      mode: mode.value,
      handlers,
    })
    if (result.success) {
      userInput.value = ''
    }
  }

  /** Try to execute the raw input as a complete slash command (Enter without menu). */
  const tryExecuteInput = (): boolean => {
    const parsed = parseSlashInput(userInput.value)
    if (!parsed) return false

    const cmd = resolveCommand(parsed)
    if (!cmd) return false

    showMenu.value = false
    const result = executeSlashCommand(cmd, parsed, { mode: mode.value, handlers })
    if (result.success) {
      userInput.value = ''
    }
    return result.success
  }

  // ---- Keyboard navigation ----
  const handleSlashKeydown = (e: KeyboardEvent): boolean => {
    if (!showMenu.value) return false

    const items = displayItems.value

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        if (items.length) activeIndex.value = (activeIndex.value + 1) % items.length
        return true

      case 'ArrowUp':
        e.preventDefault()
        if (items.length) activeIndex.value = (activeIndex.value - 1 + items.length) % items.length
        return true

      case 'Enter': {
        e.preventDefault()
        if (items.length > 0 && activeIndex.value >= 0 && activeIndex.value < items.length) {
          selectDisplayItem(items[activeIndex.value])
          return true
        }
        return tryExecuteInput()
      }

      case 'Escape':
        e.preventDefault()
        showMenu.value = false
        userInput.value = ''
        return true

      case 'Tab':
        e.preventDefault()
        if (items.length > 0 && activeIndex.value >= 0 && activeIndex.value < items.length) {
          selectDisplayItem(items[activeIndex.value])
        }
        return true

      default:
        return false
    }
  }

  // ---- Natural language → command suggestions ----
  const suggestions = computed<CommandSuggestion[]>(() => {
    const input = userInput.value
    if (!input || input.startsWith('/')) return []
    return matchSuggestions(input)
  })

  /** Fill the input box with a suggestion's slash command text. */
  const fillSuggestion = (suggestion: CommandSuggestion) => {
    userInput.value = suggestion.label
  }

  /** Execute a suggestion directly via the existing executor. */
  const executeSuggestion = (suggestion: CommandSuggestion) => {
    const parsed = makeParsedInput(suggestion.command, suggestion.args)
    const cmd = resolveCommand(parsed)
    if (!cmd) return

    const result = executeSlashCommand(cmd, parsed, { mode: mode.value, handlers })
    if (result.success) {
      userInput.value = ''
    }
  }

  return {
    showMenu,
    isSearching,
    displayItems,
    groupedCommands,
    activeIndex,
    handleSlashKeydown,
    selectDisplayItem,
    suggestions,
    fillSuggestion,
    executeSuggestion,
  }
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------

function cmdToDisplayItem(cmd: SlashCommand): MenuDisplayItem {
  return {
    id: cmd.id,
    displayCommand: cmd.command,
    title: cmd.title,
    description: cmd.description,
    icon: cmd.icon,
    parentCommand: cmd,
  }
}

function subToDisplayItem(parent: SlashCommand, sub: SubcommandOption): MenuDisplayItem {
  return {
    id: `${parent.id}:${sub.value}`,
    displayCommand: `${parent.command} ${sub.value}`,
    title: sub.title,
    description: sub.description,
    icon: sub.icon,
    parentCommand: parent,
    subValue: sub.value,
  }
}

function actionToDisplayItem(
  parent: SlashCommand,
  sub: SubcommandOption,
  act: SubcommandAction,
): MenuDisplayItem {
  return {
    id: `${parent.id}:${sub.value}:${act.value}`,
    displayCommand: `${parent.command} ${sub.value} ${act.value}`,
    title: act.title,
    description: act.description,
    icon: act.icon,
    parentCommand: parent,
    subValue: sub.value,
    actionValue: act.value,
  }
}

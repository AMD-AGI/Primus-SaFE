// ---------------------------------------------------------------------------
// Slash Command Schema
// ---------------------------------------------------------------------------

export interface SlashCommandArg {
  name: string
  required?: boolean
  enum?: string[]
}

export interface SubcommandAction {
  value: string
  title: string
  description: string
  icon?: string
  keywords?: string[]
}

export interface SubcommandOption {
  value: string
  title: string
  description: string
  icon?: string
  keywords?: string[]
  actions?: SubcommandAction[]
}

export interface SlashCommand {
  id: string
  command: string
  title: string
  description: string
  icon?: string
  category: string
  mode?: 'ask' | 'agent' | 'all'
  action: 'navigate' | 'fill_input' | 'run_handler' | 'switch_mode' | 'toggle_think'
  aliases?: string[]
  keywords?: string[]
  route?: string
  fillText?: string
  handlerKey?: string
  args?: SlashCommandArg[]
  subcommands?: SubcommandOption[]
}

// ---------------------------------------------------------------------------
// Workload type → route mapping
// ---------------------------------------------------------------------------

export const workloadRouteMap: Record<string, string> = {
  training: '/training',
  infer: '/infer',
  rayjob: '/rayjob',
  cicd: '/cicd',
  torchft: '/torchft',
  authoring: '/authoring',
}

// Shared actions available on all workload types
const workloadActions: SubcommandAction[] = [
  { value: 'create', title: 'Create', description: 'Create new workload', icon: 'Plus', keywords: ['new', 'add'] },
]

// Categories that collapse into a hoverable submenu row
export const submenuCategories = new Set(['Workloads'])

// ---------------------------------------------------------------------------
// Built-in commands
// ---------------------------------------------------------------------------

export const builtinSlashCommands: SlashCommand[] = [
  {
    id: 'wl',
    command: 'wl',
    title: 'Workloads',
    description: 'Go to workload page',
    icon: 'Box',
    category: 'Workloads',
    mode: 'all',
    action: 'navigate',
    aliases: ['workload', 'workloads'],
    subcommands: [
      { value: 'training', title: 'Training', description: 'Training jobs', icon: 'Box', keywords: ['train'], actions: workloadActions },
      { value: 'rayjob', title: 'RayJob', description: 'Ray jobs', icon: 'Cpu', keywords: ['ray'], actions: workloadActions },
      { value: 'infer', title: 'Inference', description: 'Inference services', icon: 'TrendCharts', keywords: ['inference'], actions: workloadActions },
      { value: 'cicd', title: 'CI/CD', description: 'CI/CD pipelines', icon: 'Refresh', keywords: ['pipeline'], actions: workloadActions },
      { value: 'torchft', title: 'TorchFT', description: 'TorchFT jobs', icon: 'Lightning', keywords: ['torch', 'ft'], actions: workloadActions },
      { value: 'authoring', title: 'Authoring', description: 'Authoring jobs', icon: 'EditPen', keywords: ['author'], actions: workloadActions },
    ],
  },

  // --- Conversation ---
  {
    id: 'clear',
    command: 'clear',
    title: 'Clear',
    description: 'Clear conversation',
    icon: 'Delete',
    category: 'Conversation',
    mode: 'all',
    action: 'run_handler',
    handlerKey: 'clear',
  },
  {
    id: 'new',
    command: 'new',
    title: 'New Chat',
    description: 'Start a new conversation',
    icon: 'Edit',
    category: 'Conversation',
    mode: 'all',
    action: 'run_handler',
    handlerKey: 'new',
  },

  // --- Mode ---
  {
    id: 'ask',
    command: 'ask',
    title: 'Ask Mode',
    description: 'Switch to Ask mode',
    icon: 'ChatDotRound',
    category: 'Mode',
    mode: 'all',
    action: 'switch_mode',
  },
  {
    id: 'agent',
    command: 'agent',
    title: 'Agent Mode',
    description: 'Switch to Agent mode',
    icon: 'MagicStick',
    category: 'Mode',
    mode: 'all',
    action: 'switch_mode',
  },
  {
    id: 'think',
    command: 'think',
    title: 'Deep Think',
    description: 'Toggle deep thinking',
    icon: 'View',
    category: 'Mode',
    mode: 'ask',
    action: 'toggle_think',
  },

  // --- Help ---
  {
    id: 'help',
    command: 'help',
    title: 'Help',
    description: 'Show all commands',
    icon: 'QuestionFilled',
    category: 'Help',
    mode: 'all',
    action: 'run_handler',
    handlerKey: 'help',
  },
]

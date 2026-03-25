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
  requiredRole?: 'admin'
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
// Route action constants — shared between executor and workload pages
// ---------------------------------------------------------------------------

export const ROUTE_ACTIONS = {
  CREATE: 'create',
} as const

// ---------------------------------------------------------------------------
// Resource command config — data-driven navigate + action resolution
// ---------------------------------------------------------------------------

export interface ResourceActionConfig {
  query: Record<string, string>
}

export interface ResourceTypeConfig {
  route: string
  actions?: Record<string, ResourceActionConfig>
}

export interface ResourceCommandConfig {
  types: Record<string, ResourceTypeConfig>
}

const defaultActions: Record<string, ResourceActionConfig> = {
  [ROUTE_ACTIONS.CREATE]: { query: { action: ROUTE_ACTIONS.CREATE } },
}

export const RESOURCE_COMMANDS: Record<string, ResourceCommandConfig> = {
  wl: {
    types: {
      training:  { route: '/training',  actions: defaultActions },
      rayjob:    { route: '/rayjob',    actions: defaultActions },
      infer:     { route: '/infer',     actions: defaultActions },
      cicd:      { route: '/cicd',      actions: defaultActions },
      torchft:   { route: '/torchft',   actions: defaultActions },
      authoring: { route: '/authoring', actions: defaultActions },
    },
  },
}

const workloadActions: SubcommandAction[] = [
  { value: ROUTE_ACTIONS.CREATE, title: 'Create', description: 'Create new workload', icon: 'Plus', keywords: ['new', 'add', '创建', '新建'] },
]

// Categories that collapse into a hoverable submenu row
export const submenuCategories = new Set(['Workloads'])

// ---------------------------------------------------------------------------
// Helper: simple navigate command factory
// ---------------------------------------------------------------------------

function nav(
  id: string,
  command: string,
  title: string,
  description: string,
  route: string,
  category: string,
  opts?: Partial<Pick<SlashCommand, 'icon' | 'aliases' | 'keywords' | 'requiredRole' | 'mode'>>,
): SlashCommand {
  return {
    id,
    command,
    title,
    description,
    icon: opts?.icon ?? 'Folder',
    category,
    mode: opts?.mode ?? 'all',
    requiredRole: opts?.requiredRole,
    action: 'navigate',
    route,
    aliases: opts?.aliases,
    keywords: opts?.keywords,
  }
}

// ---------------------------------------------------------------------------
// Built-in commands
// ---------------------------------------------------------------------------

export const builtinSlashCommands: SlashCommand[] = [
  // ==========================================================================
  // Workloads (with subcommands + actions)
  // ==========================================================================
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
    keywords: ['任务', '工作负载', 'job'],
    subcommands: [
      { value: 'training', title: 'Training', description: 'Training jobs', icon: 'Box', keywords: ['train', '训练'], actions: workloadActions },
      { value: 'rayjob', title: 'RayJob', description: 'Ray jobs', icon: 'Cpu', keywords: ['ray'], actions: workloadActions },
      { value: 'infer', title: 'Inference', description: 'Inference services', icon: 'TrendCharts', keywords: ['inference', '推理'], actions: workloadActions },
      { value: 'cicd', title: 'CI/CD', description: 'CI/CD pipelines', icon: 'Refresh', keywords: ['pipeline', '流水线'], actions: workloadActions },
      { value: 'torchft', title: 'TorchFT', description: 'TorchFT jobs', icon: 'Lightning', keywords: ['torch', 'ft', '容错'], actions: workloadActions },
      { value: 'authoring', title: 'Authoring', description: 'Authoring jobs', icon: 'EditPen', keywords: ['author', '编排'], actions: workloadActions },
    ],
  },

  // ==========================================================================
  // Artifacts
  // ==========================================================================
  nav('bench', 'bench', 'Bench', 'Benchmark & preflight tests', '/preflight/ws', 'Artifacts',
    { icon: 'Odometer', aliases: ['preflight', 'benchmark'], keywords: ['基准', '预检', 'test'] }),
  nav('datasync', 'datasync', 'Datasync', 'Data download & sync', '/download', 'Artifacts',
    { icon: 'Download', aliases: ['download'], keywords: ['数据', '下载', 'sync'] }),
  nav('image', 'image', 'Images', 'Container images', '/images', 'Artifacts',
    { icon: 'PictureFilled', aliases: ['images', 'img'], keywords: ['镜像', 'container', 'docker'] }),
  nav('secret', 'secret', 'Secrets', 'SSH & image pull secrets', '/secrets', 'Artifacts',
    { icon: 'Lock', aliases: ['secrets', 'key'], keywords: ['密钥', 'ssh', 'credential'] }),
  nav('apikey', 'apikey', 'API Keys', 'Manage API keys', '/manageapikeys', 'Artifacts',
    { icon: 'Key', aliases: ['apikeys'], keywords: ['api', 'token', '密钥'] }),

  // ==========================================================================
  // Model Lab (admin)
  // ==========================================================================
  nav('model', 'model', 'Model Square', 'Browse models', '/model-square', 'Model Lab',
    { icon: 'Grid', aliases: ['models'], keywords: ['模型', 'model'], requiredRole: 'admin' }),
  nav('playground', 'playground', 'Playground', 'AI model playground', '/playground-agent', 'Model Lab',
    { icon: 'MagicStick', keywords: ['试验', 'chat', '对话'], requiredRole: 'admin' }),
  nav('dataset', 'dataset', 'Dataset', 'Manage datasets', '/dataset', 'Model Lab',
    { icon: 'Files', aliases: ['data'], keywords: ['数据集'], requiredRole: 'admin' }),
  nav('eval', 'eval', 'Evaluation', 'Agent evaluation tasks', '/evaluation', 'Model Lab',
    { icon: 'DataAnalysis', aliases: ['evaluation'], keywords: ['评估', '评测'], requiredRole: 'admin' }),

  // ==========================================================================
  // AI Agent
  // ==========================================================================
  nav('chat', 'chat', 'Chatbot', 'Open chatbot page', '/chatbot', 'AI Agent',
    { icon: 'ChatDotRound', aliases: ['chatbot'], keywords: ['聊天', '对话', 'bot'] }),
  nav('qabase', 'qabase', 'QA Base', 'Knowledge base management', '/qabase', 'AI Agent',
    { icon: 'Collection', aliases: ['kb', 'knowledge'], keywords: ['知识库', 'qa'], requiredRole: 'admin' }),
  nav('feedback', 'feedback', 'Feedback', 'User feedback management', '/feedback-management', 'AI Agent',
    { icon: 'Comment', keywords: ['反馈', 'review'], requiredRole: 'admin' }),

  // ==========================================================================
  // Agent Infra
  // ==========================================================================
  nav('tools', 'tools', 'Tools', 'Agent tools registry', '/tools', 'Agent Infra',
    { icon: 'SetUp', aliases: ['tool'], keywords: ['工具'] }),
  nav('sandbox', 'sandbox', 'Sandbox', 'Agent sandbox environment', '/sandbox', 'Agent Infra',
    { icon: 'Monitor', keywords: ['沙盒', '环境'], requiredRole: 'admin' }),
  nav('llm', 'llm', 'LLM Gateway', 'LLM proxy & usage', '/llm-gateway', 'Agent Infra',
    { icon: 'Connection', aliases: ['gateway', 'llmgateway'], keywords: ['大模型', 'proxy', '网关'] }),
  nav('a2a', 'a2a', 'A2A Protocol', 'Agent-to-Agent protocol', '/a2a', 'Agent Infra',
    { icon: 'Share', aliases: ['agent2agent'], keywords: ['代理', 'protocol', '协议'] }),

  // ==========================================================================
  // System
  // ==========================================================================
  nav('node', 'node', 'Nodes', 'Cluster nodes', '/nodes', 'System',
    { icon: 'Cpu', aliases: ['nodes'], keywords: ['节点', 'gpu', '服务器', 'server'] }),
  nav('cluster', 'cluster', 'Clusters', 'Kubernetes clusters', '/clusters', 'System',
    { icon: 'OfficeBuilding', aliases: ['clusters', 'k8s'], keywords: ['集群', 'kubernetes'], requiredRole: 'admin' }),
  nav('ws', 'ws', 'Workspaces', 'Workspace & quota management', '/workspace', 'System',
    { icon: 'Briefcase', aliases: ['workspace', 'workspaces'], keywords: ['工作空间', 'quota', '配额'], requiredRole: 'admin' }),
  nav('user', 'user', 'Users', 'User management', '/usermanage', 'System',
    { icon: 'User', aliases: ['users'], keywords: ['用户'], requiredRole: 'admin' }),
  nav('usergroup', 'usergroup', 'User Groups', 'User group management', '/user-group', 'System',
    { icon: 'UserFilled', aliases: ['group', 'groups'], keywords: ['用户组'], requiredRole: 'admin' }),
  nav('fault', 'fault', 'Faults', 'Fault injection', '/fault', 'System',
    { icon: 'WarningFilled', aliases: ['faults'], keywords: ['故障', '注入'], requiredRole: 'admin' }),
  nav('registry', 'registry', 'Registries', 'Image registries', '/registries', 'System',
    { icon: 'Box', aliases: ['registries', 'reg'], keywords: ['仓库', 'harbor'], requiredRole: 'admin' }),
  nav('addon', 'addon', 'Addons', 'Cluster addons', '/addons', 'System',
    { icon: 'CirclePlus', aliases: ['addons'], keywords: ['插件', 'addon'], requiredRole: 'admin' }),
  nav('flavor', 'flavor', 'Flavors', 'Node flavor templates', '/nodeflavor', 'System',
    { icon: 'Document', aliases: ['flavors', 'nodeflavor'], keywords: ['规格', '模板'], requiredRole: 'admin' }),
  nav('deploy', 'deploy', 'Deploy', 'Deployment management', '/deploy', 'System',
    { icon: 'Upload', aliases: ['cd'], keywords: ['部署', 'release'], requiredRole: 'admin' }),
  nav('audit', 'audit', 'Audit Logs', 'System audit logs', '/auditlogs', 'System',
    { icon: 'Notebook', aliases: ['auditlogs', 'logs'], keywords: ['审计', '日志', 'log'], requiredRole: 'admin' }),
  nav('wlm', 'wlm', 'Workload Manage', 'System workload management', '/workload-manage', 'System',
    { icon: 'Management', aliases: ['workloadmanage'], keywords: ['管理', '工作负载管理'], requiredRole: 'admin' }),

  // ==========================================================================
  // Conversation controls
  // ==========================================================================
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
    keywords: ['清空', '清除'],
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
    keywords: ['新建', '新对话'],
  },

  // ==========================================================================
  // Mode
  // ==========================================================================
  {
    id: 'ask',
    command: 'ask',
    title: 'Ask Mode',
    description: 'Switch to Ask mode',
    icon: 'ChatDotRound',
    category: 'Mode',
    mode: 'all',
    action: 'switch_mode',
    keywords: ['问答', '提问'],
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
    keywords: ['代理', '执行'],
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
    keywords: ['思考', '深度'],
  },

  // ==========================================================================
  // Help
  // ==========================================================================
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
    keywords: ['帮助', '命令'],
  },
]

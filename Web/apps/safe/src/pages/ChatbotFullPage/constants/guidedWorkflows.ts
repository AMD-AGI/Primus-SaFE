// Guided Workflow definitions — data-driven step-by-step wizards rendered in chat

export type FieldType = 'input' | 'select' | 'number' | 'textarea'

export type OptionsLoaderName = 'workspaces' | 'clusters' | 'flavors' | 'secrets_ssh' | 'secrets_image'

export interface WizardFieldOption {
  label: string
  value: string | number
}

export interface WizardField {
  key: string
  label: string
  type: FieldType
  required?: boolean
  placeholder?: string
  options?: WizardFieldOption[]
  optionsLoader?: OptionsLoaderName
  default?: unknown
  multiple?: boolean
  suffix?: string
  min?: number
  max?: number
}

export interface WizardStep {
  title: string
  description?: string
  fields: WizardField[]
}

export interface GuidedWorkflow {
  id: string
  name: string
  icon: string
  steps: WizardStep[]
  formatMessage: (data: Record<string, unknown>) => string
}

// ---------------------------------------------------------------------------
// MVP: Create Training Workload
// ---------------------------------------------------------------------------

export const createTrainingWorkflow: GuidedWorkflow = {
  id: 'create_training',
  name: 'Create Training Workload',
  icon: '🚀',
  steps: [
    {
      title: 'Select Workspace',
      description: 'Choose the workspace where the training workload will run.',
      fields: [
        {
          key: 'workspace',
          label: 'Workspace',
          type: 'select',
          required: true,
          placeholder: 'Select a workspace',
          optionsLoader: 'workspaces',
        },
      ],
    },
    {
      title: 'Basic Information',
      description: 'Provide the workload name, container image, and entry point command.',
      fields: [
        {
          key: 'name',
          label: 'Name',
          type: 'input',
          required: true,
          placeholder: 'e.g. my-training-job',
        },
        {
          key: 'image',
          label: 'Image',
          type: 'input',
          required: true,
          placeholder: 'e.g. python:3.11',
        },
        {
          key: 'entrypoint',
          label: 'Entrypoint',
          type: 'textarea',
          required: true,
          placeholder: 'e.g. python train.py --epochs 10',
        },
      ],
    },
    {
      title: 'Resource Configuration',
      description: 'Specify CPU, GPU, memory and replica settings.',
      fields: [
        {
          key: 'cpu',
          label: 'CPU',
          type: 'number',
          required: true,
          default: 2,
          min: 1,
          max: 256,
        },
        {
          key: 'memory',
          label: 'Memory',
          type: 'number',
          required: true,
          default: 4,
          min: 1,
          max: 2048,
          suffix: 'Gi',
        },
        {
          key: 'gpu',
          label: 'GPU',
          type: 'number',
          required: true,
          default: 0,
          min: 0,
          max: 64,
        },
        {
          key: 'ephemeral_storage',
          label: 'Ephemeral Storage',
          type: 'number',
          default: 50,
          min: 0,
          max: 10000,
          suffix: 'Gi',
        },
        {
          key: 'replica',
          label: 'Replica',
          type: 'number',
          required: true,
          default: 1,
          min: 1,
          max: 512,
        },
        {
          key: 'priority',
          label: 'Priority',
          type: 'select',
          required: true,
          default: 'low',
          options: [
            { label: 'Low', value: 'low' },
            { label: 'Medium', value: 'medium' },
            { label: 'High', value: 'high' },
          ],
        },
      ],
    },
  ],

  formatMessage(data: Record<string, unknown>): string {
    const parts: string[] = ['Create training:']
    const fieldMap: [string, string][] = [
      ['name', 'name'],
      ['workspace', 'workspace'],
      ['image', 'image'],
      ['cpu', 'cpu'],
      ['memory', 'memory'],
      ['gpu', 'gpu'],
      ['ephemeral_storage', 'ephemeral_storage'],
      ['replica', 'replica'],
      ['priority', 'priority'],
      ['entrypoint', 'entrypoint'],
    ]
    for (const [key, label] of fieldMap) {
      const val = data[key]
      if (val !== undefined && val !== null && val !== '') {
        parts.push(`${label}: ${val}`)
      }
    }
    return parts.join(', ')
  },
}

// Registry: look up workflows by id
const workflowRegistry: Record<string, GuidedWorkflow> = {
  create_training: createTrainingWorkflow,
}

export function getWorkflowById(id: string): GuidedWorkflow | undefined {
  return workflowRegistry[id]
}

export function getAllWorkflows(): GuidedWorkflow[] {
  return Object.values(workflowRegistry)
}

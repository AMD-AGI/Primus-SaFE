// Quick Start data configuration

export interface QuickStartCard {
  text: string
  value: string
  icon?: string // Element Plus icon name (optional)
}

export interface QuickStartConfig {
  emoji: string
  title: string
  cards: QuickStartCard[]
}

// Ask Mode Quick Start
export const askModeQuickStart: QuickStartConfig = {
  emoji: '💡',
  title: 'Quick start:',
  cards: [
    {
      text: 'How to request access to the Primus-SaFE platform?',
      value: 'How to request access to the Primus-SaFE platform?',
      icon: 'QuestionFilled',
    },
    {
      text: 'How to connect to a Pod using SSH?',
      value: 'How to connect to a Pod using SSH?',
      icon: 'Connection',
    },
    {
      text: 'Where should training data be stored?',
      value: 'Where should training data be stored?',
      icon: 'Folder',
    },
    {
      text: 'How to create a distributed training workload?',
      value: 'How to create a distributed training workload?',
      icon: 'Box',
    },
  ],
}

// Normal User Quick Start
export const normalUserQuickStart: QuickStartConfig = {
  emoji: '🚀',
  title: 'Quick start:',
  cards: [
    {
      text: 'Create training: name ops-agent, workspace prod, image python:3.11, cpu 2, memory 4, replica 1, priority low, gpu 0, ephemeral_storage 50, entrypoint sleep 60',
      value:
        'create training, name: ops-agent, workspace: prod, image: python:3.11, cpu: 2, memory: 4, replica: 1, priority: low, gpu:0, ephemeral_storage: 50, entrypoint: sleep 60',
    },
    {
      text: 'Stop training workloadID',
      value: 'stop training workloadID',
    },
    {
      text: 'Preheat python3.11 image in prod workspace',
      value: 'Preheat python3.11 image in prod workspace',
    },
  ],
}

// Workspace Admin Quick Start (extends Normal User)
export const workspaceAdminQuickStart: QuickStartConfig = {
  emoji: '🚀',
  title: 'Quick start:',
  cards: [
    ...normalUserQuickStart.cards,
    {
      text: 'Stop GPU tasks that have been idle for more than 1 Hour in the prod workspace.',
      value: 'Stop GPU tasks that have been idle for more than 1 Hour in the prod workspace.',
    },
  ],
}

// Agent Mode Quick Start
export const agentModeQuickStart: QuickStartConfig = {
  emoji: '🤖',
  title: 'Quick start:',
  cards: [
    {
      text: 'Stop GPU tasks that have been idle for more than 1 Hour in the prod workspace.',
      value: 'Stop GPU tasks that have been idle for more than 1 Hour in the prod workspace.',
    },
    {
      text: 'Create a bench task by specifying nodes tus1-p13-g47 and tus1-p13-g48',
      value: 'Create a bench task by specifying nodes tus1-p13-g47 and tus1-p13-g48',
    },
    {
      text: 'Set taints on tus1-p13-g47 and tus1-p13-g48 using NoSchedule:opsAgent',
      value: 'Set taints on tus1-p13-g47 and tus1-p13-g48 using NoSchedule:opsAgent',
    },
    {
      text: 'Remove taints from nodes tus1-p13-g47 and tus1-p13-g48',
      value: 'Remove taints from nodes tus1-p13-g47 and tus1-p13-g48',
    },
    {
      text: 'Create the test-ops workspace on the x-flannel cluster, specifying amd-mi325x-example as the node-flavor',
      value:
        'Create the test-ops workspace on the x-flannel cluster, specifying amd-mi325x-example as the node-flavor',
    },
    {
      text: 'Move tus1-p1-g41 and tus1-p1-g33 from ray-cluster to prod',
      value: 'Move tus1-p1-g41 and tus1-p1-g33 from ray-cluster to prod',
    },
    {
      text: 'Batch unmanage the following nodes in the x-flannel cluster:  ...',
      value: 'Batch unmanage the following nodes in the x-flannel cluster:  ...',
    },
    {
      text: 'Batch delete the following nodes: ...',
      value: 'Batch delete the following nodes: ...',
    },
  ],
}

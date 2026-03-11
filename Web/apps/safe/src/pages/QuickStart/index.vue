<template>
  <div class="safe-page quickstart">
    <h2 class="text-lg font-600 mb-4">Quick Start</h2>
    <el-card class="p-6 mb-6" shadow="never">
      <div class="intro">
        <p class="text-gray-500 mb-4">
          A one-stop model management and training platform that helps developers quickly deploy
          diversified machine learning models, accelerating intelligent application innovation.
        </p>

        <div class="steps">
          <div
            class="step-card"
            v-for="c in cards"
            :key="c.to"
            @click="go(c.to)"
            @keydown.enter="go(c.to)"
          >
            <el-icon class="bg-icon" aria-hidden="true"><component :is="c.icon" /></el-icon>
            <h4>{{ c.title }}</h4>
            <p>
              {{ c.desc }}
            </p>
          </div>
        </div>
      </div>
    </el-card>

    <el-card class="p-6" shadow="never">
      <h3 class="text-md font-500 mb-3">System Overview</h3>
      <el-descriptions class="sys-desc" :column="3">
        <el-descriptions-item label="NodeFlavors"
          >Standardize CPU/GPU/memory/disks and extended resources into shareable
          templates.</el-descriptions-item
        >
        <el-descriptions-item label="Secrets"
          >Central place for SSH keys and image registry credentials.</el-descriptions-item
        >
        <el-descriptions-item label="Nodes"
          >Machines registered with flavor/secret/labels for scheduling.</el-descriptions-item
        >
        <el-descriptions-item label="Clusters"
          >Kubernetes clusters with networking/version settings and node
          membership.</el-descriptions-item
        >
        <el-descriptions-item label="Workspaces"
          >Logical resource & team boundary with policy, replicas, and
          volumes.</el-descriptions-item
        >
        <el-descriptions-item label="Users">Manage platform users and roles.</el-descriptions-item>
        <el-descriptions-item label="Faults"
          >Track node or cluster exceptions.</el-descriptions-item
        >
      </el-descriptions>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { Cpu, Key, Monitor, Collection, Setting } from '@element-plus/icons-vue'
import { driver, type DriveStep } from 'driver.js'
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '@/stores/user'
import 'driver.js/dist/driver.css'

const router = useRouter()
const route = useRoute()

const user = useUserStore()
const cards = [
  {
    title: 'Step 1 · Create Node Flavor',
    to: '/nodeflavor',
    icon: Cpu,
    desc: 'Define a reusable resource template (CPU/GPU, memory, root/data disks, extended resources like ephemeral-storage/RDMA).',
  },
  {
    title: 'Step 2 · Create Secret',
    to: '/secrets',
    icon: Key,
    desc: 'Store SSH or image credentials (username/keys/password) for secure access and pulls.',
  },
  {
    title: 'Step 3 · Register Node',
    to: '/nodes',
    icon: Monitor,
    desc: 'Bind a flavor and SSH secret, set hostname/IP/port, and add custom labels.',
  },
  {
    title: 'Step 4 · Create Cluster',
    to: '/clusters',
    icon: Collection,
    desc: 'Name the cluster, attach SSH & image secrets, pick network plugin/version, add nodes, and (optionally) tune apiserver args.',
  },
  {
    title: 'Step 5 · Configure Workspace',
    to: '/workspace',
    icon: Setting,
    desc: 'Name and describe the workspace, assign a node flavor & replica, set queue policy/preemption/default, invite managers, and mount volumes.',
  },
]
const go = (to: string) => router.push(to)

function waitForEl(selector: string, timeout = 6000): Promise<Element> {
  return new Promise((resolve, reject) => {
    const hit = document.querySelector(selector)
    if (hit) return resolve(hit)
    const obs = new MutationObserver(() => {
      const el = document.querySelector(selector)
      if (el) {
        obs.disconnect()
        resolve(el)
      }
    })
    obs.observe(document.body, { childList: true, subtree: true })
    setTimeout(() => {
      obs.disconnect()
      reject(new Error('wait timeout: ' + selector))
    }, timeout)
  })
}

const steps: DriveStep[] = [
  {
    element: '[data-tour="menu-nodeflavors"]',
    popover: {
      title: 'Node Flavors',
      description: 'Define reusable resource templates for scheduling.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '[data-tour="menu-secrets"]',
    popover: {
      title: 'Secrets',
      description: 'Store SSH and registry credentials for access and pulls.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '[data-tour="menu-nodes"]',
    popover: {
      title: 'Nodes',
      description: 'Register a machine and label it for targeted scheduling.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '[data-tour="menu-clusters"]',
    popover: {
      title: 'Clusters',
      description: 'Configure Kubernetes version, networking, and node membership.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '[data-tour="menu-workspace"]',
    popover: {
      title: 'Workspaces',
      description: 'Scope team resources, quotas, replicas, and queue policies.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '.steps',
    popover: {
      title: 'Quick Start',
      description:
        'Missed anything? Come back here to review all steps and jump into any card for details.',
      side: 'bottom',
    },
  },
]

let d: ReturnType<typeof driver> | null = null

function startTour(startIndex = 0) {
  const next = route.query.next as string
  d = driver({
    steps,
    allowClose: true,
    showProgress: true,
    overlayColor: 'rgba(0,0,0,.55)',
    stagePadding: 6,
    nextBtnText: 'Next',
    prevBtnText: 'Previous',
    doneBtnText: 'Done',
    onDestroyed: () => {
      user.markQuickStartSeen()
      if (next) router.replace(next)
    },
  })
  d.drive(startIndex)
}

onMounted(async () => {
  await Promise.all([
    waitForEl('[data-tour="menu-nodeflavors"]'),
    waitForEl('[data-tour="menu-secrets"]'),
    waitForEl('[data-tour="menu-nodes"]'),
    waitForEl('[data-tour="menu-clusters"]'),
    waitForEl('[data-tour="menu-workspace"]'),
    waitForEl('.steps'),
  ])

  if (user.shouldAutoShowQuickStart) {
    startTour(0)
  }
})
</script>

<style scoped>
.quickstart {
  padding: 24px;
}

/* Responsive grid, falls back to horizontal scroll on narrow screens */
.steps {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 14px;
}
@media (max-width: 1100px) {
  .steps {
    display: flex;
    overflow-x: auto;
    gap: 12px;
    padding-bottom: 6px;
    scroll-snap-type: x mandatory;
  }
  .step-card {
    min-width: 260px;
    scroll-snap-align: start;
  }
}

/* Glass card: clear in dark mode; increased height (less whitespace) */
.step-card {
  cursor: pointer;
  position: relative;
  border-radius: 14px;
  padding: 16px 16px 18px;
  min-height: 188px; /* Adjust here for more height, e.g. 208px */
  background:
    radial-gradient(2px 2px at 10% 15%, rgb(255 255 255 / 5%) 20%, transparent 21%) 0 0/12px 12px,
    radial-gradient(2px 2px at 80% 70%, rgb(255 255 255 / 4%) 20%, transparent 21%) 0 0/14px 14px,
    color-mix(in oklab, var(--el-bg-color) 78%, transparent 22%);
  border: 1px solid color-mix(in oklab, var(--el-border-color) 70%, transparent 30%);
  backdrop-filter: blur(10px) saturate(140%);
  -webkit-backdrop-filter: blur(10px) saturate(140%);
  box-shadow:
    0 0 0 1px rgb(255 255 255 / 4%) inset,
    0 8px 22px -10px rgb(0 0 0 / 40%);
  color: var(--el-text-color-primary);
  transition:
    transform 0.18s ease,
    box-shadow 0.18s ease,
    border-color 0.18s ease;
}
.step-card:hover {
  transform: translateY(-2px);
  box-shadow:
    0 0 0 1px color-mix(in oklab, var(--el-color-primary) 50%, transparent 50%) inset,
    0 12px 28px -10px color-mix(in oklab, var(--el-color-primary) 70%, black 30%);
  border-color: color-mix(in oklab, var(--el-color-primary) 50%, var(--el-border-color) 50%);
}

/* Large background icon at bottom-right: very faint */
.step-card .bg-icon {
  position: absolute;
  right: 10px;
  bottom: 6px;
  width: 120px;
  height: 120px;
  font-size: 120px; /* Element icons use font-size to control size */
  opacity: 0.1;
  pointer-events: none;
  filter: drop-shadow(0 2px 6px rgb(0 0 0 / 0.25));
  color: color-mix(in oklab, var(--el-color-primary) 70%, white 30%);
}

/* Title/body contrast */
.step-card h4 {
  margin: 0 0 6px;
  font-weight: 700;
  font-size: 15px;
  letter-spacing: 0.2px;
  color: color-mix(in oklab, var(--el-color-primary) 90%, white 10%);
  text-shadow: 0 0 14px rgb(99 102 241 / 22%);
}
.step-card p {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: color-mix(in oklab, var(--el-text-color-regular) 95%, white 5%);
}

/* Add more background weight in dark mode to prevent grayish appearance */
:deep(html.dark) .step-card {
  background:
    radial-gradient(2px 2px at 10% 15%, rgb(255 255 255 / 4.5%) 20%, transparent 21%) 0 0/12px 12px,
    radial-gradient(2px 2px at 80% 70%, rgb(255 255 255 / 3.5%) 20%, transparent 21%) 0 0/14px 14px,
    color-mix(in oklab, var(--el-bg-color) 64%, transparent 36%);
  border-color: color-mix(in oklab, var(--el-border-color) 55%, transparent 45%);
}

/* ===== System Overview - descriptions styled as soft card grid ===== */
.sys-desc {
  --gap: 12px;
}
:deep(.sys-desc .el-descriptions__table) {
  border-collapse: separate;
  border-spacing: var(--gap) var(--gap);
}
:deep(.sys-desc .el-descriptions__cell) {
  background: color-mix(in oklab, var(--el-bg-color) 86%, transparent 14%);
  border: 1px solid color-mix(in oklab, var(--el-border-color) 70%, transparent 30%);
  border-radius: 12px;
  padding: 12px 14px;
  box-shadow: 0 1px 0 rgb(255 255 255 / 5%) inset;
}
:deep(.sys-desc .el-descriptions__label) {
  color: color-mix(in oklab, var(--el-text-color-secondary) 95%, white 5%);
  font-weight: 600;
  padding-right: 8px;
}
:deep(.sys-desc .el-descriptions__content) {
  color: var(--el-text-color-regular);
}

/* Responsive columns: 3 on desktop, 2 on medium, 1 on small screens */
@media (max-width: 1280px) {
  :deep(.sys-desc .el-descriptions__table) {
    border-spacing: 10px 10px;
  }
  :deep(.sys-desc .el-descriptions__label) {
    width: 120px;
  }
}
@media (max-width: 900px) {
  :deep(.sys-desc .el-descriptions__table) {
    border-spacing: 8px 8px;
  }
}
</style>

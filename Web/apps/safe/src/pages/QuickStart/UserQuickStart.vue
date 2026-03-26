<template>
  <div class="safe-page user-qs">
    <!-- ─── Hero ─── -->
    <header class="qs-hero">
      <span class="qs-hero__kicker">Primus SaFE</span>
      <h1 class="qs-hero__title">Quick Start</h1>
      <p class="qs-hero__sub">
        Five steps from container image to a running training job —
        prepare, debug, secure, store, launch.
      </p>
    </header>

    <!-- ─── Step Cards (2 × 2 grid) ─── -->
    <section class="qs-steps">
      <article
        v-for="(s, i) in steps"
        :key="s.title"
        class="qs-step"
        tabindex="0"
        @click="go(s.to)"
        @keydown.enter="go(s.to)"
      >
        <div class="qs-step__top">
          <span class="qs-step__badge">{{ i + 1 }}</span>
          <span class="qs-step__tag">{{ s.tag }}</span>
        </div>
        <h3 class="qs-step__title">{{ s.title }}</h3>
        <p class="qs-step__desc">{{ s.desc }}</p>
        <ul class="qs-step__hints">
          <li v-for="h in s.hints" :key="h">{{ h }}</li>
        </ul>
        <span class="qs-step__cta">{{ s.cta }} <i class="cta-arrow">→</i></span>
      </article>
    </section>

    <!-- ─── Quick Reference ─── -->
    <section class="qs-ref">
      <h2 class="qs-section-title">Quick Reference</h2>
      <div class="qs-ref__list">
        <div
          v-for="r in refs"
          :key="r.task"
          class="qs-ref__row"
          tabindex="0"
          @click="goWithTour(r.to, r.tourId)"
          @keydown.enter="goWithTour(r.to, r.tourId)"
        >
          <span class="qs-ref__task">{{ r.task }}</span>
          <span class="qs-ref__path">{{ r.path }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { driver, type DriveStep } from 'driver.js'
import { useUserStore } from '@/stores/user'
import 'driver.js/dist/driver.css'

const router = useRouter()
const route = useRoute()
const user = useUserStore()

const go = (to: string) => router.push(to)
const goWithTour = (to: string, tourId: string) =>
  router.push({ path: to, query: { tour: tourId } })

/* ── 4 core steps (from Primus SaFE Quick Start guide) ── */
const steps = [
  {
    tag: 'Container Registry',
    title: 'Prepare Your Image',
    desc: 'Import a container image from a supported registry, or construct a Harbor proxy URL for public images.',
    hints: [
      'Import (recommended) — UI → Container Images → Import; available once status is Ready',
      'Harbor proxy (public only) — prepend your cluster\'s proxy base to the image path',
    ],
    cta: 'Go to Images',
    to: '/images',
  },
  {
    tag: 'Interactive Development',
    title: 'Debug with Authoring',
    desc: 'Spin up an Authoring pod and connect via WebShell or SSH for interactive debugging.',
    hints: [
      'WebShell — Actions → SSH → choose container & shell',
      'SSH — connect on port 2222; upload via scp / sftp',
      'VS Code / Cursor Remote-SSH file explorer also supported',
    ],
    cta: 'Open Authoring',
    to: '/authoring',
  },
  {
    tag: 'Security',
    title: 'Add Your SSH Key',
    desc: 'Register your SSH public key so you can securely connect to Authoring pods and push to Git repos.',
    hints: [
      'Go to Secrets → Add → SSH Key to register your public key',
      'Once added, use your private key to SSH into any Authoring pod',
    ],
    cta: 'Go to Secrets',
    to: '/secrets',
  },
  {
    tag: 'Data & Storage',
    title: 'Know Your Storage',
    desc: 'Storage is auto-mounted into every pod — the mount path depends on the cluster type.',
    hints: [
      'TensorWeave (TW) — Weka CSI → /wekafs',
      'OCI — NFS → /shared_nfs',
    ],
    cta: 'Go to Training',
    to: '/training',
  },
  {
    tag: 'Model Training',
    title: 'Launch a Training Job',
    desc: 'Use the guided wizard to create a training workload step by step — select workspace, configure resources, and submit.',
    hints: [
      'Guided wizard — fill a step-by-step form right inside the chat page',
      'Or clone an existing template from the Training page for quick setup',
    ],
    cta: 'Create Training (Guided)',
    to: '/chatbot?wizard=create_training',
  },
]

/* ── Quick-reference shortcuts (tourId drives per-action tour on the target page) ── */
const refs = [
  { task: 'Import image', path: 'UI → Container Images → Import', to: '/images', tourId: 'import' },
  { task: 'Start dev pod', path: 'UI → Authoring → Create', to: '/authoring', tourId: 'create' },
  { task: 'Connect to pod', path: 'Actions → SSH → WebShell / Copy SSH', to: '/authoring', tourId: 'connect' },
  { task: 'Upload files', path: 'scp -P 2222 file user@host:path', to: '/authoring', tourId: 'upload' },
  { task: 'Launch training', path: 'UI → Training → Clone → Start', to: '/training', tourId: 'create' },
  { task: 'Save custom image', path: 'Authoring → Actions → Save Image', to: '/authoring', tourId: 'save-image' },
  { task: 'Prewarm image', path: 'UI → Prewarm Image → select image', to: '/images', tourId: 'prewarm' },
  { task: 'Download logs', path: 'Training → select job → Logs → Download', to: '/training', tourId: 'logs' },
]

/* ── Menu Guided Tour (driver.js) ── */
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

/* All possible tour steps — filtered at runtime based on what's in the DOM */
const allTourSteps: DriveStep[] = [
  {
    element: '[data-tour="menu-images"]',
    popover: {
      title: 'Images',
      description: 'Import or manage container images before using them in workloads.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '[data-tour="menu-training"]',
    popover: {
      title: 'Training',
      description: 'Create and monitor training jobs — clone templates for quick setup.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '[data-tour="menu-authoring"]',
    popover: {
      title: 'Authoring',
      description: 'Interactive dev pods — connect via WebShell or SSH to debug and iterate.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '[data-tour="menu-secrets"]',
    popover: {
      title: 'SSH Key',
      description: 'Register your SSH public key here. Navigate to Secrets → Add → SSH Key to get started.',
      side: 'right',
      align: 'start',
    },
  },
  {
    element: '.qs-steps',
    popover: {
      title: 'Your Quick Start Guide',
      description:
        'Follow the five steps to go from container image to a running training job. Click any card to jump in.',
      side: 'bottom',
    },
  },
]

let d: ReturnType<typeof driver> | null = null

function startTour(startIndex = 0) {
  // Only include steps whose target elements actually exist (permission-dependent menus)
  const visibleSteps = allTourSteps.filter(
    (s) => !s.element || document.querySelector(s.element as string),
  )
  if (!visibleSteps.length) return

  const next = route.query.next as string
  d = driver({
    steps: visibleSteps,
    allowClose: true,
    showProgress: true,
    overlayColor: 'rgba(0,0,0,.55)',
    stagePadding: 6,
    nextBtnText: 'Next',
    prevBtnText: 'Previous',
    doneBtnText: 'Done',
    onDestroyed: () => {
      user.markUserQuickStartSeen()
      if (next) router.replace(next)
    },
  })
  d.drive(startIndex)
}

onMounted(async () => {
  // Wait for guaranteed elements; optional menus may or may not appear
  await Promise.all([
    waitForEl('[data-tour="menu-images"]').catch(() => null),
    waitForEl('[data-tour="menu-training"]').catch(() => null),
    waitForEl('[data-tour="menu-authoring"]').catch(() => null),
    waitForEl('[data-tour="menu-secrets"]').catch(() => null),
    waitForEl('.qs-steps'),
  ])

  if (user.shouldAutoShowUserQuickStart) {
    startTour(0)
  }
})
</script>

<style scoped>
/* ═══════════════════════════════════════════════════════
   User Quick Start — Liquid-glass, Apple-inspired design
   Hierarchy via contrast · no glow · soft edges
   ═══════════════════════════════════════════════════════ */

/* ────────────── Layout ────────────── */
.user-qs {
  padding: 32px 28px 48px;
  max-width: 1200px;
  margin: 0 auto;
}

/* ────────────── Hero ────────────── */
.qs-hero {
  margin-bottom: 36px;
}
.qs-hero__kicker {
  display: block;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--safe-primary);
  margin-bottom: 6px;
}
.qs-hero__title {
  font-size: 28px;
  font-weight: 700;
  color: var(--safe-text);
  margin: 0 0 8px;
  letter-spacing: -0.02em;
}
.qs-hero__sub {
  font-size: 15px;
  color: var(--safe-muted);
  line-height: 1.6;
  margin: 0;
  max-width: 560px;
}

/* ────────────── Step Grid ────────────── */
.qs-steps {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
  margin-bottom: 44px;
}

/* ────────────── Glass Step Card ────────────── */
.qs-step {
  position: relative;
  display: flex;
  flex-direction: column;
  padding: 22px 24px 20px;
  border-radius: 16px;
  cursor: pointer;

  /* Liquid glass: semi-transparent + blur */
  background:
    linear-gradient(
      180deg,
      color-mix(in oklab, var(--safe-card) 82%, transparent 18%) 0%,
      color-mix(in oklab, var(--safe-card) 72%, transparent 28%) 100%
    );
  backdrop-filter: blur(24px) saturate(130%);
  -webkit-backdrop-filter: blur(24px) saturate(130%);

  /* Subtle border */
  border: 1px solid color-mix(in oklab, var(--safe-border) 55%, transparent 45%);

  /* Inner top-edge highlight + soft shadow */
  box-shadow:
    inset 0 0.5px 0 0 rgb(255 255 255 / 0.18),
    0 1px 3px rgb(0 0 0 / 0.03),
    0 4px 14px -4px rgb(0 0 0 / 0.06);

  transition:
    transform 0.22s ease,
    box-shadow 0.22s ease,
    border-color 0.22s ease;
}

/* Subtle top accent gradient */
.qs-step::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  border-radius: 16px 16px 0 0;
  background: linear-gradient(90deg, transparent 5%, var(--safe-primary) 50%, transparent 95%);
  opacity: 0.18;
  pointer-events: none;
  transition: opacity 0.22s ease;
}

.qs-step:hover {
  transform: translateY(-2px);
  border-color: color-mix(in oklab, var(--safe-primary) 28%, var(--safe-border) 72%);
  box-shadow:
    inset 0 0.5px 0 0 rgb(255 255 255 / 0.22),
    0 2px 6px rgb(0 0 0 / 0.04),
    0 10px 24px -6px rgb(0 0 0 / 0.09);
}
.qs-step:hover::before {
  opacity: 0.35;
}
.qs-step:focus-visible {
  outline: 2px solid var(--safe-primary);
  outline-offset: 2px;
}

/* Badge + Tag row */
.qs-step__top {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}
.qs-step__badge {
  width: 30px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  font-size: 13px;
  font-weight: 700;
  background: color-mix(in oklab, var(--safe-primary) 12%, transparent 88%);
  color: var(--safe-primary);
  flex-shrink: 0;
}
.qs-step__tag {
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--safe-muted);
}

/* Title & description — contrast through weight/size */
.qs-step__title {
  font-size: 17px;
  font-weight: 650;
  color: var(--safe-text);
  margin: 0 0 6px;
  letter-spacing: -0.01em;
}
.qs-step__desc {
  font-size: 13px;
  color: var(--safe-muted);
  line-height: 1.55;
  margin: 0 0 12px;
}

/* Hints — recessed sub-panel for depth */
.qs-step__hints {
  list-style: none;
  margin: 0 0 16px;
  padding: 10px 12px;
  border-radius: 10px;
  background: color-mix(in oklab, var(--safe-card-2) 50%, transparent 50%);
  border: 1px solid color-mix(in oklab, var(--safe-border) 28%, transparent 72%);
}
.qs-step__hints li {
  font-size: 12px;
  color: color-mix(in oklab, var(--safe-muted) 80%, var(--safe-text) 20%);
  line-height: 1.6;
  padding: 3px 0 3px 16px;
  position: relative;
}
.qs-step__hints li::before {
  content: '';
  position: absolute;
  left: 2px;
  top: 10px;
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: color-mix(in oklab, var(--safe-primary) 35%, var(--safe-muted) 65%);
}

/* CTA link */
.qs-step__cta {
  margin-top: auto;
  font-size: 13px;
  font-weight: 600;
  color: var(--safe-primary);
  display: flex;
  align-items: center;
  gap: 4px;
}
.cta-arrow {
  font-style: normal;
  display: inline-block;
  transition: transform 0.2s ease;
}
.qs-step:hover .cta-arrow {
  transform: translateX(3px);
}

/* ────────────── Section Titles ────────────── */
.qs-section-title {
  font-size: 18px;
  font-weight: 650;
  color: var(--safe-text);
  margin: 0 0 16px;
  letter-spacing: -0.01em;
}

/* ────────────── Quick Reference ────────────── */
.qs-ref__list {
  border-radius: 14px;
  overflow: hidden;
  border: 1px solid color-mix(in oklab, var(--safe-border) 38%, transparent 62%);
  background: color-mix(in oklab, var(--safe-card) 52%, transparent 48%);
  backdrop-filter: blur(12px) saturate(120%);
  -webkit-backdrop-filter: blur(12px) saturate(120%);
}
.qs-ref__row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 11px 18px;
  cursor: pointer;
  transition: background 0.15s ease;
  border-bottom: 1px solid color-mix(in oklab, var(--safe-border) 18%, transparent 82%);
}
.qs-ref__row:last-child {
  border-bottom: none;
}
.qs-ref__row:hover {
  background: color-mix(in oklab, var(--safe-primary) 5%, transparent 95%);
}
.qs-ref__row:focus-visible {
  outline: 2px solid var(--safe-primary);
  outline-offset: -2px;
}
.qs-ref__task {
  font-size: 13px;
  font-weight: 600;
  color: var(--safe-text);
  flex-shrink: 0;
}
.qs-ref__path {
  font-size: 12px;
  color: var(--safe-muted);
  font-family: 'SF Mono', 'Cascadia Code', ui-monospace, monospace;
  text-align: right;
}

/* ────────────── Dark Mode Fine-tuning ────────────── */
:deep(html.dark) .qs-step {
  box-shadow:
    inset 0 0.5px 0 0 rgb(255 255 255 / 0.06),
    0 1px 3px rgb(0 0 0 / 0.12),
    0 4px 14px -4px rgb(0 0 0 / 0.22);
}
:deep(html.dark) .qs-step:hover {
  box-shadow:
    inset 0 0.5px 0 0 rgb(255 255 255 / 0.08),
    0 2px 6px rgb(0 0 0 / 0.16),
    0 10px 24px -6px rgb(0 0 0 / 0.3);
}
:deep(html.dark) .qs-step__hints {
  background: color-mix(in oklab, var(--safe-card-2) 35%, transparent 65%);
  border-color: color-mix(in oklab, var(--safe-border) 22%, transparent 78%);
}
/* ────────────── Responsive ────────────── */
@media (max-width: 860px) {
  .qs-steps {
    grid-template-columns: 1fr;
  }
  .qs-ref__row {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }
  .qs-ref__path {
    text-align: left;
  }
}
@media (max-width: 640px) {
  .user-qs {
    padding: 20px 16px 36px;
  }
  .qs-hero__title {
    font-size: 24px;
  }
}

/* ────────────── Reduce motion ────────────── */
@media (prefers-reduced-motion: reduce) {
  .qs-step,
  .qs-step::before,
  .qs-ref__row,
  .cta-arrow {
    transition: none;
  }
}
</style>

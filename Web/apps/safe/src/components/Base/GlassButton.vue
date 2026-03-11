<template>
  <button
    class="glass-btn"
    :class="[
      `glass--${props.variant}`,
      `glass--${props.size}`,
      { 'is-loading': props.loading, 'is-block': props.block },
    ]"
    :disabled="props.disabled || props.loading"
    @click="emit('click', $event)"
  >
    <slot name="icon" />
    <span class="label"><slot /></span>
  </button>
</template>

<script setup lang="ts">
interface Props {
  variant?: 'primary' | 'neutral' | 'danger'
  size?: 'sm' | 'md' | 'lg'
  block?: boolean
  loading?: boolean
  disabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'primary',
  size: 'md',
  block: false,
  loading: false,
  disabled: false,
})

const emit = defineEmits<{ (e: 'click', ev: MouseEvent): void }>()
</script>

<style scoped>
/* Design variables: adjustable as needed */
.glass-btn {
  --gb-radius: 14px;
  --gb-pad-y: 0.55rem;
  --gb-pad-x: 0.9rem;
  --gb-gap: 0.5rem;
  --gb-blur: 12px;
  --gb-sat: 160%;
  --gb-opacity: 0.18; /* Panel body opacity */
  --gb-stroke: 1px; /* Outer stroke width */
  --gb-shadow: 0 10px 28px -12px rgb(0 0 0 / 0.55);
  --gb-t: 0.18s;

  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--gb-gap);
  padding: var(--gb-pad-y) var(--gb-pad-x);
  border: 0;
  border-radius: var(--gb-radius);
  background:
    linear-gradient(180deg, rgb(255 255 255 / 0.28), rgb(255 255 255 / 0.08)) /* Subtle top highlight */,
    color-mix(in oklab, var(--el-bg-color) 70%, transparent 30%); /* Base */
  color: var(--el-text-color-primary);
  -webkit-backdrop-filter: blur(var(--gb-blur)) saturate(var(--gb-sat));
  backdrop-filter: blur(var(--gb-blur)) saturate(var(--gb-sat));
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 0.35),
    /* Inner top highlight */ var(--gb-shadow);
  cursor: pointer;
  user-select: none;
  transition:
    transform var(--gb-t) ease,
    box-shadow var(--gb-t) ease,
    background var(--gb-t) ease;
  overflow: hidden;
}

/* Gradient stroke (edge glow only), via content-box / border-box mask */
.glass-btn::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  padding: var(--gb-stroke);
  background: conic-gradient(
    from 210deg,
    color-mix(in oklab, var(--gb-accent, var(--el-color-primary)) 85%, white 15%),
    color-mix(in oklab, var(--gb-accent, var(--el-color-primary)) 20%, transparent 80%),
    color-mix(in oklab, var(--gb-accent, var(--el-color-primary)) 85%, white 15%)
  );
  -webkit-mask:
    linear-gradient(#000 0 0) content-box,
    linear-gradient(#000 0 0) border-box;
  -webkit-mask-composite: xor;
  mask-composite: exclude;
  opacity: 0.55;
  pointer-events: none;
}

/* Diagonal highlight sweep (appears on hover) */
.glass-btn::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: linear-gradient(
    75deg,
    transparent 44%,
    rgba(255, 255, 255, 0.22) 50%,
    transparent 56%
  );
  background-repeat: no-repeat;
  background-size: 220% 220%; /* Enlarge background, use offset for sweep effect */
  background-position: -120% 0%; /* Initially off-screen to the left */
  opacity: 0; /* Hidden by default */
  transition:
    background-position 0.8s ease,
    opacity 0.25s ease;
  pointer-events: none;
  z-index: 0; /* Below text/icons */
}
.glass-btn:hover::after {
  opacity: 0.35; /* Moderate intensity, not too glaring */
  background-position: 120% 0%; /* Sweep from left to right */
}

/* Variant: primary / neutral / danger */
.glass--primary {
  --gb-accent: var(--el-color-primary);
}
.glass--neutral {
  --gb-accent: color-mix(in oklab, var(--el-text-color-secondary) 60%, var(--el-color-primary) 40%);
}
.glass--danger {
  --gb-accent: var(--el-color-danger);
}

/* Different sizes */
.glass--sm {
  --gb-pad-y: 0.42rem;
  --gb-pad-x: 0.7rem;
  font-size: 13px;
  --gb-radius: 12px;
}
.glass--md {
  font-size: 14px;
}
.glass--lg {
  --gb-pad-y: 0.72rem;
  --gb-pad-x: 1.05rem;
  font-size: 15px;
  --gb-radius: 16px;
}
.is-block {
  width: 100%;
}

/* Icon sizing: Element Plus <el-icon> can be placed directly in slot */
::v-slotted(svg),
::v-slotted(.el-icon) {
  font-size: 1.1em;
}

/* Interactive state */
.glass-btn:hover {
  transform: translateY(-1px);
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 0.45),
    0 14px 34px -12px color-mix(in oklab, var(--gb-accent, var(--el-color-primary)) 40%, black 60%);
}
.glass-btn:hover::after {
  opacity: 1;
  transform: translateX(25%);
}

.glass-btn:active {
  transform: translateY(0) scale(0.997);
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 0.25),
    0 10px 20px -14px color-mix(in oklab, var(--gb-accent, var(--el-color-primary)) 35%, black 65%);
}

/* Focus-visible: keyboard accessibility */
.glass-btn:focus-visible {
  outline: 2px solid color-mix(in oklab, var(--gb-accent, var(--el-color-primary)) 70%, white 30%);
  outline-offset: 2px;
}

/* Disabled / loading */
.glass-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
  transform: none;
}
.is-loading .label {
  opacity: 0.85;
}
.is-loading::after {
  /* Use ::after as a simple loading overlay/highlight */
  opacity: 0.6;
  animation: sweep 1.2s linear infinite;
}
@keyframes sweep {
  0% {
    transform: translateX(-45%);
  }
  100% {
    transform: translateX(45%);
  }
}

/* Dark mode: darken base, increase stroke contrast */
:deep(html.dark) .glass-btn {
  background:
    linear-gradient(180deg, rgb(255 255 255 / 0.14), rgb(255 255 255 / 0.05)),
    color-mix(in oklab, var(--el-bg-color) 45%, transparent 55%);
}

/* Fallback: styles when backdrop-filter is not supported */
@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px))) {
  .glass-btn {
    background: color-mix(in oklab, var(--el-bg-color) 80%, white 20%);
  }
}
</style>

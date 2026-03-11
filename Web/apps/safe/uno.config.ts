import { defineConfig, presetMini, presetIcons, presetAttributify } from 'unocss'

export default defineConfig({
  rules: [
    // textx-12 / textx-12.5 → font-size: calc(12px * var(--scale, 1))
    [
      /^textx-(\d+(?:\.\d+)?)$/,
      ([, n]) => ({
        'font-size': `calc(${n}px * var(--scale, 1))`,
        'line-height': '1.4', // Recommended unitless line-height
      }),
    ],
    [
      /^hx-(\d+(?:\.\d+)?)$/,
      ([, d]) => ({
        height: `calc(${d}px * var(--scale))`,
      }),
    ],
    [
      /^wx-(\d+(?:\.\d+)?)$/,
      ([, d]) => ({
        width: `calc(${d}px * var(--scale))`,
      }),
    ],
  ],
  presets: [presetMini(), presetIcons(), presetAttributify()],
  safelist: [
    // Keep all class names that might be generated dynamically at runtime
    'grid-cols-1',
    'grid-cols-2',
    'grid-cols-3',
    'grid-cols-4',
    'grid-cols-5',
    'grid-cols-6',
    'col-span-1',
    'col-span-2',
    'col-span-3',
    'col-span-4',
    'col-span-5',
    'col-span-6',
  ],
})

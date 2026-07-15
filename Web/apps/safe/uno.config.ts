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
  shortcuts: {
    // Semantic type scale — the ONLY font sizes pages should use.
    // Each maps to the scale-aware `textx-N` rule above.
    'fs-caption': 'textx-12', // table meta, secondary text, tags
    'fs-body': 'textx-13', // default body text, table cells
    'fs-label': 'textx-14', // form labels, menu items, buttons
    'fs-subtitle': 'textx-16', // card / section titles
    'fs-title': 'textx-18', // page titles
    'fs-display': 'textx-22', // detail-page hero titles (rare)
  },
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

import { defineConfig, presetMini, presetIcons, presetAttributify } from 'unocss'

export default defineConfig({
  presets: [
    presetMini(), 
    presetIcons({
      scale: 1.2,
      warn: true,
      cdn: 'https://esm.sh/',
    }),
    presetAttributify(),
  ],
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
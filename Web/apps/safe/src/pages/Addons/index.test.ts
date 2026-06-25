import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const pageSource = readFileSync(new URL('./index.vue', import.meta.url), 'utf-8')

describe('Addons page dialog state', () => {
  it('clears the addon name when opening the create dialog', () => {
    expect(pageSource).toContain("state.curAction = 'Create'")
    expect(pageSource).toContain("state.curName = ''")
  })
})

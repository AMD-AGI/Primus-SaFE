import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const dialogSource = readFileSync(new URL('./AddDialog.vue', import.meta.url), 'utf-8')

describe('Dynamo/Optimus AddDialog', () => {
  it('does not expose a separate memFraction input field', () => {
    expect(dialogSource).not.toContain('label="memFraction"')
  })
})

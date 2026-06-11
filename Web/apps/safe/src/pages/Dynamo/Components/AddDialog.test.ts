import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const dialogSource = readFileSync(new URL('./AddDialog.vue', import.meta.url), 'utf-8')

describe('Dynamo/Optimus AddDialog', () => {
  it('does not expose a separate memFraction input field', () => {
    expect(dialogSource).not.toContain('label="memFraction"')
  })

  it('exposes Optimus router policy and role-specific entrypoint editors', () => {
    expect(dialogSource).toContain('label="routerPolicy"')
    expect(dialogSource).toContain('value="round-robin"')
    expect(dialogSource).toContain('optimusRoleSections')
    expect(dialogSource).toContain('setRoleEntrypoint')
    expect(dialogSource).toContain('getRoleBackendEngine')
    expect(dialogSource).toContain('setRoleBackendEngine')
  })

  it('groups Optimus resource and entrypoint fields by role card', () => {
    expect(dialogSource).toContain('optimus-role-card')
    expect(dialogSource).toContain('EntryPoint Parameters')
    expect(dialogSource).toContain('v-if="!isOptimus"')
  })

  it('does not keep unreachable Optimus entrypoint preview code in the Dynamo-only section', () => {
    expect(dialogSource).not.toContain('optimusBackendEntrySections')
  })
})

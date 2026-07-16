import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const dialogSource = readFileSync(new URL('./AddDialog.vue', import.meta.url), 'utf-8')

describe('Dynamo/Infera AddDialog', () => {
  it('does not expose a separate memFraction input field', () => {
    expect(dialogSource).not.toContain('label="memFraction"')
  })

  it('exposes Infera router policy and role-specific entrypoint editors', () => {
    expect(dialogSource).toContain('label="Router Policy"')
    expect(dialogSource).toContain('value="round-robin"')
    expect(dialogSource).toContain('inferaRoleSections')
    expect(dialogSource).toContain('setRoleEntrypoint')
    expect(dialogSource).toContain('getRoleBackendEngine')
    expect(dialogSource).toContain('setRoleBackendEngine')
  })

  it('groups Infera resource and entrypoint fields by role card', () => {
    expect(dialogSource).toContain('infera-role-card')
    expect(dialogSource).toContain('EntryPoint Parameters')
    expect(dialogSource).toContain('v-if="!isInfera"')
  })

  it('keeps Infera mode switching compact', () => {
    expect(dialogSource).toContain('<el-segmented class="form-seg" v-model="modeValue"')
    expect(dialogSource).not.toContain('Standard Serving')
    expect(dialogSource).not.toContain('PD Disaggregation')
  })

  it('does not show a separate Infera creation summary', () => {
    expect(dialogSource).not.toContain('Creation Summary')
    expect(dialogSource).not.toContain('summaryItems')
  })

  it('hides role command editors behind an advanced override disclosure', () => {
    expect(dialogSource).toContain('Advanced command override')
    expect(dialogSource).toContain('commandOverrideOpen')
    expect(dialogSource).not.toContain('Preview the generated frontend command.')
  })

  it('allows overriding the Infera frontend command', () => {
    expect(dialogSource).toContain('setFrontendEntrypoint')
    expect(dialogSource).toContain('resetFrontendEntrypointFromOptions')
    expect(dialogSource).toContain('getFrontendEntrypoint')
    expect(dialogSource).not.toContain(':model-value="frontendPreview"\n                  type="textarea"\n                  readonly')
  })

  it('does not keep unreachable Infera entrypoint preview code in the Dynamo-only section', () => {
    expect(dialogSource).not.toContain('inferaBackendEntrySections')
  })
})

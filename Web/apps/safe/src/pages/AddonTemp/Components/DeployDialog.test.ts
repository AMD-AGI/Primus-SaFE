import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const dialogSource = readFileSync(new URL('./DeployDialog.vue', import.meta.url), 'utf-8')

describe('Addon DeployDialog values handling', () => {
  it('loads template defaults when the template select changes', () => {
    expect(dialogSource).toContain('<el-select v-model="form.template" @change="onTemplateChange">')
    expect(dialogSource).toContain('const applyTemplateDefaults = async')
    expect(dialogSource).toContain('const onTemplateChange = (templateId: string) =>')
  })

  it('loads template detail in edit mode so reset can restore template defaults', () => {
    expect(dialogSource).toContain('await applyTemplateDefaults(res.template, false)')
  })

  it('requires a second confirmation before submitting replacement values', () => {
    expect(dialogSource).toContain('ElMessageBox.confirm')
    expect(dialogSource).toContain('values replace the template defaults')
  })
})

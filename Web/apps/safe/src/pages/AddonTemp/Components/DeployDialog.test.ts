import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const dialogSource = readFileSync(new URL('./DeployDialog.vue', import.meta.url), 'utf-8')

describe('Addon DeployDialog values handling', () => {
  it('loads template defaults when the template select changes', () => {
    expect(dialogSource).toContain('v-if="canSelectTemplate"')
    expect(dialogSource).toContain('<el-select v-model="form.template" @change="onTemplateChange">')
    expect(dialogSource).toContain('const applyTemplateDefaults = async')
    expect(dialogSource).toContain('const onTemplateChange = (templateId: string) =>')
  })

  it('loads template options for create mode opened from the Addons page', () => {
    expect(dialogSource).toContain('const canSelectTemplate = computed(() => !props.id)')
    expect(dialogSource).toContain('} else {\n    await fetchTemps()\n  }')
  })

  it('maps full values from helmStatus before falling back to spec defaults', () => {
    expect(dialogSource).toContain('templateDetail.helmStatus?.valuesYaml')
    expect(dialogSource).toContain('typeof templateDetail.helmStatus?.values ===')
    expect(dialogSource).toContain('templateDetail.helmDefaultValues')
  })

  it('loads template detail in edit mode so reset can restore template defaults', () => {
    expect(dialogSource).toContain('await applyTemplateDefaults(res.template, false)')
  })

  it('prefills edit values from addon status when top-level values are empty', () => {
    expect(dialogSource).toContain('const getAddonDetailValues = (addon: AddonDetailData) =>')
    expect(dialogSource).toContain('addon.values || statusValues ||')
    expect(dialogSource).toContain('form.values = getAddonDetailValues(res)')
  })

  it('prefills edit values from template detail when the addon has no saved values', () => {
    expect(dialogSource).toContain('const templateDetail = await applyTemplateDefaults(res.template, false)')
    expect(dialogSource).toContain('if (!form.values && templateDetail) {')
    expect(dialogSource).toContain('form.values = getTemplateDefaultValues(templateDetail)')
  })

  it('requires a second confirmation before submitting replacement values', () => {
    expect(dialogSource).toContain('ElMessageBox.confirm')
    expect(dialogSource).toContain('values replace the template defaults')
  })
})

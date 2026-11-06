import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const read = (rel: string) => readFileSync(new URL(rel, import.meta.url), 'utf-8')

// Every form that creates a workload of a kind restricted by enableUserDir offers the field, sends
// it, and fills it back on Clone/Resume from the workload detail.
// The third column says whether the form has an Edit mode, where hostpath cannot be patched.
const forms: Array<[string, string, boolean]> = [
  ['../../pages/Training/Components/AddDialog.vue', 'Train', true],
  ['../../pages/TorchFT/Components/AddDialog.vue', 'Train', true],
  ['../../pages/Monarch/Components/AddDialog.vue', 'Train', false],
  ['../../pages/RayJob/Components/AddDialog.vue', 'Ray', true],
  ['../../pages/Infer/Components/AddDialog.vue', 'Infer', true],
  ['../../pages/Authoring/Components/AddDialog.vue', 'Authoring', true],
]

describe('extra mounts field wiring', () => {
  for (const [file, scope, hasEdit] of forms) {
    describe(file, () => {
      const src = read(file)

      it('renders the field for the workload scope, read-only on Edit', () => {
        expect(src).toContain('<ExtraHostPathField')
        expect(src).toContain('v-model="form.hostpath"')
        expect(src).toContain(`scope="${scope}"`)
        expect(src).toContain(':workspace-id="targetWorkspaceId"')
        if (hasEdit) expect(src).toContain(':disabled="isEdit"')
      })

      it('starts empty, sends the entries, and keeps them on Clone/Resume', () => {
        expect(src).toContain('hostpath: [] as string[],')
        expect(src).toContain('...hostPathPayload(hostpath),')
        expect(src).toContain('form.hostpath = normalizeHostPaths(res.hostpath)')
      })
    })
  }

  it('keeps hostpath when a workload is resumed from its detail page', () => {
    expect(read('../../composables/useWorkloadDetail.ts')).toContain(
      'Object.assign(payload, hostPathPayload(detail.hostpath))',
    )
  })

  it('tells the user where private data belongs', () => {
    const field = read('./ExtraHostPathField.vue')
    expect(field).toContain('Keep private data in')
    expect(field).toContain('it is mounted automatically and only')
    expect(field).toContain('checkHostPathEntry(entry, roots.value')
  })
})

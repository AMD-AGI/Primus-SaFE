import { describe, expect, it } from 'vitest'
import { detailRouteOf, WorkloadKind } from './type'

describe('detailRouteOf', () => {
  it.each([
    [WorkloadKind.PyTorchJob, '/training/detail'],
    [WorkloadKind.InferaDeployment, '/infera/detail'],
    [WorkloadKind.Authoring, '/authoring/detail'],
  ])('routes %s to its own detail page', (kind, path) => {
    expect(detailRouteOf(kind, 'wl-1')).toEqual({ path, query: { id: 'wl-1' } })
  })

  it('has no route for a kind without a detail page', () => {
    expect(detailRouteOf(WorkloadKind.DynamoDeployment, 'wl-1')).toBeUndefined()
  })

  it('has no route for an unknown kind', () => {
    expect(detailRouteOf('Unknown', 'wl-1')).toBeUndefined()
  })
})

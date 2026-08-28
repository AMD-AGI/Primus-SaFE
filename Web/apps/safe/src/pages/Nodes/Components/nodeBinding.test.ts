import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import type { WorkspaceItem } from '@/services'
import { buildNodeRelateRequest, migrateTargetOptions, relateWorkspaceId } from './nodeBinding'

const ws = (id: string, clusterId: string, flavorId: string): WorkspaceItem =>
  ({
    workspaceId: id,
    workspaceName: id,
    clusterId,
    flavorId,
    currentNodeCount: 0,
    scopes: [],
  }) as WorkspaceItem

describe('relateWorkspaceId', () => {
  it('addresses an add to the workspace taking the node on', () => {
    expect(relateWorkspaceId('add', 'ws-a', 'ws-b')).toBe('ws-b')
  })

  // A migration addressed to its target reads as that workspace migrating a node it does not
  // have, and is refused.
  it('addresses a remove and a migrate to the workspace giving the node up', () => {
    expect(relateWorkspaceId('remove', 'ws-a', '')).toBe('ws-a')
    expect(relateWorkspaceId('migrate', 'ws-a', 'ws-b')).toBe('ws-a')
  })
})

describe('buildNodeRelateRequest', () => {
  it('sends the target only for a migrate', () => {
    expect(
      buildNodeRelateRequest({
        action: 'migrate',
        nodeIds: ['node1'],
        targetWorkspaceId: 'ws-b',
        force: false,
      }),
    ).toEqual({ action: 'migrate', nodeIds: ['node1'], targetWorkspaceId: 'ws-b', force: false })

    // The API refuses a target on any other action rather than ignoring it.
    expect(
      buildNodeRelateRequest({
        action: 'add',
        nodeIds: ['node1'],
        targetWorkspaceId: 'ws-b',
        force: false,
      }),
    ).toEqual({ action: 'add', nodeIds: ['node1'] })
  })

  it('carries force for the actions that take a node away', () => {
    expect(
      buildNodeRelateRequest({
        action: 'remove',
        nodeIds: ['node1'],
        targetWorkspaceId: '',
        force: true,
      }),
    ).toEqual({ action: 'remove', nodeIds: ['node1'], force: true })
    expect(
      buildNodeRelateRequest({
        action: 'migrate',
        nodeIds: ['node1'],
        targetWorkspaceId: 'ws-b',
        force: true,
      }),
    ).toMatchObject({ force: true })
  })
})

describe('migrateTargetOptions', () => {
  const workspaces = [
    ws('ws-a', 'cluster1', 'flavor1'),
    ws('ws-same', 'cluster1', 'flavor1'),
    ws('ws-other-flavor', 'cluster1', 'flavor2'),
    ws('ws-no-flavor', 'cluster1', ''),
    ws('ws-other-cluster', 'cluster2', 'flavor1'),
  ]

  it('offers every other workspace in the same cluster', () => {
    const options = migrateTargetOptions(workspaces, 'ws-a').map((w) => w.workspaceId)
    // Flavor is not filtered on: whether a target can take one is the API's rule, and it has
    // more to it than this side can see -- a workspace scaled to zero may take any flavor.
    expect(options).toEqual(['ws-same', 'ws-other-flavor', 'ws-no-flavor'])
  })

  it('never offers the workspace the node is already in', () => {
    expect(migrateTargetOptions(workspaces, 'ws-a').map((w) => w.workspaceId)).not.toContain('ws-a')
  })

  // Filtering stands in for the node's own cluster and flavor. Without the source there is
  // nothing to compare against, and an empty list explains nothing -- let the API answer.
  it('falls back to every other workspace when the source is unknown', () => {
    const options = migrateTargetOptions(workspaces, 'ws-unknown').map((w) => w.workspaceId)
    expect(options).toHaveLength(workspaces.length)
  })
})

// The dialog has no component test to sit behind -- the app carries no DOM environment or
// mounting library -- so this guards the one wiring mistake the helpers above exist to
// prevent: addressing the request to the selected workspace, which is right for a bind and
// sends a migration to the wrong end.
describe('BindDialog wiring', () => {
  const source = readFileSync(new URL('./BindDialog.vue', import.meta.url), 'utf-8')

  it('routes the request through relateWorkspaceId rather than the selection', () => {
    expect(source).toContain('relateWorkspaceId(bindAction.value, props.wsId, selectedId.value)')
    expect(source).not.toContain('relateNodeToWs(selectedId.value')
  })

  it('builds the body through buildNodeRelateRequest', () => {
    expect(source).toContain('buildNodeRelateRequest({')
  })

  it('offers only admissible targets for a migration', () => {
    expect(source).toContain('migrateTargetOptions(wsStore.items, props.wsId)')
  })
})

// A migration is carried out after the request returns, so a list refreshed only on the
// response can catch the node between workspaces -- where it reads as unassigned and loses
// the actions a bound node has, Migrate included.
describe('BindDialog refresh after a migration', () => {
  const source = readFileSync(new URL('./BindDialog.vue', import.meta.url), 'utf-8')

  it('looks again once the crossing has had time to finish', () => {
    expect(source).toContain('MIGRATION_SETTLE_MS')
    expect(source).toContain('settleTimer = window.setTimeout(')
  })

  // The second look is scheduled after the dialog closes, so it can outlive the page it was
  // going to refresh.
  it('drops the pending look if the page goes away first', () => {
    expect(source).toContain('onBeforeUnmount(clearSettleTimer)')
    expect(source).toContain('clearTimeout(settleTimer)')
  })
})

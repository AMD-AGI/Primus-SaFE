import { describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import type { TeamMember, TeamMembersResp } from '@/services'
import {
  loadTeamMembers,
  refreshTeamMembers,
  type TeamMembersFetcher,
  type TeamStateMap,
} from './teamMembersCache'

const member = (userId: string) => ({ userId, name: userId, role: 'member' }) as TeamMember

// A fetcher whose responses are settled by hand, one pending call per team.
const manualFetcher = () => {
  const pending: Record<
    string,
    { resolve: (r: TeamMembersResp) => void; reject: (e: unknown) => void }[]
  > = {}
  const fetch = vi.fn<TeamMembersFetcher>(
    (teamId) =>
      new Promise((resolve, reject) => {
        ;(pending[teamId] ??= []).push({ resolve, reject })
      }),
  )
  const settle = (teamId: string) => pending[teamId]!.shift()!
  return { fetch, settle }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

describe('team members cache', () => {
  it('loads a team once and keeps the members', async () => {
    const state = reactive<TeamStateMap>({})
    const fetch = vi.fn<TeamMembersFetcher>(
      async () => ({ items: [member('u1')] }) as TeamMembersResp,
    )
    await loadTeamMembers(state, 't1', fetch)
    await loadTeamMembers(state, 't1', fetch)
    expect(fetch).toHaveBeenCalledTimes(1)
    expect(state.t1).toMatchObject({ loading: false, loaded: true, error: '' })
    expect(state.t1!.items.map((m) => m.userId)).toEqual(['u1'])
  })

  it('records a failure as an error', async () => {
    const state = reactive<TeamStateMap>({})
    const fetch = vi.fn<TeamMembersFetcher>(async () => {
      throw { response: { status: 500 } }
    })
    await loadTeamMembers(state, 't1', fetch)
    expect(state.t1).toMatchObject({ loading: false, loaded: false })
    expect(state.t1!.error).not.toBe('')
  })

  it('reloads the teams still expanded after a refresh and drops the others', async () => {
    const state = reactive<TeamStateMap>({})
    const fetch = vi.fn<TeamMembersFetcher>(
      async (id) => ({ items: [member(`${id}-u`)] }) as TeamMembersResp,
    )
    await loadTeamMembers(state, 't1', fetch)
    await loadTeamMembers(state, 't2', fetch)
    await loadTeamMembers(state, 't3', fetch)

    await refreshTeamMembers(state, ['t1', 't3'], ['t1', 't2'], fetch)

    expect(Object.keys(state)).toEqual(['t1'])
    expect(state.t1!.items.map((m) => m.userId)).toEqual(['t1-u'])
    expect(fetch).toHaveBeenCalledTimes(4)
  })

  it('drops a response that lands after a refresh removed its entry', async () => {
    const state = reactive<TeamStateMap>({})
    const { fetch, settle } = manualFetcher()
    const first = loadTeamMembers(state, 't1', fetch)

    // The team is no longer expanded when the refresh lands.
    void refreshTeamMembers(state, [], ['t1'], fetch)
    settle('t1').resolve({ items: [member('stale')] } as TeamMembersResp)
    await expect(first).resolves.toBeUndefined()
    expect(state.t1).toBeUndefined()
  })

  it('keeps the response of the reload, not the one it replaced', async () => {
    const state = reactive<TeamStateMap>({})
    const { fetch, settle } = manualFetcher()
    const first = loadTeamMembers(state, 't1', fetch)
    const refresh = refreshTeamMembers(state, ['t1'], ['t1'], fetch)

    settle('t1').reject({ response: { status: 500 } })
    await first
    expect(state.t1).toMatchObject({ loading: true, error: '' })

    settle('t1').resolve({ items: [member('fresh')] } as TeamMembersResp)
    await refresh
    await flush()
    expect(state.t1).toMatchObject({ loading: false, loaded: true, error: '' })
    expect(state.t1!.items.map((m) => m.userId)).toEqual(['fresh'])
  })
})

import type { TeamMember, TeamMembersResp } from '@/services'

/** The member list of one bound team as the Members tab shows it. */
export interface TeamState {
  loading: boolean
  loaded: boolean
  /** The caller may not read the member list (403). */
  forbidden: boolean
  error: string
  items: TeamMember[]
}

export type TeamStateMap = Record<string, TeamState>

export type TeamMembersFetcher = (teamId: string) => Promise<TeamMembersResp>

/**
 * Loads the members of a team into the map unless they are loaded or loading. A response that
 * arrives after its entry was removed or replaced is dropped.
 */
export async function loadTeamMembers(
  state: TeamStateMap,
  teamId: string,
  fetch: TeamMembersFetcher,
): Promise<void> {
  if (state[teamId]?.loaded || state[teamId]?.loading) return
  state[teamId] = { loading: true, loaded: false, forbidden: false, error: '', items: [] }
  // Read back, so that a reactive map yields the same proxy on later reads.
  const entry = state[teamId]
  try {
    const res = await fetch(teamId)
    if (state[teamId] !== entry) return
    entry.items = res?.items ?? []
    entry.loaded = true
  } catch (err) {
    if (state[teamId] !== entry) return
    if ((err as { response?: { status?: number } })?.response?.status === 403) {
      entry.forbidden = true
      entry.loaded = true
    } else {
      entry.error = 'Failed to load the team members.'
    }
  } finally {
    entry.loading = false
  }
}

/**
 * Drops every cached member list and reloads the teams that are expanded and still bound.
 */
export async function refreshTeamMembers(
  state: TeamStateMap,
  boundIds: readonly string[],
  expandedIds: readonly string[],
  fetch: TeamMembersFetcher,
): Promise<void> {
  Object.keys(state).forEach((k) => delete state[k])
  const bound = new Set(boundIds)
  await Promise.all(
    expandedIds.filter((id) => bound.has(id)).map((id) => loadTeamMembers(state, id, fetch)),
  )
}

import request from '@/services/request'
import type { ListTeamsResp, TeamMembersResp } from './type'

/** getTeams lists the teams of the identity service. The list is empty when the service is unreachable. */
export const getTeams = (): Promise<ListTeamsResp> => request.get('/teams', { silentErrors: true })

/** getTeamMembers lists the members of one team. A refusal (403) is left to the caller. */
export const getTeamMembers = (teamId: string): Promise<TeamMembersResp> =>
  request.get(`/teams/${encodeURIComponent(teamId)}/members`, { silentErrors: true })

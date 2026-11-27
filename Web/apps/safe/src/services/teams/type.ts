// A person in the identity service, as a team member or a team manager
export interface TeamMember {
  userId: string
  email: string
  name: string
  preferredName: string
  role: string
}

// GET /teams: a team of the identity service
export interface TeamItem {
  id: string
  displayName: string
  description: string
  memberCount: number
  managers: TeamMember[]
}

export interface ListTeamsResp {
  totalCount: number
  items: TeamItem[]
}

// GET /teams/:name/members
export interface TeamMembersResp {
  teamId: string
  totalCount: number
  items: TeamMember[]
}

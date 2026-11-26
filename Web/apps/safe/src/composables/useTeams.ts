import { computed, ref } from 'vue'
import { getTeams } from '@/services/teams'
import type { TeamItem } from '@/services/teams/type'

/** A workspace access source split into its kind and the value after the colon. */
export interface ParsedSource {
  kind: 'direct' | 'team' | 'group' | 'default' | 'other'
  value: string
}

/** parseSource splits a source such as direct, team:<id>, group:<name> or default. */
export function parseSource(source: string): ParsedSource {
  if (source === 'direct' || source === 'default') return { kind: source, value: '' }
  const idx = source.indexOf(':')
  const kind = idx < 0 ? '' : source.slice(0, idx)
  if (kind === 'team' || kind === 'group') return { kind, value: source.slice(idx + 1) }
  return { kind: 'other', value: source }
}

/** Loads the team list of the identity service once and resolves team ids to display names. */
export function useTeams() {
  const teams = ref<TeamItem[]>([])
  const loaded = ref(false)
  const loading = ref(false)

  const nameById = computed(() => new Map(teams.value.map((t) => [t.id, t.displayName])))

  /** Fetches the list. A failure leaves it empty so callers fall back to the team id. */
  async function loadTeams() {
    if (loaded.value || loading.value) return
    loading.value = true
    try {
      teams.value = (await getTeams())?.items ?? []
    } catch {
      teams.value = []
    } finally {
      loaded.value = true
      loading.value = false
    }
  }

  /** Display name of a team, or its id when the name is unknown. */
  const teamLabel = (id: string): string => nameById.value.get(id) || id

  /** Readable text of a source: team sources show the team display name when known. */
  function sourceLabel(source: string): string {
    const parsed = parseSource(source)
    if (parsed.kind === 'team') return `team: ${teamLabel(parsed.value)}`
    if (parsed.kind === 'group') return `group: ${parsed.value}`
    return parsed.kind === 'other' ? parsed.value : parsed.kind
  }

  return { teams, loaded, loading, loadTeams, teamLabel, sourceLabel }
}
